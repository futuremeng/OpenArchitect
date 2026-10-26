package store

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
)

func TestRowsToMapsIgnoresColumnOrder(t *testing.T) {
	// 故意把列序打乱：按下标取列的写法在这里必然错位。
	r := sqlite.Rows{
		Columns: []string{"name", "element_id", "repo_id"},
		Values: [][]any{
			{"Basket", "cmp:x.Basket", "repo:/x"},
			{"Order", "cmp:x.Order", "repo:/x"},
		},
	}
	got := rowsToMaps(r)
	if len(got) != 2 {
		t.Fatalf("rows = %d", len(got))
	}
	if str(got[0]["element_id"]) != "cmp:x.Basket" || str(got[1]["name"]) != "Order" {
		t.Fatalf("mapping = %v", got)
	}
}

func TestStringListRoundTrip(t *testing.T) {
	raw := jsonString([]string{"A", "带\"引号"})
	var back []string
	if err := json.Unmarshal([]byte(raw), &back); err != nil {
		t.Fatal(err)
	}
	if len(stringList(raw)) != 2 {
		t.Fatalf("stringList(%s) = %v", raw, stringList(raw))
	}
	if stringList("not json") != nil {
		t.Fatal("corrupt json must yield no entries, not panic")
	}
	if got := stringList(""); len(got) != 0 {
		t.Fatalf("empty column = %v", got)
	}
}

func TestEscapeLike(t *testing.T) {
	// got 两侧带 `%` 通配，want 同样写全 `%...%`，比较整段 LIKE 模式。
	cases := map[string]string{
		"plain": `%plain%`,
		"a%b":   `%a\%b%`,
		"a_b":   `%a\_b%`,
		`a\b`:   `%a\\b%`,
	}
	for in, want := range cases {
		if got := "%" + escapeLike(in) + "%"; got != want {
			t.Errorf("escapeLike(%q) -> %q want %q", in, got, want)
		}
	}
}
