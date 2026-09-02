package wiki

import "testing"

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
