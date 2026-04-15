# Git Versioning

Rules for managing repository state with `git` in `remindb`.

**Use when:** committing, inspecting history, or any operation that writes to `.git`.

**Current stage:** pre-implementation. Work lands as a **linear history on `main`** — no feature branches, no merges, no rewriting of pushed commits. Revisit when a second contributor joins.

**Priority when rules conflict:** signed provenance > atomic commits > clean linear history > speed.

---

## 1. Every Commit Must Be Signed ★

Every commit on `main` must carry a verifiable signature by the user.
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

## 2. Commit Often, Commit Small ★

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
3. Commit.

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

## 3. Linear History on `main`

While solo and pre-implementation:

- Commit directly to `main`; do not create branches.
- Never `git merge` — nothing to merge.
- Never `git rebase` pushed commits.
- Amending the **most recent unpushed** commit is fine for a typo or a missed file. Anything older: make a new commit.

```bash
# Good — fix the just-made, unpushed commit
git add forgotten_file.go
git commit --amend --no-edit

# Bad — rewriting pushed history
git rebase -i HEAD~5
git push --force
```

---

## 4. Commit Message Format

One subject line, imperative mood, ≤72 chars, lower-case, optional `scope`.
Body only when the *why* isn't obvious from the diff.

```
# Good
feat(store): preallocate nodes slice with len hint
fix(parser): skip BOM only at byte 0, not mid-stream
refactor: move Emitter interface to consumer package

# Bad
Updated parser.go
fixed bug
WIP
```

### Always pass the message via HEREDOC

Shell escaping mangles newlines and quotes in `-m "…"`. HEREDOC is the safe form.

```bash
git commit -m "$(cat <<'EOF'
feat(parser): accept UTF-8 BOM

Some editors emit U+FEFF at the start of UTF-8 files. Skip those
three bytes before lexing so callers don't have to strip them.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

The `Co-Authored-By` trailer is required on every commit Claude authors. Use the line above verbatim.

---

## 5. Staging Discipline

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

## 6. Pre-Commit Checklist

Before each commit, in order:

1. `git status` — know what's changing.
2. `git diff --staged` — read the patch.
3. Tests pass for touched packages (`go test ./parser/...`).
4. `go vet ./...` clean for touched packages.
5. Commit.
6. `git log -1 --show-signature` — confirm signature.

Don't skip hooks. `--no-verify` bypasses the repo's own safety net; use it only if the user explicitly asks.

If a hook fails, the commit **did not happen**. Fix the failure, re-stage, make a **new** commit — do not `--amend` a commit that was never created.

---

## 7. Signing Caveats

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

## 8. Other Common Caveats

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
git switch main
```

Never commit in detached HEAD — the commit will be unreachable once HEAD moves.

### Dirty working tree blocks an operation

Don't `git stash` reflexively. Read the diff first; dirty files may be intentional WIP the user wants folded into the next commit, not hidden.

### Remote and local have diverged

Should not happen on a solo linear `main`. If it does, **stop and ask**. Do not force-push. Do not merge blindly.

---

## 9. Anti-Patterns — Do Not

- `git add .` / `git add -A` without reading `git status` first.
- `git commit --no-verify` unless the user explicitly asked.
- `git commit --no-gpg-sign` or `-c commit.gpgsign=false` — ever.
- `git push --force` / `--force-with-lease` on `main`.
- `git reset --hard` without confirming the working tree is disposable.
- `git clean -fd` without listing first (`git clean -nd`).
- `git checkout .` / `git restore .` — wipes uncommitted work silently.
- `git rebase -i` on anything pushed.
- Batching a session's edits into one commit "to keep history clean".
- Committing `.env`, `*.key`, `id_*`, `*.pem`, credentials, or files >1 MB without asking.
- `--amend` after push.
- `--allow-empty` as a progress marker.

---

## Priority When Rules Conflict

1. **Signed provenance** — every commit verifiable, no exceptions.
2. **Atomic commits** — one idea per commit, even if it means more commits.
3. **Clean linear history** — no merges, no rewriting of pushed work.
4. **Speed** — commit as soon as a chunk is coherent; don't let uncommitted work pile up.
