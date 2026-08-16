// Package model 定义与导出数据(jsonlines)对应的数据结构，
// 字段名与 dump 中的 JSON key 保持一致。
package model

// Subject 条目（subject.jsonlines）
type Subject struct {
	ID           int64          `json:"id"`
	Type         int            `json:"type"`
	Name         string         `json:"name"`
	NameCN       string         `json:"name_cn"`
	Infobox      string         `json:"infobox"`
	Platform     int            `json:"platform"`
	Summary      string         `json:"summary"`
	NSFW         bool           `json:"nsfw"`
	Date         string         `json:"date"`
	Favorite     Favorite       `json:"favorite"`
	Series       bool           `json:"series"`
	Tags         []Tag          `json:"tags,omitempty"`
	Score        float64        `json:"score,omitempty"`
	ScoreDetails map[string]int `json:"score_details,omitempty"`
	Rank         int            `json:"rank,omitempty"`
	MetaTags     []string       `json:"meta_tags,omitempty"`
}

// Favorite 收藏状态（想看/看过/在看/搁置/抛弃）
type Favorite struct {
	Wish    int `json:"wish"`
	Done    int `json:"done"`
	Doing   int `json:"doing"`
	OnHold  int `json:"on_hold"`
	Dropped int `json:"dropped"`
}

// Tag 条目标签
type Tag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Person 人物（person.jsonlines）
type Person struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Type     int      `json:"type"`
	Career   []string `json:"career"`
	Infobox  string   `json:"infobox"`
	Summary  string   `json:"summary"`
	Comments int      `json:"comments"`
	Collects int      `json:"collects"`
}

// Character 角色（character.jsonlines）
type Character struct {
	ID       int64  `json:"id"`
	Role     int    `json:"role"`
	Name     string `json:"name"`
	Infobox  string `json:"infobox"`
	Summary  string `json:"summary"`
	Comments int    `json:"comments"`
	Collects int    `json:"collects"`
}

// Episode 章节（episode.jsonlines）
type Episode struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	NameCN      string  `json:"name_cn"`
	Description string  `json:"description"`
	Airdate     string  `json:"airdate"`
	Disc        int     `json:"disc"`
	Duration    string  `json:"duration"`
	SubjectID   int64   `json:"subject_id"`
	Sort        float64 `json:"sort"`
	Type        int     `json:"type"`
}

// SubjectRelation 条目关联（subject-relations.jsonlines）
type SubjectRelation struct {
	SubjectID        int64 `json:"subject_id"`
	RelationType     int   `json:"relation_type"`
	RelatedSubjectID int64 `json:"related_subject_id"`
	Order            int   `json:"order"`
}

// SubjectPerson 条目-人物（subject-persons.jsonlines）
type SubjectPerson struct {
	PersonID  int64  `json:"person_id"`
	SubjectID int64  `json:"subject_id"`
	Position  int    `json:"position"`
	AppearEps string `json:"appear_eps"`
}

// SubjectCharacter 条目-角色（subject-characters.jsonlines）
type SubjectCharacter struct {
	CharacterID int64 `json:"character_id"`
	SubjectID   int64 `json:"subject_id"`
	Type        int   `json:"type"`
	Order       int   `json:"order"`
}

// PersonCharacter 人物-角色（person-characters.jsonlines）
type PersonCharacter struct {
	PersonID    int64  `json:"person_id"`
	SubjectID   int64  `json:"subject_id"`
	CharacterID int64  `json:"character_id"`
	Type        int    `json:"type"`
	Summary     string `json:"summary"`
}

// PersonRelation 人物/角色关联（person-relations.jsonlines）
type PersonRelation struct {
	PersonType      string `json:"person_type"`
	PersonID        int64  `json:"person_id"`
	RelatedPersonID int64  `json:"related_person_id"`
	RelationType    int    `json:"relation_type"`
	Spoiler         bool   `json:"spoiler"`
	Ended           bool   `json:"ended"`
}
