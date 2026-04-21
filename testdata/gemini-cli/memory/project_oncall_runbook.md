---
name: Oncall runbook notes
description: Recent runbook updates, escalation contacts, and dashboard links for infra-api oncall
type: project
---

# Oncall Runbook (current as of 2026-04-19)

## Weekly rotation

- Current week (2026-04-19 → 2026-04-25): Priya
- Next week (2026-04-26 → 2026-05-02): Marcus
- Week after (2026-05-03 → 2026-05-09): Jordan

Rotation handoff happens Friday 16:00 CET. Outgoing oncall posts a summary of open alerts and ongoing investigations in `#platform-oncall`.

## Escalation

1. **First responder:** oncall engineer (pager via Opsgenie)
2. **Subject-matter escalation:** infra-api team lead (currently Priya)
3. **Cross-team (auth-proxy, notification-service):** escalate through platform group chat `#platform-sos`
4. **Incident commander:** if impact is multi-service or customer-facing, engage the IC via PagerDuty incident_type=major

## Dashboards

- Primary health: https://grafana.internal/d/infra-api
- Kubernetes cluster: https://grafana.internal/d/k8s-cluster
- Request tracing: https://tempo.internal/explore?datasource=infra-api
- SLO burn rate: https://grafana.internal/d/slo-infra-api — page on burn > 5%/hr sustained for 30min

## Known open concerns

- **k8s client retry storm risk (PLAT-1903 follow-up):** jitter fix shipped 2026-04-05 but observability for retry amplification is incomplete. Track on PLAT-1945.
- **Namespace finalizer rollout (PLAT-1847 follow-up):** finalizer added to new namespaces, but existing production namespaces still missing the annotation. Backfill planned 2026-04-22.
- **Vault token renewal:** service-account token renewer has no alerting. If renewal fails silently, requests start 403-ing ~12h later. Priya is writing a synthetic check; ETA 2026-04-24.

**How to apply:** If you get paged for a 403 cascade, check Vault token renewal status first (`vault token lookup -accessor $SA_ACCESSOR`). If 5xx spikes with no deploy, check retry amplification on the PLAT-1945 panel before blaming downstream.
