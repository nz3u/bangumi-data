// Package wiki 提供 wiki 原始字符串的轻量解析。
//
// 完整语法见 https://github.com/bangumi/wiki-syntax-spec 与
// https://github.com/bangumi/wiki-parser-go。本包只实现前端展示
// 最常用的场景：从 {{Infobox ...}} 模板中提取 key/value 字段。
// 更复杂的语法（层级列表、嵌套模板等）后续按需扩展。
package wiki

import (
	"fmt"
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

// ExtractNameCN 从 wiki 原始字符串中提取「简体中文名」字段值，
// 无该字段或解析失败时返回空串（导入与升级回填共用）。
func ExtractNameCN(raw string) string {
	if raw == "" {
		return ""
	}
	ib, err := ParseInfobox(raw)
	if err != nil {
		return ""
	}
	cn := ib.Get("简体中文名")
	if idx := strings.IndexAny(cn, "\r\n"); idx >= 0 {
		cn = cn[:idx]
	}
	return strings.TrimSpace(cn)
}

// Parse 是 ParseInfobox 的简写。
func Parse(raw string) (*Infobox, error) {
	return ParseInfobox(raw)
}
