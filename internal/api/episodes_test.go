package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/norm"
)

func mustExec(t *testing.T, conn *sql.DB, q string) {
	t.Helper()
	if _, err := conn.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// newEpisodesTestHandler 构建带测试数据的章节搜索接口。
// 数据形态对齐真实库：s1 动画（原名带符号、含正篇与 OP 章节）、
// s2 音乐专辑（碟轨型章节，disc 非零）、s4 供「仅条目标题命中」分级验证。
func newEpisodesTestHandler(t *testing.T) *handler {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.InitSchema(conn); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	mustExec(t, conn, `INSERT INTO subjects (id, type, name, name_cn, search_norm, favorite, tags, meta_tags) VALUES
		(1, 2, '少女☆歌剧 Revue Starlight', '少女歌剧', '少女歌剧revuestarlight', '{"done":100}', '[]', '[]'),
		(2, 3, 'Test Album', '测试专辑', 'testalbum测试专辑', '{"done":5}', '[]', '[]'),
		(3, 4, 'Some Game', '', 'somegame', '{}', '[]', '[]'),
		(4, 2, '舞台少女谭', '', '舞台少女谭', '{"done":50}', '[]', '[]')`)

	insertEpisode := func(id int64, name, nameCN, airdate string, disc, epType int, sort float64, subjectID int64) {
		t.Helper()
		_, err := conn.Exec(`INSERT INTO episodes (id, name, name_cn, description, airdate, disc, duration, subject_id, sort, type, search_norm)
			VALUES (?, ?, ?, '', ?, ?, '', ?, ?, ?, ?)`,
			id, name, nameCN, airdate, disc, subjectID, sort, epType, norm.Join(name, nameCN))
		if err != nil {
			t.Fatalf("insert episode %d: %v", id, err)
		}
	}
	insertEpisode(101, "第1話 舞台少女", "第1话 舞台少女", "2021-01-01", 0, 0, 1, 1)
	insertEpisode(102, "第2話 舞台少女", "", "", 0, 0, 2, 1)
	insertEpisode(103, "Track 1", "序曲", "2020-05-01", 1, 0, 1, 2)
	insertEpisode(104, "オープニング", "OP", "", 0, 2, 0, 3)
	insertEpisode(105, "第1話", "开幕", "", 0, 0, 1, 4)
	insertEpisode(106, "舞台少女 OP", "", "", 0, 2, 0, 1)

	if err := db.FinalizeSchema(conn); err != nil {
		t.Fatalf("FinalizeSchema: %v", err)
	}

	cons, err := common.Load("")
	if err != nil {
		t.Fatalf("common.Load: %v", err)
	}
	return &handler{db: conn, cons: cons}
}

// doGetEpisodes 以给定查询串调用章节搜索并解析响应。
func doGetEpisodes(t *testing.T, h *handler, query string) (total int64, ids []int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/episodes/search", h.searchEpisodes)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/episodes/search?"+query, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				ID      int64 `json:"id"`
				Type    int   `json:"type"`
				Subject struct {
					ID   int64  `json:"id"`
					Type int    `json:"type"`
					Name string `json:"name"`
				} `json:"subject"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || !body.OK {
		t.Fatalf("解析响应失败: %v / %s", err, w.Body.String())
	}
	for _, it := range body.Data.Items {
		ids = append(ids, it.ID)
	}
	return body.Data.Total, ids
}

func wantIDs(t *testing.T, got []int64, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("命中 %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("命中 %v, want %v", got, want)
		}
	}
}

func TestSearchEpisodesFTS(t *testing.T) {
	h := newEpisodesTestHandler(t)

	// 章节标题命中（101/102/106 章节标题子串，tier 1）+ 105 经所属条目「舞台少女谭」命中
	// （tier 3）；同级内按条目/类型/集数排序，分级优先于人气，故 105 靠后
	_, ids := doGetEpisodes(t, h, "q=舞台少女")
	wantIDs(t, ids, 101, 102, 106, 105)

	// 归一化命中：查询「1話舞台」去符号后是 101 原名「第1話 舞台少女」的子串
	_, ids = doGetEpisodes(t, h, "q=1話舞台")
	wantIDs(t, ids, 101)

	// 所属条目标题命中：原名带符号「少女☆歌剧」，查询词无符号仍可命中其全部章节
	_, ids = doGetEpisodes(t, h, "q=少女歌剧")
	wantIDs(t, ids, 101, 102, 106)

	// 章节类型筛选（OP）
	_, ids = doGetEpisodes(t, h, "ep_type=2")
	wantIDs(t, ids, 104, 106)

	// 作品类型筛选（动画 = 条目 type 2，排除音乐碟轨与游戏章节）
	_, ids = doGetEpisodes(t, h, "type=2")
	wantIDs(t, ids, 101, 102, 105, 106)

	// 条目 ID 精确筛选
	_, ids = doGetEpisodes(t, h, "subject_id=2")
	wantIDs(t, ids, 103)

	// 光盘筛选
	_, ids = doGetEpisodes(t, h, "disc=1")
	wantIDs(t, ids, 103)

	// 播出时间范围
	_, ids = doGetEpisodes(t, h, "airdate_from=2020-12-01&airdate_to=2021-12-31")
	wantIDs(t, ids, 101)

	// 纯符号查询词：不参与索引，返回空集
	total, ids := doGetEpisodes(t, h, "q=！！！")
	if total != 0 || len(ids) != 0 {
		t.Fatalf("纯符号查询应返回空集，got total=%d ids=%v", total, ids)
	}
}

func TestSearchEpisodesTierOrder(t *testing.T) {
	h := newEpisodesTestHandler(t)

	// 章节标题命中（101/102/106，tier 1）应排在仅条目标题命中的 105（tier 3）之前：
	// 105 所属条目「舞台少女谭」人气(50)虽低于 s1(100)，但分级优先于人气；
	// 同级内按条目人气 -> 条目 -> 类型 -> 集数排序。
	_, ids := doGetEpisodes(t, h, "q=舞台少女")
	wantIDs(t, ids, 101, 102, 106, 105)
}

func TestGetSubjectEpisodesSortByType(t *testing.T) {
	h := newEpisodesTestHandler(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/subjects/:id/episodes", h.getSubjectEpisodes)

	fetch := func(query string) []int64 {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/subjects/1/episodes"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		var body struct {
			Data struct {
				Items []struct {
					ID   int64 `json:"id"`
					Type int   `json:"type"`
				} `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		var ids []int64
		for _, it := range body.Data.Items {
			ids = append(ids, it.ID)
		}
		return ids
	}

	// 默认按集数（sort）排序；sort=type 时正篇在前、OP 等特殊章节跟随
	wantIDs(t, fetch(""), 106, 101, 102)
	wantIDs(t, fetch("?sort=type"), 101, 102, 106)
}
