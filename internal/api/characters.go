package api

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// characterDetail 角色详情。
type characterDetail struct {
	ID       int64  `json:"id"`
	Role     int    `json:"role"`
	RoleName string `json:"role_name"`
	Name     string `json:"name"`
	Infobox  string `json:"infobox"`
	Summary  string `json:"summary"`
	Comments int    `json:"comments"`
	Collects int    `json:"collects"`
	// 出演作品
	Subjects []characterSubjectItem `json:"subjects"`
	// 声优/演员（person_characters 关联）
	CVs []cvItem `json:"cvs"`
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
		conds []string
		args  []any
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
				conds = append(conds, "c.name LIKE ?")
				args = append(args, "%"+q+"%")
			}
		} else {
			// 短于 trigram 最小长度的关键词无法命中全文索引，回退 LIKE
			conds = append(conds, "c.name LIKE ?")
			args = append(args, "%"+q+"%")
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
	// COUNT(*) OVER() 将总数统计与数据页读取合并为一次扫描
	rows, err := h.db.Query(`SELECT c.id, c.name, c.role, c.collects, c.comments, COUNT(*) OVER()
		FROM characters c`+where+` ORDER BY c.id LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type characterBrief struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Role     int    `json:"role"`
		RoleName string `json:"role_name"`
		Collects int    `json:"collects"`
		Comments int    `json:"comments"`
	}
	items := make([]characterBrief, 0, size)
	var total int64
	for rows.Next() {
		var it characterBrief
		if err := rows.Scan(&it.ID, &it.Name, &it.Role, &it.Collects, &it.Comments, &total); err != nil {
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

	d := characterDetail{Subjects: []characterSubjectItem{}, CVs: []cvItem{}}
	err := h.db.QueryRow(`SELECT id, role, name, infobox, summary, comments, collects
		FROM characters WHERE id = ?`, id).
		Scan(&d.ID, &d.Role, &d.Name, &d.Infobox, &d.Summary, &d.Comments, &d.Collects)
	if err != nil {
		fail(c, 404, "角色不存在")
		return
	}
	d.RoleName = h.cons.CharacterRoles[d.Role]

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
	srows.Close()

	// 声优/演员（CV）
	crows, err := h.db.Query(`SELECT pc.person_id, p.name, p.type, pc.subject_id, COALESCE(s.name, ''), pc.summary
		FROM person_characters pc
		JOIN persons p ON p.id = pc.person_id
		LEFT JOIN subjects s ON s.id = pc.subject_id
		WHERE pc.character_id = ?
		ORDER BY pc.type, pc.subject_id, pc.person_id`, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	for crows.Next() {
		var it cvItem
		if err := crows.Scan(&it.PersonID, &it.Name, &it.Type, &it.SubjectID, &it.SubjectName, &it.Summary); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.PersonTypes[it.Type]
		d.CVs = append(d.CVs, it)
	}
	crows.Close()

	respOK(c, d)
}
