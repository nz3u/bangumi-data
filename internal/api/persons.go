package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
	"bangumi-subject-go/internal/wiki"
)

// searchPersons 人物搜索。
func (h *handler) searchPersons(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	var (
		conds    []string
		args     []any
		fullScan bool // LIKE 回退时过滤条件必须逐行求值
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
				fullScan = true
				like := "%" + q + "%"
				conds = append(conds, "(p.name LIKE ? OR p.name_cn LIKE ?)")
				args = append(args, like, like)
			}
		} else {
			// 短于 trigram 最小长度的关键词无法命中全文索引，回退 LIKE
			fullScan = true
			like := "%" + q + "%"
			conds = append(conds, "(p.name LIKE ? OR p.name_cn LIKE ?)")
			args = append(args, like, like)
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

	// 计数与取数策略同 searchSubjects：可走索引时拆分查询避免全表物化，
	// LIKE 回退场景保留 COUNT(*) OVER() 合并为一次扫描。
	countCol := ""
	var totalPtr *int64
	total := int64(0)
	if fullScan {
		countCol = ", COUNT(*) OVER()"
		totalPtr = &total
	} else if err := h.getDB().QueryRow("SELECT COUNT(*) FROM persons p"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.getDB().Query(`SELECT p.id, p.name, p.name_cn, p.type, p.career, p.comments, p.collects`+countCol+`
		FROM persons p`+where+` ORDER BY p.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type personBrief struct {
		ID       int64    `json:"id"`
		Name     string   `json:"name"`
		NameCN   string   `json:"name_cn"`
		Type     int      `json:"type"`
		TypeName string   `json:"type_name"`
		Career   []string `json:"career"`
		Comments int      `json:"comments"`
		Collects int      `json:"collects"`
	}
	items := make([]personBrief, 0, size)
	for rows.Next() {
		var it personBrief
		var career string
		dest := []any{&it.ID, &it.Name, &it.NameCN, &it.Type, &career, &it.Comments, &it.Collects}
		if totalPtr != nil {
			dest = append(dest, totalPtr)
		}
		if err := rows.Scan(dest...); err != nil {
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
// 关联人物改由 /persons/:id/collaborators（人物合作）接口提供，前端抽屉直接引用。
type personDetail struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	NameCN    string       `json:"name_cn"`
	Type      int          `json:"type"`
	TypeName  string       `json:"type_name"`
	Career    []string     `json:"career"`
	Infobox   []wiki.Field `json:"infobox,omitempty"`
	Summary   string       `json:"summary"`
	Comments  int          `json:"comments"`
	Collects  int          `json:"collects"`
	Works     int64        `json:"works_count"`
	Roles     int64        `json:"roles_count"`
}

// getPerson 人物详情。关联人物/角色不再在此返回：
// 前端改用 /persons/:id/collaborators（人物合作）接口。
func (h *handler) getPerson(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	var (
		p           model.Person
		career      string
		nameCN      string
	)
	err := h.getDB().QueryRow(`SELECT id, name, name_cn, type, career, infobox, summary, comments, collects
		FROM persons WHERE id = ?`, id).Scan(&p.ID, &p.Name, &nameCN, &p.Type, &career, &p.Infobox, &p.Summary, &p.Comments, &p.Collects)
	if err != nil {
		fail(c, 404, "人物不存在")
		return
	}

	d := personDetail{
		ID:       p.ID,
		Name:     p.Name,
		NameCN:   nameCN,
		Type:     p.Type,
		TypeName: h.cons.PersonTypes[p.Type],
		Career:   parseStrings(career),
		Summary:  p.Summary,
		Comments: p.Comments,
		Collects: p.Collects,
	}
	if ib, err := wiki.ParseInfobox(p.Infobox); err == nil {
		d.Infobox = ib.Fields
	}

	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM subject_persons WHERE person_id = ?", id).Scan(&d.Works); err != nil {
		fail(c, 500, err.Error())
		return
	}
	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM person_characters WHERE person_id = ?", id).Scan(&d.Roles); err != nil {
		fail(c, 500, err.Error())
		return
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
	if err := h.getDB().QueryRow("SELECT COUNT(*) FROM subject_persons sp JOIN subjects s ON s.id = sp.subject_id"+where, args...).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	queryArgs := append(args, size, (page-1)*size)
	rows, err := h.getDB().Query(`SELECT s.id, s.name, s.name_cn, s.type, s.platform, s.date, s.score, s.rank, sp.position, sp.appear_eps
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
	if err := h.getDB().QueryRow(`SELECT COUNT(DISTINCT sp2.person_id)
		FROM subject_persons sp1
		JOIN subject_persons sp2 ON sp2.subject_id = sp1.subject_id AND sp2.person_id <> sp1.person_id
		WHERE sp1.person_id = ?`, id).Scan(&total); err != nil {
		fail(c, 500, err.Error())
		return
	}

	rows, err := h.getDB().Query(`SELECT sp2.person_id, p.name, p.type, COUNT(*) AS cnt
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
