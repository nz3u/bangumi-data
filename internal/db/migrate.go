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
	"html"
	"log"
	"strings"

	"bangumi-subject-go/internal/norm"
	"bangumi-subject-go/internal/wiki"
)

const nameCNBackfillDone = "name_cn_backfilled"
const subjectSearchBuilt = "subject_search_built"
const tagStatsBuilt = "tag_stats_built"
const tagMapsBuilt = "tag_maps_built"
const episodesFTSBuilt = "episodes_fts_built"
const entitiesDecoded = "entities_decoded"

// UpgradeSchema 幂等升级旧库结构。步骤顺序经过安排：
// 先补列、解码上游 HTML 实体，再回填派生列并构建 FTS——
// 使每张 FTS 只构建一次，且内容基于解码后的最终文本；
// 空库（尚未导入）各步骤自动跳过且不产生「0 行」日志。
// 各步骤均以 schema_meta 标记或列结构判断是否已完成，
// 新导入的库（FinalizeSchema 已置全部标记）仅做毫秒级检查。
func UpgradeSchema(conn *sql.DB) error {
	if err := ExecMulti(conn, `CREATE TABLE IF NOT EXISTS schema_meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return err
	}

	// 1. 补列（文本解码与派生列回填的前置条件），并同步补建配套索引
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
	subjectAdded := false
	for _, col := range []string{"aliases", "search_norm"} {
		has, err := tableHasColumn(conn, "subjects", col)
		if err != nil {
			return fmt.Errorf("检查 subjects.%s: %w", col, err)
		}
		if !has {
			if _, err := conn.Exec(fmt.Sprintf(`ALTER TABLE subjects ADD COLUMN %s TEXT NOT NULL DEFAULT ''`, col)); err != nil {
				return fmt.Errorf("补建 subjects.%s: %w", col, err)
			}
			log.Printf("已为 subjects 补建 %s 列", col)
			subjectAdded = true
		}
	}
	hasEps, err := tableExists(conn, "episodes")
	if err != nil {
		return fmt.Errorf("检查 episodes: %w", err)
	}
	epAdded := false
	if hasEps {
		hasNorm, err := tableHasColumn(conn, "episodes", "search_norm")
		if err != nil {
			return fmt.Errorf("检查 episodes.search_norm: %w", err)
		}
		if !hasNorm {
			if _, err := conn.Exec(`ALTER TABLE episodes ADD COLUMN search_norm TEXT NOT NULL DEFAULT ''`); err != nil {
				return fmt.Errorf("补建 episodes.search_norm: %w", err)
			}
			log.Println("已为 episodes 补建 search_norm 列")
			epAdded = true
		}
	}

	// 2. HTML 实体解码：上游 wiki 数据的文本字段带 MediaWiki 转义（&amp; &lt; &#39; …），
	//    按原文存储会以「&amp;」形态原样显示，且破坏检索（归一化丢弃 "&" 后
	//    "A&amp;B" 折叠为 "aampb"，按 "ab" 搜不到）。解码必须先于一切回填与
	//    FTS 构建，使后续构建都基于解码后的文本（否则需要整体重建 FTS）。
	//    解码函数会对变更行重算 aliases/search_norm/name_cn 派生列，
	//    其余行的派生列由下方回填统一补齐。新导入的库在导入时已解码，
	//    由 FinalizeSchema 置标记跳过。
	if done, err := metaGet(conn, entitiesDecoded); err != nil {
		return err
	} else if done != "1" {
		changed := int64(0)
		for _, st := range []struct {
			table string
			fn    func(*sql.DB) (int64, error)
		}{
			{"subjects", decodeSubjectEntities},
			{"episodes", decodeEpisodeEntities},
			{"persons", func(c *sql.DB) (int64, error) { return decodePersonEntities(c, "persons") }},
			{"characters", func(c *sql.DB) (int64, error) { return decodePersonEntities(c, "characters") }},
			{"person_characters", decodePersonCharSummary},
		} {
			has, err := tableExists(conn, st.table)
			if err != nil {
				return fmt.Errorf("检查 %s: %w", st.table, err)
			}
			if !has {
				continue
			}
			n, err := st.fn(conn)
			if err != nil {
				return fmt.Errorf("解码 HTML 实体: %w", err)
			}
			changed += n
		}
		if changed > 0 {
			log.Printf("已解码 HTML 实体（&amp; 等）共 %d 行（耗时一次性，之后跳过）", changed)
		}
		if err := metaSet(conn, entitiesDecoded, "1"); err != nil {
			return err
		}
	}

	// 3. 回填「简体中文名」（解码后仍为空 infobox 的行；新库导入时已写入，
	//    由 FinalizeSchema 置标记跳过）
	if done, err := metaGet(conn, nameCNBackfillDone); err != nil {
		return err
	} else if altered || done != "1" {
		for _, table := range []string{"persons", "characters"} {
			n, err := backfillNameCN(conn, table)
			if err != nil {
				return fmt.Errorf("回填 %s.name_cn: %w", table, err)
			}
			if n > 0 {
				log.Printf("已从 infobox 回填 %s.name_cn %d 行", table, n)
			}
		}
		if err := metaSet(conn, nameCNBackfillDone, "1"); err != nil {
			return err
		}
	}

	// 4. 人物/角色 FTS 缺少 name_cn 时重建（填充的是解码+回填后的最终文本）
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
		if tableRows(conn, "persons") > 0 || tableRows(conn, "characters") > 0 {
			log.Println("已重建 persons_fts / characters_fts（含 name_cn）")
		}
	}

	// 5. 条目检索列回填与条目 FTS：
	//    subjects 原本只按 name / name_cn 建 trigram 索引，存在两类漏召回——
	//    检索词被原名中的符号切断（「少女歌剧」匹配不到「少女☆歌剧」）、
	//    检索词只出现在 infobox 的别名里（「Kaguya Hime」）。
	//    补建 aliases（infobox 别名）与 search_norm（归一化后的可搜索文本），
	//    并把 subjects_fts 重建为只索引 search_norm 的单列表。
	//    68 万行一次性回填约需数十秒，完成后置标记跳过。
	if done, err := metaGet(conn, subjectSearchBuilt); err != nil {
		return err
	} else if subjectAdded || done != "1" {
		n, err := backfillSubjectSearch(conn)
		if err != nil {
			return fmt.Errorf("回填 subjects 检索列: %w", err)
		}
		if n > 0 {
			log.Printf("已从 infobox 回填 subjects.aliases/search_norm %d 行（耗时一次性，之后跳过）", n)
		}
		if err := metaSet(conn, subjectSearchBuilt, "1"); err != nil {
			return err
		}
	}
	hasNormFTS, err := ftsHasColumn(conn, "subjects_fts", "search_norm")
	if err != nil {
		return fmt.Errorf("检查 subjects_fts 结构: %w", err)
	}
	if !hasNormFTS {
		if err := ExecMulti(conn, `DROP TABLE IF EXISTS subjects_fts;`); err != nil {
			return err
		}
		if err := ExecMulti(conn, ftsSubjectSQL); err != nil {
			return err
		}
		if err := ExecMulti(conn, ftsSubjectPopulateSQL); err != nil {
			return err
		}
		if tableRows(conn, "subjects") > 0 {
			log.Println("已重建 subjects_fts（单列 search_norm）")
		}
	}

	// 6. 标签/元标签派生表（聚合表=建议数据源，倒排映射表=标签过滤索引）：
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
		if nAgg+nMap > 0 {
			log.Printf("已构建标签/元标签聚合与倒排映射表（聚合 %d 行、映射 %d 行，耗时一次性，之后跳过）", nAgg, nMap)
		}
		if err := metaSet(conn, tagStatsBuilt, "1"); err != nil {
			return err
		}
		if err := metaSet(conn, tagMapsBuilt, "1"); err != nil {
			return err
		}
	}

	// 7. 章节检索列回填与章节 FTS：
	//    episodes.search_norm（归一化后的 name + name_cn）已在步骤 1 补列，
	//    此处回填空行并构建只索引该列的 episodes_fts。章节搜索中「命中所属
	//    条目标题」的部分复用 subjects_fts（见 api.searchEpisodes），无需重复存储。
	//    旧库一次性回填约 170 万行，需 1~2 分钟，完成后置标记跳过。
	if hasEps {
		ftsDone, err := metaGet(conn, episodesFTSBuilt)
		if err != nil {
			return err
		}
		needsBackfill := epAdded || ftsDone != "1"
		if needsBackfill {
			n, err := backfillEpisodesSearch(conn)
			if err != nil {
				return fmt.Errorf("回填 episodes.search_norm: %w", err)
			}
			if n > 0 {
				log.Printf("已回填 episodes.search_norm %d 行（耗时一次性，之后跳过）", n)
			}
			if err := metaSet(conn, episodesFTSBuilt, "1"); err != nil {
				return err
			}
		}
		hasEpsFTS, err := tableExists(conn, "episodes_fts")
		if err != nil {
			return fmt.Errorf("检查 episodes_fts: %w", err)
		}
		if needsBackfill || !hasEpsFTS {
			if err := ExecMulti(conn, `DROP TABLE IF EXISTS episodes_fts;`); err != nil {
				return err
			}
			if err := ExecMulti(conn, ftsEpisodesSQL); err != nil {
				return err
			}
			if err := ExecMulti(conn, ftsEpisodesPopulateSQL); err != nil {
				return err
			}
			if tableRows(conn, "episodes") > 0 {
				log.Println("已构建 episodes_fts（单列 search_norm）")
			}
		}
	}
	return nil
}

// NeedsUpgrade 快速判断数据库是否需要执行结构迁移/回填（只读，毫秒级）。
// serve 启动据此决定走同步快路径（无需迁移时的空操作检查），还是转入
// 后台迁移并开启维护模式（避免迁移耗时阻塞站点访问）。
// 空库（尚未导入）无需迁移——建表由 EnsureIndexes 负责，迁移在导入后进行。
// 保守策略：拿不准一律返回 true，误报仅使本次检查走后台维护路径，
// 漏报则退化为启动时同步执行；新增迁移步骤时须同步更新本函数的检查项。
func NeedsUpgrade(conn *sql.DB) bool {
	// 任一业务表缺失视为空库/全新库
	for _, table := range []string{"subjects", "episodes", "persons", "characters"} {
		has, err := tableExists(conn, table)
		if err != nil || !has {
			return false
		}
	}
	// 空库无需迁移
	if tableRows(conn, "subjects") == 0 {
		return false
	}
	// 迁移标记任一缺失即需要
	for _, flag := range []string{
		nameCNBackfillDone, subjectSearchBuilt, tagStatsBuilt,
		tagMapsBuilt, episodesFTSBuilt, entitiesDecoded,
	} {
		if done, err := metaGet(conn, flag); err != nil || done != "1" {
			return true
		}
	}
	// 迁移目标列缺失即需要
	for _, tc := range [][2]string{
		{"persons", "name_cn"}, {"characters", "name_cn"},
		{"subjects", "aliases"}, {"subjects", "search_norm"},
		{"episodes", "search_norm"},
	} {
		has, err := tableHasColumn(conn, tc[0], tc[1])
		if err != nil || !has {
			return true
		}
	}
	// FTS 结构（列缺失或整表缺失时迁移会重建）
	for _, fc := range [][2]string{
		{"subjects_fts", "search_norm"},
		{"persons_fts", "name_cn"}, {"characters_fts", "name_cn"},
	} {
		has, err := ftsHasColumn(conn, fc[0], fc[1])
		if err != nil || !has {
			return true
		}
	}
	if has, err := tableExists(conn, "episodes_fts"); err != nil || !has {
		return true
	}
	// 增量索引缺失即需要（与 ensureSQL 的清单保持一致）
	for _, idx := range []string{
		"idx_sp_subj_person", "idx_pc_subj_person",
		"idx_episodes_type", "idx_episodes_airdate",
	} {
		has, err := indexExists(conn, idx)
		if err != nil || !has {
			return true
		}
	}
	return false
}

// tableRows 返回表行数（表不存在时返回 0）。
func tableRows(conn *sql.DB, table string) int64 {
	var n int64
	_ = conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n
}

// indexExists 检查索引是否存在。
func indexExists(conn *sql.DB, name string) (bool, error) {
	var n int64
	err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&n)
	return n > 0, err
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

// backfillSubjectSearch 全表扫描 search_norm 为空的行，解析 infobox 抽取别名，
// 并与 name / name_cn 一起算出归一化检索串后批量更新。
// 先收集再写回，避免读游标与批量更新争抢连接（同 backfillNameCN）。
func backfillSubjectSearch(conn *sql.DB) (int64, error) {
	rows, err := conn.Query(`SELECT id, name, name_cn, infobox FROM subjects WHERE search_norm = ''`)
	if err != nil {
		return 0, err
	}
	type searchRow struct {
		id      int64
		aliases string
		norm    string
	}
	updates := make([]searchRow, 0, 4096)
	scanned := 0
	for rows.Next() {
		var (
			id      int64
			name    string
			nameCN  string
			infobox string
		)
		if err := rows.Scan(&id, &name, &nameCN, &infobox); err != nil {
			rows.Close()
			return 0, err
		}
		scanned++
		if len(updates) > 0 && scanned%200000 == 0 {
			log.Printf("回填 subjects 检索列：已扫描 %d 行…", scanned)
		}
		aliases := wiki.ExtractAliasesText(infobox)
		updates = append(updates, searchRow{id: id, aliases: aliases, norm: norm.Join(name, nameCN, aliases)})
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
		stmt, err := tx.Prepare(`UPDATE subjects SET aliases = ?, search_norm = ? WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return int64(applied), err
		}
		for _, u := range updates[start:end] {
			if _, err := stmt.Exec(u.aliases, u.norm, u.id); err != nil {
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

// backfillEpisodesSearch 全表扫描 search_norm 为空的章节行，
// 用 name / name_cn 算出归一化检索串后批量更新。
// 标题原本就为空的行（多为音乐碟轨）归一化结果仍为空，无需写回，
// 跳过它们可省掉约 1/5 的无效写入。
func backfillEpisodesSearch(conn *sql.DB) (int64, error) {
	rows, err := conn.Query(`SELECT id, name, name_cn FROM episodes WHERE search_norm = ''`)
	if err != nil {
		return 0, err
	}
	type searchRow struct {
		id   int64
		norm string
	}
	updates := make([]searchRow, 0, 4096)
	scanned := 0
	for rows.Next() {
		var (
			id     int64
			name   string
			nameCN string
		)
		if err := rows.Scan(&id, &name, &nameCN); err != nil {
			rows.Close()
			return 0, err
		}
		scanned++
		if len(updates) > 0 && scanned%500000 == 0 {
			log.Printf("回填 episodes 检索列：已扫描 %d 行…", scanned)
		}
		if name == "" && nameCN == "" {
			continue
		}
		updates = append(updates, searchRow{id: id, norm: norm.Join(name, nameCN)})
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
		stmt, err := tx.Prepare(`UPDATE episodes SET search_norm = ? WHERE id = ?`)
		if err != nil {
			tx.Rollback()
			return int64(applied), err
		}
		for _, u := range updates[start:end] {
			if _, err := stmt.Exec(u.norm, u.id); err != nil {
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

// decodeSubjectEntities 解码 subjects 文本字段中的 HTML 实体，
// 并从解码后的 infobox 重算 aliases 与 search_norm。返回实际变更的行数。
// 仅写回有变化的行，避免对合法含 '&' 文本（如「A&B」）的无谓重写。
func decodeSubjectEntities(conn *sql.DB) (int64, error) {
	rows, err := conn.Query(`SELECT id, name, name_cn, summary, infobox FROM subjects
		WHERE name LIKE '%&%' OR name_cn LIKE '%&%' OR summary LIKE '%&%' OR infobox LIKE '%&%'`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id                         int64
		name, nameCN, summary, ib  string
		aliases, norm              string
	}
	updates := make([]row, 0, 1024)
	for rows.Next() {
		var (
			r                        row
			name, nameCN, summary, ib string
		)
		if err := rows.Scan(&r.id, &name, &nameCN, &summary, &ib); err != nil {
			rows.Close()
			return 0, err
		}
		r.name = normDecode(name)
		r.nameCN = normDecode(nameCN)
		r.summary = normDecode(summary)
		r.ib = normDecode(ib)
		r.aliases = wiki.ExtractAliasesText(r.ib)
		r.norm = norm.Join(r.name, r.nameCN, r.aliases)
		if r.name != name || r.nameCN != nameCN || r.summary != summary || r.ib != ib {
			updates = append(updates, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := applyUpdates(conn, `UPDATE subjects SET name=?, name_cn=?, summary=?, infobox=?, aliases=?, search_norm=? WHERE id=?`,
		func(stmt *sql.Stmt) error {
			for _, u := range updates {
				if _, err := stmt.Exec(u.name, u.nameCN, u.summary, u.ib, u.aliases, u.norm, u.id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return 0, err
	}
	return int64(len(updates)), nil
}

// decodeEpisodeEntities 解码 episodes 文本字段并重算 search_norm。
func decodeEpisodeEntities(conn *sql.DB) (int64, error) {
	rows, err := conn.Query(`SELECT id, name, name_cn, description FROM episodes
		WHERE name LIKE '%&%' OR name_cn LIKE '%&%' OR description LIKE '%&%'`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id                      int64
		name, nameCN, desc, norm string
	}
	updates := make([]row, 0, 1024)
	for rows.Next() {
		var (
			r                    row
			name, nameCN, desc   string
		)
		if err := rows.Scan(&r.id, &name, &nameCN, &desc); err != nil {
			rows.Close()
			return 0, err
		}
		r.name = normDecode(name)
		r.nameCN = normDecode(nameCN)
		r.desc = normDecode(desc)
		r.norm = norm.Join(r.name, r.nameCN)
		if r.name != name || r.nameCN != nameCN || r.desc != desc {
			updates = append(updates, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := applyUpdates(conn, `UPDATE episodes SET name=?, name_cn=?, description=?, search_norm=? WHERE id=?`,
		func(stmt *sql.Stmt) error {
			for _, u := range updates {
				if _, err := stmt.Exec(u.name, u.nameCN, u.desc, u.norm, u.id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return 0, err
	}
	return int64(len(updates)), nil
}

// decodePersonEntities 解码 persons/characters 文本字段，
// 并从解码后的 infobox 重取「简体中文名」。
func decodePersonEntities(conn *sql.DB, table string) (int64, error) {
	rows, err := conn.Query(fmt.Sprintf(`SELECT id, name, infobox, summary FROM %s
		WHERE name LIKE '%%&%%' OR infobox LIKE '%%&%%' OR summary LIKE '%%&%%'`, table))
	if err != nil {
		return 0, err
	}
	type row struct {
		id                 int64
		name, ib, summary, nameCN string
	}
	updates := make([]row, 0, 64)
	for rows.Next() {
		var (
			r                    row
			name, ib, summary    string
		)
		if err := rows.Scan(&r.id, &name, &ib, &summary); err != nil {
			rows.Close()
			return 0, err
		}
		r.name = normDecode(name)
		r.ib = normDecode(ib)
		r.summary = normDecode(summary)
		r.nameCN = wiki.ExtractNameCN(r.ib)
		if r.name != name || r.ib != ib || r.summary != summary {
			updates = append(updates, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := applyUpdates(conn, fmt.Sprintf(`UPDATE %s SET name=?, infobox=?, summary=?, name_cn=? WHERE id=?`, table),
		func(stmt *sql.Stmt) error {
			for _, u := range updates {
				if _, err := stmt.Exec(u.name, u.ib, u.summary, u.nameCN, u.id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return 0, err
	}
	return int64(len(updates)), nil
}

// decodePersonCharSummary 解码 person_characters.summary（角色出演感想）。
func decodePersonCharSummary(conn *sql.DB) (int64, error) {
	rows, err := conn.Query(`SELECT rowid, summary FROM person_characters WHERE summary LIKE '%&%'`)
	if err != nil {
		return 0, err
	}
	type row struct {
		rowid   int64
		summary string
	}
	updates := make([]row, 0, 64)
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.rowid, &r.summary); err != nil {
			rows.Close()
			return 0, err
		}
		if d := normDecode(r.summary); d != r.summary {
			r.summary = d
			updates = append(updates, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := applyUpdates(conn, `UPDATE person_characters SET summary=? WHERE rowid=?`,
		func(stmt *sql.Stmt) error {
			for _, u := range updates {
				if _, err := stmt.Exec(u.summary, u.rowid); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		return 0, err
	}
	return int64(len(updates)), nil
}

// applyUpdates 在单个事务中执行解码更新（变更行数量级最多数万行，无需分批）。
func applyUpdates(conn *sql.DB, updateSQL string, exec func(*sql.Stmt) error) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(updateSQL)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := exec(stmt); err != nil {
		stmt.Close()
		tx.Rollback()
		return err
	}
	if err := stmt.Close(); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// normDecode 解码 HTML 实体（无 '&' 时原样返回，见 importer.decodeText）。
func normDecode(s string) string {
	if strings.Contains(s, "&") {
		return html.UnescapeString(s)
	}
	return s
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
		// 心跳仅在确有回填产出时打印，避免无产出的全表扫描留下「0 行」痕迹
		if len(updates) > 0 && scanned%100000 == 0 {
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
