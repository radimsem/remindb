# FTS5 search-query syntax

Reference for `remind`'s `MemorySearch`. Load when you need to construct a non-trivial query.

Search goes through SQLite FTS5. Pre-processing: **bare queries are quoted per-word and `OR`-joined**; anything that already looks like FTS5 passes through unchanged.

The server checks for any of: `OR  AND  NOT  NEAR(  "  :  *  (`

- Any present → pass through unchanged (already FTS5).
- Else → whitespace-split, each word quoted, joined with ` OR `. Quoting makes internal punctuation (hyphens, dots) match literally instead of leaking into FTS5.

```
"token bucket rate limit"  → "token" OR "bucket" OR "rate" OR "limit"   (matches ≥1 word, ranked by hit count)
"database"                 → "database"                                  (single bare word, quoted)
"ZEBRA-4471"               → "ZEBRA-4471"                                (punctuation matched literally, no error)
"token AND bucket"         → passed through                             (both required)
"\"token bucket\""         → passed through                             (exact adjacent phrase)
```

How to construct queries:

1. **Keyword lists, not sentences.** Strip function words ("how", "the", "do", "I") — they dilute OR ranking.
2. **Bare multi-word for broad recall** — "any-of" matching, ranked by how many words hit.
3. **FTS5 operators for precision:** `"exact phrase"` (adjacent, in order) · `a AND b` (both) · `a NOT b` (exclude b) · `prefix*` (prefix match) · `NEAR(a b, 5)` (within 5 tokens).
4. **Internal punctuation is handled for you.** Bare queries auto-quote each term, so `rate-limit` or `ZEBRA-4471` match without erroring. Explicit quoting (`"rate-limit"`) still works for multi-word phrases.

```
# Bad  — stopwords dilute:  "how do I configure the rate limiter middleware"
# Good — keywords only:     "rate limiter middleware configure"
# Best — known phrase:      "\"rate limiter middleware\""
# All terms required:       "rate AND limiter AND redis"
```
