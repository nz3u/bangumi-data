package api

import (
	"database/sql"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/norm"
)

// episodeSubjRef 章节所属条目的精简信息（章节搜索列表展示用）。
type episodeSubjRef struct {
	ID        int64  `json:"id"`
	Type      int    `json:"type"`
	TypeName  string `json:"type_name"`
	Name      string `json:"name"`
	NameCN    string `json:"name_cn"`
	PlatformN string `json:"platform_name"`
	Date      string `json:"date"`
}

// episodeBrief 章节搜索列表项：章节字段（含义同 Archive 的 episode 表：
// sort=集数、disc=所在光盘、airdate=播出时间、duration=时长）+ 所属条目。
type episodeBrief struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	NameCN      string        `json:"name_cn"`
	Description string        `json:"description"`
	Airdate     string        `json:"airdate"`
	Disc        int           `json:"disc"`
	Duration    string        `json:"duration"`
	Sort        float64       `json:"sort"`
	Type        int           `json:"type"`
	TypeName    string        `json:"type_name"`
	Subject     episodeSubjRef `json:"subject"`
}

// episodeSubjectCols 章节搜索结果中条目侧的列（与 scanEpisodeBrief 的扫描顺序对应）。
const episodeSubjectCols = `s.id, s.type, s.name, s.name_cn, s.platform, s.date`

// searchEpisodes 章节搜索与筛选。
// 参数：q（全文搜索，命中章节标题或所属条目标题）、subject_id、type（作品类型）、
// ep_type（章节类型 0正篇/1SP/2OP/3ED/4Trailer/5MAD/6其他）、disc、
// airdate_from、airdate_to（播出时间为自由文本，ISO 格式可作范围匹配）、
// sort(id|subject|sort|airdate|popularity)、order、page、size
func (h *handler) searchEpisodes(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds        []string
		args         []any
		tierOrder    string // 命中按匹配位置分级排序（仅长关键词时启用）
		tierArgs     []any
		usesSubjects bool   // 条件是否引用条目列（决定计数是否需要 JOIN）
		shortQ       bool   // 短关键词（<3 字符）：无法走 trigram 索引，走命中集临时表
		qn           string
	)

	if q != "" {
		// 查询词与索引同口径归一化，因此「少女歌剧」能命中「少女☆歌剧」。
		// 全为符号的查询词不参与索引，按无匹配处理。
		qn = norm.Fold(q)
		if qn == "" {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		if utf8.RuneCountInString(qn) >= ftsMinRunes {
			// 双路检索：章节自身标题走 episodes_fts，所属条目标题（原名/中文名/别名）
			// 复用 subjects_fts，OR 合并。两个子查询不依赖外层、各自走 trigram 索引，
			// 由 multi-index OR 分别经主键与 idx_episodes_subject 索引取候选，
			// 代价与命中集成正比，无需 Go 侧先取命中集。
			like := "%" + qn + "%"
			conds = append(conds,
				"(e.id IN (SELECT rowid FROM episodes_fts WHERE search_norm LIKE ?)"+
					" OR e.subject_id IN (SELECT rowid FROM subjects_fts WHERE search_norm LIKE ?))")
			args = append(args, like, like)
		} else {
			// <3 字符的查询词构不成 trigram，FTS 的 LIKE 会退化为内容表全扫描，
			// 且「计数+取数」会扫描两遍。改走命中集临时表（见下方 short 分支）。
			shortQ = true
		}
		// 分级排序（章节标题命中 > 条目标题命中）对长短查询词语义一致
		tierOrder, tierArgs = episodeTierOrder(q)
	}

	if v, ok := parseIntQuery(c, "subject_id"); ok {
		conds = append(conds, "e.subject_id = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "type"); ok {
		conds = append(conds, "s.type = ?")
		args = append(args, v)
		usesSubjects = true
	}
	if v, ok := parseIntQuery(c, "ep_type"); ok {
		conds = append(conds, "e.type = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "disc"); ok {
		conds = append(conds, "e.disc = ?")
		args = append(args, v)
	}
	if v := c.Query("airdate_from"); v != "" {
		conds = append(conds, "e.airdate >= ?")
		args = append(args, v)
	}
	if v := c.Query("airdate_to"); v != "" {
		conds = append(conds, "e.airdate <= ?")
		args = append(args, v)
	}

	page, size := pagination(c)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// 计数来源：条件未引用条目列时无需 LEFT JOIN——SQLite 不会自动消除该连接，
	// 170 万行逐行主键回表 subjects 是浏览页计数的主要耗时（无 JOIN 时走覆盖索引仅毫秒级）。
	countFrom := " FROM episodes e"
	if usesSubjects {
		countFrom += " LEFT JOIN subjects s ON s.id = e.subject_id"
	}

	// 排序。sort 为空：有关键词时分级 + 条目人气（同条目内按类型/集数聚在一起），
	// 无关键词时按 ID 序。
	order := strings.ToLower(c.DefaultQuery("order", "asc"))
	if order != "desc" {
		order = "asc"
	}
	var orderBy string
	switch c.Query("sort") {
	case "id":
		orderBy = "e.id " + order
	case "subject":
		orderBy = "e.subject_id " + order + ", e.type, e.sort, e.id"
	case "sort":
		orderBy = "e.sort " + order + ", e.subject_id, e.id"
	case "airdate":
		orderBy = "e.airdate " + order + ", e.id"
	case "popularity":
		orderBy = "CAST(COALESCE(json_extract(s.favorite, '$.done'), 0) AS INTEGER) " + order + ", e.subject_id, e.type, e.sort, e.id"
	default:
		if tierOrder != "" {
			orderBy = "CAST(COALESCE(json_extract(s.favorite, '$.done'), 0) AS INTEGER) DESC, e.subject_id, e.type, e.sort, e.id"
		} else {
			orderBy = "e.id " + order
		}
	}
	if tierOrder != "" {
		orderBy = tierOrder + ", " + orderBy
	}

	if shortQ {
		h.searchEpisodesShort(c, qn, conds, args, tierArgs, usesSubjects, orderBy, page, size)
		return
	}

	queryArgs := make([]any, 0, len(args)+len(tierArgs)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tierArgs...)
	queryArgs = append(queryArgs, size, (page-1)*size)

	dataSQL := "SELECT e.id, e.name, e.name_cn, e.description, e.airdate, e.disc, e.duration, e.sort, e.type, " +
		episodeSubjectCols + " FROM episodes e LEFT JOIN subjects s ON s.id = e.subject_id" +
		where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"

	var total int64
	if err := h.getDB().QueryRow("SELECT COUNT(*)"+countFrom+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.getDB().Query(dataSQL, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]*episodeBrief, 0, size)
	for rows.Next() {
		item, err := h.scanEpisodeBrief(rows)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}

// searchEpisodesShort 短关键词（<3 字符）的章节检索：
// 短词构不成 trigram，FTS 的 LIKE 退化为内容表全扫描，且「计数 + 取数」
// 会把同样的扫描做两遍。这里用临时表一次性物化命中集（两路扫描各一遍），
// 计数退化为命中集行数（无筛选条件时为 O(1) 索引计数），取数按需分页；
// 临时表按连接隔离，事务固定单连接，并发请求互不干扰。
func (h *handler) searchEpisodesShort(c *gin.Context, qn string, conds []string, args, tierArgs []any, usesSubjects bool, orderBy string, page, size int) {
	tx, err := h.getDB().Begin()
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer tx.Rollback()

	like := "%" + qn + "%"
	steps := []struct {
		sql  string
		args []any
	}{
		{`CREATE TEMP TABLE IF NOT EXISTS ep_hits (id INTEGER PRIMARY KEY)`, nil},
		{`DELETE FROM ep_hits`, nil},
		// 章节自身标题命中
		{`INSERT INTO ep_hits(id) SELECT rowid FROM episodes_fts WHERE search_norm LIKE ?`, []any{like}},
		// 所属条目标题命中（原名/中文名/别名，复用 subjects_fts）展开为章节
		{`INSERT OR IGNORE INTO ep_hits(id) SELECT e.id FROM episodes e
			JOIN subjects_fts f ON f.rowid = e.subject_id WHERE f.search_norm LIKE ?`, []any{like}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(st.sql, st.args...); err != nil {
			fail(c, 500, err.Error())
			return
		}
	}

	var where string
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// 计数：无筛选条件时直接数命中集；带筛选时经命中集回表后过滤
	countSQL := "SELECT COUNT(*) FROM ep_hits"
	if len(conds) > 0 {
		countSQL = "SELECT COUNT(*) FROM ep_hits h JOIN episodes e ON e.id = h.id"
		if usesSubjects {
			countSQL += " LEFT JOIN subjects s ON s.id = e.subject_id"
		}
		countSQL += where
	}
	var total int64
	if err := tx.QueryRow(countSQL, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	dataSQL := "SELECT e.id, e.name, e.name_cn, e.description, e.airdate, e.disc, e.duration, e.sort, e.type, " +
		episodeSubjectCols + " FROM ep_hits h JOIN episodes e ON e.id = h.id" +
		" LEFT JOIN subjects s ON s.id = e.subject_id" + where +
		" ORDER BY " + orderBy + " LIMIT ? OFFSET ?"

	queryArgs := make([]any, 0, len(args)+len(tierArgs)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tierArgs...)
	queryArgs = append(queryArgs, size, (page-1)*size)

	rows, err := tx.Query(dataSQL, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]*episodeBrief, 0, size)
	for rows.Next() {
		item, err := h.scanEpisodeBrief(rows)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}

// episodeTierOrder 构造章节结果的分级排序表达式及其参数：
//
//	0 章节标题完全等于查询词
//	1 章节标题原样子串命中（查询词本身带符号时才能原样命中）
//	2 所属条目标题完全等于查询词
//	3 仅所属条目标题子串/归一化命中
//
// 只对命中集求值，不改变 SELECT 的列。参数顺序与占位符出现顺序一致。
func episodeTierOrder(q string) (string, []any) {
	eq := q
	like := "%" + escapeLike(q) + "%"
	expr := "CASE WHEN e.name = ? COLLATE NOCASE OR e.name_cn = ? COLLATE NOCASE THEN 0" +
		" WHEN e.name LIKE ? ESCAPE '\\' OR e.name_cn LIKE ? ESCAPE '\\' THEN 1" +
		" WHEN s.name = ? COLLATE NOCASE OR s.name_cn = ? COLLATE NOCASE THEN 2" +
		" ELSE 3 END"
	return expr, []any{eq, eq, like, like, eq, eq}
}

// scanEpisodeBrief 读取章节搜索行（LEFT JOIN 条目侧可能为 NULL——
// 存在少量 subject_id 在 subjects 表中无对应的孤儿章节）。
func (h *handler) scanEpisodeBrief(row interface{ Scan(...any) error }) (*episodeBrief, error) {
	var (
		e                     episodeBrief
		sID, sType, sPlatform sql.NullInt64
		sName, sNameCN, sDate sql.NullString
	)
	if err := row.Scan(&e.ID, &e.Name, &e.NameCN, &e.Description, &e.Airdate, &e.Disc,
		&e.Duration, &e.Sort, &e.Type,
		&sID, &sType, &sName, &sNameCN, &sPlatform, &sDate); err != nil {
		return nil, err
	}
	e.TypeName = h.cons.EpisodeTypes[e.Type]
	e.Subject = episodeSubjRef{
		ID:        sID.Int64,
		Type:      int(sType.Int64),
		TypeName:  h.cons.SubjectTypeCN(int(sType.Int64)),
		Name:      sName.String,
		NameCN:    sNameCN.String,
		PlatformN: h.cons.PlatformCN(int(sType.Int64), int(sPlatform.Int64)),
		Date:      sDate.String,
	}
	return &e, nil
}
