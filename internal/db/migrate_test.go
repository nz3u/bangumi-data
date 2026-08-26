package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// legacySchema 旧版表结构（persons/characters 无 name_cn，FTS 仅 name 列），
// 模拟升级前的存量库。
const legacySchema = `
CREATE TABLE subjects (id INTEGER PRIMARY KEY, type INTEGER NOT NULL, name TEXT NOT NULL,
    name_cn TEXT NOT NULL DEFAULT '', infobox TEXT NOT NULL DEFAULT '', platform INTEGER NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT '', nsfw INTEGER NOT NULL DEFAULT 0, date TEXT NOT NULL DEFAULT '',
    favorite TEXT NOT NULL DEFAULT '', series INTEGER NOT NULL DEFAULT 0, tags TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0, score_details TEXT NOT NULL DEFAULT '', rank INTEGER NOT NULL DEFAULT 0,
    meta_tags TEXT NOT NULL DEFAULT '');
CREATE TABLE persons (id INTEGER PRIMARY KEY, name TEXT NOT NULL, type INTEGER NOT NULL,
    career TEXT NOT NULL DEFAULT '', infobox TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0, collects INTEGER NOT NULL DEFAULT 0);
CREATE TABLE characters (id INTEGER PRIMARY KEY, role INTEGER NOT NULL, name TEXT NOT NULL,
    infobox TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0, collects INTEGER NOT NULL DEFAULT 0);
CREATE VIRTUAL TABLE subjects_fts USING fts5(name, name_cn, tokenize = 'trigram');
CREATE VIRTUAL TABLE persons_fts USING fts5(name, tokenize = 'trigram');
CREATE VIRTUAL TABLE characters_fts USING fts5(name, tokenize = 'trigram');
`

const personInfobox = `{{Infobox Person
|简体中文名= 宫崎骏
|别名={
[宮崎駿]
}
|性别= 男
}}`

const characterInfobox = `{{Infobox Crt
|简体中文名= 鲁路修·兰佩路基
|性别= 男
}}`

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestUpgradeSchemaFromLegacy(t *testing.T) {
	conn := openTestDB(t)
	if err := ExecMulti(conn, legacySchema); err != nil {
		t.Fatalf("建旧表: %v", err)
	}
	mustExec(t, conn, `INSERT INTO persons (id, name, type, career, infobox) VALUES (1, '宮崎駿', 1, '[]', '`+personInfobox+`')`)
	mustExec(t, conn, `INSERT INTO characters (id, role, name, infobox) VALUES (2, 1, 'ルルーシュ・ランペルージ', '`+characterInfobox+`')`)
	mustExec(t, conn, `INSERT INTO subjects (id, type, name, tags, meta_tags) VALUES
		(10, 2, 'a', '[{"name":"奇幻","count":3},{"name":"原创","count":1}]', '["小说"]'),
		(11, 2, 'b', '[{"name":"奇幻","count":2}]', '["小说","社畜"]')`)

	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema: %v", err)
	}

	var nameCN string
	if err := conn.QueryRow(`SELECT name_cn FROM persons WHERE id = 1`).Scan(&nameCN); err != nil || nameCN != "宫崎骏" {
		t.Errorf("persons.name_cn = %q, err=%v, want 宫崎骏", nameCN, err)
	}
	if err := conn.QueryRow(`SELECT name_cn FROM characters WHERE id = 2`).Scan(&nameCN); err != nil || nameCN != "鲁路修·兰佩路基" {
		t.Errorf("characters.name_cn = %q, err=%v, want 鲁路修·兰佩路基", nameCN, err)
	}

	// FTS 应能命中中文名
	assertFTSHit(t, conn, "persons_fts", "宫崎骏", 1)
	assertFTSHit(t, conn, "characters_fts", "兰佩路基", 2)

	// name_cn 索引应与全新导入的库结构一致
	for _, idx := range []string{"idx_persons_name_cn", "idx_characters_name_cn"} {
		var n int64
		if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("升级后缺少索引 %s (err=%v)", idx, err)
		}
	}

	// 幂等：再次调用为空操作且不破坏数据
	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema 二次调用: %v", err)
	}
	if err := conn.QueryRow(`SELECT name_cn FROM persons WHERE id = 1`).Scan(&nameCN); err != nil || nameCN != "宫崎骏" {
		t.Errorf("二次升级后 persons.name_cn = %q, err=%v", nameCN, err)
	}
	assertFTSHit(t, conn, "persons_fts", "宫崎骏", 1)

	// 标签派生表应被构建且计数正确（重复调用不叠加）
	assertTagAgg(t, conn, "subject_tags_agg", "奇幻", 2)
	assertTagAgg(t, conn, "subject_meta_tags_agg", "小说", 2)
	assertTagAgg(t, conn, "subject_meta_tags_agg", "社畜", 1)
	assertTagMap(t, conn, "subject_tags_map", "奇幻", []int64{10, 11})
	assertTagMap(t, conn, "subject_meta_tags_map", "社畜", []int64{11})
}

func assertTagAgg(t *testing.T, conn *sql.DB, table, name string, wantCnt int64) {
	t.Helper()
	var cnt int64
	err := conn.QueryRow(`SELECT cnt FROM `+table+` WHERE name = ?`, name).Scan(&cnt)
	if err != nil || cnt != wantCnt {
		t.Errorf("%s[%s] = (%d, %v), want cnt %d", table, name, cnt, err, wantCnt)
	}
}

func assertTagMap(t *testing.T, conn *sql.DB, table, tag string, wantIDs []int64) {
	t.Helper()
	rows, err := conn.Query(`SELECT subject_id FROM `+table+` WHERE tag_name = ? ORDER BY subject_id`, tag)
	if err != nil {
		t.Fatalf("查询 %s[%s]: %v", table, tag, err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(got) != len(wantIDs) {
		t.Errorf("%s[%s] = %v, want %v", table, tag, got, wantIDs)
		return
	}
	for i := range got {
		if got[i] != wantIDs[i] {
			t.Errorf("%s[%s] = %v, want %v", table, tag, got, wantIDs)
			return
		}
	}
}

func TestFinalizeSchemaMarksBackfillDone(t *testing.T) {
	conn := openTestDB(t)
	if err := InitSchema(conn); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	mustExec(t, conn, `INSERT INTO persons (id, name, name_cn, type) VALUES (1, 'x', '测试中文名', 1)`)
	mustExec(t, conn, `INSERT INTO characters (id, role, name) VALUES (2, 1, 'z')`)
	mustExec(t, conn, `INSERT INTO subjects (id, type, name, tags, meta_tags) VALUES
		(10, 1, 's', '[{"name":"奇幻","count":9}]', '["小说"]')`)
	if err := FinalizeSchema(conn); err != nil {
		t.Fatalf("FinalizeSchema: %v", err)
	}

	// 新导入路径：name_cn 已在导入时写入，标记应已置位 -> 升级直接跳过回填
	done, err := metaGet(conn, nameCNBackfillDone)
	if err != nil || done != "1" {
		t.Fatalf("schema_meta 标记 = %q, err=%v, want 1", done, err)
	}
	// 标签派生表由导入路径构建并置标记
	assertTagAgg(t, conn, "subject_tags_agg", "奇幻", 1)
	assertTagAgg(t, conn, "subject_meta_tags_agg", "小说", 1)
	assertTagMap(t, conn, "subject_tags_map", "奇幻", []int64{10})
	assertTagMap(t, conn, "subject_meta_tags_map", "小说", []int64{10})
	if done, err = metaGet(conn, tagStatsBuilt); err != nil || done != "1" {
		t.Fatalf("tag_stats_built 标记 = %q, err=%v, want 1", done, err)
	}
	if done, err = metaGet(conn, tagMapsBuilt); err != nil || done != "1" {
		t.Fatalf("tag_maps_built 标记 = %q, err=%v, want 1", done, err)
	}
	before := countRows(t, conn, `SELECT COUNT(*) FROM persons WHERE name_cn = ''`)
	if before != 0 {
		t.Fatalf("前置条件失败：persons 存在空 name_cn")
	}
	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema: %v", err)
	}
	if got := countRows(t, conn, `SELECT COUNT(*) FROM characters WHERE name_cn <> ''`); got != 0 {
		t.Errorf("characters.name_cn 被意外回填 %d 行（infobox 为空应保持空串）", got)
	}
	assertFTSHit(t, conn, "persons_fts", "测试中文", 1) // FTS 正常可用
}

func mustExec(t *testing.T, conn *sql.DB, q string) {
	t.Helper()
	if _, err := conn.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func countRows(t *testing.T, conn *sql.DB, q string) int64 {
	t.Helper()
	var n int64
	if err := conn.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func assertFTSHit(t *testing.T, conn *sql.DB, table, phrase string, wantID int64) {
	t.Helper()
	var id int64
	err := conn.QueryRow(`SELECT rowid FROM `+table+` WHERE `+table+` MATCH ?`, `"`+phrase+`"`).Scan(&id)
	if err != nil || id != wantID {
		t.Errorf("%s MATCH %q: got id=%d err=%v, want %d", table, phrase, id, err, wantID)
	}
}
