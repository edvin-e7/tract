package store

import "strings"

// ftsQuery turns free-text user input into a safe FTS5 MATCH expression.
//
// Raw input can't go straight into MATCH: characters like ", -, *, : and ( are
// FTS5 operators and would either error or silently change meaning. We split on
// whitespace, strip non-alphanumeric edges from each token, drop empties, and
// re-quote every token as a phrase joined by AND. Trailing * on the last token
// gives prefix-matching so "go" matches "golang" while typing.
func ftsQuery(q string) string {
	fields := strings.Fields(q)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		clean := strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
				return r
			default:
				return -1
			}
		}, f)
		if clean == "" {
			continue
		}
		tokens = append(tokens, clean)
	}
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, len(tokens))
	for i, t := range tokens {
		if i == len(tokens)-1 {
			// prefix-match the final token for type-ahead search
			parts[i] = `"` + t + `"*`
		} else {
			parts[i] = `"` + t + `"`
		}
	}
	return strings.Join(parts, " AND ")
}
