// Package update 实现「下载 Archive 最新导出并导入/更新数据库」的完整流程：
//
//   - 无数据库（首次运行）：从 aux/latest.json 获取最新导出，多线程下载
//     压缩包到数据库所在目录，走完整导入流程；
//   - 已有数据库：对比 config.json 记录的版本（未记录视为落后，兼容旧版本
//     程序创建的库），落后则下载最新导出 -> 导入临时库 -> 完整性检查 ->
//     删除旧库后换名，任一步失败原库不受影响。
package update

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/download"
	"bangumi-subject-go/internal/importer"
)

// Options 更新流程参数；零值字段取与 import 一致的默认值。
type Options struct {
	DBPath    string // 数据库路径（默认 data/bangumi.db）
	CommonDir string // common yaml 目录（默认内嵌）
	Limit     int64  // 每个文件最多导入行数（0 全部，测试用）
	Threads   int    // 多线程下载并发连接数
	Force     bool   // 已是最新也强制重下重导
	KeepZip   bool   // 导入完成后保留压缩包

	LatestURL string // 空则用 download.LatestURL；测试可指向本地服务
}

// Run 执行一次下载+导入/更新。已是最新版本时返回 (nil, nil)，
// 否则返回导入统计。版本号在成功后写入数据目录的 config.json。
func Run(ctx context.Context, opt Options) (*importer.Stats, error) {
	if opt.DBPath == "" {
		opt.DBPath = "data/bangumi.db"
	}
	if opt.Threads < 1 {
		opt.Threads = 1
	}
	dataDir := filepath.Dir(opt.DBPath)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	latestURL := opt.LatestURL
	if latestURL == "" {
		latestURL = download.LatestURL
	}

	cfgPath := config.FilePath(dataDir)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}

	log.Printf("查询最新导出：%s", latestURL)
	rel, err := download.FetchLatest(ctx, latestURL)
	if err != nil {
		return nil, fmt.Errorf("获取最新导出信息失败（离线环境可从 https://github.com/bangumi/Archive/releases 手动下载 dump 后运行 import 导入）: %w", err)
	}
	log.Printf("最新导出：%s（%s，%s 发布）", rel.Name, download.HumanBytes(rel.Size), rel.CreatedAt)

	dbExists := false
	if fi, statErr := os.Stat(opt.DBPath); statErr == nil && !fi.IsDir() {
		dbExists = true
	}

	switch {
	case opt.Force:
		log.Println("-force：强制重新下载导入")
	case dbExists && cfg.Version() == rel.Name:
		fmt.Printf("数据库已是最新版本：%s\n", rel.Name)
		return nil, nil
	case dbExists && cfg.Version() == "":
		log.Printf("当前库未记录版本（旧版本程序创建），视为落后，开始更新")
	case dbExists:
		log.Printf("当前库版本 %s 落后于 %s，开始更新", cfg.Version(), rel.Name)
	default:
		log.Printf("数据库不存在，开始首次下载与导入")
	}

	zipPath := filepath.Join(dataDir, rel.Name)
	if err := download.File(ctx, rel, zipPath, opt.Threads); err != nil {
		return nil, err
	}

	var stats *importer.Stats
	if dbExists {
		stats, err = importAndSwap(ctx, zipPath, opt, dataDir)
	} else {
		stats, err = importer.RunImport(ctx, zipPath, opt.Limit, opt.CommonDir, opt.DBPath)
	}
	if err != nil {
		return nil, err
	}

	// 版本信息写入 config.json（下次 update 据此判断是否落后）
	cfg.Database = &config.DatabaseInfo{
		Version:     rel.Name,
		Digest:      rel.Digest,
		Size:        rel.Size,
		SourceURL:   rel.BrowserDownloadURL,
		PublishedAt: rel.CreatedAt,
		ImportedAt:  time.Now().Format(time.RFC3339),
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return stats, fmt.Errorf("写入版本信息到 %s: %w", cfgPath, err)
	}

	if opt.KeepZip {
		log.Printf("压缩包保留在 %s", zipPath)
	} else if rmErr := os.Remove(zipPath); rmErr == nil {
		log.Printf("已删除压缩包（-keep 可保留）")
	}
	return stats, nil
}

// importAndSwap 已有库时的更新路径：临时库走原有导入流程 + 完整性检查，
// 通过后删除旧库并把临时库换名为正式库名；失败时清理临时文件，原库不动。
func importAndSwap(ctx context.Context, zipPath string, opt Options, dataDir string) (*importer.Stats, error) {
	tmpPath := opt.DBPath + ".updating"
	cleanTmp := func() {
		for _, p := range []string{tmpPath, tmpPath + "-wal", tmpPath + "-shm"} {
			_ = os.Remove(p)
		}
	}
	cleanTmp() // 清理上次中断可能残留的临时库

	stats, err := importer.RunImport(ctx, zipPath, opt.Limit, opt.CommonDir, tmpPath)
	if err != nil {
		cleanTmp()
		return nil, fmt.Errorf("导入新库失败（原数据库未受影响）: %w", err)
	}

	// 换库：先删旧库（含 WAL/SHM），再把临时库改回正式名字
	for _, p := range []string{opt.DBPath, opt.DBPath + "-wal", opt.DBPath + "-shm"} {
		_ = os.Remove(p)
	}
	for _, p := range [][2]string{
		{tmpPath, opt.DBPath},
		{tmpPath + "-wal", opt.DBPath + "-wal"},
		{tmpPath + "-shm", opt.DBPath + "-shm"},
	} {
		if _, statErr := os.Stat(p[0]); statErr == nil {
			if err := os.Rename(p[0], p[1]); err != nil {
				return nil, fmt.Errorf("启用新数据库: %w", err)
			}
		}
	}
	log.Printf("已切换到新数据库 %s", opt.DBPath)
	return stats, nil
}
