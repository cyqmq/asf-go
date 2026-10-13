package web

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.9.9", "2.0.0", -1},
		{"v2.1.0", "2.0.9", 1},
		{"1.0", "1.0.0", 0},
		{"2", "1.9", 1},
	}
	for _, c := range cases {
		got := CompareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, 期望 %d", c.a, c.b, got, c.want)
		}
	}
}