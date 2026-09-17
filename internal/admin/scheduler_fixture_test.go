package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/update"
)

const (
	oldDumpVersion = "dump-2026-09-08.210336Z.zip"
	newDumpVersion = "dump-2026-09-15.210336Z.zip"
)

// latestServer 模拟 aux/latest.json，返回的 name 可在运行期通过 setLatestName 改变，
// 用于模拟「上游在检查前后发布新导出」。
type latestServer struct {
	*httptest.Server
	mu   sync.Mutex
	name string
}

func (s *latestServer) latestName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.name
}

func (s *latestServer) set(name string) {
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
}

func newLatestServer(t *testing.T, name string) *latestServer {
	t.Helper()
	ls := &latestServer{name: name}
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := ls.latestName()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name":%q,"browser_download_url":"https://example.com/%s","created_at":"2026-09-15T21:03:37Z","digest":"sha256:deadbeef","size":123}`,
			n, n)
	}))
	t.Cleanup(ls.Server.Close)
	return ls
}

func setLatestName(t *testing.T, srv *latestServer, name string) {
	t.Helper()
	srv.set(name)
}

// newScheduledFixture 建立临时数据目录：库文件 + config.json（记录 version，自动更新已启用），
// 以及模拟上游 latest.json 的服务。返回库路径与上游服务。
func newScheduledFixture(t *testing.T, version string) (dbPath, cfgPath string, srv *latestServer) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "bangumi.db")
	if err := os.WriteFile(dbPath, []byte("sqlite-placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath = config.FilePath(dir)
	cfg := &config.Config{
		Database:   &config.DatabaseInfo{Version: version},
		AutoUpdate: &config.AutoUpdateConfig{Enabled: true},
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	return dbPath, cfgPath, newLatestServer(t, version)
}

// writeAutoUpdateEnabled 改写配置中的自动更新开关。
func writeAutoUpdateEnabled(dbPath string, enabled bool) error {
	cfgPath := config.FilePath(filepath.Dir(dbPath))
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if cfg.AutoUpdate == nil {
		cfg.AutoUpdate = &config.AutoUpdateConfig{}
	}
	cfg.AutoUpdate.Enabled = enabled
	return config.Save(cfgPath, cfg)
}

// newTestChecker 指向本地模拟的 latest.json。
func newTestChecker(dbPath string, srv *latestServer) *update.VersionChecker {
	vc := update.NewVersionChecker(dbPath)
	vc.SetLatestURL(srv.URL)
	return vc
}
