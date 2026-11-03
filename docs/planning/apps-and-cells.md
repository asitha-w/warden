# Apps and cells

Settled on 2026-11-03 after the first audience feedback. The question was whether the cell should be the application, since the application is the smallest thing an operator worries about. It is not. The cell stays the unit Warden connects into; the application is what the agent finds running there.

## The decision

1. **The cell is the unit of reach.** One VM, one server, one cluster. Every boundary Warden has is per cell and physical: the channel in the runtime, the grant on the host, the probe credential, the firewall rule for the egress address, the level of reach. An application has none of these of its own, so it cannot be a cell. Two applications on one VM would be two cells that share every boundary, and the isolation shown on the Cells page would be false.

2. **A connection finds cells; it is not a cell.** A connection is an identity at a scope, and one connection discovers many cells. The cell is what Warden actually attaches to. The agent never works on anything bigger or smaller than a cell.

3. **The application is discovered, not declared.** Nobody onboards an app. The agent looks at a cell through the connection, and through the probe where one exists, and works out what runs there: a systemd unit, a container, a Deployment in a namespace. The console shows that as a fact about the cell, under "running here". The user's unit of concern comes out of the cell, it is not put in above it.

4. **Runbooks carry the application knowledge.** A runbook says how to recognise a workload and what to do about it. Supporting a new application means writing a runbook, not creating an object. This keeps the earlier decision that labels are the only glue: labels still select cells, and a runbook is still attached to a label selector; what is added is that a runbook also says what it applies to on the cell.

## What this means for the agent

- The orchestrator still routes by the responder table and hands the investigator the runbooks whose selector matches the cell.
- The investigator gains a discovery step: before reasoning about a signal, it lists what the cell runs, using the tools its level allows. At Level 0 that is only what the provider knows (VM state, cluster node pools). At Level 1 on a VM it is units, processes and listening ports through the hands; on a cluster it is workloads through the API. The probe adds what the telemetry shows.
- Each runbook gets an **applies when** block: a unit name, an image or container name, a Kubernetes label, a listening port, a file path. The investigator evaluates it against what discovery found and records in the run which runbooks applied and why. This is the deterministic half of "guided by runbooks"; the model reasons only inside what the runbook scoped.
- A cell can run several applications and an application can show on several cells. The run binds to one cell; a finding names the application so the operator reads "billing on vm-prod-02", not "vm-prod-02".

## What this means for the console

- The cell page gets a **Running here** section: the applications discovery found, which runbook recognised each, and when it was last seen. Below Level 1 it reads "unknown until granted" rather than showing nothing.
- The runbook page shows the **applies when** block beside the label selector and lists the cells where it currently matches.
- Findings and runs carry the application name as a column where discovery has one.
- No Apps page, no app object, no app onboarding.

## Open

- The exact tool the investigator uses for discovery on a VM (a read-only Ansible fact module versus a catalogue action) belongs with `hands-ssh.md`.
- Whether discovery runs on a schedule per cell or only at the start of a run. The first version runs it per run and caches the result on the cell with a timestamp.
