# Security and data

The short version of what Warden installs, what it can reach, and how it handles your credentials and data. The [Azure](#docs/azure) and [UpCloud](#docs/upcloud) pages have the per-level detail.

## What runs on your side

- On a Level 1 VM: OpenSSH (already there), a `warden` user, the OpenTelemetry Collector with its supervisor. Nothing Warden wrote.
- On a Level 2 cluster: the OpenTelemetry Collector as a DaemonSet and a Deployment, with a read-only ClusterRole. On a private AKS cluster, one pod running the plain OpenSSH client for the tunnel.
- At Level 0: nothing.

## Network

- Outbound from your cells: HTTPS 443 to your tenant hostname.
- Inbound to your cells: SSH 22 from the tenant's one static egress address, only on VMs you granted. Nothing else. The reverse tunnel removes even that.
- The egress address is on the tenant page and in the API. It changes only with notice.

## Identities and credentials

- Azure: a service principal in your directory. Consent or a federated credential; no secret changes hands by default. `Reader` for reading; a custom role with only the catalogue's actions for writing. Validation refuses a read identity that carries write actions.
- UpCloud: a sub-account token, valid at most a year, pinned to the egress address. Permissions are per resource with no read and write split; the policy gate is the split.
- Kubernetes: Warden's own identities, through Azure RBAC for Kubernetes on AKS, through dedicated ServiceAccounts on UpCloud. Never Secrets. On UpCloud, the admin kubeconfig is used once and discarded; Warden records when.
- SSH: a per-tenant certificate authority; certificates valid for ten minutes, one per run, serial equal to the run id. No private key per host is stored anywhere.

All credentials live in the tenant runtime's secret store, encrypted at rest. They are used by deterministic code after the policy gate. The model never sees them.

## Actions

Every action passes the responder policy: allow, ask, deny. An approval shows the exact arguments, binds to them, is single-use and expires. Delete-class actions on cells and clusters, and cluster upgrades, are denied in every mode. On a VM, actions are a catalogue of Ansible roles; a free-form command exists only under ask and shows the command.

## Audit

Every action is recorded twice: in Warden's run record with the policy decision, the arguments and the evidence, and on your side in the provider's activity or audit log, or in the host's sshd and sudo logs where the run id appears as the certificate serial.

## Revocation

Each level can be revoked from your side without Warden: remove the role assignment or permission, delete the `warden` user, remove the firewall rule, delete the namespace. The per-level pages list the step.

## Data

Telemetry is stored per tenant in the tenant's own store. Logs and tool results go to the model only as the focused evidence windows an investigation reads, after redaction of known secret formats. Model providers are used under zero-retention terms and your data is not used for training.
