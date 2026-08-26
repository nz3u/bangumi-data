package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSplitChunks(t *testing.T) {
	cases := []struct {
		name          string
		size          int64
		threads       int
		wantN         int64
		minChunkBytes int64
	}{
		{"小文件单块", 1024, 8, 1, minChunkSize},
		{"均分", 100, 4, 4, 0}, // 配合临时调小 minChunkSize 使用
		{"不整除", 103, 4, 4, 0},
		{"连接数多于所需", 100, 32, 32, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.minChunkBytes > 0 {
				old := minChunkSize
				minChunkSize = c.minChunkBytes
				defer func() { minChunkSize = old }()
			} else {
				old := minChunkSize
				minChunkSize = 1
				defer func() { minChunkSize = old }()
			}
			chunks := splitChunks(c.size, c.threads)
			if int64(len(chunks)) != c.wantN {
				t.Fatalf("分块数 = %d, want %d", len(chunks), c.wantN)
			}
			var off int64
			for i := range chunks {
				ch := &chunks[i]
				if ch.start != off {
					t.Fatalf("块 %d 起点=%d, want %d", i, ch.start, off)
				}
				if ch.end < ch.start {
					t.Fatalf("块 %d 区间非法: [%d,%d]", i, ch.start, ch.end)
				}
				off = ch.end + 1
			}
			if off != c.size {
				t.Fatalf("分块总覆盖 %d, want %d", off, c.size)
			}
		})
	}
}

func TestParseDigest(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	hexVal := hex.EncodeToString(raw)
	got, ok := parseDigest("sha256:" + hexVal)
	if !ok || string(got) != string(raw) {
		t.Errorf("parseDigest 合法摘要失败: ok=%v got=%x", ok, got)
	}
	for _, bad := range []string{"", "sha256:", "md5:" + hexVal, "sha256:zz", "sha256:abcd", hexVal} {
		if _, ok := parseDigest(bad); ok {
			t.Errorf("parseDigest(%q) 应不可识别", bad)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0KiB"},
		{434740633, "414.6MiB"},
	}
	for _, c := range cases {
		if got := HumanBytes(c.in); got != c.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// rangeServer 模拟 GitHub release asset：支持 Range 的静态文件服务。
func rangeServer(t *testing.T, content []byte, hits *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		rng := r.Header.Get("Range")
		if rng == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Write(content)
			return
		}
		var start, end int
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", http.StatusBadRequest)
			return
		}
		if start < 0 || end >= len(content) || start > end {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(content)))
			http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestFileMultiThreaded(t *testing.T) {
	old := minChunkSize
	minChunkSize = 1 << 20
	defer func() { minChunkSize = old }()

	content := make([]byte, 5<<20+12345)
	rand.Read(content)
	srv := rangeServer(t, content, nil)

	dst := filepath.Join(t.TempDir(), "dump.zip")
	rel := &Release{
		Name:               "dump-test.zip",
		BrowserDownloadURL: srv.URL,
		Size:               int64(len(content)),
		Digest:             digestOf(content),
	}
	if err := File(context.Background(), rel, dst, 4); err != nil {
		t.Fatalf("File: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Error("多线程下载内容与源不一致")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Error("成功后不应残留 .part 文件")
	}
}

func TestFileReuseExisting(t *testing.T) {
	old := minChunkSize
	minChunkSize = 1 << 20
	defer func() { minChunkSize = old }()

	content := make([]byte, 3<<20)
	rand.Read(content)
	hits := 0
	srv := rangeServer(t, content, &hits)

	dir := t.TempDir()
	dst := filepath.Join(dir, "dump.zip")
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatal(err)
	}
	rel := &Release{
		Name:               "dump-test.zip",
		BrowserDownloadURL: srv.URL,
		Size:               int64(len(content)),
		Digest:             digestOf(content),
	}
	before := hits
	if err := File(context.Background(), rel, dst, 4); err != nil {
		t.Fatalf("File 复用已有文件: %v", err)
	}
	if hits != before {
		t.Errorf("复用时不应发起下载请求，实际请求 %d 次", hits-before)
	}

	// 内容损坏：应重新下载并修复
	if err := os.WriteFile(dst, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := File(context.Background(), rel, dst, 4); err != nil {
		t.Fatalf("File 校验失败后重下: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(content) {
		t.Error("重新下载后内容与源不一致")
	}
}

func TestFileDigestMismatchFails(t *testing.T) {
	old := minChunkSize
	minChunkSize = 1 << 20
	defer func() { minChunkSize = old }()

	content := randomBytes(2 << 20)
	srv := rangeServer(t, content, nil)
	dst := filepath.Join(t.TempDir(), "dump.zip")
	rel := &Release{
		Name:               "dump-test.zip",
		BrowserDownloadURL: srv.URL,
		Size:               int64(len(content)),
		Digest:             digestOf([]byte("other")),
	}
	if err := File(context.Background(), rel, dst, 2); err == nil {
		t.Fatal("摘要不一致时应报错")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Error("失败后不应残留 .part 文件")
	}
}

func TestFileSingleStreamFallback(t *testing.T) {
	content := []byte(strings.Repeat("bangumi", 4096))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 无视 Range，恒返回全量（模拟不支持断点的服务器）
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Write(content)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "dump.zip")
	rel := &Release{
		Name:               "dump-test.zip",
		BrowserDownloadURL: srv.URL,
		Size:               int64(len(content)),
		Digest:             digestOf(content),
	}
	if err := File(context.Background(), rel, dst, 8); err != nil {
		t.Fatalf("File 单线程回退: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(content) {
		t.Error("单线程下载内容与源不一致")
	}
}

// randomBytes 生成 n 字节随机内容（测试辅助）。
func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func TestFetchLatestParsesRealShape(t *testing.T) {
	body := `{
	  "browser_download_url": "https://github.com/bangumi/Archive/releases/download/archive/dump-2026-08-25.210336Z.zip",
	  "content_type": "application/zip",
	  "created_at": "2026-08-25T21:03:37Z",
	  "digest": "sha256:a95ee318a5c1769bc6630cb0593fed3da7392025d69ce29103385db97ec5d0b9",
	  "name": "dump-2026-08-25.210336Z.zip",
	  "size": 434740633,
	  "updated_at": "2026-08-25T21:04:00Z"
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	rel, err := FetchLatest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.Name != "dump-2026-08-25.210336Z.zip" || rel.Size != 434740633 ||
		!strings.HasPrefix(rel.Digest, "sha256:") || !strings.Contains(rel.BrowserDownloadURL, "releases/download") {
		t.Errorf("解析结果异常: %+v", rel)
	}
}
