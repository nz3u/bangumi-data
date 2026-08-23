package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
)

// searchSubjects 条目搜索与筛选。
// 参数：q（全文搜索）、type、platform、tag、date_from、date_to、
//
//	rank_min、score_min、nsfw(0/1)、sort(rank|score|date|favorite|id)、order、page、size
func (h *handler) searchSubjects(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds    []string
		args     []any
		fullScan bool // 过滤条件必须逐行求值（LIKE 回退/json_each），无法利用索引
	)

	if q != "" {
		if useFTS(q) {
			// 先尝试 FTS，失败则回退 LIKE（FTS 语法错误等场景）
			ids, err := h.ftsRowIDs("subjects_fts", q)
			if err == nil {
				if len(ids) == 0 {
					respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
					return
				}
				placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
				conds = append(conds, "s.id IN ("+placeholders+")")
				for _, id := range ids {
					args = append(args, id)
				}
			} else {
				fullScan = true
				like := "%" + q + "%"
				conds = append(conds, "(s.name LIKE ? OR s.name_cn LIKE ?)")
				args = append(args, like, like)
			}
		} else {
			// 短于 trigram 最小长度的关键词无法命中全文索引，回退 LIKE
			fullScan = true
			like := "%" + q + "%"
			conds = append(conds, "(s.name LIKE ? OR s.name_cn LIKE ?)")
			args = append(args, like, like)
		}
	}

	if v, ok := parseIntQuery(c, "type"); ok {
		conds = append(conds, "s.type = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "platform"); ok {
		conds = append(conds, "s.platform = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "nsfw"); ok {
		conds = append(conds, "s.nsfw = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "rank_min"); ok {
		conds = append(conds, "s.rank >= ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "score_min"); ok {
		conds = append(conds, "s.score >= ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "series"); ok {
		conds = append(conds, "s.series = ?")
		args = append(args, v)
	}
	if v := c.Query("date_from"); v != "" {
		conds = append(conds, "s.date >= ?")
		args = append(args, v)
	}
	if v := c.Query("date_to"); v != "" {
		conds = append(conds, "s.date <= ?")
		args = append(args, v)
	}
	if tag := strings.TrimSpace(c.Query("tag")); tag != "" {
		fullScan = true
		conds = append(conds, `EXISTS (SELECT 1 FROM json_each(s.tags) je WHERE je.value->>'name' = ?)`)
		args = append(args, tag)
	}
	if metaTag := strings.TrimSpace(c.Query("meta_tag")); metaTag != "" {
		fullScan = true
		conds = append(conds, `EXISTS (SELECT 1 FROM json_each(s.meta_tags) mt WHERE mt.value = ?)`)
		args = append(args, metaTag)
	}

	page, size := pagination(c)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// 排序
	sort := c.DefaultQuery("sort", "id")
	order := strings.ToLower(c.DefaultQuery("order", "asc"))
	if order != "desc" {
		order = "asc"
	}
	var orderBy string
	switch sort {
	case "rank":
		orderBy = "CASE WHEN s.rank = 0 THEN 2147483647 ELSE s.rank END " + order + ", s.id"
	case "score":
		orderBy = "s.score " + order + ", s.id"
	case "date":
		orderBy = "s.date " + order + ", s.id"
	case "favorite":
		orderBy = "CAST(json_extract(s.favorite, '$.done') AS INTEGER) DESC, s.id"
	default:
		orderBy = "s.id " + order
	}

	queryArgs := append(args, size, (page-1)*size)

	// 计数与取数策略：
	//   - 过滤条件可走索引时拆成两条查询——数据查询按主键序取满一页即提前终止，
	//     COUNT(*) 仅扫描索引；避免 COUNT(*) OVER() 窗口函数强制整表物化
	//     （无过滤时 67 万行全表扫描需数秒，拆分后 <20ms）。
	//   - 过滤条件必须逐行求值（LIKE/json_each）时，两次扫描代价翻倍，
	//     保留 COUNT(*) OVER() 合并为一次扫描更划算。
	dataSQL := "SELECT " + subjectBriefCols + " FROM subjects s" + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	var totalPtr *int64
	total := int64(0)
	if fullScan {
		dataSQL = "SELECT " + subjectBriefCols + ", COUNT(*) OVER() FROM subjects s" + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
		totalPtr = &total
	} else if err := h.db.QueryRow("SELECT COUNT(*) FROM subjects s"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.db.Query(dataSQL, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]*subjectBrief, 0, size)
	for rows.Next() {
		item, err := h.scanSubjectBrief(rows, totalPtr)
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

// relationItem 条目关联（含反向）。
type relationItem struct {
	SubjectID        int64  `json:"subject_id"`
	RelationType     int    `json:"relation_type"`
	RelationName     string `json:"relation_name"`
	RelatedSubjectID int64  `json:"related_subject_id"`
	RelatedName      string `json:"related_name"`
	RelatedNameCN    string `json:"related_name_cn"`
	RelatedType      int    `json:"related_type"`
	RelatedTypeName  string `json:"related_type_name"`
	Order            int    `json:"order"`
	// 方向：out 表示本条目 -> 关联条目，in 表示关联条目 -> 本条目
	Direction string `json:"direction"`
}

// staffItem 制作人员。
type staffItem struct {
	Position     int    `json:"position"`
	PositionName string `json:"position_name"`
	PersonID     int64  `json:"person_id"`
	PersonName   string `json:"person_name"`
	PersonType   int    `json:"person_type"`
	AppearEps    string `json:"appear_eps,omitempty"`
}

// characterItem 条目角色。
type characterItem struct {
	Type     int    `json:"type"`
	TypeName string `json:"type_name"`
	Order    int    `json:"order"`
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Role     int    `json:"role"`
	RoleName string `json:"role_name"`
	Collects int    `json:"collects"`
	Comments int    `json:"comments"`
}

// subjectDetail 条目详情。
type subjectDetail struct {
	*subjectBrief
	Infobox      string          `json:"infobox"`
	Summary      string          `json:"summary"`
	Series       bool            `json:"series"`
	ScoreDetails map[string]int  `json:"score_details,omitempty"`
	MetaTags     []string        `json:"meta_tags,omitempty"`
	Relations    []relationItem  `json:"relations"`
	Staff        []staffItem     `json:"staff"`
	Characters   []characterItem `json:"characters"`
	EpisodeCount int64           `json:"episode_count"`
}

// getSubject 条目详情。
func (h *handler) getSubject(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	var (
		infobox, summary, tags, fav, sd, mt string
		series, nsfw                        int64
	)
	err := h.db.QueryRow(`SELECT s.infobox, s.summary, s.series, s.nsfw, s.tags, s.favorite, s.score_details, s.meta_tags
		FROM subjects s WHERE s.id = ?`, id).Scan(&infobox, &summary, &series, &nsfw, &tags, &fav, &sd, &mt)
	if err != nil {
		fail(c, 404, "条目不存在")
		return
	}

	base, err := h.scanSubjectBrief(h.db.QueryRow(`SELECT `+subjectBriefCols+` FROM subjects s WHERE s.id = ?`, id), nil)
	if err != nil {
		fail(c, 404, "条目不存在")
		return
	}

	d := &subjectDetail{
		subjectBrief: base,
		Infobox:      infobox,
		Summary:      summary,
		Series:       series == 1,
		ScoreDetails: parseScoreDetails(sd),
		MetaTags:     parseStrings(mt),
		Relations:    []relationItem{},
		Staff:        []staffItem{},
		Characters:   []characterItem{},
	}

	// 关联（双向）
	rrows, err := h.db.Query(`SELECT sr.subject_id, sr.relation_type, sr.related_subject_id, sr."order",
		rs.name, rs.name_cn, rs.type
		FROM subject_relations sr
		JOIN subjects rs ON rs.id = sr.related_subject_id
		WHERE sr.subject_id = ?
		UNION ALL
		SELECT sr.related_subject_id, sr.relation_type, sr.subject_id, sr."order",
		rs.name, rs.name_cn, rs.type
		FROM subject_relations sr
		JOIN subjects rs ON rs.id = sr.subject_id
		WHERE sr.related_subject_id = ?`, id, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	for rrows.Next() {
		var (
			rel              relationItem
			rsName, rsNameCN string
			rsType           int
		)
		if err := rrows.Scan(&rel.SubjectID, &rel.RelationType, &rel.RelatedSubjectID, &rel.Order,
			&rsName, &rsNameCN, &rsType); err != nil {
			fail(c, 500, err.Error())
			return
		}
		if rel.SubjectID == id {
			rel.Direction = "out"
		} else {
			rel.Direction = "in"
		}
		rel.RelationName = h.cons.RelationCN(rsType, rel.RelationType)
		rel.RelatedName = rsName
		rel.RelatedNameCN = rsNameCN
		rel.RelatedType = rsType
		rel.RelatedTypeName = h.cons.SubjectTypeCN(rsType)
		d.Relations = append(d.Relations, rel)
	}
	rrows.Close()

	// 制作人员
	srows, err := h.db.Query(`SELECT sp.position, p.id, p.name, p.type, sp.appear_eps
		FROM subject_persons sp JOIN persons p ON p.id = sp.person_id
		WHERE sp.subject_id = ? ORDER BY sp.position, p.id`, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	for srows.Next() {
		var it staffItem
		if err := srows.Scan(&it.Position, &it.PersonID, &it.PersonName, &it.PersonType, &it.AppearEps); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.PositionName = h.cons.StaffCN(int(base.Type), it.Position)
		d.Staff = append(d.Staff, it)
	}
	srows.Close()

	// 角色
	crows, err := h.db.Query(`SELECT sc.type, sc."order", c.id, c.name, c.role, c.collects, c.comments
		FROM subject_characters sc JOIN characters c ON c.id = sc.character_id
		WHERE sc.subject_id = ? ORDER BY sc.type, sc."order", c.id`, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	for crows.Next() {
		var it characterItem
		if err := crows.Scan(&it.Type, &it.Order, &it.ID, &it.Name, &it.Role, &it.Collects, &it.Comments); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.SubjectCharTypes[it.Type]
		it.RoleName = h.cons.CharacterRoles[it.Role]
		d.Characters = append(d.Characters, it)
	}
	crows.Close()

	// 章节统计
	if err := h.db.QueryRow("SELECT COUNT(*) FROM episodes WHERE subject_id = ?", id).Scan(&d.EpisodeCount); err != nil {
		fail(c, 500, err.Error())
		return
	}

	respOK(c, d)
}

// getSubjectEpisodes 条目章节列表。
func (h *handler) getSubjectEpisodes(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	page, size := pagination(c)

	conds := []string{"subject_id = ?"}
	args := []any{id}
	if v, ok := parseIntQuery(c, "type"); ok {
		conds = append(conds, "type = ?")
		args = append(args, v)
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int64
	if err := h.db.QueryRow("SELECT COUNT(*) FROM episodes"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	queryArgs := append(args, size, (page-1)*size)
	rows, err := h.db.Query(`SELECT id, name, name_cn, description, airdate, disc, duration, subject_id, sort, type
		FROM episodes`+where+` ORDER BY sort, id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]model.Episode, 0, size)
	for rows.Next() {
		var e model.Episode
		if err := rows.Scan(&e.ID, &e.Name, &e.NameCN, &e.Description, &e.Airdate, &e.Disc, &e.Duration, &e.SubjectID, &e.Sort, &e.Type); err != nil {
			fail(c, 500, err.Error())
			return
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}
