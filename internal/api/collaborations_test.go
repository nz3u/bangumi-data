package api

import (
	"database/sql"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCollaborationNegativeOtherRoleKeepsOtherRoles(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE subjects (id INTEGER PRIMARY KEY, type INTEGER NOT NULL)`,
		`CREATE TABLE subject_persons (subject_id INTEGER NOT NULL, person_id INTEGER NOT NULL, position INTEGER NOT NULL)`,
		`CREATE TABLE person_characters (subject_id INTEGER NOT NULL, person_id INTEGER NOT NULL)`,
		`INSERT INTO subjects VALUES (101, 2), (102, 2), (103, 2)`,
		// 1 是当前人物；2 在 101 同时有应排除职位 10 和保留职位 20。
		`INSERT INTO subject_persons VALUES
			(101, 1, 1), (102, 1, 1), (103, 1, 1),
			(101, 2, 10), (101, 2, 20), (102, 2, 10), (103, 2, 20)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	appSQL, appArgs := buildCollabAppCTE(1, collabRoleFilter{staff: [][2]int{{2, 1}}})
	pairsSQL, pairsArgs := buildCollabPairsCTE(1, collabRoleFilter{negStaff: [][2]int{{2, 10}}})
	args := append(appArgs, pairsArgs...)
	rows, err := db.Query(`WITH `+appSQL+`, `+pairsSQL+` SELECT other, sid FROM pairs ORDER BY sid`, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []int64
	for rows.Next() {
		var other, subjectID int64
		if err := rows.Scan(&other, &subjectID); err != nil {
			t.Fatal(err)
		}
		if other == 2 {
			got = append(got, subjectID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []int64{101, 103}; !reflect.DeepEqual(got, want) {
		t.Errorf("negative role result = %v, want %v", got, want)
	}
}

func TestParseCollabRolesMultipleNegativeKeys(t *testing.T) {
	got := parseCollabRoles("-2:10,-4:20")
	want := [][2]int{{2, 10}, {4, 20}}
	if !reflect.DeepEqual(got.negStaff, want) || len(got.staff) != 0 {
		t.Errorf("parseCollabRoles() = %#v, want negative keys %#v only", got, want)
	}
}
