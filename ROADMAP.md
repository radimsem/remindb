# Roadmap

Candidate features for upcoming releases.

- [ ] TOML parser
- [ ] Plain-text fallback parser (`.txt`, files without extension)
- [ ] CSV / TSV parser
- [ ] HTML parser (saved web pages)
- [ ] `MemoryStats` tool — snapshot/node/token counts, hot-cold distribution, last cursor
- [ ] `MemoryDelete` tool — first-class explicit node removal
- [ ] `MemoryRollback` tool — one-call rollback to a prior snapshot via stored old content
- [ ] `MemoryRelate` tool — cross-tree links / "see also" semantics
- [ ] `sqlite-vec` embeddings stored alongside nodes in the same `.db` file
- [ ] Hybrid ranking — BM25 × cosine × temperature with tunable weights
- [ ] Pluggable embedding model — local ONNX default, optional remote API
- [ ] Parallel file parse in `compile` (errgroup, `GOMAXPROCS`)
- [ ] HTTP / SSE MCP transport alongside stdio
- [ ] Bearer-token auth for the HTTP transport
- [ ] Homebrew tap
- [ ] Nix flake
- [ ] AUR package
- [ ] Cursor IDE plugin
- [ ] Zed plugin
- [ ] Generic VS Code MCP plugin
- [ ] Prometheus / OTel metrics endpoint
- [ ] Bundled read-only html dump over the `inspect` surface
- [ ] Bench scenarios for source-code corpora
- [ ] `MemoryFetch` format — drop the per-node label header (duplicates content for heading nodes)
