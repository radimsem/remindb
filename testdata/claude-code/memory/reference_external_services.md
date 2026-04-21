---
name: External service references
description: Where to look for logs, dashboards, and docs for the webshop's external integrations
type: reference
---

# External Services

## Stripe

- Dashboard (test mode): https://dashboard.stripe.com/test/payments
- Webhook inspector: https://dashboard.stripe.com/test/webhooks — filter by `endpoint_secret` prefix `whsec_local_`
- API reference: https://docs.stripe.com/api
- SDK version pin: `stripe@17.3.1` — do not upgrade without running the full E2E checkout suite

## CloudFront + S3

- Image origin: `s3://webshop-product-images-prod` (region: `eu-west-1`)
- Distribution: `d3x4y5z6.cloudfront.net`
- Invalidation: use `aws cloudfront create-invalidation --distribution-id E2ABC123 --paths "/products/*"` after bulk image updates
- CORS policy managed in `terraform/cloudfront/cors.tf` — do not edit on the console

## Sentry

- Project: `acme-webshop-frontend`
- DSN lives in `SENTRY_DSN` env var, set in Vercel and in `.env.local` for dev repro
- Performance traces: sample rate 0.1 in prod, 1.0 in staging

## PostgreSQL (production)

- Primary: `webshop-prod.cluster-xyz.eu-west-1.rds.amazonaws.com:5432`
- Read replica: same host, `:5433` (readonly user only)
- Connection pooling via PgBouncer at `pgbouncer.internal:6432`
- Backups run at 02:30 UTC, retained 14 days, PITR enabled

## Resend (transactional email)

- Dashboard: https://resend.com/emails
- Domain: `mail.webshop.acme.com` — SPF/DKIM records in `terraform/dns/resend.tf`
- Templates rendered from `emails/*.tsx` (React Email); preview locally with `pnpm email dev`
