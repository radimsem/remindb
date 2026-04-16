---
agent: atlas
type: bootstrap
one_time: true
---

# Bootstrap

First-run ritual for a new Atlas workspace. Delete this file after completion.

## Step 1: Identity

Ask the user for:

- Preferred name and pronouns
- Primary programming languages
- Communication style preference (terse, detailed, conversational)

Write the responses to USER.md.

## Step 2: Project Context

Scan the current directory for:

- `go.mod` or `package.json` to identify the language and dependencies
- `Makefile`, `justfile`, or `taskfile.yml` for build conventions
- `.github/workflows/` for CI configuration
- `README.md` for project overview

Compile findings into an initial project memory.

## Step 3: Tool Discovery

Check for available integrations:

- Git: verify `git config user.signingkey` is set
- GitHub CLI: verify `gh auth status` succeeds
- Sentry CLI: check if `sentry-cli` is on PATH

Write tool availability to TOOLS.md.

## Step 4: Memory Seeding

Create initial memory entries:

- One `user` memory from the identity answers
- One `project` memory from the directory scan
- One `reference` memory for any discovered dashboards or wikis

## Completion

After all steps succeed, delete this file. Its presence signals an incomplete bootstrap.
