---
project: webshop
framework: next.js
language: typescript
---

# Project Instructions

## Overview

This is a Next.js 15 e-commerce application with App Router, Server Components, and Drizzle ORM on PostgreSQL.

## Architecture

- `app/` — Next.js App Router pages and layouts
- `components/` — React components, co-located with their tests
- `lib/` — Shared utilities, database client, auth helpers
- `drizzle/` — Database schema and migrations

## Conventions

### Code Style

- Use `const` by default, `let` only when mutation is required
- Prefer named exports over default exports
- Components use PascalCase files, utilities use camelCase
- All database queries go through `lib/db.ts`, never import drizzle directly in components

### Testing

- Unit tests with Vitest, co-located as `*.test.ts`
- E2E tests with Playwright in `tests/`
- Test the behavior, not the implementation
- Mock external services (Stripe, email) at the boundary

### Git

- Conventional commits: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`
- One logical change per commit
- PR titles under 70 characters
- Squash merge to main

## Common Tasks

### Adding a New Page

1. Create route in `app/(shop)/[route]/page.tsx`
2. Add server-side data fetching in the page component
3. Create client components in `components/` if interactivity is needed
4. Add E2E test covering the happy path

### Database Migrations

```bash
pnpm drizzle-kit generate
pnpm drizzle-kit migrate
```

Never edit generated migration files. If a migration is wrong, create a new one.
