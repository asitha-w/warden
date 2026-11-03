# Cell security and access model, 2026-10-31

What must exist, on the customer's side and on Warden's side, for Warden to reach a cell at each level. Four cell kinds: Azure VM, UpCloud server, AKS cluster, UpCloud Managed Kubernetes (UKS) cluster. This file is the reference the console, the grant script and the validation code are built from. `hands-ssh.md` holds the reasoning; `provider-access-and-control.md` the provider research; `competitor-access-models.md` the field comparison.

Reviewed on 2026-10-31 by an independent review against vendor documentation; the report with every URL is `research/review-cell-access-model.md`. Its corrections are folded in below and its confidence per cell is the table at the end.

## Principles

1. **Two axes.** Reach is how far Warden can see and act into a cell: Level 0 from outside, Level 1 inside, Level 2 where it exists. Autonomy is what Warden may do without a human: observe, investigate, recommend, approved execution, bounded autonomous execution. Reach is set per cell by what exists; autonomy is set per responder by policy. The gate between them is deterministic and outside the model.
2. **Two identities per connection.** A read identity that cannot write, used by every read tool. A write identity, held only by the remediator, used only after the gate says allow. Validation refuses to mark a connection connected if the read identity carries write actions. Where a provider's built-in write role is wider than the catalogue, the cell page says exactly how wide.
3. **One static egress address per tenant**, published on the tenant page and in the API, with notice before it changes. Every allowlist on the customer's side uses it. Warden appends it to existing allowlists and never replaces a list.
4. **Nothing on a cell that Warden wrote.** The probe is an unmodified OpenTelemetry Collector; the hands are OpenSSH and Ansible; the grant is a shell script the operator can read before running.
5. **No secret on a cell.** Cells hold public keys only: the tenant's SSH CA public key, the probe's enrollment token until first connect. Warden holds the CA key and the provider credentials in the tenant's secret store, encrypted at rest, never in a prompt.
6. **Every action is auditable twice**: in Warden's run record with the policy decision and the arguments, and on the target in the provider's activity or audit log, or the host's sshd and sudo logs, where the run id appears as the certificate serial.
7. **Revocation is one step per level**, listed below, and works from the customer's side without Warden.
8. **Validation checks every prerequisite below and shows what failed.** Each item is a silent onboarding failure otherwise.

## Azure VM

### Level 0, connected

| | Must exist |
|---|---|
| Customer side | An Entra directory. A service principal for Warden: a tenant administrator grants admin consent to the Warden multi-tenant app, or an app owner or Application Administrator adds a federated credential to a bring-your-own app registration trusting the tenant runtime's OIDC issuer (client secret only as a fallback). An operator holding Owner, User Access Administrator or Role Based Access Control Administrator at the scope to assign roles. Read identity: `Reader`. Write identity, optional, a custom role (needs `roleDefinitions/write` to create) with `Microsoft.Compute/virtualMachines/start/action`, `restart/action`, `deallocate/action`, `powerOff/action`. |
| Warden side | The tenant runtime with the credential. For federation: an OIDC issuer with a discovery document, audience `api://AzureADTokenExchange`, at most 100 signing keys published. Outbound to `login.microsoftonline.com` and `management.azure.com`. |
| Network | None on the customer's side. |
| Warden can read | Discovery through Azure Resource Graph. Instance view: power state, VM Agent status, provisioning state. Platform metrics without any agent: `Percentage CPU`, `Available Memory Bytes`, `Network In Total`, `Network Out Total`, `Disk Read Bytes`, `Disk Write Bytes`, `VmAvailabilityMetric`. Activity Log, 90 days. Resource Health availability status. `Reader` covers all of these; `Monitoring Reader` is not needed. |
| Warden can do | Start, restart, deallocate, power off, under policy, only with the write identity. |
| Revoke | Remove the role assignments or delete the service principal. |

### Level 1, granted

| | Must exist |
|---|---|
| Customer side | Linux with OpenSSH 8.2 or later (the `Include` drop-in), Python 3.9 or later on the host for the pinned ansible-core 2.20 line (RHEL 8 and Ubuntu 20.04 are below it), systemd. For the compose catalogue action: Docker CLI with the compose plugin 2.18 or later. Outbound 443 to the tenant hostname, which on a VNet created with API versions after 2026-03-31 needs an explicit outbound method (NAT gateway, load balancer outbound rule or public IP). Inbound TCP 22 from the tenant egress IP: an NSG rule, the host firewall if enabled, and a public IP or a routed path; where inbound is refused, the reverse tunnel unit. An operator with sudo to run the grant line once. |
| Warden side | Per-tenant SSH CA key in the tenant secret store. One-time grant token per cell, valid for one hour, bound to the cell id. OpAMP and OTLP endpoints on the tenant hostname accepting only this tenant's enrollment tokens. Pinned ansible-core and collection versions and roles. |
| Network | Port 22 from the egress IP only; nothing else inbound. The grant line is HTTPS to the tenant hostname. |
| The grant installs | In this order: user `warden`, then a sudoers drop-in `warden ALL=(ALL) NOPASSWD: ALL` (Ansible's privilege escalation cannot be limited to commands; the catalogue limits what runs), then the CA public key and principals file, then an sshd drop-in `Match User warden` with `TrustedUserCAKeys`, `AuthorizedPrincipalsFile`, password, agent and TCP forwarding off, closed with `Match All`, validated with `sshd -t` before reload. Then the collector and its supervisor as a systemd unit with the enrollment token. It posts the host public key and the instance metadata from IMDS (subscription, resource group, VM name, VM id) back with the grant token; Warden cross-checks the VM id against ARM because IMDS is unauthenticated. |
| Warden can read | What the cell's telemetry settings enable: host metrics, processes, named log files, named receivers, OTLP from the application. Ansible facts on demand. |
| Warden can do | Catalogue actions as Ansible roles; free-form shell under `ask`. Check mode is the precondition where the module supports it, and the approval shows the module's diff where it has one; the compose module has check mode but no diff, so its approval shows the image change explicitly. Each run uses a certificate valid for ten minutes, principal `warden`, source address the egress IP, a numeric non-zero serial that is the run id, forwarding and user rc off. |
| Revoke | Delete the `warden` user and the sshd drop-in, or close the NSG rule. Warden side: stop minting; rotate the CA by re-running the grant. |

Azure Run Command (`Microsoft.Compute/virtualMachines/runCommands/write`, `read`, `delete`) is a later convenience to deliver the grant without an operator, not a requirement of either level.

## UpCloud server

### Level 0, connected

| | Must exist |
|---|---|
| Customer side | A sub-account. An API token created for it, valid at most 365 days, with `allowed_ip_ranges` set to the tenant egress IP (Basic auth is being deprecated). Permissions granted by the main account with `target_type: server` and the server UUID or `*`, with `storage: yes` if Warden is to read storage details. UpCloud permissions are per resource and carry no read and write split, so the Warden gate is the only split; the connection page says so. |
| Warden side | The token in the tenant secret store, its expiry tracked; the connection goes `offline` with a finding before it expires. Outbound to `api.upcloud.com`. |
| Network | None. |
| Warden can read | Server details: state, plan, zone, IP addresses, tags, storage, firewall on or off, metadata on or off. Audit log events for the server (`GET /1.3/audit-logs`, `resource_types=server`), with actor and source IP, 180 days. No metrics endpoint exists for servers; state is what Level 0 gives. Whether a sub-account may read the audit log is not documented and is tested at validation. |
| Warden can do | Start, stop, restart, add and remove firewall rules, under policy. |
| Revoke | Revoke the permission, delete the token, or delete the sub-account. |

### Level 1, granted

| | Must exist |
|---|---|
| Customer side | The same host requirements as the Azure VM (OpenSSH 8.2, Python 3.9, systemd, outbound 443). A public IPv4 address on the server, which is not guaranteed, or the reverse tunnel. Inbound 22 from the egress IP: a server firewall rule, which Warden adds with `POST /1.3/server/{uuid}/firewall_rule` (never `PUT`, which replaces the customer's chain) and which takes one to two minutes to apply; the firewall's default rule is set by the customer and Warden shows it, because an allow rule only restricts when the default is drop. The host firewall if any. `metadata: yes` on the server, which is off by default on API-created servers, so the grant can read the server UUID from `169.254.169.254`. An operator with sudo. |
| Warden side | Same as the Azure VM. |
| Network | Port 22 from the egress IP only. The server firewall does not cover the utility network interface. |
| The grant installs | Same as the Azure VM; the instance metadata comes from the metadata service. |
| Warden can read and do | Same as the Azure VM. |
| Revoke | Same as the Azure VM; the firewall rule can be removed from the UpCloud control panel without Warden. |

## AKS cluster

### Level 0, connected

| | Must exist |
|---|---|
| Customer side | The same service principal and `Reader` as the Azure VM. Write identity, optional, custom role: `Microsoft.ContainerService/managedClusters/agentPools/write`, `managedClusters/start/action`, `managedClusters/stop/action`. Note the width: `agentPools/write` is any node pool change including upgrade, and `managedClusters/write` is any cluster change including re-enabling local accounts and rewriting the API server access profile, so the latter is not granted and cluster upgrade is not in the catalogue. |
| Warden side | As the Azure VM. |
| Warden can read | Cluster and node pool state, Kubernetes version, API server access profile (public or private, authorized ranges), Entra and Azure RBAC settings, platform metrics, Activity Log, Resource Health. |
| Warden can do | Scale a node pool, start, stop, under policy; the approval shows the full node pool diff because the permission allows more than scaling. |
| Revoke | Remove the role assignments. |

### Level 1, API access

| | Must exist |
|---|---|
| Customer side | The cluster has managed Entra integration with Azure RBAC for Kubernetes enabled, in the same Entra tenant as the subscription. Role assignments on the cluster resource, which take up to five minutes to propagate. Read identity: `Azure Kubernetes Service RBAC Reader` at cluster scope, `Azure Kubernetes Service Cluster User Role` for the kubeconfig, and a custom role with the data action `Microsoft.ContainerService/managedClusters/nodes/read`, because RBAC Reader cannot list nodes. Write identity: a custom role at the permitted namespaces with the data actions `managedClusters/apps/deployments/read` and `write`, `apps/replicasets/read`, `pods/read`, `pods/delete`, `pods/exec/action`, and no `secrets/*`; the built-in `Azure Kubernetes Service RBAC Writer` is accepted instead only with the cell page stating that it reads every Secret and can act as any ServiceAccount in those namespaces. A public API server with the tenant egress IP appended to `authorizedIpRanges` (up to two minutes to apply, keep the cluster's own egress IP in the list). A private cluster reaches Level 1 only through a tunnel: the customer applies a small manifest with one pod running a plain OpenSSH client that dials out to the tenant runtime on 443 and forwards the API server through it (outbound only, nothing exposed), after which the same identities and RBAC apply. The fallback is a jump VM in the VNet carrying the Warden grant, with the API server forwarded through that SSH session. Without either, a private cluster stays at Level 0. `command invoke` is documented as not for programmatic access and is not used. Local accounts with the admin kubeconfig are accepted for a proof of concept and shown as a warning. |
| Warden side | A Kubernetes client per identity that exchanges the credential for an Entra token with audience `6dae42f8-4368-4678-94ff-3960e28e3630`: the kubelogin `spn` flow for a client secret, the workload identity client-assertion flow for a federated credential. |
| Network | The API server endpoint reachable from the egress IP. |
| Warden can read | Workloads, pods, events, node status and metrics from the metrics API with the read identity. Pod logs are promised only after the `pods/log` data action is confirmed on a live cluster; it is not documented. |
| Warden can do | Rollout undo and restart, scale, delete a pod, exec, in the permitted namespaces, with the write identity, under policy. |
| Revoke | Remove the Kubernetes RBAC role assignments or the egress IP from the authorized ranges. |

### Level 2, probe

| | Must exist |
|---|---|
| Customer side | Nodes with outbound 443 to the tenant hostname. The customer applies the probe manifest once with their own cluster-admin; this is the default and keeps Warden's identities as they are. The alternative is granting `Azure Kubernetes Service RBAC Cluster Admin` to Warden's write identity for the install and removing it afterwards, because `RBAC Admin` cannot create the namespace and no Azure data action covers `nodes/stats`. Pod Security admission and Azure Policy must allow hostPath mounts of `/var/log/pods`, `/var/lib/docker/containers` and `/` in `warden-system`. |
| Warden side | The rendered manifest for this collector version: namespace `warden-system`, ServiceAccount, ClusterRole, ClusterRoleBinding, ConfigMap, a Secret with the per-cell enrollment token, a DaemonSet with tolerations for system pool taints and the node name from the downward API, a single-replica Deployment. ClusterRole, get, list and watch: core `events`, `namespaces`, `nodes`, `nodes/spec`, `nodes/stats`, `pods`, `pods/status`, `replicationcontrollers`, `resourcequotas`, `services`; apps `daemonsets`, `deployments`, `replicasets`, `statefulsets`; batch `jobs`, `cronjobs`; autoscaling `horizontalpodautoscalers`. |
| Warden can read | Node and kubelet metrics, container logs as far as the kubelet keeps them (10 MiB times 5 files per container by default, gone on eviction), retained events, cluster-level metrics. |
| Revoke | Delete the `warden-system` namespace and the ClusterRoleBinding. |

The enrollment token Secret in `warden-system` is readable by anyone holding `secrets/*` in that namespace, which is why Warden's write identity is not scoped to `warden-system`.

## UKS cluster

### Level 0, connected

| | Must exist |
|---|---|
| Customer side | The sub-account token as for the UpCloud server, and a permission of `target_type: managed_kubernetes` on the cluster UUID. |
| Warden side | As the UpCloud server. |
| Warden can read | Cluster details: state, version, zone, plan, node groups with plans and counts, `control_plane_ip_filter`, `private_node_groups`, network. Audit log events for the cluster (`resource_types=uks`). |
| Warden can do | Scale a node group (`PATCH .../node-groups/{name}` with `count`), modify the IP filter, under policy. Denied without exception, because the same permission allows them: cluster delete, node group delete, node delete, upgrade. A `PATCH` on the cluster must resend the full `control_plane_ip_filter` every time. |
| Revoke | Revoke the permission. |

### Level 1, API access

| | Must exist |
|---|---|
| Customer side | The tenant egress IP in `control_plane_ip_filter`. Warden reads the current list, appends, and shows the full result for confirmation; a `null` list means open to the internet and the console says so before changing it, because the call replaces the list and a wrong value locks the customer out. |
| Warden side | Warden fetches the cluster kubeconfig from the API: a client certificate for the user `<cluster>-admin`, with no documented expiry, no regenerate endpoint, and rotation only through UpCloud support, so Warden records the fetch time and tells the customer that. It uses the kubeconfig once to create: namespace `warden-system`, two ServiceAccounts `warden-read` and `warden-write`, a read ClusterRole bound to the first, a write Role in each permitted namespace bound to the second with `create` on `pods/exec` and `get` on `pods/log`, the probe's ClusterRole, ServiceAccount and ClusterRoleBinding for Level 2, and a `kubernetes.io/service-account-token` Secret per ServiceAccount in `warden-system` so the customer can revoke by deleting the Secret. It stores the two tokens and discards the admin kubeconfig. On clusters at Kubernetes 1.34 or later, UKS OIDC authentication (beta) against the tenant runtime's issuer is the forward path that avoids the admin kubeconfig. Whether a sub-account permission allows the kubeconfig fetch is not documented and is tested at validation. |
| Network | The control plane endpoint reachable from the egress IP. |
| Warden can read and do | As AKS Level 1, with the service account tokens. |
| Revoke | Delete the two ServiceAccounts or their token Secrets, or remove the egress IP from the IP filter. |

### Level 2, probe

| | Must exist |
|---|---|
| Customer side | Nodes with outbound 443 to the tenant hostname; for private node groups, a running NAT gateway on the cluster's SDN network. |
| Warden side | The same manifest as AKS, applied with `warden-write`, which creates only the namespaced objects; the ClusterRole and ClusterRoleBinding already exist from the Level 1 bootstrap. Kubelet endpoint authentication on UKS nodes is not documented and is tested on the first cluster. |
| Revoke | As AKS. |

## The policy gate on top

Every action, at every level, passes the responder's policy: `allow`, `ask`, `deny`. `ask` opens an approval that shows the exact arguments, binds to them, is single-use and expires. A responder in Autonomous mode turns `ask` into `allow` only for the actions it lists. Delete-class actions on cells and clusters, and cluster upgrades, are denied regardless of mode. The investigator never holds a write credential; the remediator holds it and runs only after the gate.

## Confidence after review

| Cell kind | Level | Confidence | Why |
|---|---|---|---|
| Azure VM | 0 | High | Every role, action and metric verified; the earlier gaps (consent role, who assigns roles, federation limits) are now listed. |
| Azure VM | 1 | Medium | Host items corrected (OpenSSH 8.2, Python 3.9, Match All, outbound route); the OpAMP supervisor is alpha and untested on a real host. |
| UpCloud server | 0 | High | Token model, permissions and the audit log API verified. |
| UpCloud server | 1 | Medium | Firewall default, propagation and metadata flag are now prerequisites; public address and sub-account audit log access are tested at validation. |
| AKS | 0 | High | Roles verified; the width of the write actions is stated and upgrade is excluded. |
| AKS | 1 | Medium | Custom roles replace the built-ins for nodes and for a Secrets-free write identity; `pods/log` is unverified until tested on a live cluster; private clusters are out at this level. |
| AKS | 2 | Medium | Customer-applied manifest is the default; the Warden-installed path needs time-boxed Cluster Admin and is opt-in. |
| UKS | 0 | High | Endpoints, fields and the deny list verified. |
| UKS | 1 | Medium | Admin kubeconfig privileges and lifetime are undocumented; lockout hazard handled by append-and-confirm; sub-account kubeconfig fetch untested. |
| UKS | 2 | Medium | Node egress and NAT gateway verified; kubelet authentication on UKS nodes undocumented. |

## To test on real infrastructure before the design is final

1. UpCloud: a sub-account token with `managed_kubernetes` fetching the kubeconfig, and reading the audit log.
2. UKS: the groups and privileges in the admin kubeconfig certificate; kubelet authentication on nodes.
3. AKS: `kubectl logs` under RBAC Reader; the custom data-action roles above on a real cluster.
4. Hosts: `sshd -T -C user=warden,addr=<egress>` on each target image to confirm the Match block; the OpAMP supervisor on an UpCloud server.
