# How Warden reaches your infrastructure

Warden manages your estate as **cells**: one VM, one server, one cluster at a time. How far Warden can see into a cell and act on it is its **level of reach**. This page is the model; the [Azure](#docs/azure) and [UpCloud](#docs/upcloud) pages give the exact steps for each level, and the [how-to list](#docs/howto) links straight to them.

## The pieces

- **Tenant.** Your workspace. It gets its own **runtime**: isolated workers, its own credentials, and **one static egress address** that every allowlist on your side uses. The address is on the tenant page and never changes without notice.
- **Connection.** An identity at a scope on a provider: a service principal in your Entra directory, or a sub-account token on UpCloud. A connection discovers what Warden could manage and reads what the provider knows from outside. It manages nothing by itself.
- **Cell.** The most specific thing Warden manages, onboarded from what a connection discovered. Each cell has its own isolated channel in the runtime, its labels, and its level of reach. A connection finds cells; the cell is what Warden attaches to. The agents never work on anything bigger or smaller than a cell.
- **Grant.** The one line you run on a VM to let Warden in. It installs the probe and the SSH trust together. See [the Warden grant](#docs/grant).
- **Probe.** An unmodified OpenTelemetry Collector that dials out to the runtime. It collects; it never executes. See [the probe](#docs/probe).
- **Agents.** One team per tenant: the orchestrator you talk to, the investigator, and the deterministic remediator and verifier. The investigator never holds a credential that can change anything.
- **Runbooks.** Your own notes, in Markdown, attached to a label selector over cells. A runbook also says what it applies to on the cell, such as a unit name, a container image or a Kubernetes label, so the agents know which of your applications they are looking at.

## Applications are found, not declared

You do not onboard an application. The agents look at a cell through its connection, and through the probe where one exists, and work out what runs there: a systemd unit, a container, a Deployment in a namespace. The cell page lists it under **Running here**, with the runbook that recognised it. One cell can run several applications and one application can run on several cells; a finding names both, so you read "billing on vm-prod-02".

What the agents can see depends on the cell's level. At Level 0 they know only what the provider knows. On a VM, units and ports appear at Level 1 with the grant; on a cluster, workloads appear at Level 1 with API access. Until then the cell page says "unknown until granted".

## Levels of reach

Reach is set per cell by what exists. The provider side is Level 0; everything deeper needs something on your side.

::layers vm

**VM or server** (Azure VM, UpCloud server)

| Level | Name | What exists | What Warden can do |
|---|---|---|---|
| 0 | Connected | A connection whose scope covers the VM | See state, hypervisor metrics, the activity or audit log; start, stop, restart under policy |
| 1 | Granted | The Warden grant was run on the VM; port 22 allows the tenant egress address | In-guest metrics and logs through the probe; actions on the host through SSH, run as an Ansible catalogue under policy |

::layers k8s

**Kubernetes cluster** (AKS, UpCloud Managed Kubernetes)

| Level | Name | What exists | What Warden can do |
|---|---|---|---|
| 0 | Connected | A connection whose scope covers the cluster | See cluster and node pool state, versions, the activity or audit log; scale a pool, start, stop under policy |
| 1 | API access | A kube identity for Warden and the API server allows the tenant egress address | Read workloads, events and logs; rollout undo, restart, scale, exec in permitted namespaces under policy |
| 2 | Probe | The probe runs in the cluster | Node and kubelet metrics, container logs with history, retained events |

On a VM, telemetry and hands arrive together with the grant. On a cluster, hands come at Level 1 and the probe is a separate step on top.

## Autonomy is the other axis

Reach says what is possible. **Autonomy** says what Warden may do without you, and it is set per responder, not per cell: observe, investigate, recommend, execute after approval, execute within bounds. Every action passes a deterministic policy gate outside the model: `allow` runs now, `ask` opens an approval that shows the exact arguments and expires, `deny` refuses. Delete-class actions on cells and clusters are denied in every mode.

## Two identities per connection

Where the provider allows it, a connection carries a **read identity** that cannot write and a **write identity** held only by the remediator. Validation refuses to mark a connection connected when the read identity carries write rights. Where the provider has no read and write split, the connection page says so, and the policy gate is the only split.

## What never happens

- Nothing Warden wrote runs on your cell. The probe is the stock OpenTelemetry Collector; the hands are OpenSSH and Ansible; the grant is a script you can read first.
- No secret is stored on a cell. Cells hold the tenant's SSH certificate authority **public** key and a one-time enrollment token.
- No inbound connection from Warden other than SSH on port 22 from the one egress address, and only on VMs you granted. Where you refuse inbound, the cell dials out instead.
- The model never sees a credential. Credentials live in the tenant's secret store and are used by deterministic code after the policy gate.
