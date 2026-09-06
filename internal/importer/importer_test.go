package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"bangumi-subject-go/internal/db"

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

func writeDumpFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
