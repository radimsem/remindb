# remindb

Token-efficient agentic memory database with an MCP interface.

- **Language:** Go
- **Storage:** SQLite
- **Protocol:** MCP
- **Format:** TOON

Status: pre-implementation. See [`docs/PLAN.md`](docs/PLAN.md) for the full
architecture and implementation plan.

## Benchmarks

Token savings compared to raw file operations (Read, Grep, List) that
agents fall back to without a structured memory layer. Each bar shows
the percentage of tokens saved.

**Orientation** — `MemoryTree` vs listing + reading every file:

```
codex        █████████████████▌································  ~35%   ~6.1k → ~4.0k  tok
openclaw     ████████████▌·····································  ~25%  ~19.6k → ~14.6k tok
gemini-cli   ████████████▎·····································  ~24%   ~3.1k → ~2.3k  tok
claude-code  ██████████▊·······································  ~22%   ~4.5k → ~3.5k  tok
```

**Search** — `MemorySearch` + `MemoryFetch` vs grep + reading matched files:

```
targeted/1k  █████████████████████████████████████████████▊····  ~92%  ~17.8k → ~1.5k tok
broad/2k     ██████████████████████████████████████████████▌···  ~93%  ~20.6k → ~1.4k tok
narrow/500   ████████████████████████████████████████████████▎·  ~97%  ~18.4k → ~0.6k tok
```

**Fetch** — `MemoryFetch` with token budget vs reading the whole file:

```
budget 500   ███████████████████████████████████████████████···  ~94%  ~13.1k → ~0.8k tok
budget 1000  ████████████████████████████████████████████▍·····  ~89%  ~13.1k → ~1.5k tok
budget 2000  ███████████████████████████████████████▉··········  ~80%  ~13.1k → ~2.6k tok
budget 4000  █████████████████████████████████████▉············  ~76%  ~13.1k → ~3.2k tok
```

**Delta** — `MemoryDelta` vs re-reading the modified file:

```
delta        ███████████████████████████████████████████████·   ~94%   ~0.9k → ~0.05k tok
```

**Session** — orient, explore, deep-read, modify, re-orient, follow-up, modify, verify:

```
openclaw     ████████████████████████████████··················  ~64%  ~101.5k → ~36.5k tok
codex        ███████████████████████████·······················  ~54%   ~34.4k → ~15.8k tok
claude-code  ██████████████████████████▎·······················  ~53%   ~31.4k → ~14.9k tok
gemini-cli   ██████████████████████▋···························  ~45%   ~19.2k → ~10.5k tok
```

> [!NOTE]
> These benchmarks use small sample corpora (~3k–19k tokens) to reduce repository size.
> A real-world knowledge base like an Obsidian vault with 100 articles (~300k words)
> would push session savings to an estimated **85–92%**, because the budget-bounded
> fetch and compact search become increasingly selective as the corpus grows.

Run the benchmarks yourself:

```bash
go test -run='^$' -bench='BenchmarkTokens' -benchtime=1x .
```

## License

MIT — see [`LICENSE`](LICENSE).
