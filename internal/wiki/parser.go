// Package wiki 提供 wiki 原始字符串的轻量解析。
//
// 完整语法见 https://github.com/bangumi/wiki-syntax-spec 与
// https://github.com/bangumi/wiki-parser-go。本包只实现前端展示
// 最常用的场景：从 {{Infobox ...}} 模板中提取 key/value 字段。
// 更复杂的语法（层级列表、嵌套模板等）后续按需扩展。
package wiki

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Field infobox 中的一个字段。
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Infobox 解析结果。
type Infobox struct {
	// Template 模板名，如 "Infobox animanga/Novel"、"Infobox Crt"
	Template string  `json:"template"`
	Fields   []Field `json:"fields"`
}

// ParseInfobox 从原始 wiki 字符串中提取 infobox 字段。
// 支持 {{Infobox xxx ... }} 模板，按行解析 |key= value。
// 值为 { ... } 的多行结构（如别名列表）原样保留在 Value 中。
func ParseInfobox(raw string) (*Infobox, error) {
	start := strings.Index(raw, "{{Infobox")
	if start < 0 {
		return nil, fmt.Errorf("未找到 Infobox 模板")
	}
	// 找到匹配的结束 }}
	depth := 0
	end := -1
	for i := start; i < len(raw); i++ {
		switch {
		case strings.HasPrefix(raw[i:], "{{"):
			depth++
			i++
		case strings.HasPrefix(raw[i:], "}}"):
			depth--
			i++
			if depth == 0 {
				end = i + 1
				i = len(raw) // 结束外层循环
			}
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("Infobox 模板未闭合")
	}

	body := raw[start:end]
	ib := &Infobox{}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i == 0 { // {{Infobox xxx
			name := strings.TrimPrefix(line, "{{Infobox")
			name = strings.TrimSpace(name)
			ib.Template = "Infobox " + name
			continue
		}
		if line == "" || line == "}}" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue // 跳过非字段行
		}
		line = strings.TrimPrefix(line, "|")
		line = strings.TrimSpace(line)
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// 值可能跨多行（{ ... } 结构），收集后续以 } 结尾的段落
		if strings.HasPrefix(value, "{") && !strings.HasSuffix(value, "}") {
			for j := i + 1; j < len(lines); j++ {
				value += "\n" + lines[j]
				if strings.HasSuffix(strings.TrimSpace(lines[j]), "}") {
					break
				}
			}
		}
		ib.Fields = append(ib.Fields, Field{Key: key, Value: value})
	}
	return ib, nil
}

// Get 取指定 key 的字段值，未找到返回空串。
func (ib *Infobox) Get(key string) string {
	for _, f := range ib.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// parse 解析 infobox；raw 为空或解析失败时返回 false（各提取函数的统一入口）。
func parse(raw string) (*Infobox, bool) {
	if raw == "" {
		return nil, false
	}
	ib, err := ParseInfobox(raw)
	if err != nil {
		return nil, false
	}
	return ib, true
}

// fieldLine 取字段值的第一行（多行值只保留首行）并去除首尾空白。
func fieldLine(ib *Infobox, key string) string {
	v := ib.Get(key)
	if idx := strings.IndexAny(v, "\r\n"); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

// ExtractNameCN 从 wiki 原始字符串中提取「简体中文名」字段值，
// 无该字段或解析失败时返回空串（导入与升级回填共用）。
func ExtractNameCN(raw string) string {
	ib, ok := parse(raw)
	if !ok {
		return ""
	}
	return fieldLine(ib, "简体中文名")
}

// aliasItemRe 匹配别名列表中的一项：[Shoujo Kageki Revue Starlight]。
// 方括号内不再允许出现方括号或换行，避免把后续字段误当成别名。
var aliasItemRe = regexp.MustCompile(`\[([^\[\]\n]*)\]`)

// SplitAliasItem 拆分别名列表中的一项为子类 key 与值。
//
// 角色/人物模板（Infobox Crt、Infobox Person）的别名带子类前缀：
//
//	[罗马字|Hirasawa Yui]   -> ("罗马字", "Hirasawa Yui")
//	[英文名|Yui Hirasawa]   -> ("英文名", "Yui Hirasawa")
//	[日文名|平沢 唯]        -> ("日文名", "平沢 唯")
//
// 条目模板与部分角色别名是裸值（[Chou Kaguya-hime!]、[Zero]），此时 key 为空。
// 前缀为空（[|value]）时按裸值处理，避免把无意义的竖线带进检索串。
func SplitAliasItem(item string) (key, value string) {
	item = strings.TrimSpace(item)
	idx := strings.Index(item, "|")
	if idx <= 0 {
		return "", item
	}
	return strings.TrimSpace(item[:idx]), strings.TrimSpace(item[idx+1:])
}

// aliasValues 取 infobox「别名」列表的全部值：剥离子类前缀、跳过空值、按值去重。
func aliasValues(ib *Infobox) []string {
	block := ib.Get("别名")
	if block == "" {
		return nil
	}
	m := aliasItemRe.FindAllStringSubmatch(block, -1)
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	seen := make(map[string]struct{}, len(m))
	for _, g := range m {
		_, v := SplitAliasItem(g[1])
		if v == "" {
			continue
		}
		// 真实数据里「英文名」与「罗马字」常取同值，去重可减小索引体积
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// aliasValue 取「别名」中指定子类 key 的首个非空值，如 key="罗马字"。
func aliasValue(ib *Infobox, key string) string {
	for _, g := range aliasItemRe.FindAllStringSubmatch(ib.Get("别名"), -1) {
		if k, v := SplitAliasItem(g[1]); k == key && v != "" {
			return v
		}
	}
	return ""
}

// romajiValue 取罗马字：优先「别名」列表中的 [罗马字|...]，
// 其次少数资料用的顶层字段 |罗马字=（如 |罗马字= Kyouno Madoka）。
func romajiValue(ib *Infobox) string {
	if v := aliasValue(ib, "罗马字"); v != "" {
		return v
	}
	return fieldLine(ib, "罗马字")
}

// ExtractAliases 从 wiki 原始字符串的 infobox「别名」字段中提取别名值列表。
// 别名字段形如：
//
//	|别名={
//	[Chou Kaguya-hime!]
//	[Cosmic Princess Kaguya!]
//	[罗马字|Hirasawa Yui]
//	}
//
// 带子类前缀的项只保留值（[罗马字|Hirasawa Yui] -> Hirasawa Yui），
// 子类 key（罗马字/英文名/昵称…）不参与检索；值为空的项跳过。
// 无该字段、解析失败或列表为空时返回 nil（导入与升级回填共用）。
func ExtractAliases(raw string) []string {
	ib, ok := parse(raw)
	if !ok {
		return nil
	}
	return aliasValues(ib)
}

// ExtractAliasesText 同 ExtractAliases，返回空格连接后的单串，便于直接入库。
func ExtractAliasesText(raw string) string {
	return strings.Join(ExtractAliases(raw), " ")
}

// ExtractRomaji 提取角色/人物资料中的罗马字（假名的拉丁转写），
// 用于按罗马字检索（Hirasawa Yui -> 平泽唯）。两种写法都支持：
//
//	|别名={
//	[罗马字|Hirasawa Yui]
//	}
//
// 以及顶层字段 |罗马字= Kyouno Madoka（少量资料用这种写法）。
// 无罗马字信息或解析失败时返回空串。
func ExtractRomaji(raw string) string {
	ib, ok := parse(raw)
	if !ok {
		return ""
	}
	return romajiValue(ib)
}

// ExtractPersonAliasesText 返回人物/角色检索用的别名串：全部别名值 + 罗马字。
// 罗马字通常已在别名列表中（[罗马字|...]），此时不重复追加；
// 对把罗马字写在顶层 |罗马字= 的资料则补到末尾，保证按罗马字检索不遗漏。
// 导入与升级回填共用，口径必须一致。
func ExtractPersonAliasesText(raw string) string {
	ib, ok := parse(raw)
	if !ok {
		return ""
	}
	values := aliasValues(ib)
	if romaji := romajiValue(ib); romaji != "" && !slices.Contains(values, romaji) {
		values = append(values, romaji)
	}
	return strings.Join(values, " ")
}

// Parse 是 ParseInfobox 的简写。
func Parse(raw string) (*Infobox, error) {
	return ParseInfobox(raw)
}
