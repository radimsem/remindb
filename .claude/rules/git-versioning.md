# Git Versioning

Rules for managing repository state with `git` in `remindb`.

**Use when:** committing, branching, tagging, merging, or any operation that writes to `.git`.

**Branching model:** `dev` is the integration trunk; `main` is a fast-forward-only pointer to the latest stable release tag. Topic branches (`feat/`, `fix/`, `chore/`, `docs/`) fork from `dev` and squash-merge back into `dev`. Pre-release tags (`vX.Y.Z-rc.N`) live on `dev`. Stable tags (`vX.Y.Z`) are cut on `dev`, then `main` is fast-forwarded to that commit. Patches to non-current minors live on lazily-created `release/vX.Y` branches and never merge back. Commit messages are subject-only — no descriptive body.

**Priority when rules conflict:** signed provenance > atomic commits > clean linear history > speed.

---

## 1. Every Commit Must Be Signed ★

Every commit on `dev`, `main`, `feat/*`, and `release/*` must carry a verifiable signature by the user.
Before the first commit of a session, verify the **local** config:

```bash
git config --local user.signingkey   # non-empty: key id, fingerprint, or SSH pubkey path
git config --local commit.gpgsign    # must be "true"
git config --local gpg.format        # openpgp | ssh | x509 — must match the key type
```

If any of the three is missing or wrong, **stop and ask the user to set them**.
Do not proceed with `--no-gpg-sign`, `-c commit.gpgsign=false`, or `-c user.signingkey=...` overrides. Those flags exist for emergencies the user did not sanction.

### After each commit, confirm the signature attached

```bash
git log -1 --show-signature
```

A correct run prints `Good signature` (GPG) or `Good "…" signature` (SSH).
If the output shows `gpg: skipped`, `No signature`, or `error: …`, the commit is **unsigned** — `git reset --soft HEAD~1`, fix config, recommit. Do not push unsigned work.

---

## 2. Branching Model ★

Four kinds of branches.

| Branch | Purpose | Lifespan |
|---|---|---|
| `main` | Stable pointer; HEAD always at the latest stable release tag | Forever |
| `dev` | Integration trunk; rc tags, topic-branch merges, and direct small commits converge here | Forever |
| `<prefix>/<slug>` | Topic branch — one change in flight, forked off `dev`. `<prefix>` ∈ `feat`, `fix`, `chore`, `docs` (see §3) | Until squash-merged into `dev` |
| `release/vX.Y` | Patch line for a non-current minor, forked lazily off `vX.Y.0` | Until v(X.Y) goes EOL |

```
   main pointer       dev (integration trunk; always advancing)        feat/toml-parser
   ────────────       ──────────────────────────────────────────       ────────────────

                      o  feat: yaml parser
                      │
   [main ───────────► o  ◄── v0.2.0]   ── stable cut: tag on dev,
                      │                   then `git switch main && git merge --ff-only dev`
                      │
                      ├──── fork ────────►  o  start TOML
                      │                     │
                      │                     o  AST shaping
                      │                     │
                      │                     o  register .toml
                      │ ◄── squash-merge ───┘
                      o  feat(parser): toml
                      │
                      o  ◄── v0.3.0-rc.1   [tag on dev only; main untouched]
                      │
                      o  fix(compiler): race in workspace scan
                      │
                      o  ◄── v0.3.0-rc.2
                      │
   [main ───────────► o  ◄── v0.3.0]   ── ff main forward to this commit
                      │
                      o  feat(mcp): 0.4 work begins  [main stays at v0.3.0]
                      ▼
```

Direct commits land on `dev`, never on `main`. `main` only moves via `git merge --ff-only` from `dev` (at minor cuts) or from `release/vX.Y` (at Case A patches — see §7). Any commit appearing on `main` that isn't already an ancestor of `dev` is a mistake. See §11.

`dev`, `main`, and every `release/*` are linear by construction — squash-merges and fast-forwards never produce merge commits. Cherry-picks add commits one at a time; never reorder.

---

## 3. Topic Branches: `<prefix>/<slug>` Off `dev` ★

Every PR-style change forks from `dev` as a **topic branch**. **`dev` is the only PR target** — new issues, contributor PRs, and any non-trivial work all PR there. `main` and `release/*` are downstream of `dev`; nothing PRs there directly.

### The four prefixes

The prefix expresses the *purpose* of the branch. The set is exhaustive — every change fits one (or it isn't a `dev`-bound change at all). All four are first-class; none is a special case of another.

| Prefix | Purpose | Example slugs |
|---|---|---|
| `feat/<slug>` | New functionality or user-visible behavior | `feat/toml-parser`, `feat/mcp-stats-tool` |
| `fix/<slug>` | Bug fix landing on `dev` (not a backport-only patch — see §8 Case B) | `fix/parser-utf16`, `fix/race-in-tracker` |
| `chore/<slug>` | Non-functional housekeeping — deps, tooling, lint, CI | `chore/bump-go-1.24`, `chore/golangci-config` |
| `docs/<slug>` | Documentation only — READMEs, rules, skills | `docs/git-workflow`, `docs/mcp-tool-conventions` |

Slugs are kebab-case, ≤4 words, descriptive. Bad: `feat/new-stuff`, `fix/bug`, `chore/update`, `radim-test`. Good: `feat/toml-parser`, `fix/wal-checkpoint`, `chore/bump-deps`.

A change that genuinely spans two intents (e.g., a feature plus a chore-grade dep bump it depends on) is **two PRs**, not one branch carrying both. Split before forking.

### Workflow (identical for every prefix)

Fork:

```bash
git switch dev
git pull
git switch -c fix/parser-utf16
```

Work, sign, push (`-u` sets the upstream automatically):

```bash
git push -u origin fix/parser-utf16
```

Squash-merge back so `dev` keeps one commit per logical change:

```bash
git switch dev
git pull
git merge --squash fix/parser-utf16
git commit -s -m "$(cat <<'EOF'
fix(parser): handle malformed UTF-16

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push origin dev

git branch -d fix/parser-utf16
git push origin --delete fix/parser-utf16   # if pushed
```

```bash
# Bad — fork off main (main lags dev between releases; you'd start from a stale base)
git switch -c feat/toml-parser main

# Bad — branch without a sanctioned prefix (won't tell future you what intent the branch carries)
git switch -c update-things
git switch -c experimental
git switch -c radim-test

# Bad — regular merge (creates a merge commit on dev; breaks linearity)
git merge feat/toml-parser

# Bad — rebase-and-merge keeping every wip commit on dev
git rebase dev feat/toml-parser
git merge --ff-only feat/toml-parser

# Good — squash-merge from dev with a prefixed branch
git switch dev
git merge --squash feat/toml-parser
git commit -s -m "feat(parser): toml support"
```

### Solo exception

For tiny changes (a typo fix, a one-line config bump, a dependency tidy), committing directly on `dev` is fine — the ceremony of a topic branch isn't worth it. Rule of thumb: if you'd want a single commit on `dev` representing this change anyway, just commit it directly. Anything that would merit a multi-commit working branch goes through a `<prefix>/<slug>` topic branch.

---

## 4. Commit Often, Commit Small ★

A commit captures one coherent thought. If the message needs "and" to describe it, split it.

```
# Bad
feat: add parser, hook up emitter, fix flaky test, bump deps

# Good
feat(parser): accept UTF-8 BOM at byte 0
feat(emit):   wire parser output to writer
fix(test):    stabilize ordering in TestEmit_Sorts
chore(deps):  bump x/sync/errgroup to v0.8
```

### Land work in chunks as it completes

After each logical unit — a passing test, a green refactor, a small feature:

1. `git status` — confirm only the intended files changed.
2. `git diff --staged` — read the exact patch.
3. Confirm you're on the right branch (see §10 step 3).
4. Commit.

Don't batch a session's worth of edits into one commit. If the context ends mid-task, a committed partial chunk is recoverable; an uncommitted partial chunk is lost.

### One logical change per commit, not one file per commit

```
# Bad — split by file
commit A: modify parser.go
commit B: modify parser_test.go  (the test for A)

# Good — one idea, all files implementing it
commit: feat(parser): accept UTF-8 BOM
  parser/parser.go
  parser/parser_test.go
```

---

## 5. Commit Message Format ★

One subject line, imperative mood, ≤72 chars, lower-case, optional `scope`. **No descriptive body** — the subject must capture the change in full. If the subject can't say it, the commit holds two intents; split it (see §4). The only sanctioned post-subject content is the `Co-Authored-By` trailer (when Claude authored the commit).

```
# Good
feat(store): preallocate nodes slice with len hint
fix(parser): skip BOM only at byte 0, not mid-stream
refactor: move Emitter interface to consumer package
chore(release): cut v0.3.0
chore(release): bump plugin manifests to 0.3.0
docs(git): switch to dev/main hybrid branching model

# Bad
Updated parser.go                                ← past tense, vague
fixed bug                                        ← what bug?
WIP                                              ← never as a commit subject
feat(parser): accept BOM and also fix nesting    ← two intents — split
feat(parser): UTF-8 BOM\n\nSome editors emit…    ← descriptive body — drop it
```

The scope `release` is reserved for tag-time housekeeping (version bumps in plugin manifests, release notes, the cut itself).

### HEREDOC for trailer-bearing messages; bare `-m` is fine without a trailer

Shell escaping mangles newlines and quotes in `-m "…"`. HEREDOC is the safe form whenever the message spans more than one line — i.e., whenever the `Co-Authored-By` trailer is appended.

```bash
# Claude-authored: subject + blank line + trailer, via HEREDOC
git commit -m "$(cat <<'EOF'
feat(parser): accept UTF-8 BOM at byte 0

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"

# Human-authored: single-line, bare -m
git commit -s -m "fix(parser): skip BOM only at byte 0"
```

The `Co-Authored-By` trailer is required on every commit Claude authors. Use the running model name (e.g., `Opus 4.7`) — see auto-memory.

---

## 6. Staging Discipline

### Stage by name, never `.` or `-A`

`git add .` sweeps in editor swap files, `.env`, debug logs, coredumps — anything `.gitignore` missed.

```bash
# Bad
git add .
git add -A

# Good
git add parser/parser.go parser/parser_test.go
```

### Scan `git status` for secrets and big files before committing

Flag and stop if anything resembles `.env*`, `*.key`, `id_*`, `*.pem`, credentials, database dumps, or a file over ~1 MB. Ask the user before including it.

### Check for forgotten untracked files

```bash
git status --short
# ?? parser/new_lexer.go   <- intentional? include it or ignore it.
```

An orphan file left on disk but uncommitted is a silent footgun next session.

---

## 7. Pre-Release Iteration & Stable Cuts ★

### Pre-release tags on `dev`

When `dev` is feature-frozen for a minor, tag the current HEAD:

```bash
git switch dev
git tag -s v0.3.0-rc.1 -m "v0.3.0-rc.1"
git push origin v0.3.0-rc.1
```

Testers install from the rc tag (`go install module@v0.3.0-rc.1`) or pull `dev` directly. If a regression surfaces, fix it on `dev` (via a `fix/...` branch or — for a small fix — a direct commit), then tag a new rc:

```bash
git tag -s v0.3.0-rc.2 -m "v0.3.0-rc.2"
git push origin v0.3.0-rc.2
```

Old rc tags are **never deleted**. They're historical markers a future bisector might reach for. The release workflow filter (`'v*', '!v*-*'` in `.github/workflows/release.yml`) skips them — only stable tags publish artifacts.

### Stable cut: tag on `dev`, ff `main` to it

When the rc looks clean:

```bash
git switch dev
git tag -s v0.3.0 -m "v0.3.0"
git push origin v0.3.0

git switch main
git merge --ff-only dev
git push origin main
```

`main` and `dev` now point at the same commit. The release workflow fires on the stable tag and publishes the artifact.

```bash
# Bad — regular merge (creates a merge commit on main; breaks linearity)
git switch main
git merge dev

# Bad — squash-merge dev into main (loses per-commit signatures and provenance)
git switch main
git merge --squash dev
git commit -s -m "release v0.3.0"

# Good — fast-forward only
git switch main
git merge --ff-only dev
```

If `git merge --ff-only dev` fails with **"not a fast-forward"**, `main` and `dev` have diverged — **stop and ask**. The fix is human, not automatic. See §12.

---

## 8. Patch Releases ★

Two cases, picked by where `dev` is when the patch ships.

### Case A — `dev` hasn't moved past the latest minor yet

If no work for the next minor has begun on `dev`:

```bash
git switch dev
# fix lands on dev (via fix/<slug> topic branch or a direct commit)
git tag -s v0.3.1 -m "v0.3.1"
git push origin v0.3.1

git switch main
git merge --ff-only dev
git push origin main
```

Same flow as a minor release, smaller scope. No release branch needed.

### Case B — `dev` has moved on; lazily create `release/v0.3`

If `dev` already has v0.4 work cooking, `main` can't ff to `dev` (it'd ship the half-done v0.4 features). Create the release branch from the stable tag instead.

```bash
# 1. Fix lands on dev FIRST (the invariant; see §9)
git switch dev
# ... commit fix on dev as usual ...
git push origin dev

# 2. Create the release branch from the v0.3.0 tag
git switch -c release/v0.3 v0.3.0
git push -u origin release/v0.3

# 3. Cherry-pick the fix from dev (-x records the source SHA in the message)
git cherry-pick -x <sha-of-fix-on-dev>

# 4. Tag and push
git tag -s v0.3.1 -m "v0.3.1"
git push origin release/v0.3 v0.3.1
```

```
   dev                          release/v0.3
   ─────                        ────────────

   o  feat: 0.4 work begins
   │
   o  fix(parser): UTF-16            ◄── original (lands on dev first)
   │
   o  feat: more 0.4 work
   │
   │       ┌── lazy fork from v0.3.0 tag ───►   o  v0.3.0
   │       │                                     │
   │       │   cherry-pick -x ───────────────►   o  fix(parser): UTF-16
   │       │                                     │   (cherry picked from <sha>)
   │       │                                     │
   │       │                                     o  ◄── v0.3.1
   ▼
```

Under Case B, `main` does **not** move. It stays at `v0.3.0` (the latest minor stable). Users on v0.3.x get the patch via the tag (`go install module@v0.3.1`); users tracking `main` are still on v0.3.0 and will jump to v0.4.0 at the next minor cut.

**Release branches never merge back into `dev` or `main`.** The fix is already on `dev` (you cherry-picked from it). Backporting flows one-way: `dev` → `release/vX.Y`. A subsequent v0.3.2 cherry-picks again onto the same `release/v0.3`.

When v(X.Y) reaches end-of-life (no longer supported), delete the branch but keep the tags:

```bash
git branch -d release/v0.3
git push origin --delete release/v0.3
# v0.3.0, v0.3.1, ... tags persist forever
```

---

## 9. The "Fix Lands on `dev` First" Invariant ★

Every fix lives on `dev` first, then fans out via cherry-pick to whatever release branches need it.

```bash
# Bad — fix only on release/v0.3, never on dev
git switch release/v0.3
# commit the fix here directly
# (now dev's v0.4 work might silently regress on the same bug)

# Good
git switch dev
# commit the fix here (or via fix/<slug> topic branch)
git switch release/v0.3
git cherry-pick -x <sha>
```

The invariant guarantees that `dev` is always **at least as fixed as** every active release branch. Two minors silently diverging in behavior is how upgrades bite users.

### Documented exception — fix only applies to the older minor

Sometimes a v0.3.x bug doesn't exist on `dev` (the surrounding code was rewritten for v0.4 and the bug doesn't apply). Land the fix on `release/v0.3` directly and note in the commit message *why* it's not going to `dev`:

```
fix(parser): UTF-16 BOM in legacy code path

This code path was rewritten in 0.4 — the bug doesn't exist on dev.
Patch is intentionally release/v0.3-only.
```

This is the only sanctioned way a commit lives on a release branch without an ancestor on `dev`.

---

## 10. Pre-Commit Checklist

Before each commit, in order:

1. `git status` — know what's changing.
2. `git diff --staged` — read the patch.
3. Confirm you're on the right branch:
   - topic-branch work → `<prefix>/<slug>` where `<prefix>` is one of `feat`, `fix`, `chore`, `docs` (forked off `dev`)
   - small/direct change → `dev`
   - patch backport → `release/vX.Y`
   - **never `main`** — see §11
4. Tests pass for touched packages (`go test ./parser/...`).
5. `go vet ./...` clean for touched packages.
6. Commit.
7. `git log -1 --show-signature` — confirm signature.

Don't skip hooks. `--no-verify` bypasses the repo's own safety net; use it only if the user explicitly asks.

If a hook fails, the commit **did not happen**. Fix the failure, re-stage, make a **new** commit — do not `--amend` a commit that was never created.

---

## 11. Never Commit Directly on `main` ★

`main` is a fast-forward-only pointer to the latest stable release tag. The only operations that touch `main` are:

- `git merge --ff-only dev` — at a stable minor cut (§7) or Case A patch (§8).
- `git merge --ff-only release/vX.Y` — equivalent to the above for Case A patches when `dev == release branch tip`.
- `git push origin main` — propagating the ff'd pointer.
- `git switch main`, `git pull` — read-only.

Any direct commit on `main` is an accident. If you find yourself on `main` with a dirty tree:

```bash
# Stash, switch to dev or a topic branch, pop
git stash
git switch dev          # or <prefix>/<slug>
git stash pop
```

If you've already committed on `main` but **haven't pushed**:

```bash
git switch dev
git cherry-pick main          # bring the commit to dev
git switch main
git reset --hard origin/main  # discard the local commit on main
```

If you've already **pushed** a commit to `main` directly: **stop and ask**. Rewriting published `main` invalidates signatures on every descendant and on every release branch that forked from a stable tag. The user will want to decide whether to forward the commit through `dev` and live with the noise, or rewrite history (rare, last resort).

---

## 12. Signing Caveats

Symptom → cause → fix. Keep fixes persistent (rc files), not per-session.

### `gpg: signing failed: Inappropriate ioctl for device`

GPG wants a TTY for the passphrase prompt; the shell has none.

```bash
export GPG_TTY=$(tty)   # add to ~/.bashrc, ~/.zshrc, or fish equivalent
```

### `error: gpg failed to write commit object`

The GPG agent isn't running or the key isn't loaded.

```bash
gpgconf --launch gpg-agent
gpg --list-secret-keys            # confirm the key appears
echo test | gpg --clearsign       # force a passphrase prompt + cache it
```

### Signing format mismatch

`gpg.format` disagrees with the key type (e.g., SSH key but `gpg.format=openpgp`).

```bash
# For SSH signing:
git config --local gpg.format ssh
git config --local user.signingkey ~/.ssh/id_ed25519.pub
git config --local gpg.ssh.allowedSignersFile ~/.config/git/allowed_signers
```

### Commit succeeds but `--show-signature` says "No signature"

`commit.gpgsign` is `false` in local config while `true` globally, or vice versa. **Local wins.** Re-check:

```bash
git config --local --get-regexp '^(user\.signingkey|commit\.gpgsign|gpg\.)'
```

### Pre-commit hook fails *before* the signing prompt

Signing happens after hooks. If a hook exits non-zero, the key never gets invoked — don't blame the key. Read the hook output first.

---

## 13. Other Common Caveats

### `git merge --ff-only dev` fails with "not a fast-forward"

`main` and `dev` have diverged. This shouldn't happen under §11. **Stop and ask.** Do not force-push. Likely causes:

- A direct commit landed on `main` (§11). Cherry-pick it to `dev`, then re-attempt the ff merge.
- A Case B patch tagged on `release/v0.X` was ff'd into `main`, and `dev` doesn't yet have an equivalent commit. Forward-port the patch's content to `dev` first, then ff `main` to `dev` at the next minor.
- A `release/*` branch tip was force-pushed (it shouldn't be — release branches are append-only).

### Cherry-pick conflict during a Case B backport

The fix on `dev` references code that has changed since v0.3.0. Resolve the conflict in the editor, `git add` the resolved files, `git cherry-pick --continue`. Keep the message footprint (`(cherry picked from commit <sha>)`) intact — that's the audit trail for the backport.

### "Nothing to commit" after staging

Usually a line-ending (`core.autocrlf`) or file-mode (`core.filemode`) normalization. Confirm with `git diff --staged --stat` — if empty, the working tree already matches `HEAD`.

### Accidentally committed a large file (not yet pushed)

```bash
git rm --cached path/to/big.bin
echo 'path/to/big.bin' >> .gitignore
git add .gitignore
git commit --amend --no-edit
```

If already pushed, **stop and ask the user** — rewriting published history invalidates signatures on every descendant commit.

### Detached HEAD

Reattach before doing anything:

```bash
git switch dev    # not main — never commit on main (§11)
```

Never commit in detached HEAD — the commit will be unreachable once HEAD moves.

### Dirty working tree blocks an operation

Don't `git stash` reflexively. Read the diff first; dirty files may be intentional WIP the user wants folded into the next commit, not hidden.

---

## 14. Anti-Patterns — Do Not

- Commit directly on `main`. Use `dev` or a `<prefix>/<slug>` topic branch.
- Branch off `main` for new work — features, fixes, anything. Always fork topic branches from `dev`.
- Branch without a sanctioned prefix (`feat`, `fix`, `chore`, `docs`). The prefix is what tells future-you and reviewers what intent the branch carries.
- Merge `dev` into `main` with a regular merge or a squash-merge. Always `git merge --ff-only`.
- Merge `release/vX.Y` back into `dev` or `main` with a regular merge. Cherry-pick is one-way.
- Tag a stable `vX.Y.Z` on `dev` and walk away without ff-merging `main`. Tag and ff are one operation; don't split them across days where someone else might fork off `dev` at an unstable point.
- Delete an rc tag because "we shipped the stable already". Tags are historical markers; they cost nothing to keep.
- Reorder commits on a `release/vX.Y` branch via interactive rebase. Cherry-picks land in time order; `git log` is the audit trail.
- Force-push to `main`, `dev`, or any `release/*`. All three are append-only.
- `git add .` / `git add -A` without reading `git status` first.
- `git commit --no-verify` unless the user explicitly asked.
- `git commit --no-gpg-sign` or `-c commit.gpgsign=false` — ever.
- `git reset --hard` on a branch with unpushed commits without confirming the working tree is disposable.
- `git clean -fd` without listing first (`git clean -nd`).
- `git checkout .` / `git restore .` — wipes uncommitted work silently.
- `git rebase -i` on anything pushed.
- Batching a session's edits into one commit "to keep history clean".
- Include a descriptive body in commit messages. Subject must capture the change in full; if it can't, the commit holds two intents — split.
- Committing `.env`, `*.key`, `id_*`, `*.pem`, credentials, or files >1 MB without asking.
- `--amend` after push.
- `--allow-empty` as a progress marker.

---

## Priority When Rules Conflict

1. **Signed provenance** — every commit verifiable, no exceptions.
2. **Atomic commits** — one idea per commit, even if it means more commits.
3. **Clean linear history** — `dev`/`main`/`release/*` all linear; no merges, no rewriting of pushed work.
4. **Speed** — commit as soon as a chunk is coherent; don't let uncommitted work pile up.
