package pics

import (
	"path/filepath"
	"strings"
	"testing"

	"bangumi-subject-go/internal/db"
)

func TestExtractRel(t *testing.T) {
	const crt = "/pic/crt/"
	const cover = "/pic/cover/"
	cases := []struct{ name, in, base, want string }{
		// 人物：large 原始 URL（含查询参数）
		{"person large", "https://lain.bgm.tv/pic/crt/l/a6/e8/1_prsn_k7wpt.jpg?r=1723962294", crt, "a6/e8/1_prsn_k7wpt.jpg"},
		// 角色：png 格式
		{"char png", "https://lain.bgm.tv/pic/crt/l/bb/cc/2_prsn_x.png", crt, "bb/cc/2_prsn_x.png"},
		// 条目封面：large / common / medium / grid 各种形态归一化到同一条记录
		{"subject large", "https://lain.bgm.tv/pic/cover/l/b0/e3/826310_9nS09.jpg", cover, "b0/e3/826310_9nS09.jpg"},
		{"subject common", "https://lain.bgm.tv/pic/cover/c/b0/e3/826310_9nS09.jpg", cover, "b0/e3/826310_9nS09.jpg"},
		{"subject medium", "https://lain.bgm.tv/r/200/pic/cover/l/b0/e3/826310_9nS09.jpg", cover, "b0/e3/826310_9nS09.jpg"},
		{"subject grid", "https://lain.bgm.tv/r/100x100/pic/cover/g/b0/e3/826310_9nS09.jpg", cover, "b0/e3/826310_9nS09.jpg"},
		{"empty", "", crt, ""},
		{"non cdn", "https://example.com/other/path.jpg", crt, ""},
	}
	for _, c := range cases {
		if got := extractRel(c.in, c.base); got != c.want {
			t.Errorf("%s: extractRel(%q, %q) = %q, want %q", c.name, c.in, c.base, got, c.want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	const rel = "a6/e8/1_prsn_k7wpt.jpg"
	const relCover = "b0/e3/826310_9nS09.jpg"
	cases := []struct{ name, kind, size, rel, want string }{
		{"person l", KindPerson, "l", rel, "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"person large", KindPerson, "large", rel, "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"person m", KindPerson, "m", rel, "https://lain.bgm.tv/r/200/pic/crt/l/" + rel},
		{"person s", KindPerson, "small", rel, "https://lain.bgm.tv/r/100/pic/crt/l/" + rel},
		{"person g", KindPerson, "grid", rel, "https://lain.bgm.tv/r/100x100/pic/crt/l/" + rel},
		{"person default", KindPerson, "unknown", rel, "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"character l", KindCharacter, "l", rel, "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"character m", KindCharacter, "medium", rel, "https://lain.bgm.tv/r/200/pic/crt/l/" + rel},
		{"subject l", KindSubject, "l", relCover, "https://lain.bgm.tv/pic/cover/l/" + relCover},
		{"subject m", KindSubject, "m", relCover, "https://lain.bgm.tv/r/200/pic/cover/l/" + relCover},
		{"subject s", KindSubject, "s", relCover, "https://lain.bgm.tv/r/100/pic/cover/l/" + relCover},
		{"subject g", KindSubject, "g", relCover, "https://lain.bgm.tv/r/100x100/pic/cover/l/" + relCover},
		{"invalid kind", "book", "l", rel, ""},
		{"empty rel", KindPerson, "l", "", ""},
	}
	for _, c := range cases {
		if got := BuildURL(c.kind, c.rel, c.size); got != c.want {
			t.Errorf("%s: BuildURL(%q,%q,%q) = %q, want %q", c.name, c.kind, c.rel, c.size, got, c.want)
		}
	}
}

func TestValidKind(t *testing.T) {
	for _, k := range []string{KindPerson, KindSubject, KindCharacter} {
		if !ValidKind(k) {
			t.Errorf("ValidKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "book", "persons", "Person"} {
		if ValidKind(k) {
			t.Errorf("ValidKind(%q) = true, want false", k)
		}
	}
}

// TestOpenFresh 全新安装：Open 后三张分表均存在。
func TestOpenFresh(t *testing.T) {
	svc, err := Open(filepath.Join(t.TempDir(), "bgm_pic.db"), "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer svc.Close()
	for _, k := range []string{KindPerson, KindSubject, KindCharacter} {
		var n int
		if err := svc.conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, k).Scan(&n); err != nil || n != 1 {
			t.Errorf("表 %s 不存在 (err=%v, n=%d)", k, err, n)
		}
	}
}

// TestOpenUpgradeLegacyDB 旧库升级：仅有 person 表的旧版库，
// Open 后自动补建 subject / character 且原数据保留。
func TestOpenUpgradeLegacyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bgm_pic.db")
	old, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if _, err := old.Exec(`CREATE TABLE person (
		id  INTEGER PRIMARY KEY,
		url TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("建旧表: %v", err)
	}
	if _, err := old.Exec(`INSERT INTO person(id, url) VALUES(1, 'a6/e8/legacy.jpg')`); err != nil {
		t.Fatalf("写入旧数据: %v", err)
	}
	db.Close(old)

	svc, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open 升级: %v", err)
	}
	defer svc.Close()

	var rel string
	if err := svc.conn.QueryRow(`SELECT url FROM person WHERE id = 1`).Scan(&rel); err != nil || rel != "a6/e8/legacy.jpg" {
		t.Errorf("旧数据丢失: rel=%q err=%v", rel, err)
	}
	for _, k := range []string{KindSubject, KindCharacter} {
		var n int
		if err := svc.conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, k).Scan(&n); err != nil || n != 1 {
			t.Errorf("升级后缺少表 %s (err=%v, n=%d)", k, err, n)
		}
	}
}

// TestResolvePathAndSizes ResolvePath 只返回不含主机的相对路径，
// 各尺寸前缀与 BuildURL 的主机之后部分完全一致。
func TestResolvePathAndSizes(t *testing.T) {
	svc, err := Open(filepath.Join(t.TempDir(), "bgm_pic.db"), "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer svc.Close()
	if err := svc.store(kinds[KindSubject], 7, "b0/e3/x.jpg"); err != nil {
		t.Fatalf("store: %v", err)
	}

	cases := []struct{ size, want string }{
		{"", "/pic/cover/l/b0/e3/x.jpg"},
		{"l", "/pic/cover/l/b0/e3/x.jpg"},
		{"large", "/pic/cover/l/b0/e3/x.jpg"},
		{"m", "/r/200/pic/cover/l/b0/e3/x.jpg"},
		{"medium", "/r/200/pic/cover/l/b0/e3/x.jpg"},
		{"s", "/r/100/pic/cover/l/b0/e3/x.jpg"},
		{"small", "/r/100/pic/cover/l/b0/e3/x.jpg"},
		{"g", "/r/100x100/pic/cover/l/b0/e3/x.jpg"},
		{"grid", "/r/100x100/pic/cover/l/b0/e3/x.jpg"},
	}
	for _, c := range cases {
		st, p := svc.ResolvePath(KindSubject, 7, c.size)
		if st != "ok" || p != c.want {
			t.Errorf("ResolvePath(subject,7,%q) = (%q,%q), want (ok,%q)", c.size, st, p, c.want)
		}
		if full := BuildURL(KindSubject, "b0/e3/x.jpg", c.size); full != "https://lain.bgm.tv"+c.want {
			t.Errorf("BuildURL(size=%q) 与 PathURL 拼接不一致: %q", c.size, full)
		}
	}

	// 未配置 Key 且无记录：直接 failed、空路径
	if st, p := svc.ResolvePath(KindPerson, 999, ""); st != "failed" || p != "" {
		t.Errorf("ResolvePath 无记录 = (%q,%q), want (failed,\"\")", st, p)
	}
	// 非法类型
	if st, p := svc.ResolvePath("book", 1, ""); st != "failed" || p != "" {
		t.Errorf("ResolvePath(book) = (%q,%q), want (failed,\"\")", st, p)
	}
}

// TestResolveRoundTrip 写入相对路径后 Resolve 应返回 ok 及按类型/尺寸拼装的 URL。
func TestResolveRoundTrip(t *testing.T) {
	svc, err := Open(filepath.Join(t.TempDir(), "bgm_pic.db"), "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer svc.Close()
	if err := svc.store(kinds[KindSubject], 42, "b0/e3/x.jpg"); err != nil {
		t.Fatalf("store: %v", err)
	}
	status, url := svc.Resolve(KindSubject, 42, "")
	if status != "ok" || url != "https://lain.bgm.tv/pic/cover/l/b0/e3/x.jpg" {
		t.Errorf("Resolve(subject,42,\"\") = (%q,%q)", status, url)
	}
	// size=grid 应返回 r/100x100 小图
	if _, url := svc.Resolve(KindSubject, 42, "grid"); url != "https://lain.bgm.tv/r/100x100/pic/cover/l/b0/e3/x.jpg" {
		t.Errorf("Resolve(subject,42,\"grid\") url = %q", url)
	}
	// 未配置 Key 时未知记录直接 failed；非法 kind 同样 failed
	if st, _ := svc.Resolve(KindPerson, 7, ""); st != "failed" {
		t.Errorf("Resolve(person,7) = %q, want failed", st)
	}
	if st, u := svc.Resolve("book", 7, ""); st != "failed" || u != "" {
		t.Errorf("Resolve(book,7) = (%q,%q), want (failed,\"\")", st, u)
	}
	// 空标记（确认无图）后 Resolve 恒为 failed
	if err := svc.store(kinds[KindCharacter], 9, ""); err != nil {
		t.Fatalf("store 空标记: %v", err)
	}
	if st, _ := svc.Resolve(KindCharacter, 9, "g"); st != "failed" {
		t.Errorf("Resolve(character,9) = %q, want failed", st)
	}
	if !strings.HasPrefix(BuildURL(KindCharacter, "x/y.jpg", "l"), "https://lain.bgm.tv/pic/crt/l/") {
		t.Error("character URL 前缀错误")
	}
}
