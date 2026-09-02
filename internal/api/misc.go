package api

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// health 健康检查；附带编译期注入的版本号，供前端页头展示（前后端版本同源）。
func (h *handler) health(c *gin.Context) {
	if err := h.db.Ping(); err != nil {
		fail(c, 500, "数据库不可用: "+err.Error())
		return
	}
	respOK(c, gin.H{"status": "ok", "version": h.version})
}

// dbInfo 数据库版本与上游最新导出的对比状态。
// database 为空表示本地无版本记录（旧版本程序创建的库，前端显示「旧版本」）；
// latest 为空表示尚未成功获取上游元信息（离线等），此时不提示可更新；
// update_available=true 时前端展示更新提醒。
func (h *handler) dbInfo(c *gin.Context) {
	if h.dbver == nil {
		respOK(c, gin.H{"database": nil, "latest": nil, "update_available": false})
		return
	}
	respOK(c, h.dbver.Status())
}

// stats 各表行数统计（用于确认导入状态）。
func (h *handler) stats(c *gin.Context) {
	tables := []string{
		"subjects", "persons", "characters", "episodes",
		"subject_relations", "subject_persons", "subject_characters",
		"person_characters", "person_relations",
	}
	res := gin.H{}
	for _, t := range tables {
		var n int64
		if err := h.db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err != nil {
			fail(c, 500, err.Error())
			return
		}
		res[t] = n
	}
	respOK(c, res)
}

// constants 返回全部 id 常量映射，前端据此渲染分类名称。
func (h *handler) constants(c *gin.Context) {
	cons := h.cons
	respOK(c, gin.H{
		"subject_types":           cons.SubjectTypes,
		"person_types":            cons.PersonTypes,
		"character_roles":         cons.CharacterRoles,
		"episode_types":           cons.EpisodeTypes,
		"subject_character_types": cons.SubjectCharTypes,
		"platforms":               cons.Platforms,
		"subject_relations":       cons.SubjectRelations,
		"staffs":                  cons.Staffs,
		"person_relations":        cons.PersonRelations,
	})
}

// ftsMinRunes trigram 分词器按 3 个字符建索引，查询词达到该长度才能命中全文索引。
const ftsMinRunes = 3

// useFTS 关键词足够长时走 FTS 索引；更短的关键词由调用方回退 LIKE 子串匹配。
func useFTS(q string) bool {
	return utf8.RuneCountInString(q) >= ftsMinRunes
}

// ftsPhrase 构造 FTS5 trigram 短语查询。
func ftsPhrase(q string) string {
	q = strings.ReplaceAll(q, `"`, `""`)
	return `"` + q + `"`
}

// parseIntQuery 解析整数查询参数，为空返回 0,false。
func parseIntQuery(c *gin.Context, key string) (int, bool) {
	s := c.Query(key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func intParam(c *gin.Context, key string) (int64, bool) {
	v, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ftsNormRowIDs 条目检索：在 subjects_fts.search_norm 上做子串匹配，返回命中的 rowid。
// 传入的 qn 必须是 norm.Fold 的结果——标点、符号与空白都已被删除，
// 因此可以直接拼进 LIKE 模式，不存在通配符注入。
//
// 这里刻意用 LIKE 而非 MATCH：FTS5 + trigram 会把 LIKE '%x%' 自动改写为基于
// 3-gram 的索引查找，与 MATCH 等价；且 >=3 字符走索引、<3 字符自动退化为顺序扫描，
// 调用方无需再按关键词长度分支。
//
// 注意：绝不能加 ESCAPE —— 带 ESCAPE 时 SQLite 不再做上述改写，
// 查询计划从 "INDEX 0:L3" 退化为全表顺序扫描（实测 0.6ms -> 248ms）。
func (h *handler) ftsNormRowIDs(qn string) ([]int64, error) {
	rows, err := h.db.Query(`SELECT rowid FROM subjects_fts WHERE search_norm LIKE ?`, "%"+qn+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ftsRowIDs 通用 FTS 搜索：返回匹配 rowid 的集合。
// table: persons_fts / characters_fts（条目检索请走 ftsNormRowIDs）
func (h *handler) ftsRowIDs(table, q string) ([]int64, error) {
	sql := fmt.Sprintf(`SELECT rowid FROM %s WHERE %s MATCH ?`, table, table)
	rows, err := h.db.Query(sql, ftsPhrase(q))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
