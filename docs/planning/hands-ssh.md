# Hands on a VM: SSH from the runtime, 2026-10-31

Decided with the user on 2026-10-31. Settles the UpCloud server hands question and unifies Azure VM hands with it.

## The design

- **Transport is SSH, direct from the tenant runtime.** The runtime has one static egress IP. The cell allows port 22 from that IP only: an NSG rule plus the host firewall on Azure, a server firewall rule on UpCloud that Warden can add through the API. No jump box, ever.
- **Fallback when inbound is refused or the cell has no public address:** the cell opens a reverse SSH tunnel to the runtime with a plain openssh systemd unit (`ssh -R`), and Warden connects through it. Outbound only, no Warden-written agent.
- **Execution layer is Ansible over that SSH.** Each catalogue action is a role shipped and pinned with the runtime: check mode and diff are the precondition and the approval content, assert tasks are the verification, the JSON callback is the evidence. Free-form commands are the shell module under `ask`. Run through ansible-runner inside the tenant's container, one temp directory per run. Kubernetes cells stay on the API; Ansible is for hosts.
- **Azure Run Command** is the bootstrap on an existing Azure VM (place the grant, install the probe) and the fallback when 22 is blocked. On UpCloud the bootstrap is the install line or user data.

## Levels of reach on a VM cell

Decided 2026-10-31. The connection comes first; the grant moves a cell up one level.

- **Level 0, connected.** The connection discovered the VM. Warden manages it from outside: state, activity log, hypervisor metrics, power actions. Nothing on the VM. This is today's basic tier.
- **Level 1, granted.** The operator ran the Warden grant on the VM. The probe and the SSH trust arrive together, so the cell gains inside telemetry and in-guest hands in one step. The grant reports the instance metadata so the cell links to its discovered record. This is today's probe tier.

The Cells page shows the level per cell. These are levels of reach; the PRD's autonomy levels apply on top through the policy gate.

## Levels of reach on a Kubernetes cell

One level more than a VM, and the last two in the other order.

- **Level 0, connected.** The connection discovered the cluster. Node pools, cluster health, activity log, provider actions (scale a pool, upgrade, stop).
- **Level 1, API access.** Warden holds a kube context and the API server allows the egress IP, or, for a private AKS cluster, a tunnel pod (plain OpenSSH client, outbound only) forwards the API server to the runtime; a jump VM with the grant is the fallback. Live reads of workloads, events and pod logs, and all the hands: rollout undo, restart, scale, exec. AKS: a role assignment through the connection plus the authorized IP range. UKS: the API kubeconfig turned into a scoped service account plus the IP filter. No human on the cluster; the grant is a click.
- **Level 2, probe.** The collector DaemonSet and cluster collector, installed by Warden through Level 1. Adds node and kubelet metrics, container logs with history, retained events.

On a VM, telemetry and hands arrive together in the grant. On Kubernetes, hands come first and the probe is optional on top.

## The Warden grant

What the customer runs once per cell is the "Warden grant": one install line that creates a `warden` user, a sudoers drop-in, the SSH trust below, and the probe. Revoking the grant is deleting the user.

**To start with, the operator runs it by hand.** The cell page shows the line, the egress IP to allow on port 22, and the CA public key for comparison. The line fetches the script from the tenant's hostname with a one-time token; the script posts the host public key back with the same token, and Warden pins it. This is identical on Azure and UpCloud. Delivering the grant through Azure Run Command is a later convenience, not a dependency.

## Authentication to the VM

- **Per-tenant SSH certificate authority.** The runtime holds one CA key per tenant in the tenant's secret store. The grant installs the CA public key on the cell (`TrustedUserCAKeys`) with `AuthorizedPrincipalsFile` allowing principal `warden`. No private key per cell is ever stored.
- **Short-lived certificates per run.** For every run Warden mints a user certificate valid for minutes, principal `warden`, with `source-address` set to the egress IP and port forwarding, agent forwarding and X11 disabled. The certificate serial is the run id, so sshd's log on the host names the run. Revocation is not minting; a compromised CA is rotated by re-running the grant.
- **Host authentication.** The install line reports the host public key back to Warden at onboarding and Warden pins it; strict host key checking from then on. A changed host key puts the channel in `offline` with a finding, not a silent re-trust.
- **Host floor.** OpenSSH 8.2 or later, for the `Include` drop-in; the `Match User warden` block is closed with `Match All` and checked with `sshd -t` before reload. Python 3.9 or later for the pinned ansible-core.
- **No passwords, no Entra SSH, no shared keys.** Where a customer's sshd cannot take a CA (unusual, OpenSSH has supported it since 5.4), the fallback is a per-cell ed25519 key in `authorized_keys` with `from=` the egress IP and the same option restrictions, rotated by Warden.
- **Privilege.** `warden` has `NOPASSWD: ALL` sudo through a drop-in. Ansible's privilege escalation cannot be limited to commands (it runs a Python interpreter, not the catalogue command), so the catalogue and the policy gate are what limit what runs, and the cell page says the user has full sudo.

## Reviewed

The independent review of 2026-10-31 (`research/review-cell-access-model.md`) corrected the host floors above; `cell-access-model.md` is the reference.

## What the cell page shows

Hands: SSH direct or SSH tunnel, grant installed on date, CA trust present, host key pinned, sudo scope. A cell with the probe but no grant shows hands as none and every write responder as not applicable.
