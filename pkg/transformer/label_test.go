package transformer

import (
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestLabel_Heading(t *testing.T) {
	n := &parser.ContextNode{NodeType: parser.NodeHeading, Content: "Installation Guide"}
	setLabel(n)
	if n.Label != "Installation Guide" {
		t.Errorf("Label = %q", n.Label)
	}
}

func TestLabel_Code(t *testing.T) {
	n := &parser.ContextNode{NodeType: parser.NodeCode, Content: "go\nfunc main() {}"}
	setLabel(n)
	const want = "Code (go): func main() {}"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_CodeNoLang(t *testing.T) {
	n := &parser.ContextNode{NodeType: parser.NodeCode, Content: "echo hello world"}
	setLabel(n)
	const want = "Code: echo hello world"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_List(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeList,
		Format:   parser.FormatPlain,
		Content:  "- first item\n- second item\n- third item",
	}
	setLabel(n)
	const want = "3-item list: first item"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_ListToon(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeList,
		Format:   parser.FormatToon,
		Content:  "users[5]{id,name}:\n  1,alice\n  2,bob\n  3,carol\n  4,dave\n  5,eve",
	}
	setLabel(n)
	const want = "users[5]{id,name}"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_Table(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeTable,
		Content:  "Name\tAge\nAlice\t30\nBob\t25",
	}
	setLabel(n)
	const want = "Table: Name, Age (2 rows)"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_Text(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeText,
		Content:  "This is the first sentence. And this is the second.",
	}
	setLabel(n)
	const want = "This is the first sentence."
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_KV(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeKV,
		Content:  "server:\n  host: localhost\n  port: 8080",
	}
	setLabel(n)
	if n.Label != "server" {
		t.Errorf("Label = %q, want %q", n.Label, "server")
	}
}

func TestLabel_KVMultiKey(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeKV,
		Content:  "a: 1\nb: 2\nc: 3\nd: 4",
	}
	setLabel(n)
	if n.Label != "a, b, c" {
		t.Errorf("Label = %q, want %q", n.Label, "a, b, c")
	}
}

func TestLabel_Preamble(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodePreamble,
		Content:  "title: My Doc\nauthor: Alice\ndate: 2024-01-01",
	}
	setLabel(n)
	const want = "Preamble: title, author, date"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_EmbedImage(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeEmbed,
		Format:   "image",
		Content:  "![A cat](cat.png)",
	}
	setLabel(n)

	const want = "Image: A cat"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_EmbedVideoNoAlt(t *testing.T) {
	n := &parser.ContextNode{
		NodeType: parser.NodeEmbed,
		Format:   "video",
		Content:  "[](clip.mp4)",
	}
	setLabel(n)

	const want = "Video: clip.mp4"
	if n.Label != want {
		t.Errorf("Label = %q, want %q", n.Label, want)
	}
}

func TestLabel_Truncation(t *testing.T) {
	long := strings.Repeat("x", 100)
	n := &parser.ContextNode{NodeType: parser.NodeHeading, Content: long}
	setLabel(n)

	if len(n.Label) != maxLabelLen {
		t.Errorf("Label len = %d, want %d", len(n.Label), maxLabelLen)
	}
	if !strings.HasSuffix(n.Label, "...") {
		t.Errorf("Label should end with '...'")
	}
}
