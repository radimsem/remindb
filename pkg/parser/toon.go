package parser

import (
	"strings"

	toon "github.com/toon-format/toon-go"
)

// ToonSavingsThreshold is the minimum fraction of bytes TOON must save versus plain rendering before it is preferred.
const ToonSavingsThreshold = 0.15

// Try TOON encoding for items under key. Returns (encoded, true) when the array is TOON-friendly and the encoded form beats plain by ToonSavingsThreshold.
func tryToonList(key string, items []any, plain string) (string, bool) {
	if !isToonFriendly(items) {
		return "", false
	}

	var v any = items
	if key != "" {
		v = map[string]any{key: items}
	}

	encoded, err := toon.MarshalString(v)
	if err != nil {
		return "", false
	}

	encoded = strings.TrimRight(encoded, "\n")
	if !beatsPlain(plain, encoded) {
		return "", false
	}

	return encoded, true
}

// A list is TOON-friendly when every element is a scalar, or every element is a map with identical scalar-keyed fields.
func isToonFriendly(items []any) bool {
	if len(items) == 0 {
		return false
	}

	first, isObj := items[0].(map[string]any)
	if !isObj {
		for _, it := range items {
			if !isScalar(it) {
				return false
			}
		}
		return true
	}

	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok || len(m) != len(first) {
			return false
		}
		for k, v := range m {
			if _, ok := first[k]; !ok {
				return false
			}
			if !isScalar(v) {
				return false
			}
		}
	}
	return true
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, map[any]any, []any:
		return false
	}
	return true
}

func beatsPlain(plain, encoded string) bool {
	if len(plain) == 0 {
		return false
	}
	saved := float64(len(plain)-len(encoded)) / float64(len(plain))
	return saved >= ToonSavingsThreshold
}
