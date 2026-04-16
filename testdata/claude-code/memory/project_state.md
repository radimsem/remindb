---
name: Webshop project state
description: Current sprint goals, blockers, and architectural decisions for the webshop project
type: project
---

## Current Sprint (2026-04-14 to 2026-04-28)

Goals:
- Implement checkout flow with Stripe integration
- Add order history page
- Fix cart persistence bug on session expiry

## Blockers

- Stripe webhook endpoint needs HTTPS tunnel for local development
- PostgreSQL connection pooling hitting limits under load test

## Recent Decisions

- Chose server actions over API routes for mutations
- Product images served from S3 via CloudFront, not stored locally
- Cart state lives in a database-backed session, not localStorage

**Why:** localStorage carts were lost on device switch and caused stale price issues. Database-backed sessions sync across devices and validate prices server-side.

**How to apply:** All cart operations go through `lib/cart.ts` server actions. Never read cart from client state.
