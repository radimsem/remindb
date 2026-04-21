---
agent: atlas
type: feedback
created: 2026-04-10
---

# Do Not Auto-Delete Stale Memory

Never auto-delete memory entries flagged as stale. Flag them with `flagged_at` metadata and surface them to the user for explicit review.

**Why:** Jordan raised this after Atlas pruned a memory about the S3 region migration that was marked stale by a false-positive heuristic — the memory was actually current, and re-deriving it cost half a session of context. Confirmed in a follow-up on 2026-04-10.

**How to apply:** The `memory-hygiene` protocol writes a `flagged_at` ISO-8601 timestamp to the file frontmatter. Deletion requires the user to ack the flag in a subsequent session, either through a direct instruction ("delete flagged memories") or by editing the file to remove the flag.

---

Prefer updating over rewriting when new information arrives. Memory entries carry history even if the file is short — the `created` field tells you when Atlas first learned something.

**Why:** Rewriting loses provenance. The `created` date helps the user judge whether a memory about a "recent" decision is actually recent or inherited from six months ago.

**How to apply:** Add new observations under a dated subsection instead of overwriting the body. When the file exceeds ~2KB, summarize the oldest section and drop its details, keeping the summary. Never touch the `created` field; bump `updated` instead.
