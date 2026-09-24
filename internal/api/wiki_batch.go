package api

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// wikiBatchRequest uses dump table names so clients can import rows without
// translating the presentation-oriented /api/subjects and /api/persons APIs.
type wikiBatchRequest struct {
	SubjectIDs   []int64  `json:"subject_ids"`
	PersonIDs    []int64  `json:"person_ids"`
	CharacterIDs []int64  `json:"character_ids"`
	Include      []string `json:"include"`
}

type wikiBatchData struct {
	Version int                         `json:"version"`
	Tables  map[string][]map[string]any `json:"tables"`
}

var wikiTables = []string{
	"subjects", "episodes", "subject_relations", "persons", "characters",
	"subject_persons", "subject_characters", "person_characters", "person_relations",
}

// wikiBatch returns original wiki dump columns. Each requested component can
// also be fetched on its own, with explicit person and character IDs.
//
// @Summary      批量获取 wiki dump 原始列
// @Description  按 ID 返回指定表的原列数据；关联双向查找，实体资料只展开一层。
// @Tags         维基数据
// @Accept       json
// @Produce      json
// @Param        request body wikiBatchRequest true "ID 与 include 表名"
// @Success      200 {object} apiEnvelope{data=wikiBatchData}
// @Failure      400 {object} apiEnvelope
// @Failure      500 {object} apiEnvelope
// @Router       /api/wiki/batch [post]
func (h *handler) wikiBatch(c *gin.Context) {
	var req wikiBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求体不合法: "+err.Error())
		return
	}
	if len(req.SubjectIDs)+len(req.PersonIDs)+len(req.CharacterIDs) == 0 ||
		len(req.SubjectIDs) > 100 || len(req.PersonIDs) > 100 || len(req.CharacterIDs) > 100 {
		fail(c, 400, "每类 ID 须提供 1 到 100 个（至少提供一类）")
		return
	}
	for _, ids := range [][]int64{req.SubjectIDs, req.PersonIDs, req.CharacterIDs} {
		for _, id := range ids {
			if id <= 0 {
				fail(c, 400, "ID 必须为正整数")
				return
			}
		}
	}
	req.SubjectIDs, req.PersonIDs, req.CharacterIDs = uniqueIDs(req.SubjectIDs), uniqueIDs(req.PersonIDs), uniqueIDs(req.CharacterIDs)
	include := make(map[string]bool)
	if len(req.Include) == 0 {
		for _, name := range wikiTables {
			include[name] = true
		}
	} else {
		for _, name := range req.Include {
			valid := false
			for _, table := range wikiTables {
				if table == name {
					valid = true
					break
				}
			}
			if !valid {
				fail(c, 400, "未知的 include 表: "+name)
				return
			}
			include[name] = true
		}
	}

	db := h.getDB()
	tables := make(map[string][]map[string]any, len(wikiTables))
	for _, name := range wikiTables {
		tables[name] = []map[string]any{}
	}
	read := func(name, column string, ids []int64) error {
		if !include[name] || len(ids) == 0 {
			return nil
		}
		rows, err := wikiSelect(db, name, column, ids)
		if err == nil {
			tables[name] = rows
		}
		return err
	}
	for _, table := range []string{"subjects", "episodes", "subject_persons", "subject_characters", "person_characters"} {
		column := "subject_id"
		if table == "subjects" {
			column = "id"
		}
		if err := read(table, column, req.SubjectIDs); err != nil {
			fail(c, 500, err.Error())
			return
		}
	}
	if include["subject_relations"] && len(req.SubjectIDs) > 0 {
		rows, err := wikiSelectEither(db, "subject_relations", "subject_id", "related_subject_id", req.SubjectIDs)
		if err != nil {
			fail(c, 500, err.Error())
			return
		}
		tables["subject_relations"] = rows
		if include["subjects"] {
			related := make([]int64, 0, 2*len(rows))
			for _, row := range rows {
				related = append(related, row["subject_id"].(int64), row["related_subject_id"].(int64))
			}
			more, err := wikiSelect(db, "subjects", "id", uniqueIDs(related))
			if err != nil {
				fail(c, 500, err.Error())
				return
			}
			tables["subjects"] = mergeWikiEntities(tables["subjects"], more)
		}
	}
	personIDs := append([]int64{}, req.PersonIDs...)
	characterIDs := append([]int64{}, req.CharacterIDs...)
	for _, row := range tables["subject_persons"] {
		personIDs = append(personIDs, row["person_id"].(int64))
	}
	for _, row := range tables["subject_characters"] {
		characterIDs = append(characterIDs, row["character_id"].(int64))
	}
	for _, row := range tables["person_characters"] {
		personIDs = append(personIDs, row["person_id"].(int64))
		characterIDs = append(characterIDs, row["character_id"].(int64))
	}
	personIDs, characterIDs = uniqueIDs(personIDs), uniqueIDs(characterIDs)
	for _, spec := range []struct {
		name, column string
		ids          []int64
	}{
		{"persons", "id", personIDs}, {"characters", "id", characterIDs},
	} {
		if err := read(spec.name, spec.column, spec.ids); err != nil {
			fail(c, 500, err.Error())
			return
		}
	}
	if include["person_relations"] {
		for _, spec := range []struct {
			kind string
			ids  []int64
		}{{"prsn", personIDs}, {"crt", characterIDs}} {
			if len(spec.ids) == 0 {
				continue
			}
			rows, err := wikiSelectPersonRelations(db, spec.kind, spec.ids)
			if err != nil {
				fail(c, 500, err.Error())
				return
			}
			tables["person_relations"] = append(tables["person_relations"], rows...)
		}
		personRelated, characterRelated := []int64{}, []int64{}
		for _, row := range tables["person_relations"] {
			if row["person_type"] == "prsn" {
				personRelated = append(personRelated, row["person_id"].(int64), row["related_person_id"].(int64))
			}
			if row["person_type"] == "crt" {
				characterRelated = append(characterRelated, row["person_id"].(int64), row["related_person_id"].(int64))
			}
		}
		for _, spec := range []struct {
			name string
			ids  []int64
		}{{"persons", personRelated}, {"characters", characterRelated}} {
			if !include[spec.name] || len(spec.ids) == 0 {
				continue
			}
			more, err := wikiSelect(db, spec.name, "id", uniqueIDs(spec.ids))
			if err != nil {
				fail(c, 500, err.Error())
				return
			}
			tables[spec.name] = mergeWikiEntities(tables[spec.name], more)
		}
	}
	respOK(c, wikiBatchData{Version: 1, Tables: tables})
}

func mergeWikiEntities(rows, more []map[string]any) []map[string]any {
	seen := make(map[int64]bool, len(rows))
	for _, row := range rows {
		seen[row["id"].(int64)] = true
	}
	for _, row := range more {
		id := row["id"].(int64)
		if !seen[id] {
			seen[id] = true
			rows = append(rows, row)
		}
	}
	return rows
}

func uniqueIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// Table and column arguments are fixed strings from the call sites above.
func wikiSelect(db *sql.DB, table, column string, ids []int64) ([]map[string]any, error) {
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	if len(ids) > 400 {
		out := make([]map[string]any, 0)
		for start := 0; start < len(ids); start += 400 {
			end := start + 400
			if end > len(ids) {
				end = len(ids)
			}
			part, err := wikiSelect(db, table, column, ids[start:end])
			if err != nil {
				return nil, err
			}
			out = append(out, part...)
		}
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return wikiQuery(db, "SELECT * FROM "+table+" WHERE "+column+" IN ("+marks+")", args...)
}

func wikiSelectEither(db *sql.DB, table, a, b string, ids []int64) ([]map[string]any, error) {
	if len(ids) > 400 {
		out := make([]map[string]any, 0)
		for start := 0; start < len(ids); start += 400 {
			end := start + 400
			if end > len(ids) {
				end = len(ids)
			}
			part, err := wikiSelectEither(db, table, a, b, ids[start:end])
			if err != nil {
				return nil, err
			}
			out = append(out, part...)
		}
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, 2*len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}
	return wikiQuery(db, "SELECT * FROM "+table+" WHERE "+a+" IN ("+marks+") OR "+b+" IN ("+marks+")", args...)
}

func wikiSelectPersonRelations(db *sql.DB, kind string, ids []int64) ([]map[string]any, error) {
	if len(ids) > 400 {
		out := make([]map[string]any, 0)
		for start := 0; start < len(ids); start += 400 {
			end := start + 400
			if end > len(ids) {
				end = len(ids)
			}
			part, err := wikiSelectPersonRelations(db, kind, ids[start:end])
			if err != nil {
				return nil, err
			}
			out = append(out, part...)
		}
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, 2*len(ids)+2)
	args = append(args, kind)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, kind)
	for _, id := range ids {
		args = append(args, id)
	}
	return wikiQuery(db, "SELECT * FROM person_relations WHERE (person_type=? AND person_id IN ("+marks+")) OR (person_type=? AND related_person_id IN ("+marks+"))", args...)
}

func wikiQuery(db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		item := make(map[string]any, len(columns))
		for i, col := range columns {
			switch v := values[i].(type) {
			case []byte:
				item[col] = string(v)
			default:
				item[col] = v
			}
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 wiki 数据: %w", err)
	}
	return out, nil
}
