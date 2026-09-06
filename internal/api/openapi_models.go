package api

import (
	"bangumi-subject-go/internal/common"
	"bangumi-subject-go/internal/config"
	"bangumi-subject-go/internal/model"
	"bangumi-subject-go/internal/update"
)

// 本文件中的类型仅供 OpenAPI 文档生成（swag）引用，描述统一响应包装与
// 各端点的 data 载荷；运行时响应由各 handler 组装，两处字段保持一致。

// apiEnvelope 统一响应包装：成功 {"ok":true,"data":...}，失败 {"ok":false,"error":"..."}。
// swag 注解经 {data=...} 替换 Data 字段的具体类型。
type apiEnvelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

// ---- 通用列表载荷 ----

// subjectSearchData GET /api/subjects/search 的 data。
type subjectSearchData struct {
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Items []subjectBrief `json:"items"`
}

// subjectTagsData GET /api/subjects/tags 的 data。
type subjectTagsData struct {
	Items []tagSuggestItem `json:"items"`
}

// tagSuggestItem 标签/元标签建议项。
type tagSuggestItem struct {
	Name string `json:"name"`
	Cnt  int64  `json:"cnt"`
}

// subjectEpisodesData GET /api/subjects/:id/episodes 的 data。
type subjectEpisodesData struct {
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Items []model.Episode `json:"items"`
}

// episodeSearchData GET /api/episodes/search 的 data。
type episodeSearchData struct {
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Items []episodeBrief `json:"items"`
}

// personSearchData GET /api/persons/search 的 data。
type personSearchData struct {
	Total int64        `json:"total"`
	Page  int          `json:"page"`
	Size  int          `json:"size"`
	Items []personBrief `json:"items"`
}

// personWorksData GET /api/persons/:id/works 的 data。
type personWorksData struct {
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
	Items []workItem `json:"items"`
}

// collaboratorsData GET /api/persons/:id/collaborators 的 data。
type collaboratorsData struct {
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Size  int               `json:"size"`
	Items []collaboratorItem `json:"items"`
}

// characterSearchData GET /api/characters/search 的 data。
type characterSearchData struct {
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
	Items []characterBrief `json:"items"`
}

// collaborationData GET /api/persons/:id/collaboration 的 data。
type collaborationData struct {
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Person collabPerson  `json:"person"`
	Items []*collabItem  `json:"items"`
}

// pairCollaborationData GET /api/persons/:id/collaboration/:other 的 data。
type pairCollaborationData struct {
	PersonA pairPerson `json:"person_a"`
	PersonB pairPerson `json:"person_b"`
	Total   int        `json:"total"`
	Items   []*pairWork `json:"items"`
}

// rolesData GET /api/persons/:id/roles 的 data。
type rolesData struct {
	Person pairPerson  `json:"person"`
	Total  int         `json:"total"`
	Items  []*roleWork `json:"items"`
}

// collaborationPositionsData GET /api/persons/:id/collaboration/positions 的 data。
type collaborationPositionsData struct {
	Self  []collabFacet `json:"self"`
	Other []collabFacet `json:"other"`
}

// ---- 服务状态与配置 ----

// healthData GET /api/health 的 data。
type healthData struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// statsData GET /api/stats 的 data（各表行数）。
type statsData struct {
	Subjects          int64 `json:"subjects"`
	Persons           int64 `json:"persons"`
	Characters        int64 `json:"characters"`
	Episodes          int64 `json:"episodes"`
	SubjectRelations  int64 `json:"subject_relations"`
	SubjectPersons    int64 `json:"subject_persons"`
	SubjectCharacters int64 `json:"subject_characters"`
	PersonCharacters  int64 `json:"person_characters"`
	PersonRelations   int64 `json:"person_relations"`
}

// dbinfoData GET /api/dbinfo 的 data。
type dbinfoData struct {
	Database        *config.DatabaseInfo `json:"database,omitempty"` // 本地版本记录；缺失=无记录（视为旧版本）
	Latest          *update.LatestInfo   `json:"latest,omitempty"`   // 上游最新导出；缺失=尚未成功获取（离线等）
	UpdateAvailable bool                 `json:"update_available"`
	CheckedAt       string               `json:"checked_at,omitempty"`
}

// constantsData GET /api/constants 的 data（id -> 名称常量映射，键为 JSON 字符串）。
type constantsData struct {
	SubjectTypes     map[string]string                    `json:"subject_types"`
	PersonTypes      map[string]string                    `json:"person_types"`
	CharacterRoles   map[string]string                    `json:"character_roles"`
	EpisodeTypes     map[string]string                    `json:"episode_types"`
	SubjectCharTypes map[string]string                    `json:"subject_character_types"`
	Platforms        map[string]map[string]common.Platform `json:"platforms"`
	SubjectRelations map[string]map[string]common.Relation `json:"subject_relations"`
	Staffs           map[string]map[string]common.Staff    `json:"staffs"`
	PersonRelations  map[string]map[string]common.Relation `json:"person_relations"`
}

// picData GET /api/pics/:kind/:id 的 data（轮询接口）。
type picData struct {
	Status string `json:"status"` // ok | pending | failed
	Path   string `json:"path"`   // 不含主机的 CDN 路径，由前端拼接图片主机
}

// adminStatusData GET /api/admin/status 的 data。
type adminStatusData struct {
	State     string   `json:"state"` // idle | updating | migrating | success | failed
	DBExists  bool     `json:"db_exists"`
	Progress  string   `json:"progress,omitempty"`
	Logs      []string `json:"logs,omitempty"`
	Stats     *importerStats `json:"stats,omitempty"`
	Error     string   `json:"error,omitempty"`
	StartedAt string   `json:"started_at,omitempty"`
	EndedAt   string   `json:"ended_at,omitempty"`
}

// importerStats 导入统计（与 /api/stats 字段一致）。
type importerStats struct {
	Subjects          int64 `json:"subjects"`
	Persons           int64 `json:"persons"`
	Characters        int64 `json:"characters"`
	Episodes          int64 `json:"episodes"`
	SubjectRelations  int64 `json:"subject_relations"`
	SubjectPersons    int64 `json:"subject_persons"`
	SubjectCharacters int64 `json:"subject_characters"`
	PersonCharacters  int64 `json:"person_characters"`
	PersonRelations   int64 `json:"person_relations"`
}

// adminConfigData GET /api/admin/config 的 data。
type adminConfigData struct {
	BgmApiKey  string               `json:"bgm_api_key,omitempty"`
	AdminToken string               `json:"admin_token,omitempty"`
	AutoUpdate *config.AutoUpdateConfig `json:"auto_update,omitempty"`
	Server     *config.ServerConfig `json:"server,omitempty"`
	Database   *config.DatabaseInfo `json:"database,omitempty"`
}

// adminActionData POST /api/admin/update|cancel|reset 的 data。
type adminActionData struct {
	Message string `json:"message,omitempty"`
}
