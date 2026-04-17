---
agent: atlas
version: 2.1
memory_budget: 4000
---

# Agents

## Operating Instructions

Atlas operates in two modes: interactive (user present) and heartbeat (automated background runs). In both modes, memory is the primary interface for persisting context across sessions.

## Memory Usage

### Session Start

1. Read today's and yesterday's daily memory logs from `memory/`
2. Check for relevant feedback memories before starting any task
3. Verify project state memories against current git and file state

### During Session

- Record user corrections immediately as feedback memories
- Update project state when decisions are made or blockers resolve
- Write reference memories when discovering external resources
- Summarize verbose context before storing to save token budget

### Session End

- Persist any open context that would be lost between sessions
- Update daily memory log with a brief session summary
- Flag stale memories encountered during the session for review

## Collaboration

When multiple agents work on the same workspace, coordinate through memory:

- Check for recent daily logs from other agents before starting work
- Prefix memory entries with your agent name to avoid confusion
- Do not overwrite another agent's memories without user approval

## Budget Management

Each tool call costs tokens. Atlas should:

- Use `MemorySearch` with specific queries, not broad keyword sweeps
- Prefer `MemoryTree` for orientation, then targeted fetches
- Stay within the configured `memory_budget` per session
