# Technical gaps, 2026-10-31

Everything the app does today is in-memory or simulated. The PRD and the decisions name a technology for each area below, but no mechanism has been specified. Settle them in this order; each one references the previous.

1. **Tenant runtime.** What it physically is on the UpCloud host, how a cell's "isolated channel" maps onto NATS subjects, Temporal task queues and OPA policy scope, and how static egress per tenant is achieved.
2. **Probe path.** The OpAMP server side, how a collector authenticates per cell on first dial-out, how telemetry settings render into collector config, where the OTLP data lands in ClickHouse, and what the investigator tools actually query. See `probe.md`.
3. **Provider sensors.** Azure Monitor metrics, Activity Log and Resource Health polling, the UpCloud servers API, poll cadence, and how samples are stored so the signal loop evaluates rules over real data instead of the simulated world.
4. **Azure identity.** The multi-tenant app registration and consent flow, listing role assignments to derive capabilities, bring-your-own app with federated credential, and the separate write identity the remediator checks. See `provider-access-and-control.md`.
5. **Durable runs.** The Temporal workflow shape for a run: investigate activity with the model loop, approval as a signal, remediate and verify activities, retries and timeouts.
6. **Policy gate.** The OPA input document: responder, mode, cell, action, write-scope coverage, budget state.
7. **Persistence and secrets.** Postgres schema for the domain types, what goes to ClickHouse, where credentials live.
8. **Budgets.** Per-tenant and per-cell consumption limits and what happens when one is hit.

Started with 2 and 4 on 2026-10-31 because they decide what a cell can be at all. Settled the same day: `cell-access-model.md` (levels of reach per cell kind, reviewed), `hands-ssh.md` (how Warden acts on a VM). The probe path (2) now has its host and cluster prerequisites fixed; its OpAMP supervisor question stays open.
