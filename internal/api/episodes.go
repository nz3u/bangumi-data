package api

import (
	"database/sql"
	"strings"

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
		conds     []string
		args      []any
		tierOrder string // 命中按匹配位置分级排序（仅有关键词时启用）
		tierArgs  []any
	)

	if q != "" {
		// 查询词与索引同口径归一化，因此「少女歌剧」能命中「少女☆歌剧」。
		// 全为符号的查询词不参与索引，按无匹配处理。
		qn := norm.Fold(q)
		if qn == "" {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		// 双路检索：章节自身标题走 episodes_fts，所属条目标题（原名/中文名/别名）
		// 复用 subjects_fts，OR 合并。两个子查询不依赖外层、各自走 trigram 索引
		// （>=3 字符；更短自动退化为顺序扫描），由 multi-index OR 分别经主键与
		// idx_episodes_subject 索引取候选，代价与命中集成正比，无需 Go 侧
		// 先取命中集（避免超长 IN 列表与两倍往返）。
		like := "%" + qn + "%"
		conds = append(conds,
			"(e.id IN (SELECT rowid FROM episodes_fts WHERE search_norm LIKE ?)"+
				" OR e.subject_id IN (SELECT rowid FROM subjects_fts WHERE search_norm LIKE ?))")
		args = append(args, like, like)
		tierOrder, tierArgs = episodeTierOrder(q)
	}

	if v, ok := parseIntQuery(c, "subject_id"); ok {
		conds = append(conds, "e.subject_id = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "type"); ok {
		conds = append(conds, "s.type = ?")
		args = append(args, v)
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

	// 排序。sort 为空：有关键词时分级 + 条目人气（同条目内按类型/集数聚在一起），
	// 无关键词时按 ID 序。popularity 排序将无章节正篇的碟轨类条目同样按收藏降序。
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

	queryArgs := make([]any, 0, len(args)+len(tierArgs)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tierArgs...)
	queryArgs = append(queryArgs, size, (page-1)*size)

	dataSQL := "SELECT e.id, e.name, e.name_cn, e.description, e.airdate, e.disc, e.duration, e.sort, e.type, " +
		episodeSubjectCols + " FROM episodes e LEFT JOIN subjects s ON s.id = e.subject_id" +
		where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"

	var total int64
	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM episodes e LEFT JOIN subjects s ON s.id = e.subject_id"+where, args...).Scan(&total); err != nil {
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
