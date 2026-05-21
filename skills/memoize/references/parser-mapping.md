# How the parser maps Markdown to nodes

Reference for `memoize`. Load when authoring a **file for the compile plane** (see `references/write-paths.md`) and you need the exact block→node mapping or the automatic compaction rules behind a node's `token_count`.

This applies to **compiled files only** — `MemoryWrite` payloads never reach the parser (they store as one flat `text` node). remindb's Markdown parser is **block-level only** — inline emphasis, links, and code spans flatten into the parent's content and aren't addressable.

| Markdown block | Becomes | Notes |
|---|---|---|
| `#`/`##`/`###` Heading | `heading` node — owns a subtree | Each level pops to the right ancestor (H2 after H3 closes the H3 subtree). DB depth ≈ heading level. |
| Bullet / ordered list | `list` node — single leaf | Items flattened to `- text` (nesting lost). One node, but each item ranks independently in FTS5. |
| Fenced code block | `code` node — single leaf | Language tag prepended as line 1. Empty blocks dropped. |
| Table | `table` node — single leaf | Tab-separated rows, header first. |
| Paragraph | `text` node — single leaf | Soft breaks → space, hard → newline. |
| HTML block | `text` node | Trimmed; empty dropped. |
| Frontmatter (`---\n…\n---` at start, YAML/TOML) | `preamble` node — one, before body | Good for tags / per-doc metadata. |
| Horizontal rule (`---` mid-doc) | **dropped silently** | Don't use `---` to separate sections — use a heading. |

Two consequences: **headings are the *only* tree-building block** (lists/tables are always leaves); **a payload with no headings has no spine** — every block attaches to the sentinel root at depth 1.

## Shape also controls storage size — not just indexing

The same shape choices decide how many tokens a node **costs forever** — the parser compacts per node automatically:

- **Uniform records → TOON.** An array of same-keyed objects (config block, `key: value` rows, comparison matrix) is re-encoded in [TOON](https://github.com/radimsem/remindb/blob/main/docs/toon-encoding.md): shape stated once, values stream — ~40% smaller than YAML/JSON. Kept only on a ≥15% win.
- **MathML → LaTeX.** `<math>…</math>` in HTML is rebuilt as LaTeX by the same ≥15% rule; lossy conversions keep the raw MathML. See [MathML → LaTeX](https://github.com/radimsem/remindb/blob/main/docs/mathml-latex.md).

There is no encoding knob. You only control whether the win is *available*: a real table / uniform `- key: value` list **can** be TOON-compacted; the same data as a prose paragraph **cannot**. Math as `<math>` (or LaTeX) stays compact; hand-expanded into a sentence it stays a sentence. The `format` column records which won — you never author or query it.

**Consequence:** a node's `token_count` can be far below its raw byte size. That's compaction, not truncation — content is whole. Don't `MemoryFetch` a compact table/equation and "re-expand" it into prose on the next write; you'd undo the saving and fragment the index.
