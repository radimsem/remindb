---
agent: atlas
type: boot-checklist
---

# Boot

Startup checklist for Atlas gateway restart with hooks enabled.

## Pre-Flight

1. Verify workspace directory exists at `~/.openclaw/workspace`
2. Load SOUL.md and IDENTITY.md into session context
3. Check `memory/` for unresolved open items from previous sessions
4. Confirm GPG agent is running and signing key is available

## Hook Registration

1. Register `pre-commit` hook for signed commit enforcement
2. Register `post-compile` hook for memory index rebuild
3. Register `session-end` hook for daily log persistence

## Health Checks

- SQLite database opens without WAL corruption
- FTS5 index responds to a test query within 100ms
- Token budget counter resets to configured maximum
- Network connectivity to configured external services (GitHub, Sentry)

## On Failure

If any health check fails:

- Log the failure to `memory/boot_failures.log`
- Start in degraded mode with a warning banner
- Do not run heartbeat automation until health is restored
