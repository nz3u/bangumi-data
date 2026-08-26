package api

import "testing"

func TestParseTagCombo(t *testing.T) {
	cases := []struct {
		raw     string
		posNeg  tagCombo
		comment string
	}{
		{"", tagCombo{}, "空参数"},
		{"   ", tagCombo{}, "纯空白"},
		{"奇幻", tagCombo{pos: []string{"奇幻"}}, "无前缀视为正标签（兼容旧单标签）"},
		{"+奇幻", tagCombo{pos: []string{"奇幻"}}, "显式正前缀"},
		{"-科幻", tagCombo{neg: []string{"科幻"}}, "负前缀"},
		{"+奇幻,-科幻", tagCombo{pos: []string{"奇幻"}, neg: []string{"科幻"}}, "正负组合"},
		{" +奇幻 , -科幻 ，原创 ", tagCombo{pos: []string{"奇幻", "原创"}, neg: []string{"科幻"}}, "中英文逗号与空白"},
		{"+, - ,a,", tagCombo{pos: []string{"a"}}, "裸符号与空片段忽略"},
	}
	for _, c := range cases {
		got := parseTagCombo(c.raw)
		if !equalStrings(got.pos, c.posNeg.pos) || !equalStrings(got.neg, c.posNeg.neg) {
			t.Errorf("parseTagCombo(%q) = %+v, want %+v (%s)", c.raw, got, c.posNeg, c.comment)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"abc":    "abc",
		"100%":   `100\%`,
		"a_b":    `a\_b`,
		`50\off`: `50\\off`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
