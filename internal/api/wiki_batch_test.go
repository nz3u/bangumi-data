package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWikiBatchDumpColumnsAndSelectiveComponents(t *testing.T) {
	h := newEpisodesTestHandler(t)
	mustExec(t, h.getDB(), `INSERT INTO persons(id,name,type) VALUES(8,'声优',1)`)
	mustExec(t, h.getDB(), `INSERT INTO persons(id,name,type) VALUES(10,'相关人物',1)`)
	mustExec(t, h.getDB(), `INSERT INTO characters(id,name,role) VALUES(9,'角色',1)`)
	mustExec(t, h.getDB(), `INSERT INTO subject_persons(subject_id,person_id,position) VALUES(1,8,1)`)
	mustExec(t, h.getDB(), `INSERT INTO subject_characters(subject_id,character_id,type) VALUES(1,9,1)`)
	mustExec(t, h.getDB(), `INSERT INTO person_characters(person_id,subject_id,character_id,type) VALUES(8,1,9,1)`)
	mustExec(t, h.getDB(), `INSERT INTO person_relations(person_type,person_id,related_person_id,relation_type) VALUES('prsn',8,10,1)`)
	r := gin.New()
	r.POST("/api/wiki/batch", h.wikiBatch)
	body := []byte(`{"subject_ids":[1],"include":["subjects","episodes","subject_persons","subject_characters","person_characters","persons","characters","person_relations"]}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/wiki/batch", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Version int                         `json:"version"`
			Tables  map[string][]map[string]any `json:"tables"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Data.Version != 1 {
		t.Fatalf("unexpected envelope: %s", w.Body.String())
	}
	if len(result.Data.Tables["subjects"]) != 1 || len(result.Data.Tables["persons"]) != 2 || len(result.Data.Tables["characters"]) != 1 || len(result.Data.Tables["person_relations"]) != 1 {
		t.Fatalf("missing related rows: %s", w.Body.String())
	}
	if _, ok := result.Data.Tables["subjects"][0]["search_norm"]; !ok {
		t.Fatal("dump search_norm column missing")
	}
	if len(result.Data.Tables["episodes"]) != 3 {
		t.Fatalf("episodes: %d", len(result.Data.Tables["episodes"]))
	}
	if len(result.Data.Tables["subject_relations"]) != 0 {
		t.Fatal("unrequested relation table should be empty")
	}
}
