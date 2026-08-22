package api

import (
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
	"bangumi-subject-go/internal/wiki"
)

// collaborationAppCTE 某人物出现过的全部条目集合：
// 制作人员（subject-persons）与声优等角色出演（person-characters）取并集。
const collaborationAppCTE = `app AS (
	SELECT DISTINCT subject_id FROM subject_persons WHERE person_id = ?
	UNION
	SELECT DISTINCT subject_id FROM person_characters WHERE person_id = ?
)`

// collabPerson 左侧展示的人物简介。
type collabPerson struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	Type     int          `json:"type"`
	TypeName string       `json:"type_name"`
	Career   []string     `json:"career"`
	Summary  string       `json:"summary"`
	Comments int          `json:"comments"`
	Collects int          `json:"collects"`
	Subjects int64        `json:"subjects_count"` // 参与条目数（含制作与出演）
	Infobox  []wiki.Field `json:"infobox,omitempty"`
}

// collabSubject 与合作人物的共同参与条目（roles 已用常量文本替换职位 id）。
type collabSubject struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	NameCN   string   `json:"name_cn"`
	Type     int      `json:"type"`
	TypeName string   `json:"type_name"`
	Date     string   `json:"date"`
	Roles    []string `json:"roles"` // 职位中文名 / CV·角色名，去重
}

// collabItem 一位合作人物及其共同参与的条目。
type collabItem struct {
	PersonID int64            `json:"person_id"`
	Name     string           `json:"name"`
	Type     int              `json:"type"`
	TypeName string           `json:"type_name"`
	Career   []string         `json:"career"`
	Summary  string           `json:"summary"`
	Count    int64            `json:"count"` // 共同参与条目数
	Subjects []*collabSubject `json:"subjects"`
}

// getPersonCollaboration 「人物合作」页数据：人物简介 + 分页的合作人物及共同条目。
// 合作 = 两人在同一条目中出现（subject-persons 制作人员 ∪ person-characters 声优等）。
// 合作人物按共同条目数降序（倒序）分页返回，条目按日期降序（倒序）。
func (h *handler) getPersonCollaboration(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	page, size := pagination(c)

	var (
		p      model.Person
		career string
	)
	err := h.db.QueryRow(`SELECT id, name, type, career, infobox, summary, comments, collects
		FROM persons WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Type, &career, &p.Infobox, &p.Summary, &p.Comments, &p.Collects)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fail(c, 404, "人物不存在")
		} else {
			fail(c, 500, err.Error())
		}
		return
	}

	person := collabPerson{
		ID: p.ID, Name: p.Name,
		Type: p.Type, TypeName: h.cons.PersonTypes[p.Type],
		Career: parseStrings(career), Summary: p.Summary,
		Comments: p.Comments, Collects: p.Collects,
	}
	if ib, err := wiki.ParseInfobox(p.Infobox); err == nil {
		person.Infobox = ib.Fields
	}
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM (
			SELECT DISTINCT subject_id FROM subject_persons WHERE person_id = ?
			UNION
			SELECT DISTINCT subject_id FROM person_characters WHERE person_id = ?)`, id, id).
		Scan(&person.Subjects); err != nil {
		fail(c, 500, err.Error())
		return
	}

	// 合作人物汇总（UNION 对 (person_id, subject_id) 去重，COUNT 即共同条目数）；
	// COUNT(*) OVER() 在分组后统计总组数（合作人物总数）
	rows, err := h.db.Query(`WITH `+collaborationAppCTE+`,
		pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
		)
		SELECT pr.other, p.name, p.type, p.career, p.summary, COUNT(*) AS cnt, COUNT(*) OVER()
		FROM pairs pr JOIN persons p ON p.id = pr.other
		GROUP BY pr.other
		ORDER BY cnt DESC, pr.other ASC
		LIMIT ? OFFSET ?`, id, id, id, id, size, (page-1)*size)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}

	items := []*collabItem{}
	var total int64
	for rows.Next() {
		var it collabItem
		var career2 string
		if err := rows.Scan(&it.PersonID, &it.Name, &it.Type, &career2, &it.Summary, &it.Count, &total); err != nil {
			rows.Close()
			fail(c, 500, err.Error())
			return
		}
		it.TypeName = h.cons.PersonTypes[it.Type]
		it.Career = parseStrings(career2)
		it.Subjects = []*collabSubject{}
		items = append(items, &it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	if len(items) > 0 {
		if err := h.attachCollabSubjects(c, id, items); err != nil {
			fail(c, 500, err.Error())
			return
		}
	}

	respOK(c, gin.H{
		"total":  total,
		"page":   page,
		"size":   size,
		"person": person,
		"items":  items,
	})
}

// attachCollabSubjects 为当前页的合作人物填充共同条目明细：
// 制作职位 + 出演角色（CV），逐行归并到 (合作人物, 条目)，职位 id 用常量文本替换。
func (h *handler) attachCollabSubjects(c *gin.Context, id int64, items []*collabItem) error {
	index := map[int64]int{}
	inArgs := make([]any, 0, len(items))
	for i, it := range items {
		index[it.PersonID] = i
		inArgs = append(inArgs, it.PersonID)
	}
	queryArgs := append([]any{id, id}, inArgs...)
	queryArgs = append(queryArgs, inArgs...)

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(items)), ",")
	rows, err := h.db.Query(`WITH `+collaborationAppCTE+`,
		detail AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid, sp.position AS position, 0 AS is_cv, '' AS char_name
			FROM app ap JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id IN (`+placeholders+`)
			UNION ALL
			SELECT pc.person_id, ap.subject_id, -1, 1, COALESCE(ch.name, '')
			FROM app ap
			JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id IN (`+placeholders+`)
			LEFT JOIN characters ch ON ch.id = pc.character_id
		)
		SELECT d.other, d.position, d.is_cv, d.char_name, s.id, s.name, s.name_cn, s.type, s.date
		FROM detail d JOIN subjects s ON s.id = d.sid`, queryArgs...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type subKey struct {
		other int64
		sid   int64
	}
	subIndex := map[subKey]*collabSubject{}
	for rows.Next() {
		var other, sid int64
		var position, isCv int64
		var charName, name, nameCN, date string
		var stype int64
		if err := rows.Scan(&other, &position, &isCv, &charName, &sid, &name, &nameCN, &stype, &date); err != nil {
			return err
		}
		oi, ok := index[other]
		if !ok {
			continue
		}
		key := subKey{other: other, sid: sid}
		cs, ok := subIndex[key]
		if !ok {
			cs = &collabSubject{
				ID: sid, Name: name, NameCN: nameCN,
				Type: int(stype), TypeName: h.cons.SubjectTypeCN(int(stype)),
				Date: date, Roles: []string{},
			}
			subIndex[key] = cs
			items[oi].Subjects = append(items[oi].Subjects, cs)
		}
		label := ""
		if isCv == 1 {
			label = "CV"
			if charName != "" {
				label += "·" + charName
			}
		} else {
			label = h.cons.StaffCN(cs.Type, int(position))
		}
		dup := false
		for _, r := range cs.Roles {
			if r == label {
				dup = true
				break
			}
		}
		if !dup {
			cs.Roles = append(cs.Roles, label)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 条目按日期降序（空日期靠后），同日按 id 降序
	for _, it := range items {
		subs := it.Subjects
		sort.SliceStable(subs, func(i, j int) bool {
			if subs[i].Date != subs[j].Date {
				return subs[i].Date > subs[j].Date
			}
			return subs[i].ID > subs[j].ID
		})
	}
	return nil
}
