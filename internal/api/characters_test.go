package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/db"
	"bangumi-subject-go/internal/norm"
)

// newPersonSearchTestHandler 构建带角色/人物数据的检索接口。
// 数据形态对齐真实库：角色的 infobox 别名带「罗马字/英文名/昵称」子类前缀，
// 检索串按导入时的口径（norm.Join(name, name_cn, aliases)）预先算好。
func newPersonSearchTestHandler(t *testing.T) *handler {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.InitSchema(conn); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertCharacter := func(id int64, name, nameCN, aliases string) {
		t.Helper()
		if _, err := conn.Exec(`INSERT INTO characters (id, role, name, name_cn, aliases, search_norm, infobox)
			VALUES (?, 1, ?, ?, ?, ?, '')`,
			id, name, nameCN, aliases, norm.Join(name, nameCN, aliases)); err != nil {
			t.Fatalf("insert character %d: %v", id, err)
		}
	}
	// 平泽唯：罗马字 Hirasawa Yui（别名，真实 dump 形态）
	insertCharacter(466, "平沢唯", "平泽唯", "Yui Hirasawa 平沢 唯 ひらさわゆい Hirasawa Yui 呆唯")
	// 仅昵称命中「平泽唯」，用于验证精确命中的排前
	insertCharacter(900, "唯", "", "平泽唯 平沢唯")
	insertCharacter(2, "枢木スザク", "枢木朱雀", "Knight of Seven 枢木 スザク くるるぎ スザク Kururugi Suzaku")

	insertPerson := func(id int64, name, nameCN, aliases string) {
		t.Helper()
		if _, err := conn.Exec(`INSERT INTO persons (id, name, name_cn, aliases, search_norm, type, infobox)
			VALUES (?, ?, ?, ?, ?, 1, '')`,
			id, name, nameCN, aliases, norm.Join(name, nameCN, aliases)); err != nil {
			t.Fatalf("insert person %d: %v", id, err)
		}
	}
	insertPerson(1, "水樹奈々", "水树奈奈", "近藤奈々 (こんどう なな) みずき なな Mizuki Nana")

	if err := db.FinalizeSchema(conn); err != nil {
		t.Fatalf("FinalizeSchema: %v", err)
	}

	cons, err := common.Load("")
	if err != nil {
		t.Fatalf("common.Load: %v", err)
	}
	return &handler{db: conn, cons: cons}
}

// doSearchIDs 调用搜索接口并返回总数与有序 id 列表。
func doSearchIDs(t *testing.T, h *handler, path, query string) (int64, []int64) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if path == "/api/characters/search" {
		r.GET(path, h.searchCharacters)
	} else {
		r.GET(path, h.searchPersons)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?"+query, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				ID int64 `json:"id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	ids := make([]int64, 0, len(body.Data.Items))
	for _, it := range body.Data.Items {
		ids = append(ids, it.ID)
	}
	return body.Data.Total, ids
}

// TestSearchCharactersByRomaji 角色按 infobox 别名的罗马字检索：
// Hirasawa Yui（以及名在前的 Yui Hirasawa）都应命中「平泽唯」。
func TestSearchCharactersByRomaji(t *testing.T) {
	h := newPersonSearchTestHandler(t)

	for _, q := range []string{
		"Hirasawa Yui",  // 罗马字原样（查询侧归一化后丢空格）
		"hirasawa yui",  // 小写
		"hirasawayui",   // 无空格
		"Yui Hirasawa",  // 英文名（名在前语序）
		"HIRASAWA  YUI", // 多余空格 + 大写
	} {
		total, ids := doSearchIDs(t, h, "/api/characters/search", "q="+url.QueryEscape(q))
		if total != 1 || len(ids) != 1 || ids[0] != 466 {
			t.Errorf("q=%q: got total=%d ids=%v, want 1 个 466", q, total, ids)
		}
	}

	// 罗马字片段（≥3 字符走 trigram 索引）
	if total, ids := doSearchIDs(t, h, "/api/characters/search", "q=hirasawa"); total != 1 || ids[0] != 466 {
		t.Errorf("q=hirasawa: got total=%d ids=%v, want 1 个 466", total, ids)
	}

	// 角色类型过滤仍然生效：平泽唯是角色（role=1），按机体（2）查应为空
	if total, ids := doSearchIDs(t, h, "/api/characters/search", "q=hirasawa+yui&role=2"); total != 0 || len(ids) != 0 {
		t.Errorf("q=Hirasawa Yui role=2: got total=%d ids=%v, want 空", total, ids)
	}
}

// TestSearchCharactersRanksExactNameFirst 搜「平泽唯」时，中文名精确匹配的角色排在
// 仅靠别名/昵称命中的角色之前（分级排序）。
func TestSearchCharactersRanksExactNameFirst(t *testing.T) {
	h := newPersonSearchTestHandler(t)

	total, ids := doSearchIDs(t, h, "/api/characters/search", "q="+url.QueryEscape("平泽唯"))
	if total != 2 || len(ids) != 2 {
		t.Fatalf("q=平泽唯: got total=%d ids=%v, want 2 条", total, ids)
	}
	if ids[0] != 466 {
		t.Errorf("q=平泽唯: 首条 = %d, want 466（中文名精确匹配优先）", ids[0])
	}

	// 中文名/日文名子串命中
	if total, ids := doSearchIDs(t, h, "/api/characters/search", "q="+url.QueryEscape("枢木朱雀")); total != 1 || ids[0] != 2 {
		t.Errorf("q=枢木朱雀: got total=%d ids=%v, want 1 个 2", total, ids)
	}
	// 「平沢」同时命中 466 的日文原名与 900 的别名（平沢唯），原名命中排在前面
	if total, ids := doSearchIDs(t, h, "/api/characters/search", "q="+url.QueryEscape("平沢")); total != 2 || ids[0] != 466 {
		t.Errorf("q=平沢: got total=%d ids=%v, want 2 条且首条 466", total, ids)
	}
}

// TestSearchPersonsByRomaji 人物按罗马字检索：Mizuki Nana -> 水树奈奈。
func TestSearchPersonsByRomaji(t *testing.T) {
	h := newPersonSearchTestHandler(t)

	for _, q := range []string{"Mizuki Nana", "mizukinana", "Mizuki"} {
		total, ids := doSearchIDs(t, h, "/api/persons/search", "q="+url.QueryEscape(q))
		if total != 1 || len(ids) != 1 || ids[0] != 1 {
			t.Errorf("q=%q: got total=%d ids=%v, want 1 个 1", q, total, ids)
		}
	}
	if total, ids := doSearchIDs(t, h, "/api/persons/search", "q="+url.QueryEscape("水树奈奈")); total != 1 || ids[0] != 1 {
		t.Errorf("q=水树奈奈: got total=%d ids=%v, want 1 个 1", total, ids)
	}
	// 纯符号查询不参与索引，返回空集而非全表
	if total, ids := doSearchIDs(t, h, "/api/persons/search", "q="+url.QueryEscape("！！")); total != 0 || len(ids) != 0 {
		t.Errorf("q=！！: got total=%d ids=%v, want 空", total, ids)
	}
}
