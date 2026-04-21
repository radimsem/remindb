---
description: Generate, review, and apply a drizzle migration with safety checks
argument-hint: "<migration-description>"
allowed-tools: ["Bash(pnpm drizzle-kit *)", "Read", "Edit"]
---

Generate a new drizzle migration for: $ARGUMENTS

## Steps

1. Run `pnpm drizzle-kit generate` to produce the migration SQL from the current schema diff.
2. Read the newly created file under `drizzle/migrations/`. Summarize:
   - Tables created / dropped / altered
   - Columns added with NOT NULL constraints (these need backfill or a default)
   - Indexes added (are any on large tables? if so, use CONCURRENTLY)
   - Foreign key changes (potential lock escalation)
3. If the diff contains any risky operation (DROP COLUMN, DROP TABLE, ADD NOT NULL without default, ALTER COLUMN TYPE on a large table), **stop and ask** before proceeding.
4. Otherwise, run `pnpm drizzle-kit migrate` against the local dev database.
5. Verify the `__drizzle_migrations` table shows the new migration with a fresh `created_at`.

## Do not

- Edit migration files by hand. If the generated SQL is wrong, fix the schema in `drizzle/schema.ts` and regenerate.
- Run against production — this command is dev-only. Production migrations go through the GitHub Actions deploy workflow.
