package parser

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// WikilinkRef captures one wiki-link reference extracted from source text.
type WikilinkRef struct {
	Label      string
	SourceQual string
	IDHint     string
	Weight     float64
}

const wikilinkIDLen = 11

var (
	wikilinkRe    = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	kvCandidateRe = regexp.MustCompile(`^\s*\w+\s*=`)
)

func ExtractWikilinks(text string) (string, []WikilinkRef) {
	matches := wikilinkRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}

	var (
		sb   strings.Builder
		refs []WikilinkRef
		last int
	)
	sb.Grow(len(text))

	for _, m := range matches {
		matchStart, matchEnd := m[0], m[1]
		innerStart, innerEnd := m[2], m[3]

		ref, ok := parseWikilink(text[innerStart:innerEnd])
		if !ok {
			sb.WriteString(text[last:matchEnd])
			last = matchEnd
			continue
		}

		sb.WriteString(text[last:matchStart])
		sb.WriteString("[[")
		sb.WriteString(ref.Label)
		sb.WriteString("]]")

		refs = append(refs, ref)
		last = matchEnd
	}
	sb.WriteString(text[last:])

	return sb.String(), refs
}

func parseWikilink(inner string) (WikilinkRef, bool) {
	ref := WikilinkRef{Weight: 1.0}
	parts := strings.Split(inner, ";")
	paramFound := slices.ContainsFunc(parts[1:], kvCandidateRe.MatchString)

	if !paramFound {
		label := strings.TrimSpace(inner)
		if label == "" {
			return ref, false
		}

		ref.Label = label
		if isBareID(label) {
			ref.IDHint = label
		}
		return ref, true
	}

	label := strings.TrimSpace(parts[0])
	if label == "" {
		return ref, false
	}

	ref.Label = label
	if isBareID(label) {
		ref.IDHint = label
	}

	for _, p := range parts[1:] {
		applyWikilinkParam(&ref, p)
	}
	return ref, true
}

// Apply one "key=value" segment to ref.
func applyWikilinkParam(ref *WikilinkRef, segment string) {
	rawKey, rawVal, ok := strings.Cut(segment, "=")
	if !ok {
		return
	}

	key := strings.TrimSpace(rawKey)
	val := strings.TrimSpace(rawVal)

	switch key {
	case "w":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			ref.Weight = f
		}
	case "source":
		ref.SourceQual = val
	case "id":
		ref.IDHint = val
	}
}

func isBareID(s string) bool {
	if len(s) != wikilinkIDLen {
		return false
	}

	for _, r := range s {
		if !isBase62(r) {
			return false
		}
	}
	return true
}

func isBase62(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z')
}
