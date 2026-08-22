package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
)

// searchPersons 人物搜索。
func (h *handler) searchPersons(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds []string
		args  []any
	)
	if q != "" {
		if useFTS(q) {
			ids, err := h.ftsRowIDs("persons_fts", q)
			if err == nil {
				if len(ids) == 0 {
					respOK(c, listResp{Total: 0, Page: 1, Size: 30, Items: []any{}})
					return
				}
				placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
				conds = append(conds, "p.id IN ("+placeholders+")")
				for _, id := range ids {
					args = append(args, id)
				}
			} else {
				conds = append(conds, "p.name LIKE ?")
				args = append(args, "%"+q+"%")
			}
		} else {
			// 短于 trigram 最小长度的关键词无法命中全文索引，回退 LIKE
			conds = append(conds, "p.name LIKE ?")
			args = append(args, "%"+q+"%")
		}
	}
	if v, ok := parseIntQuery(c, "type"); ok {
		conds = append(conds, "p.type = ?")
		args = append(args, v)
	}

	page, size := pagination(c)
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	queryArgs := append(args, size, (page-1)*size)
	// COUNT(*) OVER() 将总数统计与数据页读取合并为一次扫描
	rows, err := h.db.Query(`SELECT p.id, p.name, p.type, p.career, p.comments, p.collects, COUNT(*) OVER()
		FROM persons p`+where+` ORDER BY p.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type personBrief struct {
		ID       int64    `json:"id"`
		Name     string   `json:"name"`
		Type     int      `json:"type"`
		TypeName string   `json:"type_name"`
		Career   []string `json:"career"`
		Comments int      `json:"comments"`
		Collects int      `json:"collects"`
	}
	items := make([]personBrief, 0, size)
	var total int64
	for rows.Next() {
		var it personBrief
		var career string
		if err := rows.Scan(&it.ID, &it.Name, &it.Type, &career, &it.Comments, &it.Collects, &total); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.PersonTypes[it.Type]
		it.Career = parseStrings(career)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}

// personRelationItem 人物/角色关联（双向）。
type personRelationItem struct {
	PersonType      string `json:"person_type"`
	PersonID        int64  `json:"person_id"`
	RelatedPersonID int64  `json:"related_person_id"`
	RelatedName     string `json:"related_name"`
	RelatedType     int    `json:"related_type"`
	RelationType    int    `json:"relation_type"`
	RelationName    string `json:"relation_name"`
	Spoiler         bool   `json:"spoiler"`
	Ended           bool   `json:"ended"`
}

// personDetail 人物详情。
type personDetail struct {
	ID        int64                `json:"id"`
	Name      string               `json:"name"`
	Type      int                  `json:"type"`
	TypeName  string               `json:"type_name"`
	Career    []string             `json:"career"`
	Infobox   string               `json:"infobox"`
	Summary   string               `json:"summary"`
	Comments  int                  `json:"comments"`
	Collects  int                  `json:"collects"`
	Works     int64                `json:"works_count"`
	Roles     int64                `json:"roles_count"`
	Relations []personRelationItem `json:"relations"`
}

// getPerson 人物详情（含关联人物/角色）。
func (h *handler) getPerson(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	var (
		p      model.Person
		career string
	)
	err := h.db.QueryRow(`SELECT id, name, type, career, infobox, summary, comments, collects
		FROM persons WHERE id = ?`, id).Scan(&p.ID, &p.Name, &p.Type, &career, &p.Infobox, &p.Summary, &p.Comments, &p.Collects)
	if err != nil {
		fail(c, 404, "人物不存在")
		return
	}

	d := personDetail{
		ID:        p.ID,
		Name:      p.Name,
		Type:      p.Type,
		TypeName:  h.cons.PersonTypes[p.Type],
		Career:    parseStrings(career),
		Infobox:   p.Infobox,
		Summary:   p.Summary,
		Comments:  p.Comments,
		Collects:  p.Collects,
		Relations: []personRelationItem{},
	}

	if err := h.db.QueryRow("SELECT COUNT(*) FROM subject_persons WHERE person_id = ?", id).Scan(&d.Works); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := h.db.QueryRow("SELECT COUNT(*) FROM person_characters WHERE person_id = ?", id).Scan(&d.Roles); err != nil {
		fail(c, 500, err.Error())
		return
	}

	// 人物关系（双向）：prsn 与 crt 分开查询
	for _, pt := range []string{"prsn", "crt"} {
		rows, err := h.db.Query(`SELECT pr.person_type, pr.person_id, pr.related_person_id, pr.relation_type, pr.spoiler, pr.ended,
			COALESCE(p.name, ''), COALESCE(p.type, 0)
			FROM person_relations pr
			LEFT JOIN persons p ON p.id = pr.related_person_id
			WHERE pr.person_type = ? AND pr.person_id = ?
			UNION ALL
			SELECT pr.person_type, pr.related_person_id, pr.person_id, pr.relation_type, pr.spoiler, pr.ended,
			COALESCE(p.name, ''), COALESCE(p.type, 0)
			FROM person_relations pr
			LEFT JOIN persons p ON p.id = pr.person_id
			WHERE pr.person_type = ? AND pr.related_person_id = ?`, pt, id, pt, id)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		for rows.Next() {
			var it personRelationItem
			var pname string
			var ptype int
			if err := rows.Scan(&it.PersonType, &it.PersonID, &it.RelatedPersonID, &it.RelationType,
				&it.Spoiler, &it.Ended, &pname, &ptype); err != nil {
				fail(c, 500, err.Error())
				return
			}
			it.RelatedName = pname
			it.RelatedType = ptype
			it.RelationName = h.cons.PersonRelationCN(pt, it.RelationType)
			d.Relations = append(d.Relations, it)
		}
		rows.Close()
	}

	respOK(c, d)
}

// workItem 人物参与的作品。
type workItem struct {
	SubjectID    int64   `json:"subject_id"`
	Name         string  `json:"name"`
	NameCN       string  `json:"name_cn"`
	Type         int     `json:"type"`
	TypeName     string  `json:"type_name"`
	Platform     int     `json:"platform"`
	PlatformName string  `json:"platform_name"`
	Date         string  `json:"date"`
	Score        float64 `json:"score"`
	Rank         int     `json:"rank"`
	Position     int     `json:"position"`
	PositionName string  `json:"position_name"`
	AppearEps    string  `json:"appear_eps,omitempty"`
}

// getPersonWorks 人物参与的作品列表。
// 参数：position（职位过滤）、subject_type、page、size。
func (h *handler) getPersonWorks(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	page, size := pagination(c)

	conds := []string{"sp.person_id = ?"}
	args := []any{id}
	if v, ok := parseIntQuery(c, "position"); ok {
		conds = append(conds, "sp.position = ?")
		args = append(args, v)
	}
	if v, ok := parseIntQuery(c, "subject_type"); ok {
		conds = append(conds, "s.type = ?")
		args = append(args, v)
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int64
	if err := h.db.QueryRow("SELECT COUNT(*) FROM subject_persons sp JOIN subjects s ON s.id = sp.subject_id"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	queryArgs := append(args, size, (page-1)*size)
	rows, err := h.db.Query(`SELECT s.id, s.name, s.name_cn, s.type, s.platform, s.date, s.score, s.rank, sp.position, sp.appear_eps
		FROM subject_persons sp JOIN subjects s ON s.id = sp.subject_id`+where+`
		ORDER BY s.date DESC, s.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]workItem, 0, size)
	for rows.Next() {
		var it workItem
		if err := rows.Scan(&it.SubjectID, &it.Name, &it.NameCN, &it.Type, &it.Platform,
			&it.Date, &it.Score, &it.Rank, &it.Position, &it.AppearEps); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.SubjectTypeCN(it.Type)
		it.PlatformName = h.cons.PlatformCN(it.Type, it.Platform)
		it.PositionName = h.cons.StaffCN(it.Type, it.Position)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}

// collaboratorItem 合作人物（通过共同参与的作品统计）。
type collaboratorItem struct {
	PersonID int64  `json:"person_id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	TypeName string `json:"type_name"`
	// 共同参与的作品数
	Count int64 `json:"count"`
}

// getPersonCollaborators 与「X」合作的人物（按共同作品数降序）。
// 对应前端「与 X 合作的人物」板块（见 example.html）。
// 参数：page、size。
func (h *handler) getPersonCollaborators(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	page, size := pagination(c)

	var total int64
	if err := h.db.QueryRow(`SELECT COUNT(DISTINCT sp2.person_id)
		FROM subject_persons sp1
		JOIN subject_persons sp2 ON sp2.subject_id = sp1.subject_id AND sp2.person_id <> sp1.person_id
		WHERE sp1.person_id = ?`, id).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.db.Query(`SELECT sp2.person_id, p.name, p.type, COUNT(*) AS cnt
		FROM subject_persons sp1
		JOIN subject_persons sp2 ON sp2.subject_id = sp1.subject_id AND sp2.person_id <> sp1.person_id
		JOIN persons p ON p.id = sp2.person_id
		WHERE sp1.person_id = ?
		GROUP BY sp2.person_id
		ORDER BY cnt DESC, sp2.person_id
		LIMIT ? OFFSET ?`, id, size, (page-1)*size)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := make([]collaboratorItem, 0, size)
	for rows.Next() {
		var it collaboratorItem
		if err := rows.Scan(&it.PersonID, &it.Name, &it.Type, &it.Count); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.PersonTypes[it.Type]
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	respOK(c, listResp{Total: total, Page: page, Size: size, Items: items})
}
