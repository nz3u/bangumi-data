package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bangumi-subject-go/internal/config"
)

const (
	oldDump = "dump-2026-07-28.210449Z.zip"
	newDump = "dump-2026-08-25.210336Z.zip"
)

// latestSrv 模拟 aux/latest.json，返回指定快照名的元信息。
func latestSrv(t *testing.T, name string) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256([]byte(name))
	body := fmt.Sprintf(`{"name":%q,"browser_download_url":"https://example.com/%s","created_at":"2026-08-25T21:03:37Z","digest":"sha256:%s","size":123}`,
		name, name, hex.EncodeToString(sum[:]))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newChecker 建立临时数据目录：可选创建数据库文件与 config.json 版本记录。
func newChecker(t *testing.T, withDB bool, version string) *VersionChecker {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bangumi.db")
	if withDB {
		if err := os.WriteFile(dbPath, []byte("sqlite-placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if version != "" {
		cfg := &config.Config{Database: &config.DatabaseInfo{Version: version}}
		if err := config.Save(config.FilePath(dir), cfg); err != nil {
			t.Fatal(err)
		}
	}
	return NewVersionChecker(dbPath)
}

func TestVersionCheckerLegacyDB(t *testing.T) {
	vc := newChecker(t, true, "")
	vc.latestURL = latestSrv(t, newDump).URL
	vc.checkOnce(context.Background())

	st := vc.Status()
	if st.Database != nil {
		t.Errorf("无记录时 Database 应为 nil，got %+v", st.Database)
	}
	if !st.UpdateAvailable {
		t.Error("旧版本程序创建的库应视为落后")
	}
	if st.Latest == nil || st.Latest.Version != newDump {
		t.Errorf("Latest 解析异常: %+v", st.Latest)
	}
	if st.CheckedAt == "" {
		t.Error("CheckedAt 未写入")
	}
}

func TestVersionCheckerUpToDate(t *testing.T) {
	vc := newChecker(t, true, newDump)
	vc.latestURL = latestSrv(t, newDump).URL
	vc.checkOnce(context.Background())

	if vc.Status().UpdateAvailable {
		t.Error("版本一致不应提示可更新")
	}
}

func TestVersionCheckerBehind(t *testing.T) {
	vc := newChecker(t, true, oldDump)
	vc.latestURL = latestSrv(t, newDump).URL
	vc.checkOnce(context.Background())

	st := vc.Status()
	if !st.UpdateAvailable {
		t.Error("落后于最新导出应提示可更新")
	}
	if st.Database == nil || st.Database.Version != oldDump {
		t.Errorf("本地记录异常: %+v", st.Database)
	}
}

// 无数据库文件（首次部署尚未导入）谈不上「更新」。
func TestVersionCheckerNoDB(t *testing.T) {
	vc := newChecker(t, false, "")
	vc.latestURL = latestSrv(t, newDump).URL
	vc.checkOnce(context.Background())

	if vc.Status().UpdateAvailable {
		t.Error("无库文件时不应提示可更新")
	}
}

// 上游不可达：保留本地记录，latest 置空且不提示。
func TestVersionCheckerOffline(t *testing.T) {
	vc := newChecker(t, true, oldDump)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 立即关闭模拟离线
	vc.latestURL = srv.URL
	vc.checkOnce(context.Background())

	st := vc.Status()
	if st.Latest != nil {
		t.Errorf("离线时 Latest 应为 nil，got %+v", st.Latest)
	}
	if st.UpdateAvailable {
		t.Error("离线时不应提示可更新")
	}
	if st.Database == nil || st.Database.Version != oldDump {
		t.Errorf("离线时应保留本地记录: %+v", st.Database)
	}
}

// 回归：serve 运行期间执行 bangumi update 写入 config.json 后，
// 无需重启或等待复查周期，下一次 Status()/api/dbinfo 即反映新状态。
func TestStatusReflectsLiveConfigUpdate(t *testing.T) {
	// 启动时本地尚无版本记录（旧库）
	vc := newChecker(t, true, "")
	vc.latestURL = latestSrv(t, newDump).URL
	vc.checkOnce(context.Background())

	st := vc.Status()
	if st.Database != nil || !st.UpdateAvailable {
		t.Fatalf("旧库应无记录且可更新: %+v", st)
	}

	// update 命令完成换库并写入最新版本号
	dir := filepath.Dir(vc.dbPath)
	cfg := &config.Config{Database: &config.DatabaseInfo{Version: newDump}}
	if err := config.Save(config.FilePath(dir), cfg); err != nil {
		t.Fatal(err)
	}

	st = vc.Status()
	if st.Database == nil || st.Database.Version != newDump {
		t.Errorf("Status 未实时读取 config.json: %+v", st.Database)
	}
	if st.UpdateAvailable {
		t.Error("已更新到最新版本后不应再提示可更新")
	}

	// 反向：上游发布更新的快照后立即恢复可更新提示
	vc.latestURL = latestSrv(t, "dump-2026-09-01.000000Z.zip").URL
	vc.checkOnce(context.Background())
	if !vc.Status().UpdateAvailable {
		t.Error("上游出现新快照后应提示可更新")
	}
}
