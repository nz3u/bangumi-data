package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

var dumpFileNames = []string{
	"subject.jsonlines", "person.jsonlines", "character.jsonlines",
	"episode.jsonlines", "subject-relations.jsonlines", "subject-persons.jsonlines",
	"subject-characters.jsonlines", "person-characters.jsonlines", "person-relations.jsonlines",
}

// fakeDump 生成一个包含全部 jsonlines 的 dump zip（仅 subjects 有数据）。
func fakeDump(t *testing.T, subjectNames ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, name := range subjectNames {
		row, _ := json.Marshal(map[string]any{"id": rand.Int63n(1 << 30), "type": 2, "name": name})
		buf.Write(row)
		buf.WriteByte('\n')
	}
	files := map[string][]byte{"subject.jsonlines": buf.Bytes()}

	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	for _, n := range dumpFileNames {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(files[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return z.Bytes()
}

// latestJSON 构造 aux/latest.json 形状的元信息。
func latestJSON(name string, content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf(`{
		"name": %q,
		"browser_download_url": "DOWNLOAD_URL",
		"content_type": "application/zip",
		"created_at": "2026-08-25T21:03:37Z",
		"digest": "sha256:%s",
		"size": %d
	}`, name, hex.EncodeToString(sum[:]), len(content))
}

// fakeArchive 启动 latest.json + 支持 Range 的 zip 下载两个服务。
type fakeArchive struct {
	latestSrv *httptest.Server
	hits      *int
}

func newFakeArchive(t *testing.T, name string, content []byte) *fakeArchive {
	t.Helper()
	hits := 0
	dlSrv := rangeZipServer(t, content, &hits)
	body := strings.Replace(latestJSON(name, content), "DOWNLOAD_URL", dlSrv.URL, 1)
	latestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(latestSrv.Close)
	return &fakeArchive{latestSrv: latestSrv, hits: &hits}
}

func (f *fakeArchive) requests() int { return *f.hits }

// rangeZipServer 模拟 GitHub release asset：支持 Range 的静态文件服务。
func rangeZipServer(t *testing.T, content []byte, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		rng := r.Header.Get("Range")
		if rng == "" {
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			w.Write(content)
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil || start > end || end >= len(content) {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.Header().Set("Content-Length", fmt.Sprint(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content[start : end+1])
	}))
}

func countSubjects(t *testing.T, dbPath string) int {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM subjects`).Scan(&n); err != nil {
		t.Fatalf("查询 subjects: %v", err)
	}
	return n
}

func readVersion(t *testing.T, dataDir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var conf struct {
		Database *struct {
			Version string `json:"version"`
		} `json:"database"`
	}
	if json.Unmarshal(b, &conf) != nil {
		t.Fatalf("config.json 解析失败: %s", b)
	}
	if conf.Database == nil {
		return ""
	}
	return conf.Database.Version
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("不应残留 %s", path)
	}
}

// TestFirstRunDownloadAndImport 首次运行：下载 -> 直接导入 -> 记录版本 -> 删压缩包。
func TestFirstRunDownloadAndImport(t *testing.T) {
	content := fakeDump(t, "钢之炼金术师", "CLANNAD")
	arch := newFakeArchive(t, "dump-v1.zip", content)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "bangumi.db")
	stats, err := Run(context.Background(), Options{
		DBPath:    dbPath,
		LatestURL: arch.latestSrv.URL,
		Threads:   4,
	})
	if err != nil {
		t.Fatalf("Run 首次更新: %v", err)
	}
	if stats == nil || stats.Subjects != 2 {
		t.Fatalf("导入统计异常: %+v", stats)
	}
	if got := countSubjects(t, dbPath); got != 2 {
		t.Errorf("subjects 行数 = %d, want 2", got)
	}
	if v := readVersion(t, dataDir); v != "dump-v1.zip" {
		t.Errorf("config.json 版本 = %q, want dump-v1.zip", v)
	}
	assertNotExists(t, filepath.Join(dataDir, "dump-v1.zip"))
	assertNotExists(t, dbPath+".updating")
}

// TestUpdateSwapsDatabase 已有旧版本库：临时库导入 -> 完整性检查 -> 换库。
func TestUpdateSwapsDatabase(t *testing.T) {
	v1 := fakeDump(t, "旧条目甲", "旧条目乙")
	arch1 := newFakeArchive(t, "dump-v1.zip", v1)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "bangumi.db")
	opts := Options{DBPath: dbPath, LatestURL: arch1.latestSrv.URL, Threads: 3}
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatalf("首次导入: %v", err)
	}
	before := countSubjects(t, dbPath)

	// 上游发布新版本（内容变化）
	v2 := fakeDump(t, "新条目A", "新条目B", "新条目C")
	arch2 := newFakeArchive(t, "dump-v2.zip", v2)
	opts.LatestURL = arch2.latestSrv.URL
	stats, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("更新: %v", err)
	}
	if stats == nil || stats.Subjects != 3 {
		t.Fatalf("更新统计异常: %+v", stats)
	}
	if got := countSubjects(t, dbPath); got != 3 {
		t.Errorf("更新后 subjects = %d, want 3（更新前 %d）", got, before)
	}
	if v := readVersion(t, dataDir); v != "dump-v2.zip" {
		t.Errorf("更新后版本 = %q, want dump-v2.zip", v)
	}
	assertNotExists(t, dbPath+".updating")
	assertNotExists(t, filepath.Join(dataDir, "dump-v1.zip"))
	assertNotExists(t, filepath.Join(dataDir, "dump-v2.zip"))
}

// TestUpToDateSkipsDownload 版本一致：不发起任何下载请求。
func TestUpToDateSkipsDownload(t *testing.T) {
	content := fakeDump(t, "某条目")
	arch := newFakeArchive(t, "dump-v1.zip", content)

	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "bangumi.db")
	opts := Options{DBPath: dbPath, LatestURL: arch.latestSrv.URL}
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatalf("首次 Run: %v", err)
	}
	hitsBefore := arch.requests()
	stats, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("重复 Run: %v", err)
	}
	if stats != nil {
		t.Error("已是最新时应返回 nil 统计")
	}
	if arch.requests() != hitsBefore {
		t.Errorf("已是最新时不应重新下载，请求数 %d -> %d", hitsBefore, arch.requests())
	}
}

// TestLegacyDBWithoutVersionUpdates 旧版本程序创建的库（无版本记录）：
// 默认视为落后并完成更新。
func TestLegacyDBWithoutVersionUpdates(t *testing.T) {
	v1 := fakeDump(t, "占位条目")
	arch1 := newFakeArchive(t, "dump-v1.zip", v1)
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "bangumi.db")

	// 先正常导入一次，再抹掉 config.json，模拟旧版程序创建的库升级
	opts := Options{DBPath: dbPath, LatestURL: arch1.latestSrv.URL}
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatalf("首次 Run: %v", err)
	}
	if err := os.Remove(filepath.Join(dataDir, "config.json")); err != nil {
		t.Fatal(err)
	}

	v2 := fakeDump(t, "升级后条目1", "升级后条目2")
	arch2 := newFakeArchive(t, "dump-v2.zip", v2)
	opts.LatestURL = arch2.latestSrv.URL
	if _, err := Run(context.Background(), opts); err != nil {
		t.Fatalf("旧库升级: %v", err)
	}
	if got := countSubjects(t, dbPath); got != 2 {
		t.Errorf("升级后 subjects = %d, want 2", got)
	}
	if v := readVersion(t, dataDir); v != "dump-v2.zip" {
		t.Errorf("升级后版本 = %q, want dump-v2.zip", v)
	}
}
