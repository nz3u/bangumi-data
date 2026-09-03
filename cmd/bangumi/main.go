// bangumi 是一个本地化的 Bangumi 数据服务：
//   - import：将 Archive 导出的 dump（zip/jsonlines）导入本地 SQLite
//   - serve ：提供 REST 查询 API（可托管前端静态文件）
//
// 单二进制跨平台运行，也支持 Docker（见 Dockerfile / docker-compose.yml）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"bangumi-subject-go/internal/admin"
	"bangumi-subject-go/internal/api"
	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/importer"
	"bangumi-subject-go/internal/pics"
	"bangumi-subject-go/internal/update"
)

	// 版本号由发布流水线在编译时通过 -ldflags "-X main.version=x.y.z" 注入
	// （取自 git 标签）；本地或 CI 直接 go build 时为 "dev"，避免误报固定版本。
	var version = "dev"

func main() {
	if len(os.Args) < 2 {
		// 无参直接进入智能启动：等价于 serve（自动处理初始化/更新/服务）
		if err := cmdServe(nil); err != nil {
			log.Fatalf("失败: %v", err)
		}
		return
	}
	var err error
	switch os.Args[1] {
	case "import":
		err = cmdImport(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "config":
		err = cmdConfig(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("bangumi %s\n", version)
		return
	default:
		// 未知子命令时视为直接 serve（兼容 docker 无 CMD 覆盖等场景）
		// 若第一个参数形如 -listen/-db 则视为 serve 透传
		if strings.HasPrefix(os.Args[1], "-") {
			err = cmdServe(os.Args[1:])
		} else {
			usage()
			os.Exit(1)
		}
	}
	if err != nil {
		log.Fatalf("失败: %v", err)
	}
}

func usage() {
	fmt.Print(`bangumi - 本地 Bangumi 数据服务

用法:
  bangumi                                         智能启动（无数据库时进入初始化页面，否则启动服务；等价于 serve）
  bangumi import -file <dump.zip 或目录> [选项]   导入数据到本地 SQLite
  bangumi update [选项]                          下载最新导出并导入/更新数据库
  bangumi serve [选项]                           启动查询 API 服务（支持在线自动更新）
  bangumi config <子命令> [参数]                  配置管理（get/set/list/generate-token）
  bangumi version                                显示版本

环境变量:
  BANGUMI_DB       SQLite 数据库路径 (默认 data/bangumi.db)
  BANGUMI_LISTEN   监听地址 (默认 :8080)
  BANGUMI_WEB      前端静态文件目录 (可选，默认使用编译期内嵌的 web/dist)
  BANGUMI_COMMON   common yaml 目录 (默认使用内嵌常量)
  BANGUMI_API_KEY  next.bgm.tv API Key（人物头像；也可放 data/config.json）

数据库版本记录于 data/config.json 的 database 字段；update 命令据此判断
当前库是否落后于 Archive 最新导出。运行 import/update/serve/config 子命令查看其选项。
自动更新（每周三 05:30 (UTC+8)）在 serve 运行状态且 config.json 中 auto_update.enabled=true 时生效，
更新期间前端展示“更新中”提示，可在 /setup 页面查看实时日志；失败或新版可用时可通过
携带 token 访问 /setup 手动触发更新。配置可通过 bangumi config 或前端 /admin 完成。
首次启动时将自动生成 admin_token 并写入 data/config.json，请妥善保管。
`)
}

func cmdImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	src := fs.String("file", "", "dump 文件：.zip 或包含 jsonlines 的解压目录")
	limit := fs.Int64("limit", 0, "每个文件最多导入行数（0 为全部，测试用）")
	commonDir := fs.String("common-dir", os.Getenv("BANGUMI_COMMON"), "common yaml 目录（默认内嵌）")
	dbPath := fs.String("db", os.Getenv("BANGUMI_DB"), "数据库路径")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `import - 导入 Archive dump 数据

用法:
  bangumi import -file <dump.zip 或目录> [-db 路径] [-limit N]

选项:
  -file         必需。dump 文件（.zip）或解压后的 jsonlines 目录
  -db           数据库路径（默认 data/bangumi.db 或 $BANGUMI_DB）
  -limit        每个文件最多导入 N 行（默认全部，测试时用小值）
  -common-dir   common yaml 目录（默认内嵌进二进制，无需指定）
`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *src == "" {
		fs.Usage()
		return fmt.Errorf("缺少 -file 参数")
	}
	if *dbPath == "" {
		*dbPath = "data/bangumi.db"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stats, err := importer.RunImport(ctx, *src, *limit, *commonDir, *dbPath)
	if err != nil {
		return err
	}
	printStats(stats)
	return nil
}

// cmdUpdate 数据下载与更新（具体流程见 internal/update 包）：
//   - 无数据库（首次运行）：多线程下载最新导出压缩包到数据目录并导入；
//   - 已有数据库：对比 config.json 记录的版本，落后则下载最新导出，
//     导入临时库 -> 完整性检查 -> 删旧库换名，失败时原库不受影响；
//   - 版本一致且未指定 -force 时跳过。
//
// config.json 未记录版本（旧版本程序创建的库）一律视为落后。
func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	dbPath := fs.String("db", os.Getenv("BANGUMI_DB"), "数据库路径")
	commonDir := fs.String("common-dir", os.Getenv("BANGUMI_COMMON"), "common yaml 目录（默认内嵌）")
	limit := fs.Int64("limit", 0, "每个文件最多导入行数（0 为全部，测试用）")
	threads := fs.Int("threads", 8, "多线程下载的并发连接数")
	force := fs.Bool("force", false, "已是最新版本也强制重新下载导入")
	keepZip := fs.Bool("keep", false, "导入完成后保留压缩包（默认删除以节省空间）")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `update - 下载 Archive 最新导出并导入/更新

首次运行（无数据库）自动从 aux/latest.json 获取最新导出，
多线程下载压缩包到数据库所在目录并走完整导入流程；
已有数据库时对比 data/config.json 记录的版本，落后则：
下载 -> 导入临时库 -> 完整性检查 -> 原子换库。

用法:
  bangumi update [-db 路径] [-threads N] [-force] [-keep]

选项:
  -db           数据库路径（默认 data/bangumi.db 或 $BANGUMI_DB）
  -threads      多线程下载的并发连接数（默认 8）
  -limit        每个文件最多导入 N 行（默认全部，测试时用小值）
  -force        已是最新版本也重新下载导入
  -keep         导入完成后保留压缩包（默认删除）
  -common-dir   common yaml 目录（默认内嵌进二进制，无需指定）

注意：更新期间请先停止 serve 服务，避免占用旧库文件导致换库失败。
`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stats, err := update.Run(ctx, update.Options{
		DBPath:    *dbPath,
		CommonDir: *commonDir,
		Limit:     *limit,
		Threads:   *threads,
		Force:     *force,
		KeepZip:   *keepZip,
	})
	if err != nil || stats == nil {
		return err
	}
	printStats(stats)
	fmt.Printf("\n数据库已更新至最新版本\n")
	return nil
}

func printStats(stats *importer.Stats) {
	fmt.Printf("\n导入完成:\n")
	fmt.Printf("  条目:        %d\n", stats.Subjects)
	fmt.Printf("  人物:        %d\n", stats.Persons)
	fmt.Printf("  角色:        %d\n", stats.Characters)
	fmt.Printf("  章节:        %d\n", stats.Episodes)
	fmt.Printf("  条目关联:    %d\n", stats.SubjectRelations)
	fmt.Printf("  条目-人物:   %d\n", stats.SubjectPersons)
	fmt.Printf("  条目-角色:   %d\n", stats.SubjectCharacters)
	fmt.Printf("  人物-角色:   %d\n", stats.PersonCharacters)
	fmt.Printf("  人物关联:    %d\n", stats.PersonRelations)
}

func cmdConfig(args []string) error {
	if len(args) == 0 {
		return printConfigUsage(nil)
	}
	dbPath := "data/bangumi.db"
	// 允许通过全局环境变量或 --db 指定
	// 简单解析：若第一个参数为 -db
	filtered := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "-db" || args[i] == "--db" {
			if i+1 < len(args) {
				dbPath = args[i+1]
				i++
				continue
			}
		}
		if v := os.Getenv("BANGUMI_DB"); v != "" && dbPath == "data/bangumi.db" {
			// 若未显式指定则取环境变量
		}
		filtered = append(filtered, args[i])
	}
	if v := strings.TrimSpace(os.Getenv("BANGUMI_DB")); v != "" {
		// 环境变量优先于默认但低于显式 --db
		hasExplicit := false
		for _, a := range args {
			if a == "-db" || a == "--db" {
				hasExplicit = true
			}
		}
		if !hasExplicit {
			dbPath = v
		}
	}
	args = filtered
	if len(args) == 0 {
		return printConfigUsage(nil)
	}
	sub := args[0]
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	cfgPath := config.FilePath(dataDir)

	switch sub {
	case "list", "show", "get":
		if len(args) == 1 {
			// list 全部
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			cfg.EnsureDefaults()
			fmt.Printf("bgm_api_key: %s\n", maskKey(cfg.BgmApiKey))
			fmt.Printf("admin_token: %s\n", maskKey(cfg.AdminToken))
			if cfg.AutoUpdate != nil {
				fmt.Printf("auto_update.enabled: %v\n", cfg.AutoUpdate.Enabled)
				fmt.Printf("auto_update.threads: %d\n", cfg.AutoUpdate.Threads)
				fmt.Printf("auto_update.keep_zip: %v\n", cfg.AutoUpdate.KeepZip)
			}
			if cfg.Server != nil {
				fmt.Printf("server.listen: %s\n", cfg.Server.Listen)
			}
			if cfg.Database != nil {
				fmt.Printf("database.version: %s\n", cfg.Database.Version)
			} else {
				fmt.Printf("database.version: (无记录)\n")
			}
			return nil
		}
		// get <key>
		key := args[1]
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		val, ok := getConfigValue(cfg, key)
		if !ok {
			return fmt.Errorf("未知配置项 %q", key)
		}
		fmt.Println(val)
		return nil
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("用法: bangumi config set <key> <value>")
		}
		key, val := args[1], args[2]
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		cfg.EnsureDefaults()
		if err := setConfigValue(cfg, key, val); err != nil {
			return err
		}
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		fmt.Printf("已设置 %s\n", key)
		return nil
	case "generate-token", "gen-token":
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		tok, err := config.GenerateToken()
		if err != nil {
			return err
		}
		cfg.AdminToken = tok
		cfg.EnsureDefaults()
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		fmt.Printf("已生成 admin_token: %s\n", tok)
		fmt.Printf("请妥善保管，后续访问 /setup 与 /admin 需携带 token（Header X-Admin-Token 或 ?token=）\n")
		return nil
	case "help", "-h", "--help":
		return printConfigUsage(nil)
	default:
		return printConfigUsage(fmt.Errorf("未知子命令 %q", sub))
	}
}

func printConfigUsage(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n\n", err)
	}
	fmt.Print(`config - 管理 data/config.json

用法:
  bangumi config list                    列出全部配置
  bangumi config get <key>               读取单项
  bangumi config set <key> <value>       设置单项
  bangumi config generate-token          生成随机 admin_token 并写入配置

可配置项:
  bgm_api_key                 next.bgm.tv API Key（人物头像）
  admin_token                 管理接口鉴权 token（自动生成，访问 /setup 与 /admin 需携带）
  auto_update.enabled         是否启用每周三 05:30 (UTC+8) 自动更新 (true/false)
  auto_update.threads         下载并发数 (1-32，0 表示默认 8)
  auto_update.keep_zip        是否保留下载的 zip (true/false)
  server.listen               监听地址（如 :8080）

示例:
  bangumi config set auto_update.enabled true
  bangumi config set bgm_api_key your_key_here
  bangumi config generate-token
`)
	if err != nil {
		return err
	}
	return nil
}

func maskKey(s string) string {
	if s == "" {
		return "(未设置)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

func getConfigValue(cfg *config.Config, key string) (string, bool) {
	switch key {
	case "bgm_api_key":
		return cfg.BgmApiKey, true
	case "admin_token":
		return cfg.AdminToken, true
	case "auto_update.enabled":
		if cfg.AutoUpdate == nil {
			return "false", true
		}
		return fmt.Sprintf("%v", cfg.AutoUpdate.Enabled), true
	case "auto_update.threads":
		if cfg.AutoUpdate == nil {
			return "0", true
		}
		return fmt.Sprintf("%d", cfg.AutoUpdate.Threads), true
	case "auto_update.keep_zip":
		if cfg.AutoUpdate == nil {
			return "false", true
		}
		return fmt.Sprintf("%v", cfg.AutoUpdate.KeepZip), true
	case "server.listen":
		if cfg.Server == nil {
			return "", true
		}
		return cfg.Server.Listen, true
	case "database.version":
		if cfg.Database == nil {
			return "", true
		}
		return cfg.Database.Version, true
	default:
		return "", false
	}
}

func setConfigValue(cfg *config.Config, key, val string) error {
	switch key {
	case "bgm_api_key":
		cfg.BgmApiKey = strings.TrimSpace(val)
	case "admin_token":
		cfg.AdminToken = strings.TrimSpace(val)
	case "auto_update.enabled":
		b := strings.ToLower(strings.TrimSpace(val))
		cfg.AutoUpdate.Enabled = b == "true" || b == "1" || b == "yes"
	case "auto_update.threads":
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
			return fmt.Errorf("threads 需为整数")
		}
		if n < 0 || n > 32 {
			return fmt.Errorf("threads 需在 0-32 之间")
		}
		cfg.AutoUpdate.Threads = n
	case "auto_update.keep_zip":
		b := strings.ToLower(strings.TrimSpace(val))
		cfg.AutoUpdate.KeepZip = b == "true" || b == "1" || b == "yes"
	case "server.listen":
		cfg.Server.Listen = strings.TrimSpace(val)
	default:
		return fmt.Errorf("未知配置项 %q", key)
	}
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", os.Getenv("BANGUMI_LISTEN"), "监听地址")
	commonDir := fs.String("common-dir", os.Getenv("BANGUMI_COMMON"), "common yaml 目录（默认内嵌）")
	dbPath := fs.String("db", os.Getenv("BANGUMI_DB"), "数据库路径")
	webDir := fs.String("web", os.Getenv("BANGUMI_WEB"), "前端静态文件目录（可选）")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `serve - 启动查询 API 服务

用法:
  bangumi serve [-listen :8080] [-db 路径] [-web 目录]

选项:
  -listen   监听地址（默认 :8080 或 $BANGUMI_LISTEN，或 data/config.json 的 server.listen）
  -db       数据库路径（默认 data/bangumi.db 或 $BANGUMI_DB）
  -web      前端静态文件目录（可选；不指定或不存在时使用内嵌 web/dist）
  -common-dir  common yaml 目录（默认内嵌，无需指定）

说明：
  serve 启动后将常驻提供查询服务；若 config.json 中 auto_update.enabled=true，
  则每周三 05:30 (UTC+8) 自动检查并在后台执行更新（下载->导入临时库->切库->自动恢复服务），
  更新期间前端展示“更新中”提示，可在 /setup 页面查看实时日志。
  任何时候均可携带 token 访问 /setup 手动触发初始化/更新；失败或检测到新版但未自动更新时
  也可通过该页面完成更新。配置可通过 bangumi config 或前端 /admin/config 修改。
`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	// listening address 优先级：命令行 > 环境变量 > 配置文件 > 默认
	if *listen == "" {
		*listen = os.Getenv("BANGUMI_LISTEN")
	}
	// 若仍为空，尝试从配置文件读取
	tmpDataDir := filepath.Dir(*dbPath)
	if *dbPath == "" {
		*dbPath = "data/bangumi.db"
	}
	tmpDataDir = filepath.Dir(*dbPath)
	if tmpDataDir == "." || tmpDataDir == "" {
		tmpDataDir = "data"
	}
	if *listen == "" {
		if cfg, _ := config.Load(config.FilePath(tmpDataDir)); cfg != nil && cfg.Server != nil && strings.TrimSpace(cfg.Server.Listen) != "" {
			*listen = strings.TrimSpace(cfg.Server.Listen)
		}
	}
	if *listen == "" {
		*listen = ":8080"
	}
	if *dbPath == "" {
		*dbPath = "data/bangumi.db"
		if v := strings.TrimSpace(os.Getenv("BANGUMI_DB")); v != "" {
			*dbPath = v
		}
	}
	dataDir := filepath.Dir(*dbPath)
	// 初始化时自动生成 admin_token（若不存在）
	if err := ensureAdminToken(dataDir); err != nil {
		log.Printf("生成 admin_token 失败: %v", err)
	}

	cons, err := common.Load(*commonDir)
	if err != nil {
		return err
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	// conn 的生命周期由 Manager 接管（更新时会关闭重开），此处不 defer 关闭，而是由 shutdown 时通过 Manager 关闭
	// 幂等补建增量索引（已存在时空操作；首次升级构建数秒）
	start := time.Now()
	if err := db.EnsureIndexes(conn); err != nil {
		// 若数据库为空（刚创建），EnsureIndexes 会创建裸表，可能报错则不阻塞启动
		log.Printf("EnsureIndexes: %v", err)
	}
	if err := db.UpgradeSchema(conn); err != nil {
		log.Printf("UpgradeSchema: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		log.Printf("索引/数据升级完成（%s）", d.Round(time.Millisecond))
	}

	// 人物头像图片库：与主库同目录的 bgm_pic.db；
	// API Key 读 config.json（同目录）或环境变量 BANGUMI_API_KEY，
	// 未配置时功能停用（头像接口返回失败，前端回退字符头像）。
	var picSvc *pics.Service
	if picSvc, err = pics.Open(filepath.Join(dataDir, "bgm_pic.db"), pics.LoadAPIKey(dataDir)); err != nil {
		log.Printf("图片服务不可用: %v（头像接口将返回未启用）", err)
		picSvc = nil
	} else {
		defer picSvc.Close()
		log.Printf("人物头像服务已就绪（API Key %s）", map[bool]string{true: "已配置", false: "未配置"}[picSvc.HasKey()])
	}

	// 数据库版本检查：启动即后台对比 Archive 最新导出（离线时静默跳过），
	// 落后则日志提醒；结果经 /api/dbinfo 供前端右下角徽标与更新提醒展示。
	dbVer := update.NewVersionChecker(*dbPath)
	verCtx, verCancel := context.WithCancel(context.Background())
	defer verCancel()
	dbVer.Start(verCtx, update.DefaultCheckInterval)

	// 更新管理器（在线更新、自动调度、维护模式与日志流）
	mgr := admin.NewManager(*dbPath, *commonDir, conn, dbVer)
	// 调度器共享主上下文（serve 运行期间常驻）
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	mgr.StartScheduler(schedCtx)

	router := api.NewRouterWithManager(conn, cons, *webDir, picSvc, version, dbVer, mgr, dataDir)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		// 初始化提示
		if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
			log.Printf("数据库不存在，请访问 http://%s/setup 完成初始化（或运行 bangumi update）", *listen)
		} else {
			// 检查版本滞后
			if st := dbVer.Status(); st.UpdateAvailable {
				log.Printf("检测到新版本 %s 可更新，可在 /setup 页面手动触发（或等待每周三 05:30 (UTC+8) 自动更新）", st.Latest.Version)
			}
		}
		log.Printf("服务已启动，监听 %s（数据库 %s）", *listen, *dbPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("正在关闭服务...")
	schedCancel()
	verCancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	// 关闭数据库（若已被 Manager 重开，则关闭当前持有的连接）
	if c := mgr.DB(); c != nil {
		_ = c.Close()
	} else {
		_ = conn.Close()
	}
	return nil
}

func ensureAdminToken(dataDir string) error {
	cfgPath := config.FilePath(dataDir)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.AdminToken) != "" {
		return nil
	}
	tok, err := config.GenerateToken()
	if err != nil {
		return err
	}
	cfg.AdminToken = tok
	cfg.EnsureDefaults()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	log.Printf("已自动生成 admin_token: %s（已写入 %s；访问 /setup 与 /admin 需携带 token，可通过 bangumi config get admin_token 查看）", tok, cfgPath)
	return nil
}
