package parser

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseMathFragment(t *testing.T, src string) *html.Node {
	t.Helper()

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}

	math := findFirstElement(doc, "math")
	if math == nil {
		t.Fatalf("no <math> element in source: %s", src)
	}
	return math
}

func findFirstElement(n *html.Node, name string) *html.Node {
	if n.Type == html.ElementNode && strings.EqualFold(n.Data, name) {
		return n
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if got := findFirstElement(c, name); got != nil {
			return got
		}
	}
	return nil
}

func TestMathmlToLatex_Tokens(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"mi letter", `<math><mi>x</mi></math>`, "x"},
		{"mi Greek pi passes through", `<math><mi>π</mi></math>`, "π"},
		{"mi Greek capital Delta passes through", `<math><mi>Δ</mi></math>`, "Δ"},
		{"mi sin passes through", `<math><mi>sin</mi></math>`, "sin"},
		{"mi multichar identifier", `<math><mi>foo</mi></math>`, "foo"},
		{"mi limit operator maps to \\lim", `<math><mi>lim</mi></math>`, `\lim`},
		{"mn integer", `<math><mn>42</mn></math>`, "42"},
		{"mn decimal", `<math><mn>3.14</mn></math>`, "3.14"},
		{"mo ASCII", `<math><mo>+</mo></math>`, "+"},
		{"mo unicode minus passes through", `<math><mo>−</mo></math>`, "−"},
		{"mo times passes through", `<math><mo>×</mo></math>`, "×"},
		{"mo pm passes through", `<math><mo>±</mo></math>`, "±"},
		{"mo leq passes through", `<math><mo>≤</mo></math>`, "≤"},
		{"mo infinity passes through", `<math><mo>∞</mo></math>`, "∞"},
		{"mo big-op sum maps to \\sum", `<math><mo>∑</mo></math>`, `\sum`},
		{"mo big-op int maps to \\int", `<math><mo>∫</mo></math>`, `\int`},
		{"mtext", `<math><mtext>note</mtext></math>`, `\text{note}`},
		{"ms literal", `<math><ms>hello</ms></math>`, `\text{"hello"}`},
		{"mspace", `<math><mi>x</mi><mspace></mspace><mi>y</mi></math>`, `x \, y`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_Layout(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			"mrow concat",
			`<math><mrow><mi>x</mi><mo>+</mo><mi>y</mi></mrow></math>`,
			"x + y",
		},
		{
			"mfrac simple",
			`<math><mfrac><mn>1</mn><mn>2</mn></mfrac></math>`,
			`\frac{1}{2}`,
		},
		{
			"mfrac nested",
			`<math><mfrac><mfrac><mn>1</mn><mn>2</mn></mfrac><mn>3</mn></mfrac></math>`,
			`\frac{\frac{1}{2}}{3}`,
		},
		{
			"msqrt single",
			`<math><msqrt><mi>x</mi></msqrt></math>`,
			`\sqrt{x}`,
		},
		{
			"msqrt expression",
			`<math><msqrt><mi>x</mi><mo>+</mo><mn>1</mn></msqrt></math>`,
			`\sqrt{x + 1}`,
		},
		{
			"mroot cube",
			`<math><mroot><mi>x</mi><mn>3</mn></mroot></math>`,
			`\sqrt[3]{x}`,
		},
		{
			"mpadded passthrough",
			`<math><mpadded><mi>x</mi></mpadded></math>`,
			"x",
		},
		{
			"mstyle passthrough",
			`<math><mstyle><mi>x</mi></mstyle></math>`,
			"x",
		},
		{
			"maction passthrough",
			`<math><maction><mi>x</mi></maction></math>`,
			"x",
		},
		{
			"mphantom wraps",
			`<math><mphantom><mi>x</mi></mphantom></math>`,
			`\phantom{x}`,
		},
		{
			"menclose wraps",
			`<math><menclose><mi>x</mi></menclose></math>`,
			`\boxed{x}`,
		},
		{
			"mfenced default parens",
			`<math><mfenced><mi>a</mi><mi>b</mi></mfenced></math>`,
			`\left(a, b\right)`,
		},
		{
			"mfenced brackets",
			`<math><mfenced open="[" close="]"><mi>a</mi></mfenced></math>`,
			`\left[a\right]`,
		},
		{
			"mfenced custom separator",
			`<math><mfenced separators=";"><mi>a</mi><mi>b</mi></mfenced></math>`,
			`\left(a; b\right)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_Scripts(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			"msup simple base",
			`<math><msup><mi>b</mi><mn>2</mn></msup></math>`,
			"b^{2}",
		},
		{
			"msup compound base",
			`<math><msup><mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow><mn>2</mn></msup></math>`,
			"{x + 1}^{2}",
		},
		{
			"msub",
			`<math><msub><mi>x</mi><mi>i</mi></msub></math>`,
			"x_{i}",
		},
		{
			"msubsup",
			`<math><msubsup><mi>x</mi><mn>0</mn><mn>1</mn></msubsup></math>`,
			"x_{0}^{1}",
		},
		{
			"munder generic",
			`<math><munder><mi>x</mi><mo>~</mo></munder></math>`,
			`\underset{~}{x}`,
		},
		{
			"mover generic",
			`<math><mover><mi>x</mi><mo>−</mo></mover></math>`,
			`\overset{−}{x}`,
		},
		{
			"munder on sum becomes _",
			`<math><munder><mo>∑</mo><mi>i</mi></munder></math>`,
			`\sum_{i}`,
		},
		{
			"mover on lim becomes ^",
			`<math><mover><mi>lim</mi><mi>n</mi></mover></math>`,
			`\lim^{n}`,
		},
		{
			"munderover generic",
			`<math><munderover><mi>x</mi><mi>a</mi><mi>b</mi></munderover></math>`,
			`\underset{a}{\overset{b}{x}}`,
		},
		{
			"munderover on sum becomes _^",
			`<math><munderover><mo>∑</mo><mi>i</mi><mi>n</mi></munderover></math>`,
			`\sum_{i}^{n}`,
		},
		{
			"munderover on int becomes _^",
			`<math><munderover><mo>∫</mo><mn>0</mn><mn>1</mn></munderover></math>`,
			`\int_{0}^{1}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_Matrices(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			"1x1 matrix",
			`<math><mtable><mtr><mtd><mi>a</mi></mtd></mtr></mtable></math>`,
			`\begin{matrix} a \end{matrix}`,
		},
		{
			"2x2 matrix",
			`<math><mtable>` +
				`<mtr><mtd><mi>a</mi></mtd><mtd><mi>b</mi></mtd></mtr>` +
				`<mtr><mtd><mi>c</mi></mtd><mtd><mi>d</mi></mtd></mtr>` +
				`</mtable></math>`,
			`\begin{matrix} a & b \\ c & d \end{matrix}`,
		},
		{
			"2x3 with expressions in cells",
			`<math><mtable>` +
				`<mtr><mtd><mi>x</mi><mo>+</mo><mn>1</mn></mtd><mtd><mn>0</mn></mtd></mtr>` +
				`<mtr><mtd><mn>0</mn></mtd><mtd><mi>y</mi><mo>−</mo><mn>1</mn></mtd></mtr>` +
				`</mtable></math>`,
			`\begin{matrix} x + 1 & 0 \\ 0 & y − 1 \end{matrix}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_Semantics(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			"semantics emits presentation child only",
			`<math><semantics><mi>x</mi><annotation>plain text</annotation></semantics></math>`,
			"x",
		},
		{
			"semantics skips annotation-xml siblings",
			`<math><semantics><mfrac><mn>1</mn><mn>2</mn></mfrac><annotation-xml encoding="MathML-Content">irrelevant</annotation-xml></semantics></math>`,
			`\frac{1}{2}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_CanonicalEquations(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{
			"quadratic formula",
			`<math><mi>x</mi><mo>=</mo>` +
				`<mfrac>` +
				`<mrow><mo>−</mo><mi>b</mi><mo>±</mo>` +
				`<msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>−</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt>` +
				`</mrow>` +
				`<mrow><mn>2</mn><mi>a</mi></mrow>` +
				`</mfrac></math>`,
			`x = \frac{− b ± \sqrt{b^{2} − 4 a c}}{2 a}`,
		},
		{
			"derivative dy over dx",
			`<math><mfrac><mi>dy</mi><mi>dx</mi></mfrac></math>`,
			`\frac{dy}{dx}`,
		},
		{
			"definite integral",
			`<math><munderover><mo>∫</mo><mn>0</mn><mn>1</mn></munderover>` +
				`<mi>f</mi><mo>(</mo><mi>x</mi><mo>)</mo><mi>dx</mi></math>`,
			`\int_{0}^{1} f ( x ) dx`,
		},
		{
			"summation",
			`<math><munderover><mo>∑</mo><mrow><mi>i</mi><mo>=</mo><mn>1</mn></mrow><mi>n</mi></munderover><msup><mi>i</mi><mn>2</mn></msup></math>`,
			`\sum_{i = 1}^{n} i^{2}`,
		},
		{
			"super and subscript stack",
			`<math><msubsup><mi>x</mi><mi>i</mi><mn>2</mn></msubsup></math>`,
			`x_{i}^{2}`,
		},
		{
			"nested fraction",
			`<math><mfrac><mn>1</mn><mrow><mn>1</mn><mo>+</mo><mfrac><mn>1</mn><mi>x</mi></mfrac></mrow></mfrac></math>`,
			`\frac{1}{1 + \frac{1}{x}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if !ok {
				t.Fatalf("ok=false; want true (got %q)", got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMathmlToLatex_Unsupported(t *testing.T) {
	tests := []struct {
		name, src string
	}{
		{"mmultiscripts bails", `<math><mmultiscripts><mi>x</mi><mn>1</mn><mn>2</mn></mmultiscripts></math>`},
		{"merror bails", `<math><merror><mi>e</mi></merror></math>`},
		{"unknown tag bails", `<math><foobar><mi>x</mi></foobar></math>`},
		{"mfrac with one child bails", `<math><mfrac><mi>x</mi></mfrac></math>`},
		{"mroot with three children bails", `<math><mroot><mi>x</mi><mn>3</mn><mn>5</mn></mroot></math>`},
		{"msubsup with two children bails", `<math><msubsup><mi>x</mi><mn>0</mn></msubsup></math>`},
		{"<ms> with brace bails", `<math><ms>oops{here}</ms></math>`},
		{"unsupported descendant bubbles up", `<math><mfrac><mi>x</mi><merror><mi>bad</mi></merror></mfrac></math>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := parseMathFragment(t, tt.src)
			got, ok := mathmlToLatex(n)

			if ok {
				t.Fatalf("ok=true (got %q); want false", got)
			}
		})
	}
}

func TestBeatsXml_SavingsThreshold(t *testing.T) {
	tests := []struct {
		name       string
		xml, latex string
		want       bool
	}{
		{
			"large MathML, tiny LaTeX",
			`<math><mi>x</mi><mo>=</mo><mn>1</mn></math>`,
			"x = 1",
			true,
		},
		{
			"no savings",
			"<math/>",
			"this is much longer than the source",
			false,
		},
		{
			"empty xml never beats",
			"",
			"x",
			false,
		},
		{
			"exactly at threshold passes",
			strings.Repeat("a", 100),
			strings.Repeat("a", 85),
			true,
		},
		{
			"just below threshold fails",
			strings.Repeat("a", 100),
			strings.Repeat("a", 86),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := beatsXml(tt.xml, tt.latex)
			if got != tt.want {
				t.Errorf("beatsXml(len=%d, len=%d) = %v, want %v", len(tt.xml), len(tt.latex), got, tt.want)
			}
		})
	}
}
