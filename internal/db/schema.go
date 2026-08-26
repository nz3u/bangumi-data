package db

import "database/sql"

// schema 建表语句（仅裸表，不含索引）。
// JSON 型字段（tags/score_details/favorite/career）以 TEXT 原样存储，
// 查询时用 SQLite JSON1 函数（json_each 等）处理。
//
// 全量导入采用"先裸表装载 -> 再建索引 -> 最后集合式填充 FTS"的顺序：
// 导入过程中无需维护任何二级索引/全文索引，速度显著更快（见 FinalizeSchema）。
const schemaSQL = `
CREATE TABLE IF NOT EXISTS subjects (
    id            INTEGER PRIMARY KEY,
    type          INTEGER NOT NULL,
    name          TEXT    NOT NULL,
    name_cn       TEXT    NOT NULL DEFAULT '',
    infobox       TEXT    NOT NULL DEFAULT '',
    platform      INTEGER NOT NULL DEFAULT 0,
    summary       TEXT    NOT NULL DEFAULT '',
    nsfw          INTEGER NOT NULL DEFAULT 0,
    date          TEXT    NOT NULL DEFAULT '',
    favorite      TEXT    NOT NULL DEFAULT '',
    series        INTEGER NOT NULL DEFAULT 0,
    tags          TEXT    NOT NULL DEFAULT '',
    score         REAL    NOT NULL DEFAULT 0,
    score_details TEXT    NOT NULL DEFAULT '',
    rank          INTEGER NOT NULL DEFAULT 0,
    meta_tags     TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS persons (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    name_cn  TEXT NOT NULL DEFAULT '',
    type     INTEGER NOT NULL,
    career   TEXT NOT NULL DEFAULT '',
    infobox  TEXT NOT NULL DEFAULT '',
    summary  TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0,
    collects INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS characters (
    id       INTEGER PRIMARY KEY,
    role     INTEGER NOT NULL,
    name     TEXT NOT NULL,
    name_cn  TEXT NOT NULL DEFAULT '',
    infobox  TEXT NOT NULL DEFAULT '',
    summary  TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0,
    collects INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS episodes (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    name_cn     TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    airdate     TEXT NOT NULL DEFAULT '',
    disc        INTEGER NOT NULL DEFAULT 0,
    duration    TEXT NOT NULL DEFAULT '',
    subject_id  INTEGER NOT NULL,
    sort        INTEGER NOT NULL DEFAULT 0,
    type        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS subject_relations (
    subject_id         INTEGER NOT NULL,
    relation_type      INTEGER NOT NULL,
    related_subject_id INTEGER NOT NULL,
    "order"            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS subject_persons (
    subject_id  INTEGER NOT NULL,
    person_id   INTEGER NOT NULL,
    position    INTEGER NOT NULL,
    appear_eps  TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS subject_characters (
    character_id INTEGER NOT NULL,
    subject_id   INTEGER NOT NULL,
    type         INTEGER NOT NULL,
    "order"      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS person_characters (
    person_id    INTEGER NOT NULL,
    subject_id   INTEGER NOT NULL,
    character_id INTEGER NOT NULL,
    type         INTEGER NOT NULL DEFAULT 0,
    summary      TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS person_relations (
    person_type       TEXT    NOT NULL,
    person_id         INTEGER NOT NULL,
    related_person_id INTEGER NOT NULL,
    relation_type     INTEGER NOT NULL,
    spoiler           INTEGER NOT NULL DEFAULT 0,
    ended             INTEGER NOT NULL DEFAULT 0
);

-- 标签/元标签聚合表（条目搜索的实时建议数据源）：
-- 从全量 subjects 的 tags/meta_tags JSON 展开统计，导入后由 FinalizeSchema 填充，
-- 旧库由 UpgradeSchema 按 tag_stats_built 标记一次性构建。
CREATE TABLE IF NOT EXISTS subject_tags_agg (
    name TEXT PRIMARY KEY,
    cnt  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS subject_meta_tags_agg (
    name TEXT PRIMARY KEY,
    cnt  INTEGER NOT NULL
);

-- 标签倒排映射表（条目搜索的标签过滤索引）：
-- 标签名 -> 含该标签的条目 id。搜索时以 IN/NOT IN 子查询替代逐行 json_each 解析，
-- WITHOUT ROWID 主键即覆盖索引，正标签命中集与负标签排除集均按索引取。
CREATE TABLE IF NOT EXISTS subject_tags_map (
    tag_name   TEXT    NOT NULL,
    subject_id INTEGER NOT NULL,
    PRIMARY KEY (tag_name, subject_id)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS subject_meta_tags_map (
    tag_name   TEXT    NOT NULL,
    subject_id INTEGER NOT NULL,
    PRIMARY KEY (tag_name, subject_id)
) WITHOUT ROWID;

-- 一次性数据迁移的完成标记（如 persons/characters.name_cn 回填）。
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// tagStatsPopulateSQL 集合式填充标签聚合表与倒排映射表：
// JSON 展开在 SQLite 内部一次完成。INSERT OR REPLACE 保证重复执行幂等
// （如旧库升级部分完成后重跑）。
const tagStatsPopulateSQL = `
INSERT OR REPLACE INTO subject_tags_agg(name, cnt)
SELECT COALESCE(je.value->>'name', '') AS name, COUNT(*) AS cnt
FROM subjects s, json_each(s.tags) je
GROUP BY 1 HAVING name <> '';

INSERT OR REPLACE INTO subject_meta_tags_agg(name, cnt)
SELECT COALESCE(mt.value, '') AS name, COUNT(*) AS cnt
FROM subjects s, json_each(s.meta_tags) mt
GROUP BY 1 HAVING name <> '';

INSERT OR REPLACE INTO subject_tags_map(tag_name, subject_id)
SELECT COALESCE(je.value->>'name', ''), s.id
FROM subjects s, json_each(s.tags) je
WHERE COALESCE(je.value->>'name', '') <> '';

INSERT OR REPLACE INTO subject_meta_tags_map(tag_name, subject_id)
SELECT COALESCE(mt.value, ''), s.id
FROM subjects s, json_each(s.meta_tags) mt
WHERE COALESCE(mt.value, '') <> '';
`

// indexSQL 二级索引。在全部数据装载完成后统一创建（FinalizeSchema），
// 比逐行插入时同步维护快数倍。
const indexSQL = `
CREATE INDEX IF NOT EXISTS idx_subjects_type     ON subjects(type);
CREATE INDEX IF NOT EXISTS idx_subjects_platform ON subjects(platform);
CREATE INDEX IF NOT EXISTS idx_subjects_date     ON subjects(date);
CREATE INDEX IF NOT EXISTS idx_subjects_rank     ON subjects(rank);
CREATE INDEX IF NOT EXISTS idx_subjects_score    ON subjects(score);
CREATE INDEX IF NOT EXISTS idx_subjects_name     ON subjects(name);
CREATE INDEX IF NOT EXISTS idx_subjects_name_cn  ON subjects(name_cn);
CREATE INDEX IF NOT EXISTS idx_persons_name ON persons(name);
CREATE INDEX IF NOT EXISTS idx_persons_name_cn ON persons(name_cn);
CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(name);
CREATE INDEX IF NOT EXISTS idx_characters_name_cn ON characters(name_cn);
CREATE INDEX IF NOT EXISTS idx_episodes_subject ON episodes(subject_id);
CREATE INDEX IF NOT EXISTS idx_episodes_subject_sort ON episodes(subject_id, sort);
CREATE INDEX IF NOT EXISTS idx_sr_subject ON subject_relations(subject_id);
CREATE INDEX IF NOT EXISTS idx_sr_related ON subject_relations(related_subject_id);
CREATE INDEX IF NOT EXISTS idx_sp_subject  ON subject_persons(subject_id);
CREATE INDEX IF NOT EXISTS idx_sp_person   ON subject_persons(person_id);
CREATE INDEX IF NOT EXISTS idx_sp_pos      ON subject_persons(subject_id, position);
CREATE INDEX IF NOT EXISTS idx_sc_subject ON subject_characters(subject_id);
CREATE INDEX IF NOT EXISTS idx_sc_character ON subject_characters(character_id);
CREATE INDEX IF NOT EXISTS idx_pc_person    ON person_characters(person_id);
CREATE INDEX IF NOT EXISTS idx_pc_character ON person_characters(character_id);
CREATE INDEX IF NOT EXISTS idx_pc_subject   ON person_characters(subject_id);
-- 覆盖索引：合作/角色类查询按 subject_id 展开时免回表（appear_eps 等宽列不触碰）
CREATE INDEX IF NOT EXISTS idx_sp_subj_person ON subject_persons(subject_id, person_id);
CREATE INDEX IF NOT EXISTS idx_pc_subj_person ON person_characters(subject_id, person_id);
CREATE INDEX IF NOT EXISTS idx_pr_person  ON person_relations(person_type, person_id);
CREATE INDEX IF NOT EXISTS idx_pr_related ON person_relations(person_type, related_person_id);
`

// ensureSQL 已有数据库升级用的增量索引（serve 启动时幂等补建）。
const ensureSQL = `
CREATE INDEX IF NOT EXISTS idx_sp_subj_person ON subject_persons(subject_id, person_id);
CREATE INDEX IF NOT EXISTS idx_pc_subj_person ON person_characters(subject_id, person_id);
`

// ftsSQL 全文搜索虚拟表。trigram tokenizer 支持中文子串匹配（>=3 字符走索引）。
// 人物/角色的 name_cn 为 infobox 中提取的简体中文名。
const ftsSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS subjects_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
CREATE VIRTUAL TABLE IF NOT EXISTS persons_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
CREATE VIRTUAL TABLE IF NOT EXISTS characters_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
`

// ftsPopulateSQL 集合式填充 FTS：单条 INSERT...SELECT 在 SQLite 内部完成，
// 避免 Go 侧逐行 Exec 的开销与 trigram 分词的往返成本。
const ftsPopulateSQL = `
INSERT INTO subjects_fts(rowid, name, name_cn) SELECT id, name, name_cn FROM subjects;
INSERT INTO persons_fts(rowid, name, name_cn) SELECT id, name, name_cn FROM persons;
INSERT INTO characters_fts(rowid, name, name_cn) SELECT id, name, name_cn FROM characters;
`

// ftsPersonCharPopulateSQL 仅重建人物/角色 FTS 时的填充语句（升级迁移用）。
const ftsPersonCharPopulateSQL = `
INSERT INTO persons_fts(rowid, name, name_cn) SELECT id, name, name_cn FROM persons;
INSERT INTO characters_fts(rowid, name, name_cn) SELECT id, name, name_cn FROM characters;
`

// ftsPersonCharSQL 仅人物/角色 FTS 的建表语句（升级迁移用）。
const ftsPersonCharSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS persons_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
CREATE VIRTUAL TABLE IF NOT EXISTS characters_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
`

// DropAll 删除全部业务表（全量重建导入时调用）。
const dropSQL = `
DROP TABLE IF EXISTS subjects;
DROP TABLE IF EXISTS persons;
DROP TABLE IF EXISTS characters;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS subject_relations;
DROP TABLE IF EXISTS subject_persons;
DROP TABLE IF EXISTS subject_characters;
DROP TABLE IF EXISTS person_characters;
DROP TABLE IF EXISTS person_relations;
DROP TABLE IF EXISTS subjects_fts;
DROP TABLE IF EXISTS persons_fts;
DROP TABLE IF EXISTS characters_fts;
DROP TABLE IF EXISTS subject_tags_agg;
DROP TABLE IF EXISTS subject_meta_tags_agg;
DROP TABLE IF EXISTS subject_tags_map;
DROP TABLE IF EXISTS subject_meta_tags_map;
`

// InitSchema 创建裸表（不含索引与 FTS），供全量导入快速装载数据。
func InitSchema(conn *sql.DB) error {
	return ExecMulti(conn, schemaSQL)
}

// FinalizeSchema 数据装载完成后调用：创建二级索引、FTS 虚拟表并集合式填充，
// 同时构建标签/元标签聚合表与倒排映射表（建议与标签过滤数据源）。
// 新导入的数据已含 name_cn 列，直接标记回填完成，避免 serve 启动时重复扫描。
func FinalizeSchema(conn *sql.DB) error {
	if err := ExecMulti(conn, indexSQL); err != nil {
		return err
	}
	if err := ExecMulti(conn, ftsSQL); err != nil {
		return err
	}
	if err := ExecMulti(conn, ftsPopulateSQL); err != nil {
		return err
	}
	if err := ExecMulti(conn, tagStatsPopulateSQL); err != nil {
		return err
	}
	return ExecMulti(conn,
		`INSERT OR REPLACE INTO schema_meta(key, value)
		 VALUES ('name_cn_backfilled', '1'), ('tag_stats_built', '1'), ('tag_maps_built', '1')`)
}

// EnsureIndexes 为已有数据库幂等补建后加的索引（serve 启动时调用）。
// 已存在时为空操作，仅首次升级需一次性构建开销。
func EnsureIndexes(conn *sql.DB) error {
	return ExecMulti(conn, ensureSQL)
}

// DropAllTables 删除全部表（用于全量重建）。
func DropAllTables(conn *sql.DB) error {
	return ExecMulti(conn, dropSQL)
}
