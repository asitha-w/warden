# Provider access and control, 2026-10-31

The reference that supersedes the per-kind detail here is `cell-access-model.md`, reviewed on 2026-10-31; this file keeps the reasoning and the alternatives considered.

The question: how far can Warden really reach into a cell, what must exist on the cell's side, what identity and network path Warden needs, and what the comparable products ask of their customers. Four cell kinds are in scope: an Azure VM, an AKS cluster, an UpCloud server, an UpCloud Managed Kubernetes (UKS) cluster.

## Three paths into a cell

Every capability Warden has on a cell comes through one of three paths. Naming them keeps the per-kind detail honest.

| Path | What it gives | What it needs |
|---|---|---|
| **Provider API** | Discovery, state, provider metrics, activity log, resource health, provider actions (start, stop, restart, scale, and on Azure: run a script in the guest) | An identity with roles at a scope. No network path to the cell, only to the provider's API. |
| **Kubernetes API** | Everything kubectl can do: read workloads and events, logs, exec, rollout undo, apply manifests | A kube context (credential and endpoint) and network reachability from the tenant runtime to the API server. |
| **In-guest** | Metrics the provider does not have (memory, disk usage, processes), log files, application receivers, and command execution | Something running on the cell. The probe gives telemetry. Command execution is a separate concern (below). |

The probe (an unmodified OpenTelemetry Collector) is telemetry only. It cannot run a command or read an arbitrary file. The "no Warden-written agent" decision holds only because, for three of the four cell kinds, the provider API or the Kubernetes API already provides hands. The UpCloud server is the exception and needs a decision (see Consequences).

## Azure VM

**Cell side, must exist.** The Azure VM Agent (waagent) installed and reporting Ready, which is the default on marketplace images. Outbound HTTPS to Azure public IPs, otherwise scripts run but results never return. Nothing inbound. Nothing else is required for the basic tier.

**Identity.** One service principal in the customer's directory. Read scope: `Reader` at subscription or resource group (`Monitoring Reader` adds nothing for platform metrics, the Activity Log or Resource Health). Write scope, as a custom role rather than Virtual Machine Contributor:

```
Microsoft.Compute/virtualMachines/runCommands/write
Microsoft.Compute/virtualMachines/runCommands/read
Microsoft.Compute/virtualMachines/runCommands/delete
Microsoft.Compute/virtualMachines/restart/action
Microsoft.Compute/virtualMachines/start/action
Microsoft.Compute/virtualMachines/deallocate/action
```

The older action-style run command uses `Microsoft.Compute/virtualMachines/runCommand/action` and is what Virtual Machine Contributor carries.

**Command execution.** Managed Run Command. Each command is an ARM sub-resource `virtualMachines/{vm}/runCommands/{name}`, runs asynchronously as root on Linux through the run command handler extension, default timeout 90 minutes, instance view keeps the last 4 KB of stdout and stderr, full output goes to an append blob when a SAS URL is supplied. Warden creates one named `warden-<runid>`, polls the instance view, reads output, deletes it. `read_file` is `cat` through the same path. The action-style variant allows one script at a time and cannot be cancelled; the managed variant is the one to use. Azure SRE Agent itself documents no in-guest VM path, only Azure CLI control-plane commands, so this is a step beyond what Microsoft's own agent does.

**Probe install.** A run command that fetches and runs the installer. Alternatively the Custom Script Extension. Both need only the write scope above. The collector then dials out on 443; no inbound rule.

**Sensors without the probe.** Azure Monitor host metrics (CPU percent, disk and network bytes, VM availability), Resource Health, Activity Log. Memory and disk usage percent are guest metrics and need an agent, so they belong to the probe tier.

**Alternatives considered.** Entra-based SSH (`AADSSHLoginForLinux` extension, `Virtual Machine Administrator Login` role) needs an inbound path and a system-assigned identity on the VM; it does not add anything over run command for Warden. Whether a service principal can use it is unverified. Azure Arc gives the same run command surface to machines outside Azure and is noted under UpCloud.

## AKS

**Cell side, must exist.** Microsoft Entra integration enabled on the cluster with Azure RBAC for Kubernetes (`--enable-aad --enable-azure-rbac`). Clusters on local accounts only can hand Warden a cluster-admin certificate through `listClusterAdminCredential`, which is acceptable for a PoC and nothing else. An API server reachable from the tenant runtime: either public with the tenant's egress IP in the authorized IP ranges, or private (see below).

**Identity.** The same service principal. Read: `Azure Kubernetes Service RBAC Reader` at cluster scope, plus `Microsoft.ContainerService/managedClusters/listClusterUserCredential/action` to fetch the kubeconfig. Write: `Azure Kubernetes Service RBAC Writer` scoped to the namespaces Warden may touch, or a custom role with the data actions for deployments write, pods delete and `pods/exec/action`. The kube client authenticates with the service principal's token using the kubelogin `spn` flow against the AKS server application audience. Reader and Writer can both be scoped to a single namespace, which maps directly onto a cell's write scope.

**Network.** Public API server: add the tenant egress IP to `apiServerAccessProfile.authorizedIPRanges`. Private cluster: no path from outside. The fallback is `az aks command invoke` (`Microsoft.ContainerService/managedClusters/runCommand/action` and `commandResults/read`), which runs kubectl in a pod inside the cluster, caps output at 512 kB, and Microsoft documents as not designed for programmatic access. Private clusters stay at Level 0 until the cluster probe carries a reverse tunnel; command invoke is not used.

**Command execution.** All through the Kubernetes API, nothing on the nodes: rollout undo (set the deployment template back to the previous ReplicaSet), rollout restart (annotation), restart a pod (delete), exec (`pods/exec`). Node-level reads are out of scope.

**Probe install.** Manifests applied through the same API into a `warden-system` namespace: a DaemonSet for node telemetry and a single-replica Deployment for cluster telemetry, with a ClusterRole limited to get, list and watch. Applying them needs write access to that namespace, so the probe tier on a cluster needs the write identity at least at onboarding. The alternative, the customer applying the manifest themselves, keeps Warden read-only on the cluster and is what most comparable products do.

## UpCloud server

**Cell side, must exist.** Nothing for the basic tier. For the probe: outbound 443.

**Identity.** A sub-account with API access enabled. The main account grants permissions per target: `server` with a server UUID, or `*` for all. Permissions are per resource and, as far as the documentation shows, not split into read and write. The `server.read` and `server.power` grants in the current code do not exist natively; Warden's own gate has to carry the split, and the Connection page should say so.

**Provider API.** Server details (state, plan, IPs, tags), start, stop, restart, firewall rules per server, SDN private networks, storage, and the audit log API (actor, source IP, 180 days). No guest metrics, no in-guest execution. Authentication is an API token valid at most a year, pinned to the egress IP. The metadata service at 169.254.169.254 is for the guest to read about itself and gives Warden nothing.

**Command execution: a decision.** Three options.

1. **SSH from the tenant runtime.** At onboarding Warden generates an ed25519 key per cell and keeps the private key in the tenant's secrets. The operator runs a one-line install that adds the public key and installs the probe; for servers created through Warden, `user_data` does the same at first boot. Warden adds a firewall rule allowing port 22 from the tenant egress IP only. Commands and file reads run over SSH with the same policy gate as run command. Cost: an inbound port, and the install step is the customer's.
2. **Azure Arc.** Arc-enabling the server gives Azure's run command and extensions on an UpCloud machine. Forces an Azure dependency on an UpCloud-only customer. Keep as an option for mixed estates.
3. **A Warden command channel beside the probe.** A small dial-out executor. Reopens the no-agent decision and the security burden that came with it.

Recommendation: option 1 for the PoC, with the executor (option 3) kept as the design that a probe-tier cell could grow into if customers refuse inbound SSH.

**Probe install.** `user_data` on creation, or the operator-run script on existing servers.

## UpCloud Managed Kubernetes

**Cell side, must exist.** `control_plane_ip_filter` on the cluster includes the tenant egress IP. Warden can set it itself through the modify-cluster call if it holds the permission.

**Identity.** Sub-account permission with target type `managed_kubernetes` on the cluster UUID. The kubeconfig from `GET /kubernetes/{uuid}/kubeconfig` is a client certificate for the user `<cluster>-admin` with undocumented lifetime and rotation only through UpCloud support. UKS also offers OIDC authentication in beta on Kubernetes 1.34 and later, which is the forward path. So at onboarding Warden uses it once: create `warden-system`, a ServiceAccount, a read ClusterRole, a write Role per allowed namespace, mint a service-account token, store that, and discard the admin kubeconfig. The Cells page shows the identity as dedicated. Verify the kubeconfig's actual identity against a real cluster.

**Everything else** is as for AKS: execution through the API, probe as DaemonSet plus Deployment, applied through the API.

## What comparable products ask for

Twenty products were checked against their own documentation on 2026-10-31; the comparison is in `competitor-access-models.md` and the per-vendor detail in `research/`. The short version:

- Read-only by default everywhere, with actions as a separate explicit grant.
- In-cluster components are outbound-only, enrolled with a one-time token, non-root, read-only RBAC with no Secrets and no exec.
- Nobody reaches into a VM. Azure SRE Agent documents no in-guest path; AWS and Google neither. PagerDuty's Rundeck runner (SSH, WinRM, SSM) and Shoreline's host agent are the only host executors, and Datadog's runner runs only scripts declared ahead of time.
- The model never holds a write credential; a separate executor or gateway does.
- Egress IPs are a published contract.

Warden reaching into VMs through run command, SSH and the probe, and completing the fix instead of handing back a pull request, is the gap it fills for small teams on VMs. The burden that comes with it is in the next section.

## Consequences for Warden's model

1. **A third capability on a cell: hands.** Sensors (provider), telemetry (probe) and hands (how Warden can act: run command, Kubernetes API, SSH, or none). The cell page shows all three; a responder whose action needs hands the cell lacks is shown as not applicable instead of failing at the gate.
2. **Capabilities derive from role assignments.** Validation lists the assignments and maps them: discover, metrics, activity, resource-health, run-command, vm-power, aks-read, aks-write, aks-command-invoke. The connection page already shows a capability list; this is where it comes from.
3. **Azure identity.** A multi-tenant Warden app with admin consent creates the service principal in the customer's directory; the customer assigns roles to it. Bring-your-own app with a federated credential needs Warden to run an OIDC issuer the customer's app trusts, because the runtime lives on UpCloud, not in Azure. Azure Lighthouse is the MSP-style alternative (delegated resource management, onboarding by ARM template, subscription or resource group scope); keep it for later.
4. **UpCloud grants are per resource.** No read and write split at the provider. Warden's gate is the only thing between a read responder and a server restart, and the console must say so.
5. **The egress IP is a first-class fact.** AKS authorized ranges, the UKS IP filter and UpCloud firewall rules all consume it. The tenant runtime design must give one stable address per tenant and show it on the tenant page before any connection is made.
6. **Private clusters** stay at Level 0 until the cluster probe can carry a reverse tunnel.
7. **UpCloud server hands: settled.** SSH direct from the static egress, reverse tunnel as fallback, Ansible as the execution layer, per-tenant SSH CA with short-lived certificates. Azure VM hands use the same path, with Run Command as bootstrap and fallback. See `hands-ssh.md`.
8. **Hands are a catalogue.** Following Datadog and PagerDuty, `run_command` on a VM becomes a set of declared actions per cell kind with parameters, preconditions and verification; a free-form command exists only under `ask` and shows the exact command in the approval. Approvals bind to the exact arguments, are single-use and expire.
9. **Azure bring-your-own-app is federated.** Warden runs an OIDC issuer, the customer's app registration trusts it with a federated credential, no secret is stored; a Bicep template carries the role assignments. Traversal documents exactly this flow.
10. **Validation refuses surplus rights.** A read identity that carries write actions is not marked connected, as Cleric does with IAM roles at save time.
11. **The read identity cannot write, below the gate.** Every read tool uses the read identity; only the remediator holds the write one. This is the AWS session-policy ceiling in Warden's terms.

## Sources

- Azure Run Command: https://learn.microsoft.com/azure/virtual-machines/windows/run-command and https://learn.microsoft.com/en-us/azure/virtual-machines/windows/run-command-managed
- Azure RBAC for Kubernetes: https://learn.microsoft.com/en-za/azure/aks/manage-azure-rbac
- AKS command invoke: https://learn.microsoft.com/en-us/azure/aks/command-invoke
- Entra SSH login: https://learn.microsoft.com/entra/identity/devices/howto-vm-sign-in-azure-ad-linux
- Arc extensions: https://learn.microsoft.com/en-us/azure/azure-arc/servers/manage-vm-extensions-template
- Azure SRE Agent permissions: https://learn.microsoft.com/en-us/azure/sre-agent/permissions, https://learn.microsoft.com/azure/sre-agent/agent-managed-identity, https://learn.microsoft.com/en-us/azure/sre-agent/create-and-set-up
- Azure Lighthouse: https://learn.microsoft.com/he-il/azure/lighthouse/concepts/architecture, https://docs.azure.cn/en-us/lighthouse/how-to/onboard-customer
- UpCloud permissions: https://developers.upcloud.com/1.3/18-permissions/, https://upcloudltd.github.io/upcloud-openapi-spec/api/permission, https://upcloud.com/docs/getting-started/accounts/subaccounts/
- UpCloud Kubernetes: https://upcloud.com/docs/products/managed-kubernetes/control-plane/, https://upcloudltd.github.io/upcloud-openapi-spec/api/kubernetes
- UpCloud metadata service: https://upcloud.com/docs/products/cloud-servers/features/metadata-service
- Datadog Bits remediation: https://docs.datadoghq.com/bits_ai/bits_ai_sre/take_action/
- Rootly private agent: https://docs.rootly.com/private-agent-kubernetes
- Mezmo AURA: https://www.mezmo.com/blog/ai-sre-agent-kubernetes-investigation
- Comparisons (third party): https://www.dash0.com/comparisons/datadog-bits-ai-alternatives, https://wetheflywheel.com/en/comparisons/cleric-vs-resolve-ai-vs-traversal/
