package wiki

import (
	"strings"
	"testing"
)

func TestExtractNameCN(t *testing.T) {
	const infobox = `{{Infobox Crt
|简体中文名= 水树奈奈
|别名={
[第二中文名|]
[英文名|]
[日文名|近藤奈々 (こんどう なな)]
}
|性别= 女
}}`
	if got := ExtractNameCN(infobox); got != "水树奈奈" {
		t.Errorf("ExtractNameCN = %q, want 水树奈奈", got)
	}

	const noField = `{{Infobox Crt
|英文名= L.L.
|性别= 男
}}`
	if got := ExtractNameCN(noField); got != "" {
		t.Errorf("ExtractNameCN(无简体中文名) = %q, want 空串", got)
	}

	const emptyValue = `{{Infobox Person
|简体中文名=
|性别= 男
}}`
	if got := ExtractNameCN(emptyValue); got != "" {
		t.Errorf("ExtractNameCN(空值) = %q, want 空串", got)
	}

	for _, bad := range []string{"", "不是 wiki 文本", "{{Infobox 未闭合"} {
		if got := ExtractNameCN(bad); got != "" {
			t.Errorf("ExtractNameCN(%q) = %q, want 空串", bad, got)
		}
	}
}

func TestExtractAliases(t *testing.T) {
	// 多行 { ... } 列表（条目 infobox 的常见形式）
	const multi = `{{Infobox animanga/TVAnime
|中文名= 超辉夜姬！
|别名={
[Chou Kaguya-hime!]
[Chou Kaguya hime!]
[Cosmic Princess Kaguya!]
[超时空辉夜姬！]
}
|话数= 1
}}`
	want := []string{"Chou Kaguya-hime!", "Chou Kaguya hime!", "Cosmic Princess Kaguya!", "超时空辉夜姬！"}
	got := ExtractAliases(multi)
	if len(got) != len(want) {
		t.Fatalf("ExtractAliases = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ExtractAliases[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if text := ExtractAliasesText(multi); text != "Chou Kaguya-hime! Chou Kaguya hime! Cosmic Princess Kaguya! 超时空辉夜姬！" {
		t.Errorf("ExtractAliasesText = %q", text)
	}

	// 单行 { [a][b] } 形式
	const single = "{{Infobox Crt\n|别名= {[Lelouch][ルルーシュ]}\n|性别= 男\n}}"
	if got := ExtractAliases(single); len(got) != 2 || got[0] != "Lelouch" || got[1] != "ルルーシュ" {
		t.Errorf("ExtractAliases(单行) = %q", got)
	}

	// 空别名列表与无别名字段：返回 nil，不产生空片段
	const emptyBlock = "{{Infobox animanga/TVAnime\n|别名={\n}\n|话数= 12\n}}"
	if got := ExtractAliases(emptyBlock); got != nil {
		t.Errorf("ExtractAliases(空列表) = %q, want nil", got)
	}
	const noAlias = "{{Infobox Crt\n|简体中文名= 水树奈奈\n|性别= 女\n}}"
	if got := ExtractAliases(noAlias); got != nil {
		t.Errorf("ExtractAliases(无别名字段) = %q, want nil", got)
	}

	for _, bad := range []string{"", "不是 wiki 文本", "{{Infobox 未闭合"} {
		if got := ExtractAliases(bad); got != nil {
			t.Errorf("ExtractAliases(%q) = %q, want nil", bad, got)
		}
	}
}

// TestExtractAliasesRomaji 角色/人物模板的别名带子类前缀（[罗马字|Hirasawa Yui]），
// 提取时只保留值——罗马字由此进入检索串，Hirasawa Yui 才能命中「平泽唯」。
func TestExtractAliasesRomaji(t *testing.T) {
	const crt = `{{Infobox Crt
|简体中文名= 平泽唯
|别名={
[第二中文名|]
[英文名|Yui Hirasawa]
[日文名|平沢 唯]
[纯假名|ひらさわゆい]
[罗马字|Hirasawa Yui]
[昵称|呆唯]
}
|性别= 女
}}`
	want := []string{"Yui Hirasawa", "平沢 唯", "ひらさわゆい", "Hirasawa Yui", "呆唯"}
	got := ExtractAliases(crt)
	if len(got) != len(want) {
		t.Fatalf("ExtractAliases = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ExtractAliases[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// 值为空的项（[第二中文名|]）不得产生空片段或只含前缀的片段
	for _, a := range got {
		if a == "" || strings.Contains(a, "|") {
			t.Errorf("ExtractAliases 含无效片段 %q", a)
		}
	}
	if text := ExtractAliasesText(crt); !strings.Contains(text, "Hirasawa Yui") {
		t.Errorf("ExtractAliasesText = %q, want 含罗马字", text)
	}

	// 裸值与带前缀混排：裸值原样保留，带前缀的只留值
	const mixed = "{{Infobox Crt\n|别名= {[Zero][罗马字|Lelouch Lamperouge]}\n|性别= 男\n}}"
	if got := ExtractAliases(mixed); len(got) != 2 || got[0] != "Zero" || got[1] != "Lelouch Lamperouge" {
		t.Errorf("ExtractAliases(混排) = %q", got)
	}
}

// TestExtractRomaji 罗马字的两种写法：别名子类 [罗马字|...] 与顶层 |罗马字=。
func TestExtractRomaji(t *testing.T) {
	const aliasForm = `{{Infobox Crt
|简体中文名= 平泽唯
|别名={
[英文名|Yui Hirasawa]
[罗马字|Hirasawa Yui]
[昵称|]
}
}}`
	if got := ExtractRomaji(aliasForm); got != "Hirasawa Yui" {
		t.Errorf("ExtractRomaji(别名子类) = %q, want Hirasawa Yui", got)
	}

	// 别名里的罗马字为空时回退到顶层字段（少量资料这样写）
	const topLevel = `{{Infobox Crt
|简体中文名= 京乃圆
|别名={
[罗马字|]
[昵称|小圆]
}
|纯假名= きょうの まどか
|罗马字= Kyouno Madoka
|昵称= 小圆
}}`
	if got := ExtractRomaji(topLevel); got != "Kyouno Madoka" {
		t.Errorf("ExtractRomaji(顶层字段) = %q, want Kyouno Madoka", got)
	}

	for _, bad := range []string{"", "不是 wiki 文本", "{{Infobox 未闭合", "{{Infobox Crt\n|简体中文名= 无名\n}}"} {
		if got := ExtractRomaji(bad); got != "" {
			t.Errorf("ExtractRomaji(%q) = %q, want 空串", bad, got)
		}
	}
}

// TestExtractPersonAliasesText 检索用的别名串 = 全部别名值 + 罗马字，
// 罗马字已在别名列表中时不得重复追加。
func TestExtractPersonAliasesText(t *testing.T) {
	const aliasForm = `{{Infobox Crt
|简体中文名= 平泽唯
|别名={
[英文名|Yui Hirasawa]
[罗马字|Hirasawa Yui]
}
}}`
	if got, want := ExtractPersonAliasesText(aliasForm), "Yui Hirasawa Hirasawa Yui"; got != want {
		t.Errorf("ExtractPersonAliasesText = %q, want %q（罗马字不重复追加）", got, want)
	}

	const topLevel = `{{Infobox Crt
|简体中文名= 京乃圆
|别名={
[昵称|小圆]
}
|罗马字= Kyouno Madoka
}}`
	if got, want := ExtractPersonAliasesText(topLevel), "小圆 Kyouno Madoka"; got != want {
		t.Errorf("ExtractPersonAliasesText(顶层罗马字) = %q, want %q", got, want)
	}

	if got := ExtractPersonAliasesText("不是 wiki 文本"); got != "" {
		t.Errorf("ExtractPersonAliasesText(非 wiki) = %q, want 空串", got)
	}
}

func TestSplitAliasItem(t *testing.T) {
	cases := []struct {
		in, key, value string
	}{
		{"罗马字|Hirasawa Yui", "罗马字", "Hirasawa Yui"},
		{" 英文名 | Yui Hirasawa ", "英文名", "Yui Hirasawa"},
		{"Zero", "", "Zero"},
		{"Chou Kaguya-hime!", "", "Chou Kaguya-hime!"},
		{"罗马字|", "罗马字", ""},
		{"|孤立竖线", "", "|孤立竖线"},
		{"", "", ""},
	}
	for _, c := range cases {
		key, value := SplitAliasItem(c.in)
		if key != c.key || value != c.value {
			t.Errorf("SplitAliasItem(%q) = (%q, %q), want (%q, %q)", c.in, key, value, c.key, c.value)
		}
	}
}
