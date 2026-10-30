package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/norm"

	_ "modernc.org/sqlite"
)

// TestImportDecodesHTMLEntities 验证导入时文本字段的 HTML 实体被解码，
// 且派生列（aliases/search_norm/name_cn）与 FTS 均基于解码后的文本构建。
func TestImportDecodesHTMLEntities(t *testing.T) {
	dir := t.TempDir()
	writeDumpFile(t, dir, "subject.jsonlines",
		`{"id":1,"type":2,"name":"A&amp;B季節物語","name_cn":"A&amp;B季节物语",`+
			`"infobox":"{{Infobox Anime\n|别名={\n[XYZ&amp;U]\n}\n}}",`+
			`"summary":"剧情简介&amp;角色介绍&#39;续","platform":1,"date":"2020-01-01",`+
			`"favorite":{"wish":1,"done":2},"series":false,"tags":[{"name":"原创","count":1}],`+
			`"meta_tags":["小说"],"score_details":{}}`,
	)
	writeDumpFile(t, dir, "episode.jsonlines",
		`{"id":100,"name":"第一話&amp;第二話","name_cn":"","description":"&lt;TM&gt;振り返り",`+
			`"airdate":"","disc":0,"duration":"","subject_id":1,"sort":1,"type":0}`,
	)

	conn, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(conn)

	stats, err := Import(context.Background(), conn, dir, 0)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if stats.Subjects != 1 || stats.Episodes != 1 {
		t.Fatalf("导入统计 = (subjects %d, episodes %d), want (1, 1)", stats.Subjects, stats.Episodes)
	}

	var name, nameCN, summary, aliases, searchNorm string
	if err := conn.QueryRow(`SELECT name, name_cn, summary, aliases, search_norm FROM subjects WHERE id = 1`).
		Scan(&name, &nameCN, &summary, &aliases, &searchNorm); err != nil {
		t.Fatalf("查询 subjects: %v", err)
	}
	if name != "A&B季節物語" || nameCN != "A&B季节物语" || summary != "剧情简介&角色介绍'续" || aliases != "XYZ&U" {
		t.Errorf("subjects 解码结果 = (%q, %q, %q, %q)", name, nameCN, summary, aliases)
	}
	if want := "ab季節物語ab季节物语xyzu"; searchNorm != want {
		t.Errorf("subjects.search_norm = %q, want %q", searchNorm, want)
	}

	var epName, epDesc, epNorm string
	if err := conn.QueryRow(`SELECT name, description, search_norm FROM episodes WHERE id = 100`).
		Scan(&epName, &epDesc, &epNorm); err != nil {
		t.Fatalf("查询 episodes: %v", err)
	}
	if epName != "第一話&第二話" || epDesc != "<TM>振り返り" {
		t.Errorf("episodes 解码结果 = (%q, %q)", epName, epDesc)
	}
	if epNorm != "第一話第二話" {
		t.Errorf("episodes.search_norm = %q, want 第一話第二話", epNorm)
	}

	// FTS 命中的应是解码后的文本（含别名；trigram 需要 >=3 字符的查询词）
	var id int64
	if err := conn.QueryRow(`SELECT rowid FROM subjects_fts WHERE subjects_fts MATCH '"季节物语"'`).Scan(&id); err != nil || id != 1 {
		t.Errorf("subjects_fts MATCH 季节物语: got id=%d err=%v, want 1", id, err)
	}
	if err := conn.QueryRow(`SELECT rowid FROM subjects_fts WHERE subjects_fts MATCH '"xyz"'`).Scan(&id); err != nil || id != 1 {
		t.Errorf("subjects_fts MATCH xyz（别名归一化）: got id=%d err=%v, want 1", id, err)
	}
	if err := conn.QueryRow(`SELECT rowid FROM episodes_fts WHERE episodes_fts MATCH '"第二話"'`).Scan(&id); err != nil || id != 100 {
		t.Errorf("episodes_fts MATCH 第二話: got id=%d err=%v, want 100", id, err)
	}
}

// TestImportPersonAliasRomaji 验证角色/人物 infobox 的别名（含罗马字）在导入时入库，
// 并进入归一化检索串与 FTS：按罗马字 Hirasawa Yui / Mizuki Nana 可命中中文名；
// 罗马字写在顶层 |罗马字= 的资料（少量）同样入库。
func TestImportPersonAliasRomaji(t *testing.T) {
	dir := t.TempDir()
	writeDumpFile(t, dir, "character.jsonlines",
		`{"id":466,"role":1,"name":"平沢唯",`+
			`"infobox":"{{Infobox Crt\n|简体中文名= 平泽唯\n|别名={\n[第二中文名|]\n[英文名|Yui Hirasawa]\n[日文名|平沢 唯]\n[罗马字|Hirasawa Yui]\n[昵称|呆唯]\n}\n|性别= 女\n}}",`+
			`"summary":"","comments":0,"collects":0}`+"\n"+
			`{"id":700,"role":1,"name":"京乃まどか",`+
			`"infobox":"{{Infobox Crt\n|简体中文名= 京乃圆\n|别名={\n[罗马字|]\n[昵称|小圆]\n}\n|罗马字= Kyouno Madoka\n}}",`+
			`"summary":"","comments":0,"collects":0}`)
	writeDumpFile(t, dir, "person.jsonlines",
		`{"id":1,"type":1,"name":"水樹奈々","career":[],`+
			`"infobox":"{{Infobox Crt\n|简体中文名= 水树奈奈\n|别名={\n[纯假名|みずき なな]\n[罗马字|Mizuki Nana]\n}\n}}",`+
			`"summary":"","comments":0,"collects":0}`)

	conn, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close(conn)

	if _, err := Import(context.Background(), conn, dir, 0); err != nil {
		t.Fatalf("Import: %v", err)
	}

	var (
		nameCN, aliases, searchNorm string
	)
	if err := conn.QueryRow(`SELECT name_cn, aliases, search_norm FROM characters WHERE id = 466`).
		Scan(&nameCN, &aliases, &searchNorm); err != nil {
		t.Fatalf("查询 characters: %v", err)
	}
	if nameCN != "平泽唯" {
		t.Errorf("characters.name_cn = %q, want 平泽唯", nameCN)
	}
	// 别名只留值：子类前缀（罗马字/英文名）与空值项都不入库
	if want := "Yui Hirasawa 平沢 唯 Hirasawa Yui 呆唯"; aliases != want {
		t.Errorf("characters.aliases = %q, want %q", aliases, want)
	}
	if want := norm.Join("平沢唯", nameCN, aliases); searchNorm != want {
		t.Errorf("characters.search_norm = %q, want %q", searchNorm, want)
	}

	// 罗马字写在顶层字段的资料：别名列表里没有罗马字，由 ExtractPersonAliasesText 补齐
	var topAliases, topNorm string
	if err := conn.QueryRow(`SELECT aliases, search_norm FROM characters WHERE id = 700`).
		Scan(&topAliases, &topNorm); err != nil {
		t.Fatalf("查询 characters 700: %v", err)
	}
	if want := "小圆 Kyouno Madoka"; topAliases != want {
		t.Errorf("characters 700.aliases = %q, want %q", topAliases, want)
	}
	if want := norm.Join("京乃まどか", "京乃圆", topAliases); topNorm != want {
		t.Errorf("characters 700.search_norm = %q, want %q", topNorm, want)
	}

	var pAliases, pSearchNorm string
	if err := conn.QueryRow(`SELECT aliases, search_norm FROM persons WHERE id = 1`).
		Scan(&pAliases, &pSearchNorm); err != nil {
		t.Fatalf("查询 persons: %v", err)
	}
	if want := "みずき なな Mizuki Nana"; pAliases != want {
		t.Errorf("persons.aliases = %q, want %q", pAliases, want)
	}

	// 检索接口的口径：归一化后的罗马字在 search_norm 上做子串匹配（走 trigram 索引）
	for _, c := range []struct {
		table, qn string
		wantID    int64
	}{
		{"characters_fts", "hirasawayui", 466},
		{"characters_fts", "yuihirasawa", 466}, // 英文名为名在前语序
		{"characters_fts", "pingzewei", 0},     // 拼音不参与索引，未命中属预期
		{"characters_fts", "kyounomadoka", 700}, // 顶层 |罗马字= 的写法
		{"persons_fts", "mizukinana", 1},
	} {
		var id int64
		err := conn.QueryRow(`SELECT rowid FROM `+c.table+` WHERE search_norm LIKE ?`, "%"+c.qn+"%").Scan(&id)
		switch {
		case c.wantID == 0 && err == nil:
			t.Errorf("%s LIKE %%%s%%: 意外命中 id=%d", c.table, c.qn, id)
		case c.wantID != 0 && (err != nil || id != c.wantID):
			t.Errorf("%s LIKE %%%s%%: got id=%d err=%v, want %d", c.table, c.qn, id, err, c.wantID)
		}
	}
}

func writeDumpFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
