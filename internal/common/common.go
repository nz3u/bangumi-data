// Package common 负责加载 bangumi/common 中的 id 常量映射
// （分类/平台/关联类型/职位等 id -> 中文名），供导入与 API 层使用。
//
// 常量来源优先级：
//  1. 运行时通过 --common-dir 指定的目录（便于子模块更新后热加载）
//  2. 编译期内嵌进二进制的 common/*.yml（go:embed，单二进制部署）
//
// common 的 yaml 大量使用 anchor/alias（如 &TYPE_BOOK / *TYPE_BOOK，
// 甚至作为映射键），且 subject_staffs.yml 别名数量会触发 yaml.v2/v3
// 的 excessive aliasing 保护。因此这里用 yaml.v3 的 Node API 解析
// （Node 保留 alias 节点不解析、不计数），再手动解析 alias 后
// 逐个节点 Decode 成目标结构。
package common

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"

	embedded "bangumi-subject-go"
)

// Relation 关联类型（条目关联 / 人物-角色关联）
type Relation struct {
	EN   string `yaml:"en"`
	CN   string `yaml:"cn"`
	JP   string `yaml:"jp"`
	Desc string `yaml:"desc"`
}

// Platform 条目平台
type Platform struct {
	ID      int    `yaml:"id"`
	Type    string `yaml:"type"`
	TypeCN  string `yaml:"type_cn"`
	Alias   string `yaml:"alias"`
	WikiTpl string `yaml:"wiki_tpl"`
}

// Category 职位分类（staff 的 categories 中的分类）
type Category struct {
	Order int    `yaml:"order"`
	EN    string `yaml:"en"`
	CN    string `yaml:"cn"`
}

// Staff 职位（subject-persons.position）
type Staff struct {
	EN         string     `yaml:"en"`
	CN         string     `yaml:"cn"`
	JP         string     `yaml:"jp"`
	Desc       string     `yaml:"desc"`
	Categories []Category `yaml:"categories"`
}

// Constants 所有 id 常量的统一访问入口。
type Constants struct {
	// 作品类型：id -> 中文名（来自 md 文档；漫画/动画/音乐/游戏/三次元）
	SubjectTypes map[int]string
	// 平台：作品类型 -> 平台 id -> 平台信息（subject_platforms.yml）
	Platforms map[int]map[int]Platform
	// 条目关联：作品类型 -> 关联类型 id -> 名称（subject_relations.yml）
	SubjectRelations map[int]map[int]Relation
	// 职位：作品类型 -> position id -> 名称（subject_staffs.yml）
	Staffs map[int]map[int]Staff
	// 人物/角色关联：prsn/crt -> 关联类型 id -> 名称（person_relations.yml）
	PersonRelations map[string]map[int]Relation

	// 以下来自 wiki_database.md 的说明
	PersonTypes      map[int]string // 人物类型：1个人 2公司 3组合
	CharacterRoles   map[int]string // 角色类型：1角色 2机体 3组织
	EpisodeTypes     map[int]string // 章节类型：0正篇 1特别篇 2OP 3ED 4Trailer 5MAD 6其他
	SubjectCharTypes map[int]string // 条目角色类型：1主角 2配角 3客串
}

var (
	mu        sync.Mutex
	constants *Constants
)

// Load 加载常量（进程内单例）。dir 为空时使用内嵌数据。
func Load(dir string) (*Constants, error) {
	mu.Lock()
	defer mu.Unlock()
	if constants != nil {
		return constants, nil
	}
	c, err := load(dir)
	if err != nil {
		return nil, err
	}
	constants = c
	return constants, nil
}

// resolve 沿 alias 链解析节点。
func resolve(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// docRoot 将 yml 文本解析为文档根映射节点。
func docRoot(b []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	root := resolve(&doc)
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, fmt.Errorf("yaml 文档结构异常")
	}
	return resolve(root.Content[0]), nil
}

// section 取根映射中指定 section 的值节点（解析 alias）。
func section(root *yaml.Node, name string) (*yaml.Node, error) {
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("根节点不是映射")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == name {
			return resolve(root.Content[i+1]), nil
		}
	}
	return nil, nil // 文件缺失该 section 不算错误
}

// walkMap 遍历映射节点，key 为解析后的标量值。
func walkMap(n *yaml.Node, fn func(key string, val *yaml.Node) error) error {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := resolve(n.Content[i])
		if k.Kind != yaml.ScalarNode {
			return fmt.Errorf("映射 key 不是标量: %s", k.Value)
		}
		if err := fn(k.Value, resolve(n.Content[i+1])); err != nil {
			return err
		}
	}
	return nil
}

// parseMap 解析 map[string]map[string]T 结构的 section。
func parseMap[T any](b []byte, sectionName string) (map[string]map[string]T, error) {
	root, err := docRoot(b)
	if err != nil {
		return nil, err
	}
	sec, err := section(root, sectionName)
	if err != nil || sec == nil {
		return nil, err
	}
	out := map[string]map[string]T{}
	err = walkMap(sec, func(key string, val *yaml.Node) error {
		inner := map[string]T{}
		if err := walkMap(val, func(k2 string, v2 *yaml.Node) error {
			var t T
			if err := v2.Decode(&t); err != nil {
				return err
			}
			inner[k2] = t
			return nil
		}); err != nil {
			return err
		}
		out[key] = inner
		return nil
	})
	return out, err
}

func mapToInt[T any](m map[string]map[string]T) (map[int]map[int]T, error) {
	out := map[int]map[int]T{}
	for k, inner := range m {
		id, err := strconv.Atoi(k)
		if err != nil {
			continue // 跳过非数字 key（如 platforms 中的 book_series）
		}
		conv := map[int]T{}
		for k2, v := range inner {
			id2, err := strconv.Atoi(k2)
			if err != nil {
				return nil, fmt.Errorf("key %s 不是数字: %w", k2, err)
			}
			conv[id2] = v
		}
		out[id] = conv
	}
	return out, nil
}

func load(dir string) (*Constants, error) {
	readFile := func(name string) ([]byte, error) {
		if dir != "" {
			p := filepath.Join(dir, name)
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("读取 %s: %w", p, err)
			}
			return b, nil
		}
		b, err := fs.ReadFile(embedded.YML, "common/"+name)
		if err != nil {
			return nil, fmt.Errorf("读取内嵌 %s: %w", name, err)
		}
		return b, nil
	}

	c := &Constants{
		SubjectTypes:     map[int]string{1: "书籍", 2: "动画", 3: "音乐", 4: "游戏", 6: "三次元"},
		Platforms:        map[int]map[int]Platform{},
		SubjectRelations: map[int]map[int]Relation{},
		Staffs:           map[int]map[int]Staff{},
		PersonRelations:  map[string]map[int]Relation{},
		PersonTypes:      map[int]string{1: "个人", 2: "公司", 3: "组合"},
		CharacterRoles:   map[int]string{1: "角色", 2: "机体", 3: "组织"},
		EpisodeTypes:     map[int]string{0: "正篇", 1: "特别篇", 2: "OP", 3: "ED", 4: "Trailer", 5: "MAD", 6: "其他"},
		SubjectCharTypes: map[int]string{1: "主角", 2: "配角", 3: "客串"},
	}

	// 条目关联（subject_relations.yml）
	if b, err := readFile("subject_relations.yml"); err == nil {
		m, err := parseMap[Relation](b, "relations")
		if err != nil {
			return nil, fmt.Errorf("解析 subject_relations.yml: %w", err)
		}
		c.SubjectRelations, err = mapToInt(m)
		if err != nil {
			return nil, fmt.Errorf("解析 subject_relations.yml: %w", err)
		}
	}

	// 平台（subject_platforms.yml）
	if b, err := readFile("subject_platforms.yml"); err == nil {
		m, err := parseMap[Platform](b, "platforms")
		if err != nil {
			return nil, fmt.Errorf("解析 subject_platforms.yml: %w", err)
		}
		c.Platforms, err = mapToInt(m)
		if err != nil {
			return nil, fmt.Errorf("解析 subject_platforms.yml: %w", err)
		}
		// 游戏平台在 platforms 下的 game_platforms 子 section
		// （PC/PS5/Switch 等），合并进作品类型 4
		root, err := docRoot(b)
		if err == nil {
			if ps, err := section(root, "platforms"); err == nil && ps != nil {
				if gs, err := section(ps, "game_platforms"); err == nil && gs != nil {
					if c.Platforms[4] == nil {
						c.Platforms[4] = map[int]Platform{}
					}
					if err := walkMap(gs, func(ks string, v *yaml.Node) error {
						id, err := strconv.Atoi(ks)
						if err != nil || id <= 0 {
							return nil
						}
						var p Platform
						if err := v.Decode(&p); err != nil {
							return err
						}
						c.Platforms[4][id] = p
						return nil
					}); err != nil {
						return nil, fmt.Errorf("解析 game_platforms: %w", err)
					}
				}
			}
		}
	}

	// 职位（subject_staffs.yml）
	if b, err := readFile("subject_staffs.yml"); err == nil {
		m, err := parseMap[Staff](b, "staffs")
		if err != nil {
			return nil, fmt.Errorf("解析 subject_staffs.yml: %w", err)
		}
		c.Staffs, err = mapToInt(m)
		if err != nil {
			return nil, fmt.Errorf("解析 subject_staffs.yml: %w", err)
		}
	}

	// 人物/角色关联（person_relations.yml，key 为 prsn/crt 字符串）
	if b, err := readFile("person_relations.yml"); err == nil {
		m, err := parseMap[Relation](b, "relations")
		if err != nil {
			return nil, fmt.Errorf("解析 person_relations.yml: %w", err)
		}
		conv := map[string]map[int]Relation{}
		for k, inner := range m {
			mi := map[int]Relation{}
			for k2, v := range inner {
				id, err := strconv.Atoi(k2)
				if err != nil {
					return nil, fmt.Errorf("解析 person_relations.yml key %s: %w", k2, err)
				}
				mi[id] = v
			}
			conv[k] = mi
		}
		c.PersonRelations = conv
	}

	return c, nil
}

// SubjectTypeCN 作品类型中文名。
func (c *Constants) SubjectTypeCN(t int) string {
	if s, ok := c.SubjectTypes[t]; ok {
		return s
	}
	return fmt.Sprintf("类型%d", t)
}

// PlatformCN 子类型中文名。
func (c *Constants) PlatformCN(subjectType, platform int) string {
	if m, ok := c.Platforms[subjectType]; ok {
		if p, ok := m[platform]; ok && p.TypeCN != "" {
			return p.TypeCN
		}
	}
	if platform == 0 {
		return "其他"
	}
	return fmt.Sprintf("子类型%d", platform)
}

// RelationCN 条目关联类型中文名。
func (c *Constants) RelationCN(subjectType, relation int) string {
	if m, ok := c.SubjectRelations[subjectType]; ok {
		if r, ok := m[relation]; ok && r.CN != "" {
			return r.CN
		}
	}
	return fmt.Sprintf("关联%d", relation)
}

// StaffCN 职位中文名。
func (c *Constants) StaffCN(subjectType, position int) string {
	if m, ok := c.Staffs[subjectType]; ok {
		if s, ok := m[position]; ok && s.CN != "" {
			return s.CN
		}
	}
	return fmt.Sprintf("职位%d", position)
}

// PersonRelationCN 人物/角色关联类型中文名。
func (c *Constants) PersonRelationCN(personType string, relation int) string {
	if m, ok := c.PersonRelations[personType]; ok {
		if r, ok := m[relation]; ok && r.CN != "" {
			return r.CN
		}
	}
	return fmt.Sprintf("关联%d", relation)
}
