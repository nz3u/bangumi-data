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
// JOIN subjects 过滤指向已失效条目的孤儿引用，保证计数与明细口径一致。
const collaborationAppCTE = `app AS (
	SELECT DISTINCT sp.subject_id FROM subject_persons sp
	JOIN subjects sa ON sa.id = sp.subject_id
	WHERE sp.person_id = ?
	UNION
	SELECT DISTINCT pc.subject_id FROM person_characters pc
	JOIN subjects sb ON sb.id = pc.subject_id
	WHERE pc.person_id = ?
)`

// ---- 棋盘筛选：按人物职位标签组合过滤 ----

// collabRoleFilter 一组职位标签（多选）：staff 为「作品类型:职位 id」组合，cv 表示声优出演。
// negStaff 为负标签（排除），cvNeg 表示排除声优出演。
type collabRoleFilter struct {
	staff   [][2]int
	negStaff [][2]int
	cv      bool
	cvNeg   bool
}

func (f collabRoleFilter) empty() bool { return len(f.staff) == 0 && len(f.negStaff) == 0 && !f.cv && !f.cvNeg }

// parseCollabRoles 解析逗号分隔的职位标签参数（如 "2:1,cv,-2:10"），非法项忽略。
// 前缀 "-" 表示负标签（排除），如 "-2:1" 表示排除 type=2, position=1 的职位。
func parseCollabRoles(param string) collabRoleFilter {
	var f collabRoleFilter
	seen := map[[2]int]bool{}
	negSeen := map[[2]int]bool{}
	for _, part := range strings.Split(param, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		neg := strings.HasPrefix(part, "-")
		if neg {
			part = part[1:]
		}
		if strings.EqualFold(part, "cv") {
			if neg {
				f.cvNeg = true
			} else {
				f.cv = true
			}
			continue
		}
		typStr, posStr, ok := strings.Cut(part, ":")
		typ, err1 := strconv.Atoi(typStr)
		pos, err2 := strconv.Atoi(posStr)
		if !ok || err1 != nil || err2 != nil {
			continue
		}
		key := [2]int{typ, pos}
		if neg {
			if !negSeen[key] {
				negSeen[key] = true
				f.negStaff = append(f.negStaff, key)
			}
		} else {
			if !seen[key] {
				seen[key] = true
				f.staff = append(f.staff, key)
			}
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

// negStaffExists 生成负标签排除的 NOT EXISTS 子查询。
// 检查 subject_persons 中是否存在匹配负标签的记录，存在则排除。
// 如果 personID 为 nil，则使用外层查询的 sp.person_id 列（用于 attachCollabSubjects 等场景）。
func negStaffExists(keys [][2]int, personID any) (string, []any) {
	if len(keys) == 0 {
		return "", nil
	}
	conds := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		conds = append(conds, "(sa_neg.type = ? AND sp_neg.position = ?)")
		args = append(args, k[0], k[1])
	}
	personCond := "sp_neg.person_id = ?"
	if personID == nil {
		// 使用外层查询的 sp.person_id 列
		personCond = "sp_neg.person_id = sp.person_id"
	}
	subQuery := `NOT EXISTS (
		SELECT 1 FROM subject_persons sp_neg 
		CROSS JOIN subjects sa_neg ON sa_neg.id = sp_neg.subject_id 
		WHERE sp_neg.subject_id = sp.subject_id 
		AND ` + personCond + `
		AND (` + strings.Join(conds, " OR ") + `)
	)`
	if personID == nil {
		return subQuery, args
	}
	// person_id 参数放在最前面
	return subQuery, append([]any{personID}, args...)
}

// buildCollabAppCTE 组装 app CTE（被搜索人物的出现条目集）。
// fa 启用时仅保留其担任所选职位的条目；返回 CTE 文本与按序参数。
// 负标签通过 NOT EXISTS 排除匹配的条目。
func buildCollabAppCTE(id int64, fa collabRoleFilter) (string, []any) {
	if fa.empty() {
		return collaborationAppCTE, []any{id, id}
	}
	var branches []string
	var args []any

	// 判断是否需要添加 staff 分支
	hasPosStaff := len(fa.staff) > 0
	hasNegStaff := len(fa.negStaff) > 0
	hasAnyFilter := hasPosStaff || hasNegStaff || fa.cv || fa.cvNeg

	if hasAnyFilter {
		// 构建 staff 分支的 WHERE 条件
		var whereParts []string
		var whereArgs []any

		if hasPosStaff {
			// 有正标签：只包含匹配的职位
			cond, cargs := staffConds("sa.type", "sp.position", fa.staff)
			whereParts = append(whereParts, "("+cond+")")
			whereArgs = append(whereArgs, cargs...)
		}
		// 如果没有正标签但有负标签，不添加条件（包含所有 staff）

		// 添加负标签排除条件（NOT EXISTS）
		if hasNegStaff {
			negCond, negArgs := negStaffExists(fa.negStaff, id)
			if negCond != "" {
				whereParts = append(whereParts, negCond)
				whereArgs = append(whereArgs, negArgs...)
			}
		}

		whereClause := "1=1"
		if len(whereParts) > 0 {
			whereClause = strings.Join(whereParts, " AND ")
		}

		branch := `SELECT sp.subject_id FROM subject_persons sp
			CROSS JOIN subjects sa ON sa.id = sp.subject_id
			WHERE sp.person_id = ? AND ` + whereClause
		args = append(args, id)
		args = append(args, whereArgs...)
		branches = append(branches, branch)
	}

	// CV 分支：cv 为正标签时包含，cvNeg 为负标签时排除
	if fa.cv || fa.cvNeg {
		branches = append(branches, `SELECT pc.subject_id FROM person_characters pc WHERE pc.person_id = ?`)
		args = append(args, id)
	}

	if len(branches) == 0 {
		return collaborationAppCTE, []any{id, id}
	}
	return "app AS (" + strings.Join(branches, "\n\t\tUNION\n\t\t") + ")", args
}

// buildCollabPairsCTE 组装 pairs CTE（(合作人物, 共同条目) 对）。
// fb 启用时仅保留合作人物担任所选职位的条目对；返回 CTE 文本与按序参数。
// 负标签通过 NOT EXISTS 排除匹配的条目。
//
// CROSS JOIN 强制以 app（小结果集）为外层循环：`person_id <> ?` 这类
// 非等值条件无法用作索引约束，普通 JOIN 下优化器可能误选对
// subject_persons/person_characters 全表扫描（210 万行，秒级开销）。
func buildCollabPairsCTE(id int64, fb collabRoleFilter) (string, []any) {
	if fb.empty() {
		return `pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap CROSS JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap CROSS JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
		)`, []any{id, id}
	}

	hasPosStaff := len(fb.staff) > 0
	hasNegStaff := len(fb.negStaff) > 0
	joinSubjects := ""
	cond := "1=1"
	var condArgs []any

	if hasPosStaff || hasNegStaff {
		joinSubjects = ` JOIN subjects sb ON sb.id = sp.subject_id`
		// 构建 staff 分支的 WHERE 条件
		var condParts []string
		if hasPosStaff {
			condStr, cargs := staffConds("sb.type", "sp.position", fb.staff)
			condParts = append(condParts, "("+condStr+")")
			condArgs = append(condArgs, cargs...)
		}
		// 添加负标签排除条件（NOT EXISTS）
		if hasNegStaff {
			negCond, negArgs := negStaffExists(fb.negStaff, id)
			if negCond != "" {
				condParts = append(condParts, negCond)
				condArgs = append(condArgs, negArgs...)
			}
		}
		if len(condParts) > 0 {
			cond = strings.Join(condParts, " AND ")
		}
	}

	// CV 分支：cv 为正标签时包含，cvNeg 为负标签时排除（WHERE 1=0）
	cvCond := "1=1"
	if fb.cv && !fb.cvNeg {
		cvCond = "1=1" // 包含 CV
	} else if fb.cvNeg {
		cvCond = "1=0" // 排除 CV
	} else if !fb.cv && !fb.cvNeg && !hasPosStaff && !hasNegStaff {
		cvCond = "1=1" // 无任何筛选时包含 CV
	} else {
		// 有 staff 筛选但无 CV 筛选：不包含 CV（只看 staff）
		cvCond = "1=0"
	}

	sql := `pairs AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid
			FROM app ap CROSS JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?` +
		joinSubjects + `
			WHERE ` + cond + `
			UNION
			SELECT pc.person_id, ap.subject_id
			FROM app ap CROSS JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id <> ?
			WHERE ` + cvCond + `
		)`
	// 参数：id（sp.person_id <> ?）, condArgs, id（pc.person_id <> ?）
	args := []any{id}
	args = append(args, condArgs...)
	args = append(args, id)
	return sql, args
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

	// 合作人物汇总：先按人物聚合出 (合作人物, 共同条目数) 并分页，
	// 之后再回表取 persons 的 name/career/summary 等大文本——
	// 合作人物可达数万，提前 JOIN 会为所有人生成并排序携带大字段的行。
	// agg 内 COUNT(*) OVER() 在分组后统计总组数（合作人物总数）。
	appSQL, appArgs := buildCollabAppCTE(id, fa)
	pairsSQL, pairsArgs := buildCollabPairsCTE(id, fb)
	args := append(append([]any{}, appArgs...), pairsArgs...)
	args = append(args, size, (page-1)*size)
	rows, err := h.db.Query(`WITH `+appSQL+`,
`+pairsSQL+`,
		agg AS (
			SELECT other, COUNT(*) AS cnt, COUNT(*) OVER() AS total
			FROM pairs GROUP BY other
		)
		SELECT a.other, p.name, p.type, p.career, p.summary, a.cnt, a.total
		FROM (SELECT * FROM agg ORDER BY cnt DESC, other ASC LIMIT ? OFFSET ?) a
		JOIN persons p ON p.id = a.other
		ORDER BY a.cnt DESC, a.other ASC`, args...)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

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
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	if len(items) > 0 {
		if err := h.attachCollabSubjects(id, items, appSQL, appArgs, fb); err != nil {
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
// appSQL/appArgs 为 positions_a 筛选后组装的 app CTE；fb（positions_b）直接作用于
// 明细行，条件与 pairs CTE 同口径，保证条目集合与统计次数一致，
// 不会混入与合作人物所选职位无关的作品。
func (h *handler) attachCollabSubjects(id int64, items []*collabItem, appSQL string, appArgs []any, fb collabRoleFilter) error {
	index := map[int64]int{}
	inArgs := make([]any, 0, len(items))
	for i, it := range items {
		index[it.PersonID] = i
		inArgs = append(inArgs, it.PersonID)
	}

	// 明细行筛选与 pairs 同口径：fb 为空时全部保留；
	// 否则制作行须命中所选职位组合、CV 行仅在勾选 cv 标签时保留。
	joinSubjects, staffCond, staffArgs := "", "1=1", []any{}
	hasPosStaff := len(fb.staff) > 0
	hasNegStaff := len(fb.negStaff) > 0

	if hasPosStaff || hasNegStaff {
		joinSubjects = ` JOIN subjects sb ON sb.id = sp.subject_id`
		// 构建 staff 分支的 WHERE 条件
		var condParts []string
		if hasPosStaff {
			condStr, cargs := staffConds("sb.type", "sp.position", fb.staff)
			condParts = append(condParts, "("+condStr+")")
			staffArgs = append(staffArgs, cargs...)
		}
		// 添加负标签排除条件（NOT EXISTS）
		if hasNegStaff {
			negCond, negArgs := negStaffExists(fb.negStaff, nil) // nil 表示使用外层 sp.person_id
			if negCond != "" {
				condParts = append(condParts, negCond)
				staffArgs = append(staffArgs, negArgs...)
			}
		}
		if len(condParts) > 0 {
			staffCond = strings.Join(condParts, " AND ")
		}
	}

	// CV 分支：cv 为正标签时包含，cvNeg 为负标签时排除
	cvCond := "1=1"
	if fb.cv && !fb.cvNeg {
		cvCond = "1=1" // 包含 CV
	} else if fb.cvNeg {
		cvCond = "1=0" // 排除 CV
	} else if !fb.cv && !fb.cvNeg && !hasPosStaff && !hasNegStaff {
		cvCond = "1=1" // 无任何筛选时包含 CV
	} else {
		// 有 staff 筛选但无 CV 筛选：不包含 CV（只看 staff）
		cvCond = "1=0"
	}

	// 参数按 SQL 文本中占位符出现顺序排列：
	// app CTE 参数 → 制作分支 IN(当前页人物) → 职位条件 → 出演分支 IN(当前页人物)
	queryArgs := append(append([]any{}, appArgs...), inArgs...)
	queryArgs = append(queryArgs, staffArgs...)
	queryArgs = append(queryArgs, inArgs...)

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(items)), ",")
	// CROSS JOIN 固定连接顺序：app/detail 均为小结果集，
	// 避免 SQLite 把大表（subjects/subject_persons）选作外层全表扫描
	rows, err := h.db.Query(`WITH `+appSQL+`,
		detail AS (
			SELECT sp.person_id AS other, ap.subject_id AS sid, sp.position AS position, 0 AS is_cv, '' AS char_name
			FROM app ap CROSS JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id IN (`+placeholders+`)`+
		joinSubjects+`
			WHERE `+staffCond+`
			UNION ALL
			SELECT pc.person_id, ap.subject_id, -1, 1, COALESCE(ch.name, '')
			FROM app ap
			CROSS JOIN person_characters pc ON pc.subject_id = ap.subject_id AND pc.person_id IN (`+placeholders+`)
			LEFT JOIN characters ch ON ch.id = pc.character_id
			WHERE `+cvCond+`
		)
		SELECT d.other, d.position, d.is_cv, d.char_name, s.id, s.name, s.name_cn, s.type, s.date
		FROM detail d CROSS JOIN subjects s ON s.id = d.sid`, queryArgs...)
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

// collabFacet 职位标签（棋盘筛选项）：同名职位跨条目类型合并为一项，
// key 为其原始键的逗号连接（如 "2:20,4:1013"）、或 "cv"；count 为涉及条目数。
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

// sortCollabFacets 排序：按 count 倒序、同数按名称（CV 与普通职位一视同仁）。
func sortCollabFacets(list []collabFacet) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Label < list[j].Label
	})
}

// facetAcc 单个职位标签的累计器：keys 为合并前的 (类型:职位) 原始键，
// seen 为去重口径的集合（self 侧为条目、other 侧为 (人物, 条目) 对）。
type facetAcc struct {
	keys []string
	seen map[[2]int64]struct{}
}

func (a *facetAcc) add(key string, k1, k2 int64) {
	dup := false
	for _, k := range a.keys {
		if k == key {
			dup = true
			break
		}
	}
	if !dup {
		a.keys = append(a.keys, key)
	}
	a.seen[[2]int64{k1, k2}] = struct{}{}
}

// getPersonCollaborationPositions 「人物合作」棋盘筛选的职位标签：
// 搜索人物 id 后调用，返回两组标签——
//   - self：当前人物在共同条目中担任的职位（含 CV），count 为涉及条目数；
//   - other：合作人物在这些条目中担任的职位，count 为涉及 (人物, 条目) 对数。
//
// 同名职位（跨条目类型的同一职位名）合并为一项，key 为原始键的逗号连接，
// 可直接透传给 positions_a/positions_b；CV 只要有声优出演即计（可与制作职位并存）。
// 该口径与单人作品页、双人合作页的前端统计一致。
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

	// 直接从基表枚举去重后的角色明细行，标签合并在 Go 侧完成：
	//   - SQL 内按 (stype,pos,sid[,oid]) DISTINCT，消除同条目多角色行的重复；
	//   - app_t/CROSS JOIN 固定 app 为外层循环，杜绝 `person_id <> ?` 引发的全表扫描。
	rows, err := h.db.Query(`WITH `+collaborationAppCTE+`,
		self_staff AS (
			SELECT DISTINCT sa.type AS stype, sp.position AS pos, ap.subject_id AS sid
			FROM app ap
			CROSS JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id = ?
			CROSS JOIN subjects sa ON sa.id = ap.subject_id
		),
		other_staff AS (
			SELECT DISTINCT sa.type AS stype, sp.position AS pos, sp.person_id AS oid, ap.subject_id AS sid
			FROM app ap
			CROSS JOIN subject_persons sp ON sp.subject_id = ap.subject_id AND sp.person_id <> ?
			CROSS JOIN subjects sa ON sa.id = ap.subject_id
		)
		SELECT 'self_s', stype, pos, 0 AS is_cv, sid AS k1, 0 AS k2 FROM self_staff
		UNION ALL
		SELECT 'self_cv', 0, 0, 1, pc.subject_id, 0
		FROM app ap1 CROSS JOIN person_characters pc ON pc.subject_id = ap1.subject_id AND pc.person_id = ?
		UNION ALL
		SELECT 'other_s', stype, pos, 0, sid, oid FROM other_staff
		UNION ALL
		SELECT 'other_cv', 0, 0, 1, pc.subject_id, pc.person_id
		FROM app ap2 CROSS JOIN person_characters pc ON pc.subject_id = ap2.subject_id AND pc.person_id <> ?`,
		id, id, id, id, id, id)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	selfAcc := map[string]*facetAcc{}
	otherAcc := map[string]*facetAcc{}
	selfCv := map[int64]struct{}{}
	otherCv := map[[2]int64]struct{}{}
	for rows.Next() {
		var side string
		var stype, pos, isCv, k1, k2 int64
		if err := rows.Scan(&side, &stype, &pos, &isCv, &k1, &k2); err != nil {
			fail(c, 500, err.Error())
			return
		}
		if isCv == 1 {
			if side == "self_cv" {
				selfCv[k1] = struct{}{} // 条目
			} else {
				otherCv[[2]int64{k2, k1}] = struct{}{} // (人物, 条目)
			}
			continue
		}
		m := selfAcc
		kk1, kk2 := int64(0), k1
		key := strconv.FormatInt(stype, 10) + ":" + strconv.FormatInt(pos, 10)
		label := h.facetLabel(stype, pos, false)
		if side != "self_s" {
			m = otherAcc
			kk1, kk2 = k2, k1 // (人物, 条目)
		}
		a, ok := m[label]
		if !ok {
			a = &facetAcc{seen: map[[2]int64]struct{}{}}
			m[label] = a
		}
		a.add(key, kk1, kk2)
	}
	if err := rows.Err(); err != nil {
		fail(c, 500, err.Error())
		return
	}

	build := func(m map[string]*facetAcc) []collabFacet {
		list := make([]collabFacet, 0, len(m))
		for label, a := range m {
			sort.Strings(a.keys)
			list = append(list, collabFacet{
				Key:   strings.Join(a.keys, ","),
				Label: label,
				Count: int64(len(a.seen)),
			})
		}
		return list
	}
	selfList := build(selfAcc)
	if n := len(selfCv); n > 0 {
		selfList = append(selfList, collabFacet{Key: "cv", Label: "CV", CV: true, Count: int64(n)})
	}
	otherList := build(otherAcc)
	if n := len(otherCv); n > 0 {
		otherList = append(otherList, collabFacet{Key: "cv", Label: "CV", CV: true, Count: int64(n)})
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
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	NameCN   string      `json:"name_cn"`
	Type     int         `json:"type"`
	TypeName string      `json:"type_name"`
	Date     string      `json:"date"`
	RolesA   []roleLabel `json:"roles_a"`
	RolesB   []roleLabel `json:"roles_b"`
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
			sid, pid           int64
			stype              int64
			position, isCv     int64
			name, nameCN, date string
			charName           string
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
				Date:   date,
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
