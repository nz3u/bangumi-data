package db

import "database/sql"

// schema 建表语句。
// JSON 型字段（tags/score_details/favorite/career）以 TEXT 原样存储，
// 查询时用 SQLite JSON1 函数（json_each 等）处理。
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

CREATE INDEX IF NOT EXISTS idx_subjects_type     ON subjects(type);
CREATE INDEX IF NOT EXISTS idx_subjects_platform ON subjects(platform);
CREATE INDEX IF NOT EXISTS idx_subjects_date     ON subjects(date);
CREATE INDEX IF NOT EXISTS idx_subjects_rank     ON subjects(rank);
CREATE INDEX IF NOT EXISTS idx_subjects_score    ON subjects(score);
CREATE INDEX IF NOT EXISTS idx_subjects_name     ON subjects(name);
CREATE INDEX IF NOT EXISTS idx_subjects_name_cn  ON subjects(name_cn);

CREATE TABLE IF NOT EXISTS persons (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    type     INTEGER NOT NULL,
    career   TEXT NOT NULL DEFAULT '',
    infobox  TEXT NOT NULL DEFAULT '',
    summary  TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0,
    collects INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_persons_name ON persons(name);

CREATE TABLE IF NOT EXISTS characters (
    id       INTEGER PRIMARY KEY,
    role     INTEGER NOT NULL,
    name     TEXT NOT NULL,
    infobox  TEXT NOT NULL DEFAULT '',
    summary  TEXT NOT NULL DEFAULT '',
    comments INTEGER NOT NULL DEFAULT 0,
    collects INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(name);

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

CREATE INDEX IF NOT EXISTS idx_episodes_subject ON episodes(subject_id);
CREATE INDEX IF NOT EXISTS idx_episodes_subject_sort ON episodes(subject_id, sort);

CREATE TABLE IF NOT EXISTS subject_relations (
    subject_id         INTEGER NOT NULL,
    relation_type      INTEGER NOT NULL,
    related_subject_id INTEGER NOT NULL,
    "order"            INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sr_subject ON subject_relations(subject_id);
CREATE INDEX IF NOT EXISTS idx_sr_related ON subject_relations(related_subject_id);

CREATE TABLE IF NOT EXISTS subject_persons (
    subject_id  INTEGER NOT NULL,
    person_id   INTEGER NOT NULL,
    position    INTEGER NOT NULL,
    appear_eps  TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sp_subject  ON subject_persons(subject_id);
CREATE INDEX IF NOT EXISTS idx_sp_person   ON subject_persons(person_id);
CREATE INDEX IF NOT EXISTS idx_sp_pos      ON subject_persons(subject_id, position);

CREATE TABLE IF NOT EXISTS subject_characters (
    character_id INTEGER NOT NULL,
    subject_id   INTEGER NOT NULL,
    type         INTEGER NOT NULL,
    "order"      INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sc_subject ON subject_characters(subject_id);
CREATE INDEX IF NOT EXISTS idx_sc_character ON subject_characters(character_id);

CREATE TABLE IF NOT EXISTS person_characters (
    person_id    INTEGER NOT NULL,
    subject_id   INTEGER NOT NULL,
    character_id INTEGER NOT NULL,
    type         INTEGER NOT NULL DEFAULT 0,
    summary      TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_pc_person    ON person_characters(person_id);
CREATE INDEX IF NOT EXISTS idx_pc_character ON person_characters(character_id);
CREATE INDEX IF NOT EXISTS idx_pc_subject   ON person_characters(subject_id);

CREATE TABLE IF NOT EXISTS person_relations (
    person_type       TEXT    NOT NULL,
    person_id         INTEGER NOT NULL,
    related_person_id INTEGER NOT NULL,
    relation_type     INTEGER NOT NULL,
    spoiler           INTEGER NOT NULL DEFAULT 0,
    ended             INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_pr_person  ON person_relations(person_type, person_id);
CREATE INDEX IF NOT EXISTS idx_pr_related ON person_relations(person_type, related_person_id);
`

// ftsSQL 全文搜索虚拟表。trigram tokenizer 支持中文子串匹配（>=3 字符走索引）。
const ftsSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS subjects_fts
    USING fts5(name, name_cn, tokenize = 'trigram');
CREATE VIRTUAL TABLE IF NOT EXISTS persons_fts
    USING fts5(name, tokenize = 'trigram');
CREATE VIRTUAL TABLE IF NOT EXISTS characters_fts
    USING fts5(name, tokenize = 'trigram');
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
`

// InitSchema 创建全部表、索引与 FTS 虚拟表。
func InitSchema(conn *sql.DB) error {
	if err := ExecMulti(conn, schemaSQL); err != nil {
		return err
	}
	return ExecMulti(conn, ftsSQL)
}

// DropAllTables 删除全部表（用于全量重建）。
func DropAllTables(conn *sql.DB) error {
	return ExecMulti(conn, dropSQL)
}
