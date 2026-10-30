package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/norm"
	"bangumi-subject-go/internal/wiki"
)

// characterDetail 角色详情。
type characterDetail struct {
	ID       int64        `json:"id"`
	Role     int          `json:"role"`
	RoleName string       `json:"role_name"`
	Name     string       `json:"name"`
	NameCN   string       `json:"name_cn"`
	Infobox  []wiki.Field `json:"infobox,omitempty"`
	Summary  string       `json:"summary"`
	Comments int          `json:"comments"`
	Collects int          `json:"collects"`
	// 出演作品
	Subjects []characterSubjectItem `json:"subjects"`
	// 声优/演员（person_characters 关联）
	CVs []cvItem `json:"cvs"`
	// 角色关联（person_relations 中 crt 行，双向）
	Relations []personRelationItem `json:"relations"`
}

type characterSubjectItem struct {
	SubjectID    int64   `json:"subject_id"`
	Name         string  `json:"name"`
	NameCN       string  `json:"name_cn"`
	Type         int     `json:"type"`
	TypeName     string  `json:"type_name"`
	CharType     int     `json:"char_type"`
	CharTypeName string  `json:"char_type_name"`
	Order        int     `json:"order"`
	Date         string  `json:"date"`
	Score        float64 `json:"score"`
}

type cvItem struct {
	PersonID    int64  `json:"person_id"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	TypeName    string `json:"type_name"`
	SubjectID   int64  `json:"subject_id"`
	SubjectName string `json:"subject_name"`
	Summary     string `json:"summary,omitempty"`
}

// characterBrief 角色搜索条目。
type characterBrief struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	NameCN   string `json:"name_cn"`
	Role     int    `json:"role"`
	RoleName string `json:"role_name"`
	Collects int    `json:"collects"`
	Comments int    `json:"comments"`
}

// searchCharacters 角色搜索。
// 关键词与入库侧的 search_norm 同口径归一化（去符号/空白、统一全半角与大小写），
// 因此在角色的 infobox 别名里出现的罗马字（Hirasawa Yui）与各类译名
// （鲁鲁修、Zero、呆唯）都能命中；≥3 字符走 trigram 索引，更短自动退化为顺序扫描。
//
//	@Summary		角色搜索
//	@Description	q 归一化后匹配原名、infobox 简体中文名与别名（含罗马字，如 Hirasawa Yui 可命中平泽唯）；≥3 字符走全文索引，更短回退子串匹配。
//	@Tags			角色
//	@Produce		json
//	@Param			q		query		string	false	"关键词"
//	@Param			role	query		integer	false	"角色类型：1 角色 / 2 机体 / 3 组织"	Enums(1, 2, 3)
//	@Param			page	query		integer	false	"页码"	default(1)
//	@Param			size	query		integer	false	"每页数量"	default(30)
//	@Success		200	{object}	apiEnvelope{data=characterSearchData}
//	@Failure		500	{object}	apiEnvelope
//	@Router			/api/characters/search [get]
func (h *handler) searchCharacters(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds     []string
		args      []any
		fullScan  bool   // 大命中集回退主表顺序扫描
		tierOrder string // 命中集按匹配位置分级排序（仅小命中集启用）
		tierArgs  []any
	)
	if q != "" {
		// 归一化结果为空说明查询词只含符号（如「！！！」）——符号不参与索引，
		// 此时按无匹配处理（返回空集而非全表）。
		qn := norm.Fold(q)
		if qn == "" {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		ids, err := h.ftsNormRowIDs("characters_fts", qn)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		if len(ids) == 0 {
			respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
			return
		}
		if len(ids) > maxEntityFTSIDs {
			// 命中集过大（多见于 1~2 个字符的宽泛查询）：IN 列表与分级排序的
			// 代价都超过一次顺序扫描，退回对 search_norm 直接 LIKE。
			fullScan = true
			conds = append(conds, "c.search_norm LIKE ? ESCAPE '\\'")
			args = append(args, "%"+escapeLike(qn)+"%")
		} else {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
			conds = append(conds, "c.id IN ("+placeholders+")")
			for _, id := range ids {
				args = append(args, id)
			}
			tierOrder, tierArgs = nameTierOrder("c", q)
		}
	}
	if v, ok := parseIntQuery(c, "role"); ok {
		conds = append(conds, "c.role = ?")
		args = append(args, v)
	}

	page, size := pagination(c)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	orderBy := "c.id"
	if tierOrder != "" {
		orderBy = tierOrder + ", c.id"
	}

	// ORDER BY 的参数紧跟在 WHERE 参数之后、LIMIT 之前。
	queryArgs := make([]any, 0, len(args)+len(tierArgs)+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tierArgs...)
	queryArgs = append(queryArgs, size, (page-1)*size)

	// 计数与取数策略同 searchSubjects：可走索引时拆分查询避免全表物化，
	// LIKE 回退场景保留 COUNT(*) OVER() 合并为一次扫描。
	countCol := ""
	var totalPtr *int64
	total := int64(0)
	if fullScan {
		countCol = ", COUNT(*) OVER()"
		totalPtr = &total
	} else if err := h.getDB().QueryRow("SELECT COUNT(*) FROM characters c"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.getDB().Query(`SELECT c.id, c.name, c.name_cn, c.role, c.collects, c.comments`+countCol+`
		FROM characters c`+where+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]characterBrief, 0, size)
	for rows.Next() {
		var it characterBrief
		dest := []any{&it.ID, &it.Name, &it.NameCN, &it.Role, &it.Collects, &it.Comments}
		if totalPtr != nil {
			dest = append(dest, totalPtr)
		}
		if err := rows.Scan(dest...); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.RoleName = h.cons.CharacterRoles[it.Role]
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}

// getCharacter 角色详情（含出演作品与 CV）。
//
//	@Summary		角色详情
//	@Description	含基本信息、infobox（结构化字段）、简介、出演作品、声优/演员（按人物去重）与角色关联（双向）。
//	@Tags			角色
//	@Produce		json
//	@Param			id	path		integer	true	"角色 ID"
//	@Success		200	{object}	apiEnvelope{data=characterDetail}
//	@Failure		400	{object}	apiEnvelope	"无效的 id"
//	@Failure		404	{object}	apiEnvelope	"角色不存在"
//	@Failure		500	{object}	apiEnvelope
//	@Router			/api/characters/{id} [get]
func (h *handler) getCharacter(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	d := characterDetail{Subjects: []characterSubjectItem{}, CVs: []cvItem{}, Relations: []personRelationItem{}}
	var rawInfobox string
	err := h.getDB().QueryRow(`SELECT id, role, name, name_cn, infobox, summary, comments, collects
		FROM characters WHERE id = ?`, id).
		Scan(&d.ID, &d.Role, &d.Name, &d.NameCN, &rawInfobox, &d.Summary, &d.Comments, &d.Collects)
	if err != nil {
		fail(c, 404, "角色不存在")
		return
	}
	d.RoleName = h.cons.CharacterRoles[d.Role]
	if ib, err := wiki.ParseInfobox(rawInfobox); err == nil {
		d.Infobox = ib.Fields
	}

	// 出演作品
	srows, err := h.getDB().Query(`SELECT s.id, s.name, s.name_cn, s.type, sc.type, sc."order", s.date, s.score
		FROM subject_characters sc
		JOIN subjects s ON s.id = sc.subject_id
		WHERE sc.character_id = ?
		ORDER BY s.date DESC, s.id`, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer srows.Close()
	for srows.Next() {
		var it characterSubjectItem
		if err := srows.Scan(&it.SubjectID, &it.Name, &it.NameCN, &it.Type, &it.CharType, &it.Order, &it.Date, &it.Score); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.SubjectTypeCN(it.Type)
		it.CharTypeName = h.cons.SubjectCharTypes[it.CharType]
		d.Subjects = append(d.Subjects, it)
	}

	// 声优/演员（CV）。按人物去重：同一人物在不同作品/译配下会重复出现，
	// 这里只保留每个人物一条（类型取最小值，主配 CV=0 优先）。
	crows, err := h.getDB().Query(`SELECT pc.person_id, p.name, MIN(pc.type)
		FROM person_characters pc
		JOIN persons p ON p.id = pc.person_id
		WHERE pc.character_id = ?
		GROUP BY pc.person_id
		ORDER BY MIN(pc.type), pc.person_id`, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer crows.Close()
	for crows.Next() {
		var it cvItem
		if err := crows.Scan(&it.PersonID, &it.Name, &it.Type); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.PersonRelationCN("prsn_cv", it.Type)
		d.CVs = append(d.CVs, it)
	}

	// 角色关联（双向）。仅取 crt 行：crt 行两端都是角色；
	// prsn 行属于人物域，ID 与角色相互独立、不可按数字混用。
	rrows, err := h.getDB().Query(`SELECT pr.relation_type, pr.related_person_id, COALESCE(c.name, ''), COALESCE(c.role, 0)
		FROM person_relations pr
		JOIN characters c ON c.id = pr.related_person_id
		WHERE pr.person_type = 'crt' AND pr.person_id = ?
		UNION ALL
		SELECT pr.relation_type, pr.person_id, COALESCE(c.name, ''), COALESCE(c.role, 0)
		FROM person_relations pr
		JOIN characters c ON c.id = pr.person_id
		WHERE pr.person_type = 'crt' AND pr.related_person_id = ?
		ORDER BY relation_type, related_person_id`, id, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rrows.Close()
	for rrows.Next() {
		var (
			rt      int
			relID   int64
			name    string
			crole   int
		)
		if err := rrows.Scan(&rt, &relID, &name, &crole); err != nil {
			fail(c, 500, err.Error())
			return
		}
		d.Relations = append(d.Relations, personRelationItem{
			PersonType:      "crt",
			PersonID:        id,
			RelatedPersonID: relID,
			RelatedName:     name,
			RelatedType:     crole,
			RelationType:    rt,
			RelationName:    h.cons.PersonRelationCN("crt", rt),
		})
	}
	if err := rrows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	respOK(c, d)
}
