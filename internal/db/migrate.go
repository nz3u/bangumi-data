// 已有数据库的一次性数据升级（serve 启动时幂等调用）。
//
// 背景：persons/characters 原本只有 name 列；为支持按 infobox 中的
// 「简体中文名」搜索，新增 name_cn 列并回填，同时把 FTS 虚拟表
// 扩展为 (name, name_cn)。旧库首次启动时自动完成迁移：
//  1. 补建 persons.name_cn / characters.name_cn 列（ALTER TABLE）
//  2. 逐行解析 infobox 回填「简体中文名」（耗时一次性操作，
//     完成后写入 schema_meta 标记，之后跳过）
//  3. FTS 表缺少 name_cn 列时重建并重新填充
//  4. 构建标签/元标签聚合表与倒排映射表（建议与标签过滤数据源，
//     tag_stats_built / tag_maps_built 标记）
package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"bangumi-subject-go/internal/wiki"
)

const nameCNBackfillDone = "name_cn_backfilled"
const tagStatsBuilt = "tag_stats_built"
const tagMapsBuilt = "tag_maps_built"

// UpgradeSchema 幂等升级旧库结构：补列 -> 回填简体中文名 -> 重建人物/角色 FTS
// -> 构建标签/元标签聚合表。
// 新导入的库各步骤均检测为已完成，直接返回（仅两次 pragma/master 查询开销）。
func UpgradeSchema(conn *sql.DB) error {
	if err := ExecMulti(conn, `CREATE TABLE IF NOT EXISTS schema_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}

	// 1. 补列
	altered := false
	for _, table := range []string{"persons", "characters"} {
		has, err := tableHasColumn(conn, table, "name_cn")
		if err != nil {
			return fmt.Errorf("检查 %s.%s: %w", table, "name_cn", err)
		}
		if !has {
			if _, err := conn.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN name_cn TEXT NOT NULL DEFAULT ''`, table)); err != nil {
				return fmt.Errorf("补建 %s.name_cn: %w", table, err)
			}
			log.Printf("已为 %s 补建 name_cn 列", table)
			altered = true
		}
	}
	// 同步补建 name_cn 索引，保证与全新导入的库结构一致
	if err := ExecMulti(conn, `CREATE INDEX IF NOT EXISTS idx_persons_name_cn ON persons(name_cn);
		CREATE INDEX IF NOT EXISTS idx_characters_name_cn ON characters(name_cn);`); err != nil {
		return fmt.Errorf("补建 name_cn 索引: %w", err)
	}

	// 2. 回填「简体中文名」（新库导入时已写入，由 FinalizeSchema 置标记跳过）
	done, err := metaGet(conn, nameCNBackfillDone)
	if err != nil {
		return err
	}
	if altered || done != "1" {
		for _, table := range []string{"persons", "characters"} {
			n, err := backfillNameCN(conn, table)
			if err != nil {
				return fmt.Errorf("回填 %s.name_cn: %w", table, err)
			}
			log.Printf("已从 infobox 回填 %s.name_cn %d 行", table, n)
		}
		if err := metaSet(conn, nameCNBackfillDone, "1"); err != nil {
			return err
		}
	}

	// 3. 人物/角色 FTS 缺少 name_cn 时重建
	rebuild := false
	for _, table := range []string{"persons_fts", "characters_fts"} {
		has, err := ftsHasColumn(conn, table, "name_cn")
		if err != nil {
			return fmt.Errorf("检查 %s 结构: %w", table, err)
		}
		if !has {
			rebuild = true
		}
	}
	if rebuild {
		if err := ExecMulti(conn, `DROP TABLE IF EXISTS persons_fts; DROP TABLE IF EXISTS characters_fts;`); err != nil {
			return err
		}
		if err := ExecMulti(conn, ftsPersonCharSQL); err != nil {
			return err
		}
		if err := ExecMulti(conn, ftsPersonCharPopulateSQL); err != nil {
			return err
		}
		log.Println("已重建 persons_fts / characters_fts（含 name_cn）")
	}

	// 4. 标签/元标签派生表（聚合表=建议数据源，倒排映射表=标签过滤索引）：
	//    旧库缺表或未标记时一次性从 subjects 的 JSON 字段展开构建（新导入的库
	//    由 FinalizeSchema 构建并置标记，此处直接跳过）。
	//    聚合表与映射表使用独立标记：已发布版本只置过 tag_stats_built，
	//    映射表须按自己的标记补建，不能复用旧标记。
	statsDone, err := metaGet(conn, tagStatsBuilt)
	if err != nil {
		return err
	}
	mapsDone, err := metaGet(conn, tagMapsBuilt)
	if err != nil {
		return err
	}
	hasAgg, err := tableExists(conn, "subject_tags_agg")
	if err != nil {
		return fmt.Errorf("检查 subject_tags_agg: %w", err)
	}
	hasMap, err := tableExists(conn, "subject_tags_map")
	if err != nil {
		return fmt.Errorf("检查 subject_tags_map: %w", err)
	}
	if statsDone != "1" || mapsDone != "1" || !hasAgg || !hasMap {
		if err := ExecMulti(conn, `CREATE TABLE IF NOT EXISTS subject_tags_agg (
				name TEXT PRIMARY KEY, cnt INTEGER NOT NULL);
			CREATE TABLE IF NOT EXISTS subject_meta_tags_agg (
				name TEXT PRIMARY KEY, cnt INTEGER NOT NULL);
			CREATE TABLE IF NOT EXISTS subject_tags_map (
				tag_name TEXT NOT NULL, subject_id INTEGER NOT NULL,
				PRIMARY KEY (tag_name, subject_id)) WITHOUT ROWID;
			CREATE TABLE IF NOT EXISTS subject_meta_tags_map (
				tag_name TEXT NOT NULL, subject_id INTEGER NOT NULL,
				PRIMARY KEY (tag_name, subject_id)) WITHOUT ROWID;`); err != nil {
			return fmt.Errorf("补建标签派生表: %w", err)
		}
		nAgg, nMap, err := buildTagDerivedTables(conn)
		if err != nil {
			return fmt.Errorf("构建标签派生表: %w", err)
		}
		log.Printf("已构建标签/元标签聚合与倒排映射表（聚合 %d 行、映射 %d 行，耗时一次性，之后跳过）", nAgg, nMap)
		if err := metaSet(conn, tagStatsBuilt, "1"); err != nil {
			return err
		}
		if err := metaSet(conn, tagMapsBuilt, "1"); err != nil {
			return err
		}
	}
	return nil
}

// tableHasColumn 检查表是否存在某列（表不存在时返回 false）。
func tableHasColumn(conn *sql.DB, table, column string) (bool, error) {
	rows, err := conn.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int64
			name      string
			typ       string
			notNull   int64
			dfltValue sql.NullString
			pk        int64
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// ftsHasColumn 通过 sqlite_master 的建表语句判断 FTS 虚拟表是否含某列
// （FTS5 的 PRAGMA table_info 不含隐式列，直接看原始 DDL 更可靠）。
func ftsHasColumn(conn *sql.DB, table, column string) (bool, error) {
	var ddl string
	err := conn.QueryRow(`SELECT COALESCE(sql, '') FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&ddl)
	if err == sql.ErrNoRows {
		return false, nil // 表不存在，视为需要重建
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(ddl, column), nil
}

func metaGet(conn *sql.DB, key string) (string, error) {
	var v string
	err := conn.QueryRow(`SELECT value FROM schema_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func metaSet(conn *sql.DB, key, value string) error {
	_, err := conn.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES (?, ?)`, key, value)
	return err
}

// tableExists 检查表是否存在。
func tableExists(conn *sql.DB, table string) (bool, error) {
	var n int64
	err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	return n > 0, err
}

// buildTagDerivedTables 全量展开 subjects.tags / subjects.meta_tags，
// 构建聚合表（标签使用次数）与倒排映射表（标签 -> 条目 id）。
// INSERT OR REPLACE 幂等，可重复调用；返回聚合行数与映射行数。
func buildTagDerivedTables(conn *sql.DB) (int64, int64, error) {
	if err := ExecMulti(conn, tagStatsPopulateSQL); err != nil {
		return 0, 0, err
	}
	var nAgg, nMap int64
	if err := conn.QueryRow(`SELECT (SELECT COUNT(*) FROM subject_tags_agg) + (SELECT COUNT(*) FROM subject_meta_tags_agg)`).Scan(&nAgg); err != nil {
		return 0, 0, err
	}
	if err := conn.QueryRow(`SELECT (SELECT COUNT(*) FROM subject_tags_map) + (SELECT COUNT(*) FROM subject_meta_tags_map)`).Scan(&nMap); err != nil {
		return 0, 0, err
	}
	return nAgg, nMap, nil
}

// backfillNameCN 全表扫描 name_cn 为空的行，解析 infobox 提取「简体中文名」后批量更新。
// 先收集再写回，避免读游标与批量更新争抢连接。
func backfillNameCN(conn *sql.DB, table string) (int64, error) {
	rows, err := conn.Query(fmt.Sprintf(`SELECT id, infobox FROM %s WHERE name_cn = ''`, table))
	if err != nil {
		return 0, err
	}
	type update struct {
		id int64
		cn string
	}
	updates := make([]update, 0, 4096)
	scanned := 0
	for rows.Next() {
		var (
			id int64
			ib string
		)
		if err := rows.Scan(&id, &ib); err != nil {
			rows.Close()
			return 0, err
		}
		scanned++
		if scanned%100000 == 0 {
			log.Printf("回填 %s.name_cn：已扫描 %d 行…", table, scanned)
		}
		if cn := wiki.ExtractNameCN(ib); cn != "" {
			updates = append(updates, update{id: id, cn: cn})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	const batch = 20000
	applied := 0
	for start := 0; start < len(updates); start += batch {
		end := start + batch
		if end > len(updates) {
			end = len(updates)
		}
		tx, err := conn.Begin()
		if err != nil {
			return int64(applied), err
		}
		stmt, err := tx.Prepare(fmt.Sprintf(`UPDATE %s SET name_cn = ? WHERE id = ?`, table))
		if err != nil {
			tx.Rollback()
			return int64(applied), err
		}
		for _, u := range updates[start:end] {
			if _, err := stmt.Exec(u.cn, u.id); err != nil {
				stmt.Close()
				tx.Rollback()
				return int64(applied), err
			}
		}
		if err := stmt.Close(); err != nil {
			tx.Rollback()
			return int64(applied), err
		}
		if err := tx.Commit(); err != nil {
			return int64(applied), err
		}
		applied = end
	}
	return int64(len(updates)), nil
}
