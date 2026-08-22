package api

import (
	"database/sql"
	"errors"
	"sort"
	"strconv"
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

// ---- 棋盘筛选：按人物职位标签组合过滤 ----

// collabRoleFilter 一组职位标签（多选）：staff 为「作品类型:职位 id」组合，cv 表示声优出演。
type collabRoleFilter struct {
	staff [][2]int
	cv    bool
}

func (f collabRoleFilter) empty() bool { return len(f.staff) == 0 && !f.cv }

// parseCollabRoles 解析逗号分隔的职位标签参数（如 "2:1,cv,1:4"），非法项忽略。
func parseCollabRoles(param string) collabRoleFilter {
	var f collabRoleFilter
	seen := map[[2]int]bool{}
	for _, part := range strings.Split(param, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, "cv") {
			f.cv = true
			continue
		}
		typStr, posStr, ok := strings.Cut(part, ":")
		typ, err1 := strconv.Atoi(typStr)
		pos, err2 := strconv.Atoi(posStr)
		if !ok || err1 != nil || err2 != nil {
			continue
		}
		key := [2]int{typ, pos}
		if !seen[key] {
			seen[key] = true
			f.staff = append(f.staff, key)
		}
	}
	return f
}

// staffConds 生成 "(sa.type = ? AND sp.position = ?) OR …" 条件及参数。
func staffConds(typeCol, posCol string, keys [][2]int) (string, []any) {
	conds := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		conds = append(conds, "("+typeCol+" = ? AND "+posCol+" = ?)")
		args = append(args, k[0], k[1])
	}
	return strings.Join(conds, " OR "), args
}

// buildCollabAppCTE 组装 app CTE（被搜索人物的出现条目集）。
// fa 启用时仅保留其担任所选职位的条目；返回 CTE 文本与按序参数。
func buildCollabAppCTE(id int64, fa collabRoleFilter) (string, []any) {
	if fa.empty() {
		return collaborationAppCTE, []any{id, id}
	}
	var branches []string
	var args []any
	if len(fa.staff) > 0 {
		cond, cargs := staffConds("sa.type", "sp.position", fa.staff)
		branches = append(branches, `SELECT sp.subject_id FROM subject_persons sp
			JOIN subjects sa ON sa.id = sp.subject_id
			WHERE sp.person_id = ? AND (`+cond+`)`)
		args = append(append(args, id), cargs...)
	}
	if fa.cv {
		branches = append(branches, `SELECT pc.subject_id FROM person_characters pc WHERE pc.person_id = ?`)
		args = append(args, id)
	}
	return "app AS (" + strings.Join(branches, "\n\t\tUNION\n\t\t") + ")", args
}

// buildCollabPairsCTE 组装 pairs CTE（(合作人物, 共同条目) 对）。
// fb 启用时仅保留合作人物担任所选职位的条目对；返回 CTE 文本与按序参数。
func buildCollabPairsCTE(id int64, fb collabRoleFilter) (string, []any) {
	if fb.empty() {
		return `pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
		)`, []any{id, id}
	}
	joinSubjects, cond, condArgs := "", "1=1", []any{}
	if len(fb.staff) > 0 {
		joinSubjects = ` JOIN subjects sb ON sb.id = sp.subject_id`
		cond, condArgs = staffConds("sb.type", "sp.position", fb.staff)
	}
	cvCond := "1=1"
	if !fb.cv {
		cvCond = "1=0"
	}
	sql := `pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?` +
		joinSubjects + `
			WHERE ` + cond + `
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
			WHERE ` + cvCond + `
		)`
	return sql, append(append([]any{id}, condArgs...), id)
}

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
// 棋盘筛选参数：positions_a（当前人物职位标签）、positions_b（合作人物职位标签），
// 逗号分隔多选，key 形如 "2:1" 或 "cv"，两组之间取交集。
func (h *handler) getPersonCollaboration(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	page, size := pagination(c)
	fa := parseCollabRoles(c.Query("positions_a"))
	fb := parseCollabRoles(c.Query("positions_b"))

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
	appSQL, appArgs := buildCollabAppCTE(id, fa)
	pairsSQL, pairsArgs := buildCollabPairsCTE(id, fb)
	args := append(append([]any{}, appArgs...), pairsArgs...)
	args = append(args, size, (page-1)*size)
	rows, err := h.db.Query(`WITH `+appSQL+`,
`+pairsSQL+`
		SELECT pr.other, p.name, p.type, p.career, p.summary, COUNT(*) AS cnt, COUNT(*) OVER()
		FROM pairs pr JOIN persons p ON p.id = pr.other
		GROUP BY pr.other
		ORDER BY cnt DESC, pr.other ASC
		LIMIT ? OFFSET ?`, args...)
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
		if err := h.attachCollabSubjects(id, items, appSQL, appArgs); err != nil {
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
// appSQL/appArgs 为棋盘筛选后组装的 app CTE，保证明细与筛选口径一致。
func (h *handler) attachCollabSubjects(id int64, items []*collabItem, appSQL string, appArgs []any) error {
	index := map[int64]int{}
	inArgs := make([]any, 0, len(items))
	for i, it := range items {
		index[it.PersonID] = i
		inArgs = append(inArgs, it.PersonID)
	}
	queryArgs := append(append([]any{}, appArgs...), id)
	queryArgs = append(queryArgs, inArgs...)
	queryArgs = append(queryArgs, inArgs...)

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(items)), ",")
	rows, err := h.db.Query(`WITH `+appSQL+`,
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

// ---- 棋盘筛选：职位标签接口 ----

// collabFacet 职位标签（棋盘筛选项）：key 形如 "2:1" 或 "cv"，count 为涉及的条目数。
type collabFacet struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	CV    bool   `json:"cv"`
	Count int64  `json:"count"`
}

// facetLabel 职位标签中文名（CV 为常量文本）。
func (h *handler) facetLabel(stype, position int64, isCv bool) string {
	if isCv {
		return "CV"
	}
	return h.cons.StaffCN(int(stype), int(position))
}

// sortCollabFacets 排序：制作职位按 count 倒序、同数按名称，CV 恒排末尾。
func sortCollabFacets(list []collabFacet) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].CV != list[j].CV {
			return !list[i].CV
		}
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Label < list[j].Label
	})
}

// getPersonCollaborationPositions 「人物合作」棋盘筛选的职位标签：
// 搜索人物 id 后调用，返回两组标签——
//   - self：当前人物在共同条目中担任的职位（含 CV），count 为涉及条目数；
//   - other：合作人物在这些条目中担任的职位，count 为涉及 (人物, 条目) 对数。
func (h *handler) getPersonCollaborationPositions(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}
	if _, ok, err := h.loadPairBrief(id); err != nil {
		fail(c, 500, err.Error())
		return
	} else if !ok {
		fail(c, 404, "人物不存在")
		return
	}

	rows, err := h.db.Query(`WITH `+collaborationAppCTE+`,
		pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
		),
		self_roles AS (
			SELECT sa.type AS stype, sp.position AS pos, 0 AS is_cv, ap.subject_id AS sid
			FROM app ap JOIN subjects sa ON sa.id = ap.subject_id
			JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id = ?
			UNION ALL
			SELECT 0, 0, 1, ap.subject_id
			FROM app ap JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id = ?
		),
		other_roles AS (
			SELECT sb.type AS stype, sp.position AS pos, 0 AS is_cv, pr.other AS oid, pr.sid AS sid
			FROM pairs pr JOIN subject_persons sp ON sp.subject_id = pr.sid AND sp.person_id = pr.other
			JOIN subjects sb ON sb.id = pr.sid
			UNION ALL
			SELECT 0, 0, 1, pr.other, pr.sid
			FROM pairs pr JOIN person_characters pc ON pc.subject_id = pr.sid AND pc.person_id = pr.other
		),
		self_facets AS (
			SELECT stype, pos, MAX(is_cv) AS is_cv, COUNT(DISTINCT sid) AS cnt
			FROM self_roles GROUP BY stype, pos
		),
		other_facets AS (
			SELECT stype, pos, MAX(is_cv) AS is_cv, COUNT(*) AS cnt
			FROM (SELECT DISTINCT stype, pos, is_cv, oid, sid FROM other_roles)
			GROUP BY stype, pos
		)
		SELECT 'self' AS side, stype, pos, is_cv, cnt FROM self_facets
		UNION ALL
		SELECT 'other', stype, pos, is_cv, cnt FROM other_facets`,
		id, id, id, id, id, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	selfList, otherList := []collabFacet{}, []collabFacet{}
	for rows.Next() {
		var side string
		var stype, pos, isCv, cnt int64
		if err := rows.Scan(&side, &stype, &pos, &isCv, &cnt); err != nil {
			fail(c, 500, err.Error())
			return
		}
		cv := isCv == 1
		fc := collabFacet{
			Key:   "cv",
			Label: h.facetLabel(stype, pos, cv),
			CV:    cv,
			Count: cnt,
		}
		if !cv {
			fc.Key = strconv.FormatInt(stype, 10) + ":" + strconv.FormatInt(pos, 10)
		}
		if side == "self" {
			selfList = append(selfList, fc)
		} else {
			otherList = append(otherList, fc)
		}
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	sortCollabFacets(selfList)
	sortCollabFacets(otherList)

	respOK(c, gin.H{"self": selfList, "other": otherList})
}

// ---- 双人合作（共同作品） ----

// pairPerson 双人合作页头部的人物简介。
type pairPerson struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Type     int      `json:"type"`
	TypeName string   `json:"type_name"`
	Career   []string `json:"career"`
	Summary  string   `json:"summary"`
}

// roleLabel 条目中的职务标签；CV 标记用于前端按职位合并时识别声优出演。
type roleLabel struct {
	Text string `json:"text"` // 职位中文名 / CV·角色名
	CV   bool   `json:"cv"`
}

// pairWork 两人物共同参与的条目，分别携带双方职务（职位 id 已转常量文本）。
type pairWork struct {
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	NameCN       string      `json:"name_cn"`
	Type         int         `json:"type"`
	TypeName     string      `json:"type_name"`
	Date         string      `json:"date"`
	RolesA       []roleLabel `json:"roles_a"`
	RolesB       []roleLabel `json:"roles_b"`
}

// loadPairBrief 读取人物简介（不存在返回 false）。
func (h *handler) loadPairBrief(id int64) (*pairPerson, bool, error) {
	var p pairPerson
	var career string
	err := h.db.QueryRow(`SELECT id, name, type, career, summary FROM persons WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Type, &career, &p.Summary)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	p.TypeName = h.cons.PersonTypes[p.Type]
	p.Career = parseStrings(career)
	return &p, true, nil
}

// appendRoleLabel 追加去重后的职务标签。
func appendRoleLabel(list []roleLabel, label roleLabel) []roleLabel {
	for _, r := range list {
		if r == label {
			return list
		}
	}
	return append(list, label)
}

// getPersonCollaborationWith 双人合作：返回两人物共同参与的条目及双方职务。
// 共同参与 = 双方都在条目中出现（subject-persons 制作 ∪ person-characters 声优等）。
// 条目按日期倒序、同日按 id 倒序；前端按职位做双向合并分组展示。
func (h *handler) getPersonCollaborationWith(c *gin.Context) {
	idA, ok1 := intParam(c, "id")
	idB, ok2 := intParam(c, "other")
	if !ok1 || !ok2 {
		fail(c, 400, "无效的 id")
		return
	}
	if idA == idB {
		fail(c, 400, "两个人物 ID 不能相同")
		return
	}

	pa, found, err := h.loadPairBrief(idA)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if !found {
		fail(c, 404, "人物不存在")
		return
	}
	pb, found, err := h.loadPairBrief(idB)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if !found {
		fail(c, 404, "人物不存在")
		return
	}

	rows, err := h.db.Query(`WITH app_a AS (
			SELECT DISTINCT subject_id FROM subject_persons WHERE person_id = ?
			UNION
			SELECT DISTINCT subject_id FROM person_characters WHERE person_id = ?
		), app_b AS (
			SELECT DISTINCT subject_id FROM subject_persons WHERE person_id = ?
			UNION
			SELECT DISTINCT subject_id FROM person_characters WHERE person_id = ?
		), shared AS (
			SELECT a.subject_id AS sid FROM app_a a JOIN app_b b ON b.subject_id = a.subject_id
		), roles AS (
			SELECT sp.person_id AS pid, sp.subject_id AS sid, sp.position AS position, 0 AS is_cv, '' AS char_name
			FROM shared sh JOIN subject_persons sp ON sp.subject_id = sh.sid AND sp.person_id IN (?, ?)
			UNION ALL
			SELECT pc.person_id, pc.subject_id, -1, 1, COALESCE(ch.name, '')
			FROM shared sh2
			JOIN person_characters pc ON pc.subject_id = sh2.sid AND pc.person_id IN (?, ?)
			LEFT JOIN characters ch ON ch.id = pc.character_id
		)
		SELECT s.id, s.name, s.name_cn, s.type, s.date,
			r.pid, r.position, r.is_cv, r.char_name
		FROM shared sh
		JOIN subjects s ON s.id = sh.sid
		JOIN roles r ON r.sid = sh.sid
		ORDER BY s.date DESC, s.id DESC`,
		idA, idA, idB, idB, idA, idB, idA, idB)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := []*pairWork{}
	index := map[int64]int{}
	for rows.Next() {
		var (
			sid, pid              int64
			stype                 int64
			position, isCv        int64
			name, nameCN, date    string
			charName              string
		)
		if err := rows.Scan(&sid, &name, &nameCN, &stype, &date, &pid, &position, &isCv, &charName); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it, ok := index[sid]
		if !ok {
			it = len(items)
			index[sid] = it
			items = append(items, &pairWork{
				ID: sid, Name: name, NameCN: nameCN,
				Type: int(stype), TypeName: h.cons.SubjectTypeCN(int(stype)),
				Date: date,
				RolesA: []roleLabel{}, RolesB: []roleLabel{},
			})
		}
		label := roleLabel{Text: "", CV: isCv == 1}
		if label.CV {
			label.Text = "CV"
			if charName != "" {
				label.Text += "·" + charName
			}
		} else {
			label.Text = h.cons.StaffCN(int(stype), int(position))
		}
		if pid == idA {
			items[it].RolesA = appendRoleLabel(items[it].RolesA, label)
		} else {
			items[it].RolesB = appendRoleLabel(items[it].RolesB, label)
		}
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	respOK(c, gin.H{
		"person_a": pa,
		"person_b": pb,
		"total":    len(items),
		"items":    items,
	})
}

// ---- 单人作品（按职务分组） ----

// roleWork 单人参与的条目，roles 为该人物的全部职务（职位 id 已转常量文本）。
type roleWork struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	NameCN   string      `json:"name_cn"`
	Type     int         `json:"type"`
	TypeName string      `json:"type_name"`
	Date     string      `json:"date"`
	Roles    []roleLabel `json:"roles"`
}

// getPersonRoles 「单人作品」页数据：人物简介 + 其参与的全部条目及职务。
// 参与范围 = subject-persons 制作人员 ∪ person-characters 声优等出演。
// 条目按日期倒序、同日按 id 倒序；前端按职务分组并做快速筛选。
func (h *handler) getPersonRoles(c *gin.Context) {
	id, found := intParam(c, "id")
	if !found {
		fail(c, 400, "无效的 id")
		return
	}

	pa, found, err := h.loadPairBrief(id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	if !found {
		fail(c, 404, "人物不存在")
		return
	}

	rows, err := h.db.Query(`WITH apps AS (
			SELECT DISTINCT subject_id AS sid FROM subject_persons WHERE person_id = ?
			UNION
			SELECT DISTINCT subject_id AS sid FROM person_characters WHERE person_id = ?
		), detail AS (
			SELECT sp.subject_id AS sid, sp.position AS position, 0 AS is_cv, '' AS char_name
			FROM apps ap JOIN subject_persons sp ON sp.subject_id = ap.sid AND sp.person_id = ?
			UNION ALL
			SELECT pc.subject_id, -1, 1, COALESCE(ch.name, '')
			FROM apps ap2
			JOIN person_characters pc ON pc.subject_id = ap2.sid AND pc.person_id = ?
			LEFT JOIN characters ch ON ch.id = pc.character_id
		)
		SELECT s.id, s.name, s.name_cn, s.type, s.date,
			d.position, d.is_cv, d.char_name
		FROM apps ap
		JOIN subjects s ON s.id = ap.sid
		JOIN detail d ON d.sid = ap.sid
		ORDER BY s.date DESC, s.id DESC`, id, id, id, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := []*roleWork{}
	index := map[int64]int{}
	for rows.Next() {
		var (
			sid                int64
			stype              int64
			position, isCv     int64
			name, nameCN, date string
			charName           string
		)
		if err := rows.Scan(&sid, &name, &nameCN, &stype, &date, &position, &isCv, &charName); err != nil {
			fail(c, 500, err.Error())
			return
		}
		it, ok := index[sid]
		if !ok {
			it = len(items)
			index[sid] = it
			items = append(items, &roleWork{
				ID: sid, Name: name, NameCN: nameCN,
				Type: int(stype), TypeName: h.cons.SubjectTypeCN(int(stype)),
				Date: date, Roles: []roleLabel{},
			})
		}
		label := roleLabel{Text: "", CV: isCv == 1}
		if label.CV {
			label.Text = "CV"
			if charName != "" {
				label.Text += "·" + charName
			}
		} else {
			label.Text = h.cons.StaffCN(int(stype), int(position))
		}
		items[it].Roles = appendRoleLabel(items[it].Roles, label)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	respOK(c, gin.H{
		"person": pa,
		"total":  len(items),
		"items":  items,
	})
}
