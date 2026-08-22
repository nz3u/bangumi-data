package pics

import "testing"

func TestExtractRel(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			in:   "https://lain.bgm.tv/pic/crt/l/a6/e8/1_prsn_k7wpt.jpg?r=1723962294",
			want: "a6/e8/1_prsn_k7wpt.jpg",
		},
		{
			in:   "https://lain.bgm.tv/pic/crt/l/bb/cc/2_prsn_x.png",
			want: "bb/cc/2_prsn_x.png",
		},
		{"", ""},
		{"https://example.com/other/path.jpg", ""},
	}
	for _, c := range cases {
		if got := ExtractRel(c.in); got != c.want {
			t.Errorf("ExtractRel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	const rel = "a6/e8/1_prsn_k7wpt.jpg"
	cases := []struct{ size, want string }{
		{"l", "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"large", "https://lain.bgm.tv/pic/crt/l/" + rel},
		{"m", "https://lain.bgm.tv/r/200/pic/crt/l/" + rel},
		{"medium", "https://lain.bgm.tv/r/200/pic/crt/l/" + rel},
		{"s", "https://lain.bgm.tv/r/100/pic/crt/l/" + rel},
		{"g", "https://lain.bgm.tv/r/100x100/pic/crt/l/" + rel},
		{"grid", "https://lain.bgm.tv/r/100x100/pic/crt/l/" + rel},
		{"unknown", "https://lain.bgm.tv/pic/crt/l/" + rel}, // 默认 l
	}
	for _, c := range cases {
		if got := BuildURL(rel, c.size); got != c.want {
			t.Errorf("BuildURL(rel,%q) = %q, want %q", c.size, got, c.want)
		}
	}
}
