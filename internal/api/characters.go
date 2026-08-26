package api

import (
	"strings"

	"github.com/gin-gonic/gin"

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

// searchCharacters 角色搜索。
func (h *handler) searchCharacters(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds    []string
		args     []any
		fullScan bool // LIKE 回退时过滤条件必须逐行求值
	)
	if q != "" {
		if useFTS(q) {
			ids, err := h.ftsRowIDs("characters_fts", q)
			if err == nil {
				if len(ids) == 0 {
					respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
					return
				}
				placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
				conds = append(conds, "c.id IN ("+placeholders+")")
				for _, id := range ids {
					args = append(args, id)
				}
			} else {
				fullScan = true
				like := "%" + q + "%"
				conds = append(conds, "(c.name LIKE ? OR c.name_cn LIKE ?)")
				args = append(args, like, like)
			}
		} else {
			// 短于 trigram 最小长度的关键词无法命中全文索引，回退 LIKE
			fullScan = true
			like := "%" + q + "%"
			conds = append(conds, "(c.name LIKE ? OR c.name_cn LIKE ?)")
			args = append(args, like, like)
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

	queryArgs := append(args, size, (page-1)*size)

	// 计数与取数策略同 searchSubjects：可走索引时拆分查询避免全表物化，
	// LIKE 回退场景保留 COUNT(*) OVER() 合并为一次扫描。
	countCol := ""
	var totalPtr *int64
	total := int64(0)
	if fullScan {
		countCol = ", COUNT(*) OVER()"
		totalPtr = &total
	} else if err := h.db.QueryRow("SELECT COUNT(*) FROM characters c"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.db.Query(`SELECT c.id, c.name, c.name_cn, c.role, c.collects, c.comments`+countCol+`
		FROM characters c`+where+` ORDER BY c.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type characterBrief struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		NameCN   string `json:"name_cn"`
		Role     int    `json:"role"`
		RoleName string `json:"role_name"`
		Collects int    `json:"collects"`
		Comments int    `json:"comments"`
	}
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
func (h *handler) getCharacter(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	d := characterDetail{Subjects: []characterSubjectItem{}, CVs: []cvItem{}, Relations: []personRelationItem{}}
	var rawInfobox string
	err := h.db.QueryRow(`SELECT id, role, name, name_cn, infobox, summary, comments, collects
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
	srows, err := h.db.Query(`SELECT s.id, s.name, s.name_cn, s.type, sc.type, sc."order", s.date, s.score
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
	crows, err := h.db.Query(`SELECT pc.person_id, p.name, MIN(pc.type)
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
	rrows, err := h.db.Query(`SELECT pr.relation_type, pr.related_person_id, COALESCE(c.name, ''), COALESCE(c.role, 0)
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
