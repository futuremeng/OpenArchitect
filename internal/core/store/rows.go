package store

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/plugins/sqlite"
)

// The driver materializes whole result sets, so mapping by column name costs
// nothing and survives adding a column (spec W1). Never index by position.
func rowMap(cols []string) map[string]int {
	m := make(map[string]int, len(cols))
	for i, name := range cols {
		m[name] = i
	}
	return m
}

func rowsToMaps(r sqlite.Rows) []map[string]any {
	index := rowMap(r.Columns)
	out := make([]map[string]any, 0, len(r.Values))
	for _, values := range r.Values {
		row := make(map[string]any, len(index))
		for name, i := range index {
			if i < len(values) {
				row[name] = values[i]
			}
		}
		out = append(out, row)
	}
	return out
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

func jsonString(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func stringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		// A corrupt column is worth a skipped row, not a crashed scan. Store
		// 没有 logger 可用，所以这里只降级不报错。
		return nil
	}
	return out
}

// escapeLike makes a user query literal inside a LIKE pattern.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
