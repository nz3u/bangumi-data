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
	"syscall"
	"time"

	"bangumi-subject-go/internal/api"
	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/importer"
	"bangumi-subject-go/internal/pics"
)

// 版本号可在构建时通过 -ldflags "-X main.version=x.y.z" 注入。
var version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "import":
		err = cmdImport(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("bangumi %s\n", version)
		return
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		log.Fatalf("失败: %v", err)
	}
}

func usage() {
	fmt.Print(`bangumi - 本地 Bangumi 数据服务

用法:
  bangumi import -file <dump.zip 或目录> [选项]   导入数据到本地 SQLite
  bangumi serve [选项]                           启动查询 API 服务
  bangumi version                                显示版本

环境变量:
  BANGUMI_DB       SQLite 数据库路径 (默认 data/bangumi.db)
  BANGUMI_LISTEN   监听地址 (默认 :8080)
  BANGUMI_WEB      前端静态文件目录 (可选，默认使用编译期内嵌的 web/dist)
  BANGUMI_COMMON   common yaml 目录 (默认使用内嵌常量)
  BANGUMI_API_KEY  next.bgm.tv API Key（人物头像；也可放 data/config.json）

运行 import 子命令查看其选项。
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

	if _, err := common.Load(*commonDir); err != nil {
		return err
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close(conn)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("开始导入 %s -> %s", *src, *dbPath)
	stats, err := importer.Import(ctx, conn, *src, *limit)
	if err != nil {
		return err
	}
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
  -listen   监听地址（默认 :8080 或 $BANGUMI_LISTEN）
  -db       数据库路径（默认 data/bangumi.db 或 $BANGUMI_DB）
  -web      前端静态文件目录（可选；不指定或不存在时使用内嵌 web/dist）
  -common-dir  common yaml 目录（默认内嵌，无需指定）
`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *listen == "" {
		*listen = ":8080"
	}
	if *dbPath == "" {
		*dbPath = "data/bangumi.db"
	}

	cons, err := common.Load(*commonDir)
	if err != nil {
		return err
	}

	conn, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close(conn)

	// 人物头像图片库：与主库同目录的 bgm_pic.db；
	// API Key 读 config.json（同目录）或环境变量 BANGUMI_API_KEY，
	// 未配置时功能停用（头像接口返回失败，前端回退字符头像）。
	dataDir := filepath.Dir(*dbPath)
	var picSvc *pics.Service
	if picSvc, err = pics.Open(filepath.Join(dataDir, "bgm_pic.db"), pics.LoadAPIKey(dataDir)); err != nil {
		log.Printf("图片服务不可用: %v（头像接口将返回未启用）", err)
		picSvc = nil
	} else {
		defer picSvc.Close()
		log.Printf("人物头像服务已就绪（API Key %s）", map[bool]string{true: "已配置", false: "未配置"}[picSvc.HasKey()])
	}

	router := api.NewRouter(conn, cons, *webDir, picSvc)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("服务已启动，监听 %s（数据库 %s）", *listen, *dbPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("正在关闭服务...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
