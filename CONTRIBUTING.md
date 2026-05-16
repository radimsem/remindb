# Contributing to remindb

Thanks for being here. `remindb` is a one-person project I maintain between classes — patches, ideas, and bug reports are genuinely welcome. This file is the routing map for contributing; the deep rules live in [`.claude/rules/`](./.claude/rules/) and the per-task workflow checklists live in [`.claude/skills/`](./.claude/skills/), so this guide tries to point at them rather than restate them.

<p align="center">
  <a href="https://github.com/radimsem/remindb/issues/new"><img src="https://img.shields.io/badge/issue-bug%20or%20feature-orange" alt="Open an issue" /></a>
  <a href="https://github.com/radimsem/remindb/compare"><img src="https://img.shields.io/badge/pull%20request-open-blue" alt="Open a pull request" /></a>
</p>

## Table of contents

- [Project design and goals](#project-design-and-goals)
- [Ways to contribute](#ways-to-contribute)
- [AI-assisted development](#ai-assisted-development)
- [Branch naming](#branch-naming)
- [Pull request process](#pull-request-process)
- [Local verification](#local-verification)
- [Pre-PR checklist](#pre-pr-checklist)
- [Documentation updates](#documentation-updates)
- [First-time contributors](#first-time-contributors)
- [Recognition](#recognition)
- [Security](#security)
- [License](#license)

## Project design and goals

`remindb` is a token-efficient agentic memory layer in a single SQLite file. The pipeline is `parser → transformer → emitter → store`; the read side is `query → mcp/tools`. See [README "Why I built this"](./README.md#why-i-built-this) for the long version.

**Goals:**

- Token-efficient memory the agent doesn't have to re-skim every session.
- One SQLite file, no daemon, no external state.
- One MCP server reachable from any MCP-capable agent.
- Portable: copy the `.db`, hand it to another machine or another agent.
- No telemetry, ever.

## Ways to contribute

- **Bug reports** — open an [issue](https://github.com/radimsem/remindb/issues/new) with a minimal repro.
- **Feature ideas** — open an issue *first* to align on scope before coding.
- **Code** — PRs against `dev` (never `main`), one logical change per PR.
- **Docs** — typos, clarifications, public skills (`skills/`), private skills (`.claude/skills/`), this guide.
- **Plugin support for new agents** — five worked examples live in [`plugins/`](./plugins/); copy the closest one.
- **Benchmarks** — add a corpus or a scenario in [`internal/bench/scenarios.go`](./internal/bench/scenarios.go) and run [`scripts/bench-agents.sh`](./scripts/bench-agents.sh).

## AI-assisted development

The repo is set up for both human and agent contributors. If you use Claude Code, Codex, Gemini CLI, or any other agent that reads markdown, the rules and workflow skills below let you describe a task in one sentence and have the agent walk the right checklist.

### What's set up for you

| Surface | Path | What's there |
|---|---|---|
| **Style + protocol rules** | [`.claude/rules/`](./.claude/rules/) | `go-concise.md` (Go style), `git-versioning.md` (branches, commits, signing), `mcp-tool-conventions.md` (MCP tool contract), `logging-conventions.md` (slog discipline). |
| **Workflow skills** | [`.claude/skills/`](./.claude/skills/) | One per common task: `add-parser`, `add-mcp-tool`, `add-store-query`, `add-fuzz-target`, `add-integration-test`, `add-bench-scenario`, `tune-temperature-policy`. |
| **Reviewer agents** | [`.claude/agents/`](./.claude/agents/) | `go-style-reviewer`, `mcp-surface-reviewer`, `migration-safety-reviewer` — dispatch before commit on changes that touch the relevant zone. |

### How to use it

If you're using Claude Code, just say what you want in plain English ("add a TOML parser", "expose a new MCP tool", "add a fuzz target for the diff engine") and the agent will find the matching skill in `.claude/skills/` and walk through it. Other agents can read those skills as plain markdown — there's nothing Claude-Code-specific in them.

If you're not using an agent at all, the rules in `.claude/rules/` are the source of truth for code style, git workflow, MCP contract, and logging discipline. Read them like any other contributor doc.

### Public vs. private skills

Two skill folders live side by side; don't confuse them.

- **`skills/remind/`, `skills/memoize/`** — *public* skills shipped to MCP clients. They teach end-user agents how to call the `Memory*` tools (read path and write path respectively). Edit these when the MCP tool surface changes.
- **`.claude/skills/`** — *private* skills for contributors. Workflow checklists for adding parsers, tools, queries, fuzz targets, etc. Edit these when you want to teach future contributors how to do a thing.

### Workflow shortcuts

Condensed from [CLAUDE.md](./CLAUDE.md):

| Task | Skill |
|---|---|
| Add a new file format to the parser | `add-parser` |
| Add or change an MCP `Memory*` tool | `add-mcp-tool` |
| Add a SQL query, column, index, or migration | `add-store-query` |
| Add a `Fuzz*` target or extend a seed corpus | `add-fuzz-target` |
| Add an end-to-end scenario (`integration_test.go`, `mcp_integration_test.go`) | `add-integration-test` |
| Add a token-savings benchmark scenario | `add-bench-scenario` |
| Tune decay / cold / notify thresholds | `tune-temperature-policy` |

## Branch naming

The full spec lives in [`.claude/rules/git-versioning.md`](./.claude/rules/git-versioning.md) §3. The essentials:

- Fork from `dev`, **never** from `main`. `main` is a release-marker branch updated only by squash-PRs from `dev` at release time.
- Use one of four prefixes — the prefix expresses *intent*:

| Prefix | Purpose |
|---|---|
| `feat/<slug>` | New functionality or user-visible behavior |
| `fix/<slug>` | Bug fix landing on `dev` |
| `chore/<slug>` | Non-functional housekeeping — deps, tooling, lint, CI |
| `docs/<slug>` | Documentation only — READMEs, rules, skills |

- Slugs are kebab-case, ≤4 words, descriptive.

| Good | Bad |
|---|---|
| `feat/toml-parser` | `feat/new-stuff` |
| `fix/wal-checkpoint` | `fix/bug` |
| `chore/bump-go-1.24` | `chore/update` |

A change spanning two intents is two PRs, not one branch carrying both.

## Pull request process

1. **Find or open an issue.** Avoid duplicate work — comment on the issue to claim it.
2. **Fork off `dev`** with a sanctioned prefix: `git switch dev && git pull && git switch -c feat/my-thing`.
3. **Implement.** Read the relevant rule from `.claude/rules/` if you're touching MCP tools, migrations, or temperature config.
4. **Sign your commits.** Every commit on `dev` and topic branches must carry a verifiable signature — see [`git-versioning.md`](./.claude/rules/git-versioning.md) §1.
5. **Run local verification** (next section).
6. **Push the branch** and open a PR against `dev`. The [PR template](./.github/PULL_REQUEST_TEMPLATE.md) auto-applies.
7. **Fill the template.** The `Verified` and `Touched` checklists exist so reviewers know what was tested and what surfaces moved. Skip checkboxes that don't apply rather than blank-checking everything.
8. **Wait for review.** I'm a college student maintaining this between classes — I'll get to your PR, but it may take a few days.

## Local verification

### make targets

The main loop:

```bash
make build              # go build ./...
make test               # go test ./...
make test-all           # full suite incl. integration tests
make fuzz               # bounded fuzz pass
make fmt lint tidy      # gofmt / golangci-lint / go mod tidy
```

### Full CI locally with [act](https://github.com/nektos/act)

[`act`](https://github.com/nektos/act) reproduces the GitHub Actions pipeline in Docker. Job names match `.github/workflows/ci.yml`:

```bash
act -j lint     # gofmt drift + golangci-lint
act -j tidy     # go.mod / go.sum drift
act -j build    # make build
act -j test     # go test -race ./...
act -j fuzz     # 5s per fuzz target
act -j vuln     # govulncheck
act push        # full pipeline against the 'push' event
```

Caveats:

- `act` requires Docker.
- First run pulls the runner image (~500 MB).
- Some third-party actions (`golangci-lint-action`, `govulncheck-action`) need network during the run.
- Add `--container-architecture linux/amd64` if you're on Apple Silicon — the default `linux/arm64` runner image is missing some of the toolchain.

## Pre-PR checklist

Before opening the PR, run through this list. Mirrors the PR template's `Verified` + `Touched` sections, with a few additions for the parts CI can't catch.

- [ ] `make fmt lint tidy` produces no diff
- [ ] `make test-all` passes locally
- [ ] `make fuzz` passes (at least the default 5s per target)
- [ ] `act push` (or per-job `act -j <name>`) is green
- [ ] Tested manually via CLI or local MCP plugin install
- [ ] If MCP tool surface changed: `skills/remind/` (read tools) or `skills/memoize/` (write tools) updated; both if the change crosses the boundary
- [ ] If temperature config changed: both public skills reflect the new values
- [ ] If parser changed: a fuzz target covers the change
- [ ] If schema changed: FTS5 triggers in sync (see `add-store-query` skill)
- [ ] If write tool changed: each call still produces exactly one snapshot
- [ ] If CLI surface changed: README CLI section updated; relevant plugin READMEs updated
- [ ] Commit subjects follow [`git-versioning.md`](./.claude/rules/git-versioning.md) §5 — they become release-notes entries
- [ ] PR template filled honestly (skip checkboxes that don't apply)

## Documentation updates

### Doc-update map

If you touch X, update Y. CI won't catch a desynced public skill or stale README — but a reviewer probably will.

| Touched | Update |
|---|---|
| MCP tool added / renamed / removed | `skills/remind/SKILL.md` (read tools) or `skills/memoize/SKILL.md` (write tools); both if the change is shared. README's MCP tools table. |
| Temperature config (`pkg/temperature/Config`) | Both public skills — `skills/remind/` documents the mental model, `skills/memoize/` documents the workflow it triggers. |
| New parser format | README's "Why I built this" formats list (currently *Markdown, HTML, JSON, YAML, TOON*). |
| CLI flag added / removed / renamed | README's CLI section. Each plugin README in `plugins/` that demos the flag. |
| New migration | README's "How it's put together" Store row if the schema description shifts. The `add-store-query` skill if a new convention emerged. |
| New private skill (`.claude/skills/`) | The `Workflow shortcuts` table in CLAUDE.md and (if it's a common task) in this CONTRIBUTING.md. |

### Commit messages drive the changelog

[`.github/workflows/release.yml`](./.github/workflows/release.yml) fires GoReleaser on stable `v*` tags (rc tags excluded by `'!v*-*'`). GoReleaser pulls commit subject lines straight into the release notes — which is why the [`git-versioning.md`](./.claude/rules/git-versioning.md) §5 rule of "subject-only, scoped, imperative" is load-bearing, not pedantic.

Ideal subjects, by scope:

```
feat(parser):   accept UTF-8 BOM at byte 0
feat(mcp):      add MemoryStats tool
fix(emit):      handle empty snapshot transactionally
fix(parser):    skip BOM only at byte 0, not mid-stream
chore(deps):    bump x/sync/errgroup to v0.8
chore(release): cut v0.4.0
docs(readme):   refresh badges, add tip jar
```

Each line is a release-notes entry. "fixed bug" or "WIP" become noise the reader has to mentally skip — please don't ship them.

## First-time contributors

If you want to start small, these are good entry points that don't require deep system knowledge:

- A typo or wording fix in any README under `plugins/` or `skills/`.
- A new test fixture under `testdata/` covering a parser edge case.
- An improvement to a skill description in `.claude/skills/*/SKILL.md` for clarity or trigger reliability.
- A new bench scenario in [`internal/bench/scenarios.go`](./internal/bench/scenarios.go) using the `add-bench-scenario` skill.
- A new `.remindb/ignore` test case.
- Fixing an error message that wasn't actionable.

For larger first contributions, open an issue first and we can scope something together.

## Recognition

Add a `Co-Authored-By: Your Name <email>` trailer to your commits if you'd like attribution. The trailer is preserved in `git log` and rendered on GitHub's commit pages, so your contribution stays attached to the work even if a reviewer squash-merges the topic branch.

## Security

Found a security issue? Don't open a public issue — see [`SECURITY.md`](./SECURITY.md) for the disclosure channels (GitHub private advisory or `security@radimsemerak.cz`) and what to expect after you report.

## License

`remindb` is MIT-licensed. By submitting a PR, you agree your contribution will be released under the same terms. The full text is in [`LICENSE`](./LICENSE).
