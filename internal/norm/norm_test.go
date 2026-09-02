package norm

import (
	"strings"
	"testing"
)

func TestFold(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串", "", ""},
		{"纯符号归零", "！！！", ""},
		{"全角空格归零", "　", ""},
		{"去符号", "少女☆歌剧 Revue Starlight", "少女歌剧revuestarlight"},
		{"去行尾叹号", "超かぐや姫！", "超かぐや姫"},
		{"去连字符", "Chou Kaguya-hime!", "choukaguyahime"},
		{"去空格", "Kaguya Hime", "kaguyahime"},
		{"大写转小写", "REVIEW STARLIGHT", "reviewstarlight"},
		{"全角转半角", "ＡＢＣ", "abc"},
		{"通配符被删除", "A_B%100", "ab100"},
		{"反斜杠被删除", `a\b`, "ab"},
		{"假名原样保留", "レヴュー", "レヴュー"},
		{"中文原样保留", "辉夜姬", "辉夜姬"},
		{"撇号被删除", "Rock'n'Roll", "rocknroll"},
		{"浊音符保留", "がぎぐ", "がぎぐ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fold(c.in); got != c.want {
				t.Errorf("Fold(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestFoldEquivalence 同一作品的不同写法归一化后必须完全一致，
// 这是"被符号切断""全半角不同"的检索词能互相命中的前提。
func TestFoldEquivalence(t *testing.T) {
	groups := [][]string{
		{"Kaguya Hime", "Kaguya-hime", "kaguyahime", "KAGUYA　HIME"},
		{"少女歌剧", "少女☆歌剧", "少女 歌剧"},
		{"Revue Starlight", "revuestarlight", "REVUE-STARLIGHT"},
	}
	for _, g := range groups {
		want := Fold(g[0])
		if want == "" {
			t.Fatalf("基准写法 %q 归一化为空", g[0])
		}
		for _, s := range g[1:] {
			if got := Fold(s); got != want {
				t.Errorf("Fold(%q) = %q, 与 %q 的 %q 不一致", s, got, g[0], want)
			}
		}
	}
}

func TestJoin(t *testing.T) {
	got := Join("少女☆歌劇 レヴュースタァライト", "少女☆歌剧 Revue Starlight", "Shoujo Kageki Revue Starlight")
	// 归一化后查询词应成为拼接串的子串——这是检索命中的判据。
	for _, q := range []string{"少女歌剧", "少女歌劇", "revuestarlight", "shoujokageki"} {
		if !strings.Contains(got, q) {
			t.Errorf("Join 结果 %q 不含查询词 %q", got, q)
		}
	}
	if strings.ContainsAny(got, "☆ ") {
		t.Errorf("Join 结果仍含分隔符: %q", got)
	}
	if got := Join(); got != "" {
		t.Errorf("Join() = %q, want 空串", got)
	}
}
