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
