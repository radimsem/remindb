# MathML → LaTeX — math that doesn't cost a paragraph

> A single equation in MathML can be 300 bytes of angle brackets. The same equation in LaTeX is 40. Memory should store the 40.

[← back to README](../README.md) · related: [TOON encoding](./toon-encoding.md) · [the node tree](./node-tree.md) · [search](./search.md)

## The problem

HTML notes carry math as **MathML** — the browser-native `<math>…</math>` tree. It renders fine, but as *stored memory* it's the same mistake TOON exists to fix: structural overhead the agent pays for on every read. A quadratic formula is a dozen nested `<mrow>`, `<msup>`, `<mfrac>` elements wrapping a handful of actual symbols. The agent doesn't need the tree — it needs the equation.

LaTeX says the same thing in a fraction of the bytes, and every model already reads it fluently. So when remindb ingests HTML, it doesn't store MathML verbatim if it can help it.

## What the converter does

On ingest, each MathML subtree is walked by [`pkg/parser/mathml.go`](../pkg/parser/mathml.go) and rebuilt as LaTeX — `<mfrac>` → `\frac{…}{…}`, `<msqrt>` → `\sqrt{…}`, `<msup>` → `^{…}`, sums/integrals → `\sum` / `\int`, matrices → `\begin{matrix}…\end{matrix}`, and so on.

It's the **same ≥15% rule as [TOON](./toon-encoding.md)**, applied to math instead of structure:

- If the LaTeX is at least **15% smaller** than the raw MathML XML (`MathmlSavingsThreshold = 0.15`), the node stores the LaTeX with `format = "latex"`.
- If it isn't — a trivially short expression, or a construct the converter can't faithfully map (it bails rather than emit wrong math) — the raw MathML is kept verbatim with `format = "mathml"`. Correctness first; the saving is only taken when it's real.

Either way it lands on a single `code` node in [the tree](./node-tree.md), and the `format` column records which form won — so nothing downstream has to guess.

## A worked example

This MathML for the quadratic formula:

```xml
<math xmlns="http://www.w3.org/1998/Math/MathML">
  <mi>x</mi><mo>=</mo>
  <mfrac>
    <mrow>
      <mo>−</mo><mi>b</mi><mo>±</mo>
      <msqrt>
        <msup><mi>b</mi><mn>2</mn></msup>
        <mo>−</mo><mn>4</mn><mi>a</mi><mi>c</mi>
      </msqrt>
    </mrow>
    <mrow><mn>2</mn><mi>a</mi></mrow>
  </mfrac>
</math>
```

— roughly **340 bytes** — converts to:

```latex
x = \frac{− b ± \sqrt{b^{2} − 4 a c}}{2 a}
```

— roughly **42 bytes**. That's an ~88% cut, far past the 15% floor, so the node stores the LaTeX. (Symbols the converter doesn't have a command for — here `−` U+2212 and `±` — pass through verbatim rather than being dropped or guessed; big operators like `∑ ∏ ∫` *do* map to `\sum \prod \int`.)

The agent reads one short line instead of a 14-element tree, every session, forever — and it's a line the model already understands without being told what MathML is.

## Why this is invisible to you

You author HTML with `<math>` in it like you always would. You never pick a representation. The only place the choice surfaces is the token count: a converted equation is simply cheaper to fetch, exactly like a TOON-encoded table — and `MemoryStats` / [search](./search.md) budgets stretch further because of it.
