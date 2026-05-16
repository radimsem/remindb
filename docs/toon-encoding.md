# TOON encoding — smaller where it actually pays

> Don't compress for the sake of compressing. Compress where the structure makes it free, and leave prose alone.

[← back to README](../README.md) · related: [node tree](./node-tree.md) · [search](./search.md)

## The problem

A lot of memory is repetitive structure: a config block, a table, a list of dicts that all have the same keys. Stored as JSON or YAML, every row repeats every key. The agent pays for `"name":`, `"weight":`, `"origin":` over and over, on every read, forever. That's pure overhead — the shape never changes, only the values do.

But the opposite mistake is just as bad: jam everything through a compressor and you mangle the irregular prose that was already as small as it's going to get, for no gain.

## What TOON does

[TOON](https://github.com/toon-format/toon) is a compact encoding for arrays of uniform objects — it states the shape once, then streams the values. For that kind of data it stores **~40% smaller** than YAML or JSON. For a paragraph of English it has nothing to offer.

So remindb doesn't pick a format globally. It decides **per node**:

1. The parser builds the node both ways — plain and TOON.
2. It keeps TOON only if it wins by **≥15%**.
3. It records the choice in a `format` column, so nothing has to guess later.

Uniform structure gets the TOON win. Irregular prose stays plain text, because pretending otherwise would cost clarity and save nothing. The 15% floor is deliberate: a marginal saving isn't worth carrying a second encoding the agent has to understand.

## Why this is invisible to you

You write Markdown, HTML, JSON, YAML, or TOON. You never choose an encoding. The [node tree](./node-tree.md) the agent sees is uniform regardless of how each node is stored underneath — `format` is an implementation detail of how many tokens that node costs, not something the read path has to reason about. The only place it surfaces is the token count: a TOON-encoded table is simply cheaper to fetch, and [search](./search.md) budgets stretch further because of it.
