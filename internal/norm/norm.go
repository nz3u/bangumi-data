// Package norm 提供条目检索用的文本归一化。
//
// 搜索侧把 name / name_cn / 别名 拼成一个归一化串建 trigram 索引，
// 查询侧用同一函数处理用户输入，从而让"被符号切断"或"全半角/大小写不同"
// 的写法都能互相命中，例如：
//
//	少女☆歌剧 Revue Starlight -> 少女歌剧revuestarlight   （少女歌剧 可命中）
//	Chou Kaguya-hime!         -> choukaguyahime           （Kaguya Hime 可命中）
package norm

import (
	"strings"
	"unicode"
)

// Fold 归一化文本：全角 ASCII 转半角、丢弃一切非字母/数字/组合记号、其余转小写。
//
// 丢弃（而非替换成空格）是关键：这样 "Kaguya-hime"、"Kaguya Hime"、"kaguyahime"
// 三种写法得到同一结果，召回率最高；代价是相邻词之间产生少量跨词 3-gram，
// 属于"多召回"而非"漏召回"，由检索结果的分级排序兜底。
//
// 副作用正好是优点：`%`、`_`、`\` 均属标点类，会被丢弃，因此 Fold 的结果
// 天然不含 LIKE 通配符，可直接拼进 LIKE 模式而无需 ESCAPE。
// （FTS5 trigram 的 LIKE 索引优化在带 ESCAPE 时会失效，见 api.searchSubjects。）
func Fold(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E: // 全角 ASCII（！＂＃…ＡＢ…ｚ）→ 半角
			r -= 0xFEE0
		case r == 0x3000: // 全角空格：与其他分隔符一样丢弃
			continue
		}
		// 只保留字母（含汉字/假名/谚文）、组合记号（浊音符等）、数字。
		// 标点 P*、符号 S*、空白 Z*、控制字符 C* 全部丢弃。
		if unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Join 把多个名称片段归一化后拼成一个检索用的归一化串。
// 片段之间的分隔符会被 Fold 丢弃，因此传 nil 片段安全。
func Join(parts ...string) string {
	return Fold(strings.Join(parts, " "))
}
