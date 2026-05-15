package parser

import (
	"reflect"
	"testing"
)

func TestExtractWikilinks_NoMatches(t *testing.T) {
	in := "no wiki links here, just [text](url) and `code`."
	out, refs := ExtractWikilinks(in)

	if out != in {
		t.Errorf("text mutated when no matches: got %q, want %q", out, in)
	}
	if refs != nil {
		t.Errorf("refs = %+v, want nil", refs)
	}
}

func TestExtractWikilinks_PlainLabel(t *testing.T) {
	out, refs := ExtractWikilinks("See [[Architecture]] for details.")

	if out != "See [[Architecture]] for details." {
		t.Errorf("output = %q", out)
	}
	if len(refs) != 1 {
		t.Fatalf("len(refs) = %d, want 1", len(refs))
	}

	want := WikilinkRef{Label: "Architecture", Weight: 1.0}
	if refs[0] != want {
		t.Errorf("ref = %+v, want %+v", refs[0], want)
	}
}

func TestExtractWikilinks_WithWeight(t *testing.T) {
	out, refs := ExtractWikilinks("[[Architecture; w=2.76]]")

	if out != "[[Architecture]]" {
		t.Errorf("normalized output = %q, want [[Architecture]]", out)
	}
	if refs[0].Weight != 2.76 {
		t.Errorf("weight = %f, want 2.76", refs[0].Weight)
	}
}

func TestExtractWikilinks_WithSource(t *testing.T) {
	out, refs := ExtractWikilinks("[[Architecture; w=2.76; source=docs/ARCH.md]]")

	if out != "[[Architecture]]" {
		t.Errorf("normalized output = %q", out)
	}
	want := WikilinkRef{Label: "Architecture", SourceQual: "docs/ARCH.md", Weight: 2.76}
	if refs[0] != want {
		t.Errorf("ref = %+v, want %+v", refs[0], want)
	}
}

func TestExtractWikilinks_WithExplicitID(t *testing.T) {
	out, refs := ExtractWikilinks("[[Architecture; w=2.76; id=3kGXxidmWBp]]")

	if out != "[[Architecture]]" {
		t.Errorf("normalized output = %q", out)
	}
	want := WikilinkRef{Label: "Architecture", IDHint: "3kGXxidmWBp", Weight: 2.76}
	if refs[0] != want {
		t.Errorf("ref = %+v, want %+v", refs[0], want)
	}
}

func TestExtractWikilinks_BareID(t *testing.T) {
	out, refs := ExtractWikilinks("[[3kGXxidmWBp]]")

	if out != "[[3kGXxidmWBp]]" {
		t.Errorf("normalized output = %q, want [[3kGXxidmWBp]]", out)
	}
	want := WikilinkRef{Label: "3kGXxidmWBp", IDHint: "3kGXxidmWBp", Weight: 1.0}
	if refs[0] != want {
		t.Errorf("ref = %+v, want %+v", refs[0], want)
	}
}

func TestExtractWikilinks_BareIDWithWeight(t *testing.T) {
	out, refs := ExtractWikilinks("[[3kGXxidmWBp; w=3.0]]")

	if out != "[[3kGXxidmWBp]]" {
		t.Errorf("normalized output = %q", out)
	}
	if refs[0].IDHint != "3kGXxidmWBp" {
		t.Errorf("IDHint = %q, want 3kGXxidmWBp (auto-detected)", refs[0].IDHint)
	}
	if refs[0].Weight != 3.0 {
		t.Errorf("weight = %f", refs[0].Weight)
	}
}

// A label with semicolons but no key=value segments must not be split.
func TestExtractWikilinks_SemicolonHeading(t *testing.T) {
	out, refs := ExtractWikilinks("[[What; why; how]]")

	if out != "[[What; why; how]]" {
		t.Errorf("normalized output = %q, want [[What; why; how]]", out)
	}
	if refs[0].Label != "What; why; how" {
		t.Errorf("label = %q, want full string preserved", refs[0].Label)
	}
}

// Bad floats and unknown keys are ignored, not fatal.
func TestExtractWikilinks_MalformedParamsTolerated(t *testing.T) {
	out, refs := ExtractWikilinks("[[X; w=not-a-number; unknown=yes]]")

	if out != "[[X]]" {
		t.Errorf("normalized output = %q", out)
	}
	if refs[0].Weight != 1.0 {
		t.Errorf("weight = %f, want 1.0 (bad float should fall back to default)", refs[0].Weight)
	}
	if refs[0].Label != "X" {
		t.Errorf("label = %q, want X", refs[0].Label)
	}
}

func TestExtractWikilinks_EmptyAndWhitespaceSkipped(t *testing.T) {
	cases := []string{"[[]]", "[[ ]]", "[[ \t ]]"}

	for _, in := range cases {
		out, refs := ExtractWikilinks(in)
		if out != in {
			t.Errorf("input %q: output = %q, want unchanged", in, out)
		}
		if refs != nil {
			t.Errorf("input %q: refs = %+v, want nil", in, refs)
		}
	}
}

func TestExtractWikilinks_MultipleInOneText(t *testing.T) {
	in := "See [[A]] and [[B; w=2]] for context."
	out, refs := ExtractWikilinks(in)

	want := "See [[A]] and [[B]] for context."
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2", len(refs))
	}

	if refs[0].Label != "A" || refs[1].Label != "B" {
		t.Errorf("refs out of order: %+v", refs)
	}
	if refs[1].Weight != 2.0 {
		t.Errorf("refs[1].Weight = %f, want 2.0", refs[1].Weight)
	}
}

func TestExtractWikilinks_TriplyNestedBrackets(t *testing.T) {
	// [[[X]]] should yield one extracted [[X]], leaving [..] surrounding it.
	out, refs := ExtractWikilinks("[[[X]]]")

	if out != "[[[X]]]" {
		t.Errorf("output = %q", out)
	}
	if len(refs) != 1 || refs[0].Label != "X" {
		t.Errorf("refs = %+v, want one ref labeled X", refs)
	}
}

// Inner brackets prevent the regex from matching (no nested wiki-links).
func TestExtractWikilinks_NestedBracketsDoNotMatch(t *testing.T) {
	in := "[[a [[b]] c]]"
	out, refs := ExtractWikilinks(in)

	// Only the innermost [[b]] is extracted; the surrounding [[a ...c]] is not matched.
	want := "[[a [[b]] c]]"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if len(refs) != 1 || refs[0].Label != "b" {
		t.Errorf("refs = %+v, want one ref labeled b", refs)
	}
}

func TestExtractWikilinks_LabelTrimmed(t *testing.T) {
	_, refs := ExtractWikilinks("[[  Architecture  ; w=1.0]]")

	if refs[0].Label != "Architecture" {
		t.Errorf("label = %q, want trimmed Architecture", refs[0].Label)
	}
}

func TestExtractWikilinks_IDHintFalsePositives(t *testing.T) {
	// 10-char and 12-char strings must not be detected as IDs.
	cases := []struct {
		in   string
		want string // expected IDHint
	}{
		{"[[abcdefghij]]", ""},             // 10 chars
		{"[[abcdefghijkl]]", ""},           // 12 chars
		{"[[abcdefghij-]]", ""},            // 11 chars but non-base62
		{"[[abcdefghij1]]", "abcdefghij1"}, // 11 chars, base62 only
	}

	for _, tc := range cases {
		_, refs := ExtractWikilinks(tc.in)
		if refs[0].IDHint != tc.want {
			t.Errorf("input %q: IDHint = %q, want %q", tc.in, refs[0].IDHint, tc.want)
		}
	}
}

// Whole-table round-trip: parse the rewritten output again and confirm idempotence.
func TestExtractWikilinks_IdempotentOnRewrittenOutput(t *testing.T) {
	in := "[[Architecture; w=2.76; source=docs/ARCH.md]]"
	first, refs1 := ExtractWikilinks(in)
	second, refs2 := ExtractWikilinks(first)

	if first != second {
		t.Errorf("second pass mutated text: %q → %q", first, second)
	}

	// On the second pass, no params remain so refs are simpler — only Label populated.
	if len(refs2) != 1 || refs2[0].Label != refs1[0].Label {
		t.Errorf("second-pass refs = %+v", refs2)
	}
	// Weight on the second pass resets to default 1.0 (no params in the normalized form).
	if refs2[0].Weight != 1.0 || refs2[0].SourceQual != "" {
		t.Errorf("second-pass should have lost params, got %+v", refs2[0])
	}
}

// Ensure non-overlapping refs maintain their index order.
func TestExtractWikilinks_OrderPreserved(t *testing.T) {
	out, refs := ExtractWikilinks("[[a]][[b]][[c]]")

	if out != "[[a]][[b]][[c]]" {
		t.Errorf("output = %q", out)
	}

	labels := []string{refs[0].Label, refs[1].Label, refs[2].Label}
	if !reflect.DeepEqual(labels, []string{"a", "b", "c"}) {
		t.Errorf("labels = %v, want [a b c]", labels)
	}
}
