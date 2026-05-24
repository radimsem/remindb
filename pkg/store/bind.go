package store

import "strings"

func bindStrings(vals []string) (clause string, args []any) {
	if len(vals) == 0 {
		return "", nil
	}

	placeholders := make([]string, len(vals))
	args = make([]any, len(vals))
	for i, v := range vals {
		placeholders[i] = "?"
		args[i] = v
	}

	return strings.Join(placeholders, ","), args
}
