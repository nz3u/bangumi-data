package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	"bangumi-subject-go/internal/norm"

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
CREATE TABLE episodes (id INTEGER PRIMARY KEY, name TEXT NOT NULL,
    name_cn TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '',
    airdate TEXT NOT NULL DEFAULT '', disc INTEGER NOT NULL DEFAULT 0,
    duration TEXT NOT NULL DEFAULT '', subject_id INTEGER NOT NULL,
    sort INTEGER NOT NULL DEFAULT 0, type INTEGER NOT NULL DEFAULT 0);
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
	mustExec(t, conn, `INSERT INTO subjects (id, type, name, summary, infobox, tags, meta_tags) VALUES
		(12, 2, 'A&amp;Bテスト', '剧情&amp;&lt;战斗&gt;简介', '{{Infobox Anime
|别名={
[X&amp;Y]
}
}}', '[]', '[]')`)
	mustExec(t, conn, `INSERT INTO episodes (id, name, name_cn, subject_id, sort, type) VALUES
		(100, '第1話：狂乱の旗', '第1话：狂乱之旗', 10, 1, 0),
		(101, 'オープニング', '', 10, 0, 2),
		(102, 'C&amp;D&#39;E', '', 12, 1, 0)`)
	mustExec(t, conn, `INSERT INTO persons (id, name, type, career, infobox) VALUES
		(3, 'studio&amp;co', 2, '[]', '{{Infobox Person
|简体中文名= A&amp;B工作室
|性别= 男
}}')`)

	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema: %v", err)
	}

	// 章节检索列应被回填（name + name_cn 归一化拼接，标点被丢弃）
	var normCol string
	if err := conn.QueryRow(`SELECT search_norm FROM episodes WHERE id = 100`).Scan(&normCol); err != nil || normCol != "第1話狂乱の旗第1话狂乱之旗" {
		t.Errorf("episodes.search_norm = %q, err=%v, want 第1話狂乱の旗第1话狂乱之旗", normCol, err)
	}
	assertFTSHit(t, conn, "episodes_fts", "狂乱之旗", 100)

	// HTML 实体应被解码，派生列（aliases/search_norm/name_cn）随解码后的文本重算
	var name, summary, aliases, searchNorm string
	if err := conn.QueryRow(`SELECT name, summary, aliases, search_norm FROM subjects WHERE id = 12`).Scan(&name, &summary, &aliases, &searchNorm); err != nil {
		t.Fatalf("查询 subjects 12: %v", err)
	}
	if name != "A&Bテスト" || summary != "剧情&<战斗>简介" || aliases != "X&Y" {
		t.Errorf("subjects 12 解码结果 = (%q, %q, %q), want (A&Bテスト, 剧情&<战斗>简介, X&Y)", name, summary, aliases)
	}
	if want := norm.Join("A&Bテスト", "", "X&Y"); searchNorm != want {
		t.Errorf("subjects 12 search_norm = %q, want %q", searchNorm, want)
	}
	if err := conn.QueryRow(`SELECT name, search_norm FROM episodes WHERE id = 102`).Scan(&name, &searchNorm); err != nil || name != "C&D'E" {
		t.Errorf("episodes 102 解码结果 = (%q, %v), want (C&D'E)", name, err)
	}
	if searchNorm != "cde" {
		t.Errorf("episodes 102 search_norm = %q, want cde", searchNorm)
	}
	var pName, pNameCN string
	if err := conn.QueryRow(`SELECT name, name_cn FROM persons WHERE id = 3`).Scan(&pName, &pNameCN); err != nil || pName != "studio&co" || pNameCN != "A&B工作室" {
		t.Errorf("persons 3 解码结果 = (%q, %q, %v), want (studio&co, A&B工作室)", pName, pNameCN, err)
	}
	assertFTSHit(t, conn, "subjects_fts", "テスト", 12)

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

	// 幂等：再次调用为空操作且不破坏数据（含已解码的实体行）
	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema 二次调用: %v", err)
	}
	if err := conn.QueryRow(`SELECT name_cn FROM persons WHERE id = 1`).Scan(&nameCN); err != nil || nameCN != "宫崎骏" {
		t.Errorf("二次升级后 persons.name_cn = %q, err=%v", nameCN, err)
	}
	if err := conn.QueryRow(`SELECT name FROM subjects WHERE id = 12`).Scan(&name); err != nil || name != "A&Bテスト" {
		t.Errorf("二次升级后 subjects 12.name = %q, err=%v, want A&Bテスト", name, err)
	}
	assertFTSHit(t, conn, "persons_fts", "宫崎骏", 1)
	assertFTSHit(t, conn, "episodes_fts", "狂乱之旗", 100)

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

// TestNeedsUpgradeAndEmptyDB 验证 NeedsUpgrade 的判定与空库升级的静默行为：
// 空库无需迁移（避免启动日志出现「0 行」操作记录），升级后 FTS 表可用。
func TestNeedsUpgradeAndEmptyDB(t *testing.T) {
	// 空库（裸表无数据）：无需迁移
	conn := openTestDB(t)
	if err := InitSchema(conn); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if NeedsUpgrade(conn) {
		t.Error("空库应无需迁移")
	}
	// 空库升级应静默完成，且 FTS 表存在（避免空表导致查询接口报错）
	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema(空库): %v", err)
	}
	for _, ft := range []string{"subjects_fts", "persons_fts", "characters_fts", "episodes_fts"} {
		has, err := tableExists(conn, ft)
		if err != nil || !has {
			t.Errorf("空库升级后缺少 %s (err=%v)", ft, err)
		}
	}
	// 幂等
	if err := UpgradeSchema(conn); err != nil {
		t.Fatalf("UpgradeSchema(空库二次): %v", err)
	}

	// 存量库（有数据、标记缺失）：需要迁移
	legacy := openTestDB(t)
	if err := ExecMulti(legacy, legacySchema); err != nil {
		t.Fatalf("建旧表: %v", err)
	}
	mustExec(t, legacy, "INSERT INTO subjects (id, type, name, tags, meta_tags) VALUES (1, 2, 'x', '[]', '[]')")
	mustExec(t, legacy, "INSERT INTO episodes (id, name, subject_id) VALUES (10, 'ep', 1)")
	if !NeedsUpgrade(legacy) {
		t.Error("存量库应需要迁移")
	}
	// 迁移 + 补建增量索引后：无需迁移
	if err := EnsureIndexes(legacy); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	if err := UpgradeSchema(legacy); err != nil {
		t.Fatalf("UpgradeSchema: %v", err)
	}
	if NeedsUpgrade(legacy) {
		t.Error("迁移完成后的库应无需再次迁移")
	}
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
