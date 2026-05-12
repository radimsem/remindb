package parser

import (
	"strings"
	"testing"
)

func TestParseHtml_Heading(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<h1>H1</h1><h2>H2</h2>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(nodes))
	}

	h1 := nodes[0]
	if h1.NodeType != NodeHeading || h1.Content != "H1" {
		t.Errorf("h1 = {%v, %q}, want {NodeHeading, H1}", h1.NodeType, h1.Content)
	}
	if len(h1.Children) != 1 {
		t.Fatalf("h1.Children = %d, want 1", len(h1.Children))
	}

	h2 := h1.Children[0]
	if h2.NodeType != NodeHeading || h2.Content != "H2" || h2.Depth != 2 {
		t.Errorf("h2 = {%v, %q, depth=%d}, want {NodeHeading, H2, 2}", h2.NodeType, h2.Content, h2.Depth)
	}
}

func TestParseHtml_FrameStack(t *testing.T) {
	src := "<h1>A</h1><h2>A.1</h2><h1>B</h1><p>under B</p>"

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2 (A, B)", len(nodes))
	}
	if nodes[0].Content != "A" || len(nodes[0].Children) != 1 || nodes[0].Children[0].Content != "A.1" {
		t.Errorf("A subtree wrong: %+v", nodes[0])
	}
	if nodes[1].Content != "B" || len(nodes[1].Children) != 1 || nodes[1].Children[0].Content != "under B" {
		t.Errorf("B subtree wrong: %+v", nodes[1])
	}
}

func TestParseHtml_Paragraph(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<p>hello <em>world</em></p>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeText || nodes[0].Content != "hello world" {
		t.Errorf("nodes[0] = {%v, %q}, want {NodeText, \"hello world\"}", nodes[0].NodeType, nodes[0].Content)
	}
}

func TestParseHtml_Link(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte(`<p>see <a href="https://x.com">x</a> for more</p>`))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	const want = "see [x](https://x.com) for more"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_Br(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<p>line one<br>line two<br>line three</p>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	const want = "line one\nline two\nline three"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_BrAbsorbsAdjacentWhitespace(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<p>line one <br> line two</p>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	const want = "line one\nline two"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_Entities(t *testing.T) {
	src := `<p>foo &amp; bar &lt; baz &gt; qux &nbsp;end &copy;2025</p>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	const want = "foo & bar < baz > qux end ©2025"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_InlineImage(t *testing.T) {
	src := `<p>see <img src="diagram.png" alt="arch"> for the architecture</p>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	const want = "see ![arch](diagram.png) for the architecture"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_List(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<ul><li>one</li><li>two</li></ul>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeList {
		t.Fatalf("nodes[0].NodeType = %v, want NodeList", nodes[0].NodeType)
	}
	if nodes[0].Content != "- one\n- two" {
		t.Errorf("list content = %q", nodes[0].Content)
	}
}

func TestParseHtml_OrderedList(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<ol><li>first</li><li>second</li></ol>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeList {
		t.Fatalf("nodes[0].NodeType = %v, want NodeList", nodes[0].NodeType)
	}
	if nodes[0].Content != "- first\n- second" {
		t.Errorf("ordered list content = %q", nodes[0].Content)
	}
}

func TestParseHtml_DefinitionList(t *testing.T) {
	src := `<dl><dt>HTTP</dt><dd>protocol</dd><dt>DNS</dt><dd>name lookup</dd></dl>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeList {
		t.Fatalf("nodes[0].NodeType = %v, want NodeList", nodes[0].NodeType)
	}

	const want = "- HTTP\n- protocol\n- DNS\n- name lookup"
	if nodes[0].Content != want {
		t.Errorf("definition list content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_NestedList(t *testing.T) {
	src := "<ul>" +
		"<li>top1</li>" +
		"<li>top2" +
		"<ul>" +
		"<li>nested1</li>" +
		"<li>nested2" +
		"<ul><li>deep</li></ul>" +
		"</li>" +
		"</ul>" +
		"</li>" +
		"<li>top3</li>" +
		"</ul>"

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeList {
		t.Fatalf("nodes[0].NodeType = %v, want NodeList", nodes[0].NodeType)
	}
	const want = "- top1\n" +
		"- top2\n" +
		"\t- nested1\n" +
		"\t- nested2\n" +
		"\t\t- deep\n" +
		"- top3"

	if nodes[0].Content != want {
		t.Errorf("nested list content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_Table(t *testing.T) {
	src := "<table><tr><th>col1</th><th>col2</th></tr><tr><td>a</td><td>b</td></tr></table>"

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeTable {
		t.Fatalf("nodes[0].NodeType = %v, want NodeTable", nodes[0].NodeType)
	}
	if nodes[0].Content != "col1\tcol2\na\tb" {
		t.Errorf("table content = %q", nodes[0].Content)
	}
}

func TestParseHtml_Code(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<pre><code>line1\nline2</code></pre>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeCode {
		t.Fatalf("nodes[0].NodeType = %v, want NodeCode", nodes[0].NodeType)
	}
	if nodes[0].Content != "line1\nline2" {
		t.Errorf("code content = %q", nodes[0].Content)
	}
}

func TestParseHtml_Embed_Image(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte(`<img src="cat.png" alt="A cat">`))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeEmbed {
		t.Fatalf("nodes[0].NodeType = %v, want NodeEmbed", nodes[0].NodeType)
	}
	if nodes[0].Format != "image" {
		t.Errorf("Format = %q, want image", nodes[0].Format)
	}
	if nodes[0].Content != "![A cat](cat.png)" {
		t.Errorf("Content = %q", nodes[0].Content)
	}
}

func TestParseHtml_Embed_VideoWithSource(t *testing.T) {
	src := `<video><source src="clip.mp4" type="video/mp4"></video>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeEmbed || nodes[0].Format != "video" {
		t.Fatalf("nodes[0] = {%v, %q}, want {NodeEmbed, video}", nodes[0].NodeType, nodes[0].Format)
	}
	if nodes[0].Content != "[](clip.mp4)" {
		t.Errorf("Content = %q", nodes[0].Content)
	}
}

func TestParseHtml_Embed_Iframe(t *testing.T) {
	src := `<iframe src="https://example.com/embed" title="demo"></iframe>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if nodes[0].Format != "iframe" || nodes[0].Content != "[demo](https://example.com/embed)" {
		t.Errorf("got {%q, %q}", nodes[0].Format, nodes[0].Content)
	}
}

func TestParseHtml_Embed_ObjectWithData(t *testing.T) {
	src := `<object data="report.pdf" type="application/pdf" title="Q3 report"></object>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeEmbed || nodes[0].Format != "embed" {
		t.Fatalf("nodes[0] = {%v, %q}, want {NodeEmbed, embed}", nodes[0].NodeType, nodes[0].Format)
	}
	if nodes[0].Content != "[Q3 report](report.pdf)" {
		t.Errorf("Content = %q", nodes[0].Content)
	}
}

func TestParseHtml_InlineSvg(t *testing.T) {
	src := `<svg viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeCode || nodes[0].Format != "svg" {
		t.Fatalf("nodes[0] = {%v, %q}, want {NodeCode, svg}", nodes[0].NodeType, nodes[0].Format)
	}

	content := nodes[0].Content
	if strings.HasPrefix(content, "<svg") {
		t.Errorf("content should strip the <svg> wrapper, got %q", content)
	}

	attrLine, inner, ok := strings.Cut(content, "\n")
	if !ok {
		t.Fatalf("expected attrs line + inner markup separated by \\n, got %q", content)
	}
	if attrLine != `viewBox="0 0 10 10"` {
		t.Errorf("attrs line = %q, want %q", attrLine, `viewBox="0 0 10 10"`)
	}
	if !strings.Contains(inner, "rect") || !strings.Contains(inner, `width="10"`) {
		t.Errorf("inner markup missing rect data: %q", inner)
	}
}

func TestParseHtml_InlineCanvas(t *testing.T) {
	src := `<canvas width="800" height="600">Pie chart: 60% A, 40% B.</canvas>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 1 || nodes[0].NodeType != NodeCode || nodes[0].Format != "canvas" {
		t.Fatalf("nodes[0] = {%v, %q}, want {NodeCode, canvas}", nodes[0].NodeType, nodes[0].Format)
	}

	const want = `width="800" height="600"` + "\nPie chart: 60% A, 40% B."
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestParseHtml_StripsScriptStyleComment(t *testing.T) {
	src := `<p>visible</p><script>alert("bad")</script><style>p{color:red}</style><!-- comment --><p>also visible</p>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2 (only the two paragraphs)", len(nodes))
	}

	if nodes[0].Content != "visible" {
		t.Errorf("nodes[0].Content = %q, want %q", nodes[0].Content, "visible")
	}
	if nodes[1].Content != "also visible" {
		t.Errorf("nodes[1].Content = %q, want %q", nodes[1].Content, "also visible")
	}

	for _, n := range nodes {
		if strings.Contains(n.Content, "alert") || strings.Contains(n.Content, "color:red") || strings.Contains(n.Content, "comment") {
			t.Errorf("node leaked stripped content: %q", n.Content)
		}
	}
}

func TestParseHtml_Math(t *testing.T) {
	src := `<p>before</p><math><mi>x</mi><mo>=</mo><mn>1</mn></math><p>after</p>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 3 {
		t.Fatalf("len(nodes) = %d, want 3", len(nodes))
	}

	math := nodes[1]
	if math.NodeType != NodeCode || math.Format != FormatMathml {
		t.Errorf("math = {%v, %q}, want {NodeCode, mathml}", math.NodeType, math.Format)
	}
	if !strings.Contains(math.Content, "<math") {
		t.Errorf("math content missing markup: %q", math.Content)
	}
}

func TestParseHtml_SectioningLandmarks(t *testing.T) {
	src := `<article><section><h1>A</h1><p>body</p></section><section><h1>B</h1></section></article>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2 (A and B at top level)", len(nodes))
	}
	if nodes[0].Content != "A" || len(nodes[0].Children) != 1 || nodes[0].Children[0].Content != "body" {
		t.Errorf("section A wrong: %+v", nodes[0])
	}
	if nodes[1].Content != "B" {
		t.Errorf("section B wrong: %+v", nodes[1])
	}
}

func TestParseHtml_Blockquote(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<blockquote>quoted prose</blockquote>"))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if nodes[0].NodeType != NodeText || nodes[0].Content != "quoted prose" {
		t.Errorf("blockquote = {%v, %q}", nodes[0].NodeType, nodes[0].Content)
	}
}

func TestParseHtml_FigureWithCaption(t *testing.T) {
	src := `<figure><img src="g.png" alt="graph"><figcaption>Figure 1</figcaption></figure>`

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2 (embed + caption)", len(nodes))
	}
	if nodes[0].NodeType != NodeEmbed {
		t.Errorf("nodes[0] = %v, want NodeEmbed", nodes[0].NodeType)
	}
	if nodes[1].NodeType != NodeText || nodes[1].Content != "Figure 1" {
		t.Errorf("nodes[1] = {%v, %q}", nodes[1].NodeType, nodes[1].Content)
	}
}

func TestParseHtml_Empty(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte(""))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if len(nodes) != 0 {
		t.Errorf("len(nodes) = %d, want 0", len(nodes))
	}
}

func TestParseHtml_MalformedRecovers(t *testing.T) {
	nodes, err := parseHtml("doc.html", []byte("<p>hi<span>oops"))
	if err != nil {
		t.Fatalf("parseHtml on malformed input: %v", err)
	}

	if len(nodes) == 0 || nodes[0].Content == "" {
		t.Errorf("expected to recover some content, got %+v", nodes)
	}
}

func TestParseHtml_WhitespaceCollapse(t *testing.T) {
	src := "<p>  one    two\n\nthree  </p>"

	nodes, err := parseHtml("doc.html", []byte(src))
	if err != nil {
		t.Fatalf("parseHtml: %v", err)
	}

	if nodes[0].Content != "one two three" {
		t.Errorf("Content = %q, want %q", nodes[0].Content, "one two three")
	}
}

func TestParseHtml_MarkdownParity(t *testing.T) {
	md := "# Title\n\nIntro prose.\n\n- one\n- two\n\n## Sub\n\nMore prose.\n"
	htm := "<h1>Title</h1><p>Intro prose.</p><ul><li>one</li><li>two</li></ul><h2>Sub</h2><p>More prose.</p>"

	mdNodes, err := ParseBytes("doc.md", []byte(md))
	if err != nil {
		t.Fatalf("md ParseBytes: %v", err)
	}
	htmlNodes, err := ParseBytes("doc.html", []byte(htm))
	if err != nil {
		t.Fatalf("html ParseBytes: %v", err)
	}

	mdShape := shapeString(mdNodes)
	htmlShape := shapeString(htmlNodes)

	if mdShape != htmlShape {
		t.Errorf("shape mismatch:\nmd:\n%s\nhtml:\n%s", mdShape, htmlShape)
	}
}

func shapeString(nodes []*ContextNode) string {
	var sb strings.Builder
	writeShape(&sb, nodes, 0)
	return sb.String()
}

func writeShape(sb *strings.Builder, nodes []*ContextNode, depth int) {
	for _, n := range nodes {
		for range depth {
			sb.WriteString("  ")
		}

		sb.WriteString(string(n.NodeType))
		sb.WriteByte(' ')
		sb.WriteString(n.Content)
		sb.WriteByte('\n')

		writeShape(sb, n.Children, depth+1)
	}
}
