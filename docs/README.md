# remindb docs

The [main README](../README.md) is the pitch — the problem, the feature list, install, the MCP tool table, the benchmarks. This folder is the depth behind it.

If you got here from a feature bullet in the README, you're in the right place. Each page below stands on its own: it opens with the problem it solves, then how remindb solves it, in plain language.

## How it works

Start here if you want to understand the design rather than just use it.

- **[The node tree](./node-tree.md)** — why memory is a tree of typed nodes, not a folder of files. The `MemoryTree` orientation call.
- **[Temperature](./temperature.md)** — hot/cold decay, and the in-band cold-node summarization loop that fires without a cron.
- **[Versioning](./versioning.md)** — snapshots, diffs, and the tiny-payload resync (`MemoryDelta` / `MemoryDiff` / `MemoryHistory`).
- **[Search](./search.md)** — FTS5 ranked search, the OR-rewrite you must know, and budgeted fetching.
- **[TOON encoding](./toon-encoding.md)** — when uniform structure stores ~40% smaller, and why prose stays plain.
- **[MathML → LaTeX](./mathml-latex.md)** — the same ≥15% rule applied to math: MathML in HTML becomes compact LaTeX.
- **[Knowledge graph](./knowledge-graph.md)** — `[[wiki-link]]` edges, weighted traversal, and why relations are a sideband.
- **[Architecture](./architecture.md)** — the layer-by-layer map of the whole pipeline.

## Running it

- **[CLI reference](./cli.md)** — every subcommand: `compile`, `serve`, `inspect`, `bench`, `doctor`, `update`.
- **[Configuration](./configuration.md)** — the `.remindb/` directory: `config.json` feature blocks, `ignore`, `temperatures.json`.

## Contributing to the docs

Same rules as the rest of the project — see [CONTRIBUTING.md](../CONTRIBUTING.md). Docs changes go on a `docs/<slug>` branch off `dev`. The voice here is deliberately human: first person, problem-then-fix, no manual-speak. Match it.
