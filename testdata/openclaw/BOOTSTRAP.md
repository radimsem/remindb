---
agent: atlas
type: bootstrap
one_time: true
---

# Bootstrap

First-run ritual for a new Atlas workspace. Delete this file after completion.

## Step 1: Identity

The identity step establishes the baseline for how Atlas communicates with the user. Getting this wrong early means every future interaction feels slightly off. Communication style especially matters because terse users find detailed responses annoying, and detail-oriented users find terse responses dismissive.

Ask the user for:

- Preferred name and pronouns
- Primary programming languages
- Communication style preference (terse, detailed, conversational)

Write the responses to USER.md. If the user declines to answer any question, use sensible defaults and note the assumption in the memory entry so it can be corrected later.

## Step 2: Project Context

The project scan creates Atlas's initial mental model of the codebase. Without this, Atlas would need to rediscover the tech stack, build system, and CI pipeline on every task. The scan should be thorough but not exhaustive — a full AST analysis is unnecessary, but understanding the dependency graph and build targets avoids costly mistakes like suggesting a library that conflicts with existing dependencies.

Scan the current directory for:

- `go.mod` or `package.json` to identify the language and dependencies
- `Makefile`, `justfile`, or `taskfile.yml` for build conventions
- `.github/workflows/` for CI configuration
- `README.md` for project overview

Compile findings into an initial project memory. Include the Go version or Node version, the test command, and the primary entry point. These three facts prevent the majority of first-task failures.

## Step 3: Tool Discovery

Tool discovery prevents Atlas from attempting operations that will fail due to missing credentials or binaries. A common failure mode is attempting to create a signed commit when the GPG agent is not running, or trying to open a PR when the GitHub CLI token has expired. Detecting these upfront saves a round-trip of failure, diagnosis, and retry.

Check for available integrations:

- Git: verify `git config user.signingkey` is set and the GPG agent can sign a test payload
- GitHub CLI: verify `gh auth status` succeeds and the token has `repo` scope
- Sentry CLI: check if `sentry-cli` is on PATH and can authenticate against the configured project

Write tool availability to TOOLS.md. If a tool is missing or its authentication has expired, record the specific failure reason so the user can fix it without Atlas re-running the full discovery sequence.

## Step 4: Memory Seeding

Memory seeding gives Atlas a non-empty starting context for the first real task. Without seeded memories, Atlas would have no user preferences to consult and no project context to reference, making the first interaction feel like a cold start. The initial entries should be minimal but accurate — it is better to have three correct memories than ten speculative ones.

Create initial memory entries:

- One `user` memory from the identity answers, including communication style and language expertise
- One `project` memory from the directory scan, covering tech stack, build system, and test command
- One `reference` memory for any discovered dashboards, wikis, or issue trackers

## Completion

After all steps succeed, delete this file. Its presence signals an incomplete bootstrap.
