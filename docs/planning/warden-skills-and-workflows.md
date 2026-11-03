# Warden: one contact, skills, workflows that wake it

Settled on 2026-11-03 after the first show-and-tell. The audience found a hand-configured console too much. The answer is not fewer concepts underneath; it is one agent in front of them.

## Decisions

1. **Warden is the only thing the operator talks to.** It is the orchestrator of the tenant. The console is Warden's thread beside a panel that renders the backend's state; every button in the panel either opens a form only a human can fill (a credential) or sends Warden a message. Warden never takes a secret.

2. **Capabilities are skills.** A skill is a bundle: tools, a fixed policy table for those tools, a prompt fragment, and the state it reads. Setup is a skill (onboard, grant, API access, probe). Runbooks, workflows and agents are skills. Dashboards and schedules will be skills. Skills are code in the repo; Warden does not invent tools.

3. **Runbooks are written by conversation.** The operator describes an app in their own words: what runs on a cell, what is safe, what is never allowed, who to page. Warden writes the Markdown with those sections plus "Applies when", selects on the cell's app and tier labels, saves it, and says it wrote it from their words. A tenant starts with no runbooks. This closes the decision in `apps-and-cells.md`: the app is discovered on the cell, and the runbook is where the operator's knowledge of it lives.

4. **Monitoring workflows are tenant objects, and they are what wakes Warden.** A workflow has a schedule, a label selector over cells, a collector (a metric read, the provider state, or a log digest), a deterministic condition, and a trigger: a responder from the table, or Warden. It runs in the tenant runtime, costs no tokens, and records what it saw every run. Agents never poll. A model runs only when a workflow fires with the wake-warden trigger, and then only to decide which responder fits; the responder's run is the same gated run as before. The hard-wired signal rules became the seeded default workflows, visible and pausable.

5. **The log digest is the cheap tier, the probe the full one.** The digest aggregates on the cell with standard tools: new lines since the last run, how many are errors, the distinct error signatures with timestamps, host names and numbers stripped. It is a few hundred bytes and fires once per new signature. Nothing of ours is installed for it: on a VM it runs through the hands over SSH from the runtime on Warden's schedule, never from a cron on the host. The probe stays as the opt-in full-telemetry tier. (In the simulator the digest reads the world's logs directly.)

6. **Watches and standing orders are the operator's words.** In the product a monitoring workflow is a *watch*, because that is what people type. A watch with a pre-approved action is a *standing order*: "add disk alerts for production, and if disk is low rotate the logs, already approved" creates a metric watch (disk above 90 on three reads, on cells labelled env=production) that starts the disk-pressure responder, with rotate_logs allowed in advance. The order can only lift *ask* to *allow* for a catalogue action; free-form commands are never pre-approved and deny stays deny. The order belongs to the responder and the scope, so it applies whichever watch noticed first: when a log watch wakes Warden about disk pressure on a covered cell, Warden starts the responder with the same order attached. A new log watch takes a baseline on its first run and only fires on lines written after it was created.

7. **Specialists come from templates.** A template is a deterministic spec: tools, effort, tool budget, and the responders it takes over. Warden binds a template to a scope (labels) and from then on those responders start the specialist for cells in scope. Warden never writes a prompt or grants a tool a template does not carry. Templates are versioned in the repo and listed on the Agents page.

## What the tenant sees

All configuration is done by Warden in conversation. The panel beside the thread is the record of what has been done: tenant, connections, cells, runbooks, watches with their last result and fired count, agents, and whatever needs the operator's approval. Nothing in the panel configures; it shows, and every button is a message to Warden or a form only a human can fill.

## The show-and-tell line

Sign in, create the tenant, Warden asks which provider. "UpCloud": Warden opens the token form, the operator saves it, Warden says what it found and onboards. "Grant edge-01": Warden hands over the line, the level rises. "Add a runbook for edge-01": Warden asks four things and writes it. "Watch the logs on edge-01 every 10 seconds and wake me on errors": Warden creates the watch. "Add disk space alerts for production, and if disk is low clean up the logs, already approved": Warden creates the standing order. nginx crashes: the log watch fires on the new signatures, Warden says what fired and starts the service-down responder, the fix needs approval and the panel shows it, the finding resolves. The disk fills: the standing order fires, the investigator follows the runbook and the log rotation runs with the policy line "standing order", no approval, and the finding resolves. "Add an nginx specialist for the edge servers": Warden adds it from the template and the responder now starts it.

## Open

- Metric watches are data now (metric, operator, threshold, hold, selector); only the two edge checks that need provider state keep a named rule.
- The runbook composer in scripted mode sorts sentences by keyword. With a model, Warden writes the sections properly; both paths say the operator should edit.
- Dashboards and scheduled jobs are the next two skills. A dashboard is a saved query set with a layout drawn by a deterministic renderer; a schedule is a catalogue action with a cadence.
- Every 10 seconds is demo pace. Production digests run every minute or more; the workflow says its cadence on the panel.
