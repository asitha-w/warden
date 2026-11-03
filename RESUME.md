# Resume notes

Working folder and git repo for Warden. Private GitHub repo: https://github.com/asitha-w/warden (default branch `main`). Forked from Beacon on 2026-11-03 with the brand renamed throughout; the Beacon history (2026-10-29 to 2026-11-03) is in https://github.com/asitha-w/beacon.

## Why Warden exists

The first audience found Beacon's setup too much to hold: tenant, connection, cell, level, grant, probe. Warden keeps that model underneath and takes the human out of configuring it. The flow is: create the tenant, add a cloud provider credential, then the orchestrator takes over. It says what the connection found, what it can see and do at the current level, and hands over the one line to run when it needs more reach, then verifies the level went up. The configuration view is one overlay the orchestrator opens and points into: three bands (tenant, connections, cells), each showing real state and the one next action. The chat sits beside it.

Rules that carry over: the orchestrator guides, it does not own state; the overlay renders the backend's state, the model only proposes. Secrets go through a small form the agent opens, never through the chat. Customer-side steps (consent, grant, API access) stay human; the agent delivers them ready to run and checks that they landed. Every change the orchestrator makes goes through the same tools and policy gate as a remediation.

Build order agreed on 2026-11-03:

1. Setup tools for the orchestrator: validate connection, list catalogue, onboard cells, mint the grant token, request API access, check level. These wrap the existing `/api` endpoints behind the policy gate.
2. A setup-state endpoint the orchestrator reads in one call: tenant, connections, cells with level and the next step for each.
3. The new console: the overlay with three bands and the chat beside it, built from the existing design system, chat, cell cards and docs renderer. The old pages stay reachable until the new flow covers them.
4. Re-record the walkthrough from the three-step flow.

## Where to start

- `./start.sh` runs everything on http://localhost:8080; `./start.sh --dev` serves the console from `web/` so edits show on refresh. `README.md` explains the model and the run flow.
- `docs/architecture/warden-architecture.png` is the one-page picture for the team (drawn for Beacon, still accurate for the backend). `tools/record/record.py` records the walkthrough with Playwright; the Beacon recordings were not copied over.
- `docs/planning/` holds the product planning notes: the technical gap list and one file per settled topic. Read `technical-gaps.md` there before picking the next design question.
- `web/css/warden.css` is the design system source of truth. The claude.ai artifacts from Beacon's first day are listed in the Beacon repo's `ARTIFACTS.md`.

## The model, as agreed on 2026-10-30

- Tenant = a collection of cells served by one runtime (isolated workers, own credentials, static egress).
- Connection = an identity at a scope. Azure: directory, consent or own app registration, read scope, narrower write scope; validation lists roles and fills a catalogue. UpCloud: sub-account token, its grants are the frame. Connections manage nothing by themselves.
- Cell = one managed VM, cluster or server, onboarded from a catalogue, with its own isolated channel, labels, and a tier: basic (provider sensors and actions) or probe (the OpenTelemetry Collector installed; telemetry settings render into its config over OpAMP). The investigator can only read what the telemetry enables.
- Isolation is real at Warden's layer (channel, policy, budget per cell) and at the provider's layer only as far as the identity's scope; the Cells page shows dedicated versus shared identity.
- Labels are the only glue: runbooks select cells by label, owner and criticality are labels, `app=` is just a label. There is no app object. Confirmed 2026-11-03: the cell is the unit of reach, a connection finds cells, applications are discovered on a cell by the agent and recognised by runbooks (`docs/planning/apps-and-cells.md`).
- One agent team per tenant: orchestrator (talks to the operator, routes by the responder table), investigator (model), remediator and verifier (no model). "Agent" means these; the thing on a cell is the "probe".
- Runbooks guide, the responder policy decides (allow, ask, deny).

## Earlier decisions still standing

Dark-first console in the shadcn look; polls are truth and webhooks are hints; probe is an unmodified OpenTelemetry Collector over OpAMP; Postgres, ClickHouse, Temporal, OPA, NATS JetStream, Go; PoC on an UpCloud host in docker compose.

## Settled on 2026-10-31 (design only, no code)

- Levels of reach per cell: VM Level 0 connected, Level 1 granted; Kubernetes Level 0 connected, Level 1 API access, Level 2 probe. `docs/planning/cell-access-model.md` is the reviewed reference of what must exist at each level on Azure and UpCloud.
- VM hands: SSH direct from the tenant's static egress, reverse tunnel fallback, never a jump box; Ansible on top; per-tenant SSH CA with short-lived certificates; the "Warden grant" is one line the operator runs. `docs/planning/hands-ssh.md`.
- Twenty comparable products studied from their own docs: `docs/planning/competitor-access-models.md`.
- Still to test on real infrastructure: listed at the end of `cell-access-model.md`.
- The model is in the product: `web/docs/` (seven Markdown pages, the `::layers` graphic, section anchors) at `#docs`, linked from the tenant, connect and connections pages; the orchestrator has a `read_docs` tool and its prompt tells it to guide setup from those pages. Keep `web/docs/` in step with `docs/planning/cell-access-model.md`.

## Flow change on 2026-10-31

Sign in, create tenant, land on Findings. The console carries setup one step at a time: the rail item for the next step pulses with a tip and the Findings page shows the same step with a button and a doc link (no connection, onboard, install the probe). The old `#setup-connect` wizard page still exists but nothing routes to it; remove it once the recording no longer needs anything from it.

## App consistency pass, 2026-10-31 (evening)

The app now follows the reviewed access model. Cells carry `level` (0, 1, 2), `hands` (none, ssh, ssh-tunnel, kube-api) and a `grant`; tiers are gone. The Azure form asks for directory, subscription, identity (consent with a consent button and object id, or bring-your-own with federated, certificate or secret) and scopes, and shows the manual steps with the role assignment commands and the custom role JSON. The UpCloud form asks for sub-account, token and validity, and permission rows, and shows the token and grant calls. The cell page shows the ladder of reach, the manual steps for the next level (firewall rule, grant line with a one-time token, tunnel option; AKS and UKS API access steps; the probe manifest), the hands, and revocation. New endpoints: `POST /api/cells/{id}/grant`, `/api-access`, `/probe`; `/api/probe/install` is now the demo shortcut that raises every cell. A UKS cluster `uks-edge` is in the UpCloud inventory. The `#setup-connect` wizard is gone. Validation simulations follow the model (Reader only; custom roles; UpCloud audit log, token expiry, no split).

## Open items, in the order we discussed

1. Live-mode run with an API key: the investigator and orchestrator prompts have never been exercised against the model.
2. Cell page follow-ups: receivers per kind, a "test this log source" action, telemetry presets.
3. Responders and runbooks per cell page (which responders are in scope for this cell).
4. Consumption: budgets per tenant, maybe per cell.
5. Re-publish the design system artifact from the current stylesheet when the screens settle.
6. Decide whether `main` tracks `app/v1` continuously or only at milestones.
