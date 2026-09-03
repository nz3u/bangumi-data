package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
	"bangumi-subject-go/internal/norm"
)

// maxSubjectFTSIDs 命中集上限：超过时放弃 IN 列表与分级排序，改为主表顺序扫描。
// 触发者通常是 1~2 个字符的宽泛查询（命中可达数十万行），此时 IN 列表本身就是
// 主要开销，两次扫描反而更慢。
const maxSubjectFTSIDs = 30000

// searchSubjects 条目搜索与筛选。
// 参数：q（全文搜索）、type、platform、tag/meta_tag（多标签组合："+A,+B,-C"，
//
//	'+' 必须包含 / '-' 必须排除，无前缀视为 '+'；meta_tag 同语法）、
//	date_from、date_to、rank_min、score_min、nsfw(0/1)、
//	sort(rank|score|date|favorite|id)、order、page、size
func (h *handler) searchSubjects(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds     []string
		args      []any
		fullScan  bool   // 过滤条件必须逐行求值（大命中集回退 LIKE），无法利用索引
		tierOrder string // 命中集按匹配位置分级排序（仅小命中集启用）
		tierArgs  []any
	)

	if q != "" {
		// 查询词经与入库时同样的归一化：去符号、统一全半角与大小写，
		// 因而能命中「少女歌剧」->「少女☆歌剧」、「Kaguya Hime」->「Chou Kaguya-hime!」。
		// 归一化结果为空说明查询词只含符号（如「！！！」）——符号不参与索引，
		// 此时按无匹配处理（返回空集而非全表，后者对用户毫无意义）。
		qn := norm.Fold(q)
		if qn == "" {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		ids, err := h.ftsNormRowIDs(qn)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		if len(ids) == 0 {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		if len(ids) > maxSubjectFTSIDs {
			// 命中集过大（多见于 1~2 个字符的宽泛查询）：IN 列表与分级排序的
			// 代价都超过一次顺序扫描，退回对 search_norm 直接 LIKE。
			fullScan = true
			conds = append(conds, "s.search_norm LIKE ? ESCAPE '\\'")
			args = append(args, "%"+escapeLike(qn)+"%")
		} else {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
			conds = append(conds, "s.id IN ("+placeholders+")")
			for _, id := range ids {
				args = append(args, id)
			}
			tierOrder, tierArgs = subjectTierOrder(q)
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
	// 合并 tag 与 meta_tag 搜索：单个 tag 参数同时匹配两张倒排映射表（OR 语义），
	// 保持 meta_tag 参数向后兼容（单独使用时仅查元标签表）。
	if v := parseTagCombo(c.Query("tag")); len(v.pos) > 0 || len(v.neg) > 0 {
		conds, args = appendCombinedTagFilters(conds, args, v.pos, v.neg)
	} else if v := parseTagCombo(c.Query("meta_tag")); len(v.pos) > 0 || len(v.neg) > 0 {
		conds, args = appendTagFilters(conds, args, "subject_meta_tags_map", v.pos, v.neg)
	}

	page, size := pagination(c)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// 排序。sort 为空表示前端未指定：有关键词时按人气降序更符合搜索预期，
	// 否则沿用 id 序（浏览全库的默认行为保持不变）。
	sort := c.Query("sort")
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
	case "id":
		orderBy = "s.id " + order
	default:
		if tierOrder != "" {
			// 关键词搜索：同分级内把收藏人数多的排前面。
			// 例：搜「辉夜姬」时「超辉夜姬！」靠归一化命中，与一批冷门条目同级，
			// 仅按 id 排会被埋到第 11 位，按人气则回到首位。
			orderBy = "CAST(COALESCE(json_extract(s.favorite, '$.done'), 0) AS INTEGER) DESC, s.id " + order
		} else {
			orderBy = "s.id " + order
		}
	}
	// 有关键词时把匹配位置分级作为首要排序，用户指定的 sort 退为次级排序：
	// 例句搜索「少女歌剧」时，靠归一化才能命中的主条目会落在后面，
	// 需要由分级把它提到仅靠符号差异才命中不了的冷门条目之前。
	if tierOrder != "" {
		orderBy = tierOrder + ", " + orderBy
	}

	// ORDER BY 的参数紧跟在 WHERE 参数之后、LIMIT 之前。
	queryArgs := make([]any, 0, len(args)+len(tierArgs)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tierArgs...)
	queryArgs = append(queryArgs, size, (page-1)*size)

	// 计数与取数策略：
	//   - 过滤条件可走索引时拆成两条查询——数据查询按主键序取满一页即提前终止，
	//     COUNT(*) 仅扫描索引；避免 COUNT(*) OVER() 窗口函数强制整表物化
	//     （无过滤时 67 万行全表扫描需数秒，拆分后 <20ms）。
	//     标签过滤经倒排映射表（subject_tags_map 等）转为 IN/NOT IN 子查询，
	//     同样可走索引，不再逐行解析 JSON。
	//   - 过滤条件必须逐行求值（LIKE 回退）时，两次扫描代价翻倍，
	//     保留 COUNT(*) OVER() 合并为一次扫描更划算。
	dataSQL := "SELECT " + subjectBriefCols + " FROM subjects s" + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	var totalPtr *int64
	total := int64(0)
	if fullScan {
		dataSQL = "SELECT " + subjectBriefCols + ", COUNT(*) OVER() FROM subjects s" + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
		totalPtr = &total
	} else if err := h.getDB().QueryRow("SELECT COUNT(*) FROM subjects s"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.getDB().Query(dataSQL, queryArgs...)
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

// tagCombo 多标签组合解析结果：pos 为必须全部包含的正标签，neg 为必须全部不包含的负标签。
type tagCombo struct {
	pos []string
	neg []string
}

// parseTagCombo 解析多标签组合参数："+A,+B,-C"（逗号分隔，可含中文逗号）。
// '+' 前缀为正标签（要求包含），'-' 前缀为负标签（要求排除），
// 无前缀视为正标签（兼容单标签旧参数）；空片段与裸符号忽略，前后空格去除。
func parseTagCombo(raw string) tagCombo {
	var v tagCombo
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' }) {
		part = strings.TrimSpace(part)
		switch {
		case part == "" || part == "+" || part == "-": // 空片段/裸符号无标签名，忽略
		case part[0] == '-':
			v.neg = append(v.neg, strings.TrimSpace(part[1:]))
		case part[0] == '+':
			v.pos = append(v.pos, strings.TrimSpace(part[1:]))
		default:
			v.pos = append(v.pos, part)
		}
	}
	return v
}

// appendTagFilters 追加正/负标签条件：基于倒排映射表（标签名 -> subject_id）的
// 索引子查询，替代逐行 json_each 解析（68 万行全表 JSON 解析是标签搜索慢的主因）。
// 正标签 EXISTS 语义 = s.id IN (命中集)；负标签 NOT EXISTS 语义 = NOT IN (排除集)。
func appendTagFilters(conds []string, args []any, mapTable string, pos, neg []string) ([]string, []any) {
	for _, t := range pos {
		conds = append(conds, "s.id IN (SELECT subject_id FROM "+mapTable+" WHERE tag_name = ?)")
		args = append(args, t)
	}
	for _, t := range neg {
		conds = append(conds, "s.id NOT IN (SELECT subject_id FROM "+mapTable+" WHERE tag_name = ?)")
		args = append(args, t)
	}
	return conds, args
}

// appendCombinedTagFilters 同时在普通标签和元标签倒排映射表上过滤：
// 正标签 s.id IN (tags_map UNION meta_tags_map)；负标签 s.id NOT IN (两者交集)。
func appendCombinedTagFilters(conds []string, args []any, pos, neg []string) ([]string, []any) {
	for _, t := range pos {
		conds = append(conds, "s.id IN (SELECT subject_id FROM subject_tags_map WHERE tag_name = ? UNION SELECT subject_id FROM subject_meta_tags_map WHERE tag_name = ?)")
		args = append(args, t, t)
	}
	for _, t := range neg {
		conds = append(conds, "s.id NOT IN (SELECT subject_id FROM subject_tags_map WHERE tag_name = ? UNION SELECT subject_id FROM subject_meta_tags_map WHERE tag_name = ?)")
		args = append(args, t, t)
	}
	return conds, args
}

// suggestSubjectTags 标签/元标签实时建议。
// kind=tag（普通标签）|meta（元标签）|all（合并，不区分）；q 可选子串过滤（ASCII 大小写不敏感）；
// limit 默认 50、上限 5000（前端一次性拉取候选池后做拼音首字母本地过滤）。
// 返回按使用次数降序的 {name, cnt} 列表，前缀命中优先于子串命中。
// kind=all 时合并两张聚合表并按名称去重（取较大计数）。
func (h *handler) suggestSubjectTags(c *gin.Context) {
	kind := c.Query("kind")

	limit := 50
	if v, ok := parseIntQuery(c, "limit"); ok && v > 0 {
		limit = v
	}
	if limit > maxTagSuggestLimit {
		limit = maxTagSuggestLimit
	}

	q := strings.TrimSpace(c.Query("q"))
	prefix := escapeLike(q) + "%"
	likeArg := "%" + escapeLike(q) + "%"

	var subQuery string
	switch kind {
	case "meta":
		subQuery = "SELECT name, cnt FROM subject_meta_tags_agg"
	case "all":
		subQuery = "SELECT name, cnt FROM subject_tags_agg UNION ALL SELECT name, cnt FROM subject_meta_tags_agg"
	default: // "tag" 或空
		subQuery = "SELECT name, cnt FROM subject_tags_agg"
	}

	var where string
	var args []any
	if q != "" {
		where = " WHERE name LIKE ? ESCAPE '\\'"
		args = append(args, likeArg)
	}

	// kind=all 时按名称去重，取较大计数
	selectExpr := "SELECT name, MAX(cnt) AS cnt"
	if kind != "all" {
		selectExpr = "SELECT name, cnt"
	}
	groupBy := ""
	if kind == "all" {
		groupBy = " GROUP BY name"
	}

	orderBy := " ORDER BY (name LIKE ?) DESC, cnt DESC, name LIMIT ?"
	sql := selectExpr + " FROM (" + subQuery + ") t" + where + groupBy + orderBy
	args = append(args, prefix, limit)

	rows, err := h.getDB().Query(sql, args...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]gin.H, 0, limit)
	for rows.Next() {
		var (
			name string
			cnt  int64
		)
		if err := rows.Scan(&name, &cnt); err != nil {
			fail(c, 500, err.Error())
			return
		}
		items = append(items, gin.H{"name": name, "cnt": cnt})
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, gin.H{"items": items})
}

// maxTagSuggestLimit 建议接口单次返回上限。
const maxTagSuggestLimit = 5000

// escapeLike 转义 LIKE 通配符（配合 ESCAPE '\'），防止用户输入干扰匹配。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

// subjectTierOrder 构造条目结果的分级排序表达式及其参数：
//
//	0 原名或中文名完全等于查询词
//	1 原名/中文名原样子串命中（查询词本身带符号时才能原样命中）
//	2 仅别名命中
//	3 仅归一化后命中（查询词被符号切断，或全半角/大小写不同）
//
// 该表达式只对命中集求值（通常数十至数千行），不改变 SELECT 的列，
// 因此无需调整结果扫描逻辑。参数顺序与表达式中占位符的出现顺序一致。
func subjectTierOrder(q string) (string, []any) {
	eq := q
	like := "%" + escapeLike(q) + "%"
	expr := "CASE WHEN s.name = ? COLLATE NOCASE OR s.name_cn = ? COLLATE NOCASE THEN 0" +
		" WHEN s.name LIKE ? ESCAPE '\\' OR s.name_cn LIKE ? ESCAPE '\\' THEN 1" +
		" WHEN s.aliases LIKE ? ESCAPE '\\' THEN 2" +
		" ELSE 3 END"
	return expr, []any{eq, eq, like, like, like}
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
	err := h.getDB().QueryRow(`SELECT s.infobox, s.summary, s.series, s.nsfw, s.tags, s.favorite, s.score_details, s.meta_tags
		FROM subjects s WHERE s.id = ?`, id).Scan(&infobox, &summary, &series, &nsfw, &tags, &fav, &sd, &mt)
	if err != nil {
		fail(c, 404, "条目不存在")
		return
	}

	base, err := h.scanSubjectBrief(h.getDB().QueryRow(`SELECT `+subjectBriefCols+` FROM subjects s WHERE s.id = ?`, id), nil)
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
	rrows, err := h.getDB().Query(`SELECT sr.subject_id, sr.relation_type, sr.related_subject_id, sr."order",
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
	srows, err := h.getDB().Query(`SELECT sp.position, p.id, p.name, p.type, sp.appear_eps
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
	crows, err := h.getDB().Query(`SELECT sc.type, sc."order", c.id, c.name, c.role, c.collects, c.comments
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
	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM episodes WHERE subject_id = ?", id).Scan(&d.EpisodeCount); err != nil {
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
	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM episodes"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	queryArgs := append(args, size, (page-1)*size)
	rows, err := h.getDB().Query(`SELECT id, name, name_cn, description, airdate, disc, duration, subject_id, sort, type
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
