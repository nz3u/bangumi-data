package api

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/model"
)

// pagination 解析 page/size 参数。
func pagination(c *gin.Context) (page, size int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ = strconv.Atoi(c.DefaultQuery("size", "30"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 30
	}
	if size > 200 {
		size = 200
	}
	return page, size
}

// listResp 分页响应结构。
type listResp struct {
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Items any   `json:"items"`
}

func respOK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"ok": true, "data": data})
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"ok": false, "error": msg})
}

// ---- JSON 字段还原 ----

func parseTags(s string) []model.Tag {
	var tags []model.Tag
	if s != "" {
		_ = json.Unmarshal([]byte(s), &tags)
	}
	return tags
}

func parseFavorite(s string) model.Favorite {
	var f model.Favorite
	if s != "" {
		_ = json.Unmarshal([]byte(s), &f)
	}
	return f
}

func parseScoreDetails(s string) map[string]int {
	var m map[string]int
	if s != "" {
		_ = json.Unmarshal([]byte(s), &m)
	}
	return m
}

func parseStrings(s string) []string {
	var arr []string
	if s != "" {
		_ = json.Unmarshal([]byte(s), &arr)
	}
	return arr
}

// subjectBrief 列表页使用的精简条目信息。
type subjectBrief struct {
	ID        int64          `json:"id"`
	Type      int            `json:"type"`
	TypeName  string         `json:"type_name"`
	Name      string         `json:"name"`
	NameCN    string         `json:"name_cn"`
	Platform  int            `json:"platform"`
	PlatformN string         `json:"platform_name"`
	Date      string         `json:"date"`
	NSFW      bool           `json:"nsfw"`
	Score     float64        `json:"score"`
	Rank      int            `json:"rank"`
	Tags      []model.Tag    `json:"tags,omitempty"`
	Favorite  model.Favorite `json:"favorite"`
}

// scanSubjectBrief 读取 subjects 行到 subjectBrief。
// total 非 nil 时，行末需附带 COUNT(*) OVER() 的总数字段（计数与取数合并为一次扫描）。
func (h *handler) scanSubjectBrief(row interface{ Scan(...any) error }, total *int64) (*subjectBrief, error) {
	var (
		id, typ, platform, rank int64
		name, nameCN, date      string
		tags, fav               string
		nsfw                    int64
		score                   float64
	)
	dest := []any{&id, &typ, &name, &nameCN, &platform, &date, &nsfw, &score, &rank, &tags, &fav}
	if total != nil {
		dest = append(dest, total)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &subjectBrief{
		ID: id, Type: int(typ), TypeName: h.cons.SubjectTypeCN(int(typ)),
		Name: name, NameCN: nameCN,
		Platform: int(platform), PlatformN: h.cons.PlatformCN(int(typ), int(platform)),
		Date: date, NSFW: nsfw == 1, Score: score, Rank: int(rank),
		Tags: parseTags(tags), Favorite: parseFavorite(fav),
	}, nil
}

const subjectBriefCols = `s.id, s.type, s.name, s.name_cn, s.platform, s.date, s.nsfw, s.score, s.rank, s.tags, s.favorite`
