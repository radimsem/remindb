---
name: tune-temperature-policy
description: Use when changing any field in `pkg/temperature/Config` (`DecayRate`, `AccessBoost`, `ColdThreshold`, `NotifyThreshold`, `TickInterval`) or modifying `decayFactor` / `Score` / cold-node notification logic — symptoms include "tune the decay rate", "make notifications less noisy", "change the cold cutoff", "adjust the temperature window", "raise/lower the boost". Prevents silent docs drift in `skills/efficient-memo/SKILL.md` (mental-model numerics) and `skills/memoize/SKILL.md` (summarization workflow triggered by the notification).
---

# Tune the temperature policy

remindb's temperature system has five knobs in `pkg/temperature/config.go`. They're tightly coupled — changing one shifts the behavior of search ranking, the cold-set query, and the client-facing notification stream. Tuning is rarely a one-file change.

The skill exists because two public skills document this policy for agents:

- `skills/efficient-memo/SKILL.md` — owns the **numerics** in the mental model (decay rate, access boost, cold/notify thresholds, tick interval, ranking score formula, notification payload shape).
- `skills/memoize/SKILL.md` — owns the **summarization workflow** the notification triggers (`MemoryFetch` → `MemorySummarize`).

If the numbers or the workflow drift from the code, agents will reason from stale defaults.

## What the knobs do

| Knob | Default | Affects |
|---|---|---|
| `DecayRate` | `0.05` | `decayFactor = exp(-rate × elapsed_hours)` — applied each tick to every node |
| `AccessBoost` | `0.15` | Added to a node's temperature on read; capped at `1.0` by SQL `min(1.0, …)` |
| `ColdThreshold` | `0.1` | Below this, nodes are "cold" — used by `GetColdNodes` and the search relevance floor (`Score = relevance × (0.3 + 0.7 × temperature)`) |
| `NotifyThreshold` | `0.1` | Below this, the server pushes an MCP notification (`level: "warning"`, `logger: "remindb.temperature"`) — gated by per-node hysteresis dedup |
| `TickInterval` | `5 * time.Minute` | How often `Tracker.Run` decays + queries cold nodes |

## Where the change ripples

Every tune touches **four** surfaces minimum.

| File | Why |
|---|---|
| `pkg/temperature/config.go` | The knob itself (the `DefaultConfig` literal) |
| `pkg/temperature/*_test.go` | Tests that assert specific numeric outcomes (`tracker_test.go`, `cold_test.go`, `decay_test.go`) — they'll fail if defaults shift |
| `pkg/mcp/server_test.go` | If `NotifyThreshold` semantics change, the dedup/hysteresis tests need updating |
| `skills/efficient-memo/SKILL.md` | Public-facing **mental-model** docs (numerics, ranking score, notification payload, threshold descriptions) — *the easy one to forget* |
| `skills/memoize/SKILL.md` | Public-facing **summarization workflow** triggered by the notification (`MemoryFetch` → `MemorySummarize`) — touch when the trigger or the recommended response shape changes |

If the change is structural (new knob, new threshold), also add a note to `pkg/temperature/cold.go` and re-read `pkg/mcp/server.go:60-98` (`NotifyColdNodes` / `selectNewNotifications`) to confirm the hysteresis logic still makes sense.

## Tuning rationales — what to tune for what symptom

| Symptom | Knob to consider | Direction |
|---|---|---|
| Cold notifications too noisy | `NotifyThreshold` | **Lower** (e.g., 0.05) — only the very coldest get pushed; widens the hysteresis band so re-notifications are rarer |
| Cold notifications too rare | `NotifyThreshold` | **Raise** toward `ColdThreshold` |
| Hot nodes lingering at the top of search | `DecayRate` | **Raise** (e.g., 0.1) — decay is faster, ranking turnover is quicker |
| Recent reads not boosting enough | `AccessBoost` | **Raise** (e.g., 0.25) — fewer reads needed to keep a node warm |
| Tick storms (decay bursts visible in logs) | `TickInterval` | **Raise** (e.g., 15 min) — fewer, larger decays per tick (factor stays the same since it's based on elapsed hours) |
| Cold-set query returning too much / too little | `ColdThreshold` | **Adjust** to match what `GetColdNodes` should return |

Note the asymmetry: `ColdThreshold` and `NotifyThreshold` *can* be different. Currently they're both `0.1` so the cold-set and the notify-set are the same; setting `NotifyThreshold < ColdThreshold` gives you a "cold but not yet alertable" zone.

## The docs-sync step

The two public skills carry different surfaces of the policy. Walk both:

### `skills/efficient-memo/SKILL.md` — mental-model numerics

- **Frontmatter description** — mentions "warning-level cold-node notifications"
- **Mental model → Nodes** — quotes `+0.15`, `exp(-0.05 × elapsed_hours)`, `~5% per hour`, the two thresholds, and `0.1` defaults
- **Mental model → Ranking** — `score = relevance × (0.3 + 0.7 × temperature)`
- **Mental model → Notifications** — quotes the message string, hysteresis behavior, payload shape
- **Anti-patterns** — the dedup-and-rearm note, the `ColdThreshold` vs `NotifyThreshold` distinction

### `skills/memoize/SKILL.md` — workflow that follows the notification

- **Summarize a cold node** — the `MemoryFetch` → `MemorySummarize` handoff. Touch when the trigger semantics, the recommended summary shape, or `MemorySummarize`'s preserved-fields contract changes.
- **Anti-patterns** — "don't ignore a `level: warning` / `logger: remindb.temperature` notification" and "don't summarize toward a flat paragraph"

### Walking the change

Every numeric or behavioral change requires a pass through both skills. If you change `DecayRate` from `0.05` to `0.1`, every `0.05` and "5% per hour" must update in `efficient-memo`. If you decouple `ColdThreshold` and `NotifyThreshold`, the threshold paragraphs in `efficient-memo` need updating. If you change what `MemorySummarize` preserves, `memoize`'s summarize section needs updating.

The fast check:

```
grep -nE '0\.05|0\.15|0\.1|5 minutes' skills/efficient-memo/SKILL.md
grep -nE 'MemorySummarize|NotifyThreshold|ColdThreshold' skills/efficient-memo/SKILL.md skills/memoize/SKILL.md
```

Every hit is a candidate for an update.

## Quick reference

```
1. pkg/temperature/config.go               (the knob)
2. pkg/temperature/*_test.go               (assertions on numerics)
3. pkg/mcp/server_test.go                  (only if NotifyThreshold semantics change)
4. skills/efficient-memo/SKILL.md          (mental-model numerics + behavioral descriptions)
5. skills/memoize/SKILL.md                 (only if the summarization workflow or MemorySummarize contract changes)
6. go test ./pkg/temperature/... ./pkg/mcp/...    (must pass)
```

## Common mistakes

- **Changing the default but not the test that asserts it.** `tracker_test.go:103` and `cold_test.go` check specific decay outcomes from the default config. If you bump `DecayRate`, the expected post-tick temperatures must change too.
- **Expecting `NotifyThreshold > ColdThreshold` to alert on warmer nodes.** It doesn't. The cold set is gated upstream at `ColdThreshold` in `Tracker.Tick`; `NotifyThreshold` only filters *within* that set via `n.Temperature >= s.notifyThreshold` in `selectNewNotifications`. Setting `NotifyThreshold` above `ColdThreshold` just disables the filter — every node already in the cold set passes through. To widen the alerting set, raise `ColdThreshold`. To narrow it, lower `NotifyThreshold` below `ColdThreshold` (creates a "cold but not alertable" hysteresis band).
- **Skipping the public-skill docs sync.** Drift between the code and either `skills/efficient-memo/SKILL.md` (numerics) or `skills/memoize/SKILL.md` (summarization workflow) means a future Claude reasons from a stale baseline. Both skills are part of the deployed surface; treat drift as a bug.
- **Leaving `boostResultNodes` calls in mutating MCP tools.** Boost is for *read* tools (the read is the access). If you raise `AccessBoost` and a write tool also boosts, mutations look like accesses and skew temperatures up. Audit `pkg/mcp/tools/` after raising the boost.
- **Bumping `TickInterval` without thinking about hysteresis.** Notifications dedup per-node-per-cold-state. A longer tick means longer between dedup-eviction opportunities; a node oscillating around `NotifyThreshold` may go quieter than expected.

## Cross-references

- `.claude/rules/go-concise.md` — error handling, named locals
- `.claude/skills/add-mcp-tool/SKILL.md` — for the `boostResultNodes` rule when adding new tools (so the boost contract stays clean)
- `skills/efficient-memo/SKILL.md` — read-side docs target (numerics, ranking score, notification payload, threshold descriptions)
- `skills/memoize/SKILL.md` — write-side docs target (the cold-node summarization workflow and `MemorySummarize` contract)
- `pkg/temperature/decay.go` — the `Score` formula constants (`coldFloor = 0.3`, `tempWeight = 0.7`); these are not in `Config` but they shape ranking and may need to move there if you tune them
