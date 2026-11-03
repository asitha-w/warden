# Review of `docs/planning/cell-access-model.md` (2026-10-31)

Independent cloud access and security review. Every claim below was checked against vendor documentation fetched on 2026-10-31; a URL follows each item. "Not documented" means the vendor page does not say. Role and operation strings are quoted exactly as the vendor lists them (Azure matches them case-insensitively).

## Confidence table

| Cell kind | Level | Confidence | Reason |
|---|---|---|---|
| Azure VM | 0 connected | Medium | Roles and the four power actions verified; `Monitoring Reader` is redundant with `Reader`, two listed metric names are deprecated or misnamed, and the onboarding prerequisites (consent role, federated-credential limits, who can assign roles) are missing. |
| Azure VM | 1 granted | Medium | CA, principals file and all `Match` options are in the current sshd_config(5); but the "OpenSSH 6.2" floor is not backed by release notes, sshd drop-ins need 8.2, the Python floor for current ansible-core is 3.9 not "Python 3", and VMs in new VNets have no default outbound route. |
| UpCloud server | 0 connected | Medium | Permission target types and server endpoints verified; the claim "no activity log in the API" is wrong (there is an Audit Logs API), Basic auth is being deprecated in favour of API tokens that live at most one year, and no read-only permission exists as stated. |
| UpCloud server | 1 granted | Medium | Metadata default `no` on API-created servers confirmed; firewall default rule, utility-interface scope and 1-2 minute propagation are not reflected; "public IPv4 by default" is not documented. |
| AKS | 0 connected | Medium | Every role and action verified; but `managedClusters/write` and `agentPools/write` grant whole-resource reconfiguration (including re-enabling local accounts and changing the API access profile), far beyond "scale" and "upgrade". |
| AKS | 1 API access | Low | `Azure Kubernetes Service RBAC Reader` has no `nodes/read`, so "node status" is denied; no `pods/log` data action is documented; `RBAC Writer` carries `secrets/*` and `serviceaccounts/*`; `command invoke` is documented as not for programmatic access. |
| AKS | 2 probe | Low | `RBAC Admin` cannot create a namespace (`namespaces/write` is a NotDataAction), and there is no Azure data action for `nodes/stats`, so the install path as written does not work; needs `RBAC Cluster Admin` or a customer-applied manifest. |
| UKS | 0 connected | Medium | `managed_kubernetes` target type, cluster fields and endpoints verified; the deny list must also cover node-group delete, node delete and upgrade, and `PATCH` requires the full `control_plane_ip_filter` on every call. |
| UKS | 1 API access | Medium | Kubeconfig is a client certificate for user `<cluster>-admin`; its privileges and lifetime are not documented and rotation is only through UpCloud support; the IP filter defaults to open and `PATCH` replaces the list, so Warden can lock the customer out. OIDC auth (beta, 1.34+) now exists as an alternative. |
| UKS | 2 probe | Medium | Node outbound requirement and NAT-gateway rule for private node groups verified; kubelet endpoint auth and metrics-server presence on UKS are not documented. |

## Azure VM

### Level 0, connected

**Verified**
- `Reader` = `["*/read"]`. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/general
- `Monitoring Reader` = `*/read` + `Microsoft.OperationalInsights/workspaces/search/action` + `Microsoft.Support/*`. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/monitor
- All four write actions exist: `Microsoft.Compute/virtualMachines/start/action`, `restart/action`, `deallocate/action`, `powerOff/action` ("Powers off the virtual machine. Note that the virtual machine will continue to be billed."). `runCommand/action`, `runCommands/write|read|delete`, `instanceView/read` also exist. https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/compute
- Resource Graph needs only `read` on the resources ("No results are returned if you don't have at least `read` permissions"); the operation is `Microsoft.ResourceGraph/resources/read`. https://learn.microsoft.com/en-us/azure/governance/resource-graph/overview, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/management-and-governance
- Activity Log read operation `Microsoft.Insights/eventtypes/values/Read` and Resource Health `Microsoft.ResourceHealth/AvailabilityStatuses/read` exist and are reads, so `*/read` covers them. Activity log retention 90 days. https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/monitor, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/management-and-governance, https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/activity-log
- Metrics: "Access to alerts, diagnostic settings, and metrics for a resource requires read access to the resource type and scope of that resource." https://learn.microsoft.com/en-us/azure/azure-monitor/roles-permissions-security
- Federated credential: issuer must be "A URL that complies with the OIDC Discovery spec"; audience "api://AzureADTokenExchange"; max 20 per app; no wildcards; creator must be app owner or Application Administrator / Cloud Application Administrator / Global Administrator / Hybrid Identity Administrator. https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation-create-trust
- Service principal creation in the customer tenant happens at admin consent: `https://login.microsoftonline.com/{tenant}/v2.0/adminconsent?client_id=...&scope=...`; "Microsoft Entra ID requires a tenant administrator to sign in to complete the request." https://learn.microsoft.com/en-us/entra/identity-platform/v2-admin-consent, https://learn.microsoft.com/en-us/entra/identity-platform/howto-convert-app-to-be-multi-tenant

**Missing**
- Who assigns the roles: the operator needs `Microsoft.Authorization/roleAssignments/write` at the scope (Owner, User Access Administrator or Role Based Access Control Administrator); 5000 role assignments per subscription. https://learn.microsoft.com/en-us/azure/role-based-access-control/role-assignments-steps
- The custom write role needs `Microsoft.Authorization/roleDefinitions/write` on all `AssignableScopes` to create; 5000 custom roles per tenant. https://learn.microsoft.com/en-us/azure/role-based-access-control/custom-roles
- Federation: the Warden OIDC issuer must publish a discovery document and expose at most 100 signing keys ("stores only the first 100 signing keys"). https://learn.microsoft.com/en-us/entra/workload-id/workload-identity-federation
- Available Memory Bytes is a platform metric on `Microsoft.Compute/virtualMachines` (Average, PT1M, no agent listed), so memory can be a Level 0 sensor. https://learn.microsoft.com/en-us/azure/virtual-machines/monitor-vm-reference
- VM availability metric retention is 93 days; Resource Health history 30 days in the portal. https://learn.microsoft.com/en-us/azure/virtual-machines/monitor-vm-reference, https://learn.microsoft.com/en-us/azure/service-health/resource-health-overview

**Wrong**
- `Monitoring Reader` is not needed for metrics, Activity Log or Resource Health; `Reader` (`*/read`) already covers every read operation involved. It only adds Log Analytics search and support tickets. (Keep it only if Warden will run Log Analytics queries.)
- Metric names: "Network In" and "Network Out" are the REST names of "Network In Billable (Deprecated)" and "Network Out Billable (Deprecated)"; use `Network In Total` and `Network Out Total`. "VM Availability" is `VmAvailabilityMetric` (display name "VM Availability Metric"). https://learn.microsoft.com/en-us/azure/virtual-machines/monitor-vm-reference

**Corrections**
- Read identity: `Reader` only. Write identity custom role actions: `Microsoft.Compute/virtualMachines/start/action`, `Microsoft.Compute/virtualMachines/restart/action`, `Microsoft.Compute/virtualMachines/deallocate/action`, `Microsoft.Compute/virtualMachines/powerOff/action`.
- Metrics list: `Percentage CPU`, `Network In Total`, `Network Out Total`, `Disk Read Bytes`, `Disk Write Bytes`, `VmAvailabilityMetric`, `Available Memory Bytes`.
- Add to customer side: "an operator holding Owner, User Access Administrator or Role Based Access Control Administrator at the scope; a tenant administrator for the consent step; for bring-your-own app, an app owner or Application Administrator to add the federated credential".

### Level 1, granted

**Verified**
- sshd_config(5) current: `AuthorizedPrincipalsFile`, `TrustedUserCAKeys`, `PasswordAuthentication`, `AllowAgentForwarding`, `AllowTcpForwarding`, `X11Forwarding`, `PermitTunnel`, `PubkeyAuthentication`, `AuthenticationMethods`, `PermitTTY`, `ForceCommand` and `Include` are all in the Match keyword list; `Match All` is a single-token criterion; `AuthorizedPrincipalsFile` is only consulted for CAs in `TrustedUserCAKeys` and accepts `%%, %h, %U, %u`; certificates without principals are refused under `TrustedUserCAKeys`. https://man.openbsd.org/sshd_config
- ssh-keygen(1): `-s`, `-I`, `-n`, `-V` (relative end such as `+10m` via "plus sign followed by an interval"), `-z serial_number` (64-bit, non-zero for KRL purposes), `-O clear`, `-O no-agent-forwarding`, `-O no-port-forwarding`, `-O no-pty`, `-O no-user-rc`, `-O no-x11-forwarding`, `-O source-address=address_list` (CIDR list), `-O force-command=`; `-k`/`-u` KRL with `serial:` ranges. https://man.openbsd.org/ssh-keygen
- Release history: certificates and `TrustedUserCAKeys` in 5.4; `AuthorizedPrincipalsFile` and its use inside `Match` in 5.6; Ed25519 in 6.5; `Include` in 8.2. https://www.openssh.org/txt/release-5.4, https://www.openssh.org/txt/release-5.6, https://www.openssh.org/txt/release-6.5, https://www.openssh.org/txt/release-8.2
- IMDS: `http://169.254.169.254/metadata/instance?api-version=2025-04-07`, header `Metadata: true`, no `X-Forwarded-For`, fields `compute.subscriptionId`, `compute.resourceGroupName`, `compute.name`, `compute.vmId`; limited to 5 requests/second per VM; "The API is unauthenticated and open to all processes on the VM." https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service
- ansible-core managed node support matrix (target Python): 2.18 and 2.19 → 3.8-3.13; 2.20 and 2.21 → 3.9-3.14; 2.22 → 3.9-3.15; 2.23 → 3.10-3.15. 2.19 security support ends Nov 2026. https://docs.ansible.com/projects/ansible-core/devel/reference_appendices/release_and_maintenance.html
- Managed node: needs Python and "a user account that can connect through SSH to the node with an interactive POSIX shell". https://docs.ansible.com/projects/ansible/latest/installation_guide/intro_installation.html
- `community.docker.docker_compose_v2`: "Docker CLI with Docker compose plugin 2.18.0 or later", "PyYAML if `definition` is used", "does not use the Docker SDK for Python"; check mode "full"; diff mode "none". The v1 `docker_compose` module "has been removed in version 4.0.0 of community.docker". https://docs.ansible.com/projects/ansible/latest/collections/community/docker/docker_compose_v2_module.html, https://docs.ansible.com/projects/ansible/latest/collections/community/docker/docker_compose_module.html
- OpAMP supervisor: separate `opampsupervisor` binary from opentelemetry-collector-contrib, "Stability: alpha", "The design is still undergoing changes"; needs a Collector binary, a supervisor config and a running OpAMP server; default storage `/var/lib/otelcol/supervisor`. https://opentelemetry.io/docs/collector/management/, https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/cmd/opampsupervisor/README.md
- Host metrics in a container need the host filesystem mounted and `root_path`; process scraper errors for unreadable processes are muted with `mute_process_name_error` / `mute_process_io_error`. https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/receiver/hostmetricsreceiver/README.md

**Missing**
- Outbound route: "For the API released after March 31, 2026, new virtual networks default to using private subnets. Virtual machines in those subnets need an explicit outbound method" (NAT gateway, standard LB outbound rules, public IP, or firewall/NVA with UDR). Existing VNets keep default outbound access. The "outbound 443" prerequisite must name this. https://learn.microsoft.com/en-us/azure/virtual-network/ip-services/default-outbound-access
- sshd drop-in ordering: `Include` is processed where it appears; a `Match User warden` block in a drop-in that is included at the top of `sshd_config` captures every later directive of the main file. End the drop-in with `Match All` (criterion exists) and validate with `sshd -t`. https://man.openbsd.org/sshd_config
- `Include` needs OpenSSH 8.2 or later; where the distro has no `Include`, the grant must edit `/etc/ssh/sshd_config` directly. https://www.openssh.org/txt/release-8.2
- Certificate serial: a 64-bit integer, so the run id must be numeric and non-zero if KRLs are ever used ("Serial numbers are 64-bit values, not including zero"). https://man.openbsd.org/ssh-keygen
- `AuthorizedPrincipalsFile` is "only checked for valid users": the `warden` account must exist before the file is consulted (ordering inside the grant). https://man.openbsd.org/sshd_config
- OpenSSH 10.6 fixed a bug where `AuthorizedPrincipalsFile none` inside a `Match` block was read as a literal file name; do not set `none` in the Match block on older versions. https://www.openssh.org/txt/release-10.6
- Python floor by pinned ansible-core: 3.8 for 2.18/2.19, 3.9 for 2.20-2.22, 3.10 for 2.23. RHEL 8's platform Python 3.6 and Ubuntu 20.04's 3.8 are below the 2.20 floor. "Python 3" is not a sufficient prerequisite. https://docs.ansible.com/projects/ansible-core/devel/reference_appendices/release_and_maintenance.html
- The compose catalogue action: Docker CLI + compose plugin ≥ 2.18.0 on the managed node; the module has no diff support, so "check mode and diff as the approval content" does not hold for compose. https://docs.ansible.com/projects/ansible/latest/collections/community/docker/docker_compose_v2_module.html
- Check mode: "Modules that do not support check mode report nothing and do nothing" — a precondition built on check mode must be verified per module. https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_checkmode.html
- Sudo: "You cannot limit privilege escalation permissions to certain commands" (Ansible's become); a command-restricted sudoers drop-in will break module execution, which runs the Python interpreter, not the catalogue command. https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_privilege_escalation.html
- Host key pinning: the grant posts the host key over HTTPS; IMDS is unauthenticated, so the instance metadata reported by the grant must be cross-checked against ARM (`vmId`) rather than trusted. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service

**Wrong**
- "OpenSSH server 6.2 or later (certificate authentication with `AuthorizedPrincipalsFile`)": the principals file dates from 5.6 and is Match-able since 5.6; no release note between 5.4 and 6.5 mentions `TrustedUserCAKeys` becoming Match-able, so 6.2 is not a documented floor for anything. The practical floor is 8.2 because of `Include`. https://www.openssh.org/txt/release-5.6
- "sshd drop-in" is a distribution convention (`/etc/ssh/sshd_config.d/*.conf` on Debian/Ubuntu/RHEL 9); the OpenBSD manual defines no default Include path. https://man.openbsd.org/sshd_config
- sudoers "requiretty" is not documented on the fetched Ansible page; not asserted either way here.

**Corrections**
- Customer side: "Linux with OpenSSH 8.2 or later (drop-in via `Include`); Python 3.9 or later on the host (3.8 only while ansible-core 2.19 is pinned, which leaves security support in Nov 2026); systemd; an explicit outbound method for VMs in VNets created with API versions after 2026-03-31."
- The grant: end the sshd drop-in with `Match All`; run `sshd -t` before reload; create the user before writing the principals file; keep the sudoers drop-in `NOPASSWD: ALL` for the `warden` user or document that Ansible cannot run under a command-restricted sudo.
- Certificate: `ssh-keygen -s ca -I run-<id> -n warden -V +10m -z <numeric run id> -O clear -O source-address=<egress>/32 -O no-agent-forwarding -O no-port-forwarding -O no-x11-forwarding -O no-user-rc` (add `-O no-pty` only if Ansible is run without a tty).

## UpCloud server

### Level 0, connected

**Verified**
- Permission target types: `server`, `storage`, `network`, `router`, `network_gateway`, `managed_database`, `managed_kubernetes`, `managed_loadbalancer`, `managed_object_storage`, `tag_access`; `target_identifier` is a UUID or `*`; "Only main account user can grant permissions"; only option is `storage: yes|no` on `server`/`tag_access`. No read/write split is documented. https://developers.upcloud.com/1.3/18-permissions/
- Sub-account: `allow_api` ("has no effect on API tokens, API access is implicitly allowed when using API tokens"), `ip_filter` for API access; UI label "Allow Basic Auth API connections". https://developers.upcloud.com/1.3/3-accounts/, https://upcloud.com/docs/getting-started/accounts/subaccounts/
- Server details `GET /1.3/server/{uuid}`: `state`, `plan`, `zone`, `ip_addresses`, `tags`, `storage_devices`, `firewall` (on/off), `metadata`, `labels`; `POST .../start|stop|restart` with `stop_type` soft/hard. Firewall rules are a separate resource `GET/POST/PUT/DELETE /1.3/server/{uuid}/firewall_rule`. No metrics or usage endpoint for servers. https://developers.upcloud.com/1.3/8-servers/, https://developers.upcloud.com/1.3/11-firewall/
- API base `https://api.upcloud.com/1.3/`; Bearer token authentication; Basic auth "is discouraged, and it will be deprecated and disabled at a later date". https://developers.upcloud.com/1.3/2-architecture/

**Missing**
- Credential type: API tokens (`POST /1.3/account/tokens`), `expires_at` "up to 24h*365=8760h", `allowed_ip_ranges` 0-100 CIDRs (default: account IP filters, else `0.0.0.0/0`), `can_create_tokens` default false. The tenant must rotate the token at least yearly and should pin it to the egress IP. https://developers.upcloud.com/1.3/24-api-tokens/
- Audit Logs API: `GET /1.3/audit-logs`, `GET /1.3/audit-logs/export` (csv/json), filters `actions`, `origins`, `resource_types` (incl. `server`, `uks`), `q` (UUID, username or IP); fields `account_username`, `ip_address`, `resource_id`, `upcloud_correlation_id`; 180 days via API, 5 years by Sales. Whether a sub-account can read it: not documented. https://developers.upcloud.com/1.3/22-audit-logs/, https://upcloud.com/docs/getting-started/accounts/audit-logs/
- Storage detail reads (`/1.3/storage/{uuid}`) need `options.storage: yes` on the server permission. https://developers.upcloud.com/1.3/18-permissions/
- Rate limits: not documented.

**Wrong**
- "No metrics endpoint and no activity log are documented in the API": metrics is correct for servers (only managed databases have `GET /1.3/database/{uuid}/metrics`), but the activity-log half is wrong; the Audit Logs API exists and carries actor username and source IP, which satisfies principle 6 for the provider side. https://developers.upcloud.com/1.3/22-audit-logs/, https://developers.upcloud.com/1.3/16-managed-database/
- "A sub-account with API access enabled": with tokens the `allow_api` flag is irrelevant; what matters is a token created for that sub-account. https://developers.upcloud.com/1.3/3-accounts/

**Corrections**
- Customer side: "A sub-account; an API token for it (max 365 days, `allowed_ip_ranges` = tenant egress IP); permissions from the main account with `target_type: server` and the server UUID or `*`."
- Warden can read: add "Audit log events for the server (`resource_types=server`, `q=<uuid>`)".
- Warden side: add "token expiry tracked; connection goes `offline` with a finding before `expires_at`".

### Level 1, granted

**Verified**
- `metadata` create attribute: `yes / no`, default `no`, "HTTP-accessible metadata service for the server". https://developers.upcloud.com/1.3/8-servers/
- Firewall: per server; "enabled on all network interfaces except ones attached to private virtual networks"; max 1000 rules; rules evaluated in order; "The last rule is a special case and corresponds to Default Rule set through Control Panel"; "Please allow 1-2 minutes for the firewall changes to take effect"; `PUT` overwrites the whole chain, `DELETE` is per index. https://developers.upcloud.com/1.3/11-firewall/
- "Servers may have from zero to five public IPv4 and IPv6 addresses. All servers have a utility IP address that cannot be removed." https://developers.upcloud.com/1.3/8-servers/

**Missing**
- Firewall default rule: the accept/drop default is set in the Control Panel; Warden's allow rule only restricts if the default is `drop`. Validation must read the rule chain and show the default. https://developers.upcloud.com/1.3/11-firewall/
- The firewall also covers the utility interface; a rule allowing 22 from the egress IP does not stop utility-network access. https://developers.upcloud.com/1.3/11-firewall/
- Firewall propagation 1-2 minutes: an ordering constraint before the first SSH probe.
- Metadata must be turned on with a server modify when the server was API-created (`metadata: no` default); whether that needs a stopped server is not documented. https://developers.upcloud.com/1.3/8-servers/
- Warden creating the firewall rule uses `POST /1.3/server/{uuid}/firewall_rule` (never `PUT`, which overwrites the customer's chain).
- The host-side items from Azure VM Level 1 (OpenSSH 8.2, Python 3.9, Match All, sudo) apply unchanged.

**Wrong**
- "A public IPv4 address, which UpCloud servers have by default": not documented; a server may have zero public addresses. https://developers.upcloud.com/1.3/8-servers/
- The metadata service doc page `upcloud.com/docs/products/cloud-servers/features/metadata-service/` cited in `provider-access-and-control.md` returns 404; the only vendor statement is the `metadata` attribute on the servers page.

**Corrections**
- Customer side: "A public IPv4 address on the server or the reverse tunnel; server firewall default rule known (Warden shows it); `metadata: yes` on the server."
- Network: "Inbound 22 from the egress IP, added with `POST /1.3/server/{uuid}/firewall_rule`; wait up to two minutes before validation."

## AKS cluster

### Level 0, connected

**Verified**
- `Reader` `*/read` covers `Microsoft.ContainerService/managedClusters/read` and `managedClusters/agentPools/read` (both exist). https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers
- Write actions exist: `managedClusters/agentPools/write` ("Creates a new agent pool or updates an existing one"), `managedClusters/start/action`, `managedClusters/stop/action`, `managedClusters/write` ("Creates a new managed cluster or updates an existing one"). Same URL.

**Missing**
- Blast radius of the write actions: `managedClusters/write` is the permission "required to re-enable local accounts" and changes any cluster property (API server access profile, AAD profile, authorized IP ranges). `agentPools/write` also upgrades and reconfigures node pools, not only scales. The gate must treat both as cluster-reconfiguration, and the policy cannot separate scale from upgrade at the RBAC layer. https://learn.microsoft.com/en-us/azure/aks/enable-authentication-microsoft-entra-id, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers
- Node image upgrade is a separate action `managedClusters/agentPools/upgradeNodeImageVersion/action` if ever catalogued. Same permissions URL.

**Wrong**
- Nothing contradicted.

**Corrections**
- Write identity note: "`agentPools/write` = scale, upgrade and any node-pool change; `managedClusters/write` = any cluster change including local accounts and API server access profile; both denied unless the action is `scale` or `upgrade` with the argument diff shown."

### Level 1, API access

**Verified**
- Prerequisites: managed Entra integration enabled; "Microsoft Entra ID authorization for the Kubernetes API requires that the Microsoft Entra tenant configured for authentication is the same as the tenant for the subscription"; integration cannot be disabled once enabled. https://learn.microsoft.com/en-us/azure/aks/manage-azure-rbac, https://learn.microsoft.com/en-us/azure/aks/enable-authentication-microsoft-entra-id
- Namespace scope: `--scope $AKS_ID/namespaces/<namespace-name>` for RBAC Reader and RBAC Writer; service principals accepted ("the client ID of a service principal"); "New role assignments can take up to five minutes to propagate". https://learn.microsoft.com/en-us/azure/aks/manage-azure-rbac
- `Azure Kubernetes Service Cluster User Role` = `managedClusters/listClusterUserCredential/action` + `managedClusters/read`; RBAC Reader and RBAC Writer do not include `listClusterUserCredential/action` (RBAC Admin and Cluster Admin do). https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers
- Token audience: AKS server application `6dae42f8-4368-4678-94ff-3960e28e3630`, "The token audience for all supported kubelogin authentication methods"; `spn` mode takes `AZURE_CLIENT_ID` + `AZURE_CLIENT_SECRET` or a PFX; `workloadidentity` mode takes `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_FEDERATED_TOKEN_FILE`, `AZURE_AUTHORITY_HOST`; "The service principal authentication method works only with managed Microsoft Entra ID integration". https://learn.microsoft.com/en-us/azure/aks/kubelogin-authentication
- Authorized IP ranges: property `apiServerAccessProfile.authorizedIpRanges`; up to 200 ranges; "You can't use this feature with private clusters"; Standard load balancer only; up to two minutes to propagate; on creation AKS adds the API server and outbound IPs, on an existing cluster the doc only recommends including the cluster egress IP. https://learn.microsoft.com/en-us/azure/aks/api-server-authorized-ip-ranges
- Command invoke: `Microsoft.ContainerService/managedClusters/runcommand/action` and `Microsoft.ContainerService/managedClusters/commandResults/read`; output limit 512 kB; "Not for programmatic access: Use Bastion, VPN, or ExpressRoute for automated API calls"; pod scheduling timeout 60 s; can be disabled with `apiServerAccessProfile.disableRunCommand`. https://learn.microsoft.com/en-us/azure/aks/command-invoke
- `pods/exec/action` exists as data action `Microsoft.ContainerService/managedClusters/pods/exec/action`; RBAC Writer has `managedClusters/pods/*`, and an Azure wildcard "grants access to all actions that match the string" including nested `.../run/action` forms, so exec is included. https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers, https://learn.microsoft.com/en-us/azure/role-based-access-control/role-definitions
- RBAC Reader data actions include `pods/read`, `events/read`, `events.k8s.io/events/read`, `metrics.k8s.io/pods/read`, `metrics.k8s.io/nodes/read`, `apps/*/read`, `namespaces/read`, no secrets. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers
- Admin credential: with `disableLocalAccounts` "Getting static credential isn't allowed"; `Azure Kubernetes Service Cluster Admin Role` carries `listClusterAdminCredential/action`. https://learn.microsoft.com/en-us/azure/aks/manage-local-accounts-managed-azure-ad, https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers

**Missing**
- Node status read: RBAC Reader has no `Microsoft.ContainerService/managedClusters/nodes/read` (only `metrics.k8s.io/nodes/read`), so `get nodes` is denied for the read identity. The data action `managedClusters/nodes/read` exists; add it in a custom role or a Kubernetes `ClusterRole` bound to the SP's object id. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers
- Pod logs: no `managedClusters/pods/log/read` data action is listed; the closest is `managedClusters/logs/read`. How RBAC Reader authorizes `pods/log` is not documented; verify `kubectl logs` with RBAC Reader on a real cluster before the console promises "pod logs" at Level 1. https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers
- Ordering: role assignments up to five minutes, IP ranges up to two minutes; validation must retry across that window.
- Authorized IP ranges on an existing cluster: Warden must append the egress IP to the existing list, never replace it, and the list must keep the cluster's own egress IP.
- The customer needs `Microsoft.Authorization/roleAssignments/write` at cluster scope and, for namespace-scoped assignments, the namespace must already exist.
- Command invoke on a private cluster is documented as not for programmatic use; the "reduced fidelity" tier is on an unsupported path.

**Wrong**
- "write identity `Azure Kubernetes Service RBAC Writer` at the namespaces Warden may act in" understates what it grants: "this role allows accessing Secrets and running Pods as any ServiceAccount in the namespace, so it can be used to gain the API access levels of any ServiceAccount in the namespace" (`secrets/*`, `serviceaccounts/*`, `pods/*`). Principle 2's "write identity" is in practice namespace-admin-equivalent. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers
- "the kubelogin service principal flow": correct for a client secret; a bring-your-own app with a federated credential uses the `workloadidentity` flow (client assertion from a file), not `spn`. https://learn.microsoft.com/en-us/azure/aks/kubelogin-authentication
- "Both identities need `Azure Kubernetes Service Cluster User Role`": correct, and the role also carries `managedClusters/read`.

**Corrections**
- Read identity: `Azure Kubernetes Service RBAC Reader` at cluster scope + `Azure Kubernetes Service Cluster User Role` + a custom role with data action `Microsoft.ContainerService/managedClusters/nodes/read` (and `pods/log` once its data action is confirmed).
- Write identity: either accept that RBAC Writer reads Secrets and say so on the cell page, or define a custom role with data actions `managedClusters/apps/deployments/write`, `managedClusters/apps/deployments/read`, `managedClusters/apps/replicasets/read`, `managedClusters/pods/delete`, `managedClusters/pods/exec/action`, `managedClusters/pods/read` and no `secrets/*`.
- Warden side: "`spn` flow for a client secret, `workloadidentity`-equivalent client-assertion flow for a federated credential; audience `6dae42f8-4368-4678-94ff-3960e28e3630`."

### Level 2, probe

**Verified**
- Kubernetes escalation rules: a Role/ClusterRole can be created only by a caller that "already ha[s] all the permissions contained in the role, at the same scope" or holds `escalate`; bindings need the referenced permissions or `bind`. https://kubernetes.io/docs/reference/access-authn-authz/rbac/
- Authorizers run in sequence; "If any authorizer approves or denies a request, that decision is immediately returned"; AKS: "You can use both models on the same cluster" (Entra authorization webhook and Kubernetes RBAC). https://kubernetes.io/docs/reference/access-authn-authz/authorization/, https://learn.microsoft.com/en-us/azure/aks/concepts-cluster-authorization
- RBAC Admin: DataActions `managedClusters/*`, NotDataActions `resourcequotas/write`, `resourcequotas/delete`, `namespaces/write`, `namespaces/delete`. RBAC Cluster Admin: `managedClusters/*` with no exclusions. Data actions `rbac.authorization.k8s.io/clusterroles/write`, `clusterrolebindings/write`, `namespaces/write` exist. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers
- Collector RBAC: k8s_cluster receiver needs get/list/watch on `events, namespaces, nodes, nodes/spec, pods, pods/status, replicationcontrollers, resourcequotas, services`, apps `daemonsets, deployments, replicasets, statefulsets`, batch `jobs, cronjobs`, autoscaling `horizontalpodautoscalers`, one replica ("more than one replica results in duplicate data"); kubeletstats needs `nodes/stats` get/watch/list, `endpoint: ${env:K8S_NODE_NAME}:10250`, `auth_type: serviceAccount`, DaemonSet preferred; k8sattributes needs `pods, namespaces` get/watch/list and `replicasets`; filelog needs hostPath `/var/log/pods` and `/var/lib/docker/containers` read-only; hostmetrics needs hostPath `/` at `/hostfs` with `root_path: /hostfs`; events via k8sobjects (service account auth only). https://opentelemetry.io/docs/platforms/kubernetes/collector/components/
- Kubelet log retention: `containerLogMaxSize` 10Mi, `containerLogMaxFiles` 5 defaults; "If a pod is evicted from the node, all corresponding containers are also evicted, along with their logs." https://kubernetes.io/docs/concepts/cluster-administration/logging/

**Missing**
- `warden-system` creation: RBAC Admin cannot create it (`namespaces/write` excluded). Either the customer creates the namespace, or the install identity is `Azure Kubernetes Service RBAC Cluster Admin`.
- No Azure data action exists for `nodes/stats` or `nodes/proxy`; the escalation check for a ClusterRole containing `nodes/stats` under the Entra webhook is not documented. Safe path: customer applies the manifest with their own cluster-admin, or Warden installs with RBAC Cluster Admin.
- hostPath volumes (`/`, `/var/log/pods`) and `mountPropagation: HostToContainer` on the DaemonSet; clusters with Pod Security admission or Azure Policy restricting hostPath will reject the probe. https://opentelemetry.io/docs/platforms/kubernetes/collector/components/
- DaemonSet tolerations for system-pool taints and a `K8S_NODE_NAME` downward-API env var.
- The probe's ClusterRole list in the document lacks `nodes/spec`, `pods/status`, `services`, `replicationcontrollers`, `resourcequotas`, `cronjobs`, `horizontalpodautoscalers` (k8s_cluster receiver) and `nodes/stats` is named only as "node stats through the kubelet".
- "container logs with history": bounded by kubelet rotation (10Mi × 5 per container by default) and lost on eviction.
- The enrollment token Secret in `warden-system` is readable by any RBAC Writer holder in that namespace (`secrets/*`), so Warden's own write identity, if scoped there, can read it.

**Wrong**
- "Warden's write identity holds `Azure Kubernetes Service RBAC Admin` at cluster scope for the install" does not work for the namespace step. https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers

**Corrections**
- Customer side: "Either apply the probe manifest once with cluster-admin (preferred, keeps Warden read-only), or grant `Azure Kubernetes Service RBAC Cluster Admin` at cluster scope for the install and remove it afterwards. Pod Security / Azure Policy must allow hostPath `/var/log/pods`, `/var/lib/docker/containers` and `/` in `warden-system`."
- ClusterRole: core `events, namespaces, nodes, nodes/spec, nodes/stats, pods, pods/status, replicationcontrollers, resourcequotas, services` get/list/watch; apps `daemonsets, deployments, replicasets, statefulsets`; batch `jobs, cronjobs`; autoscaling `horizontalpodautoscalers`.

## UKS cluster

### Level 0, connected

**Verified**
- Permission `target_type: managed_kubernetes` exists; UUID or `*`. https://developers.upcloud.com/1.3/18-permissions/
- Cluster object: `name`, `state`, `version`, `zone`, `plan` (`dev-md` max 30 nodes, `prod-md-ha` max 120), `network`, `network_cidr`, `node_groups[]` (`plan`, `count`, `taints`, `labels`, `storage_encryption`, `utility_network_access`), `control_plane_ip_filter`, `private_node_groups`, `storage_encryption`, `labels`. Endpoints: `GET /1.3/kubernetes`, `GET /1.3/kubernetes/{uuid}`, `PATCH /1.3/kubernetes/{uuid}` (`control_plane_ip_filter` required, `labels`), `DELETE /1.3/kubernetes/{uuid}`, `PATCH /1.3/kubernetes/{uuid}/node-groups/{name}` with `{"count": n}` (scale), `GET .../available-upgrades`, `POST .../upgrade`. https://developers.upcloud.com/1.3/20-managed-kubernetes/
- Audit log `resource_types` includes `uks`. https://developers.upcloud.com/1.3/22-audit-logs/

**Missing**
- Delete-class actions in the same permission: `DELETE /1.3/kubernetes/{uuid}`, `DELETE .../node-groups/{name}`, `DELETE .../node-groups/{name}/{node_name}` (scales down and deletes that server), plus `POST .../upgrade`. The unconditional deny must list all of them. https://developers.upcloud.com/1.3/20-managed-kubernetes/
- `PATCH /1.3/kubernetes/{uuid}` requires `control_plane_ip_filter` in every body ("`null` is not accepted in `PATCH` request"); a label change must resend the current filter. Same URL.
- Audit log events for the cluster are a Level 0 read (`resource_types=uks`). https://developers.upcloud.com/1.3/22-audit-logs/
- The API token items from UpCloud server Level 0 (one-year maximum, IP pinning) apply.

**Wrong**
- Nothing contradicted.

**Corrections**
- Warden can do: "Scale a node group (`PATCH .../node-groups/{name}` `count`), modify the IP filter (`PATCH /1.3/kubernetes/{uuid}`). Denied without exception: cluster delete, node-group delete, node delete, upgrade."

### Level 1, API access

**Verified**
- `control_plane_ip_filter`: "Defaults to `null` on `POST` request, which implies no IP filtering is in place and access from any source is accepted"; `["0.0.0.0/0"]` allows all; `[]` denies all; "modifiable after cluster creation"; does not affect nodes or exposed services. https://developers.upcloud.com/1.3/20-managed-kubernetes/, https://upcloud.com/docs/products/managed-kubernetes/control-plane/
- `GET /1.3/kubernetes/{uuid}/kubeconfig` returns a kubeconfig with user `<cluster>-admin` using `client-certificate-data` / `client-key-data`; no regenerate endpoint. https://developers.upcloud.com/1.3/20-managed-kubernetes/
- UpCloud's own guide treats the primary kubeconfig as all-powerful ("the security equivalent of giving a stranger the master keys to your house") and recommends a ServiceAccount + RBAC + `kubectl create token` kubeconfig, revoked by `kubectl delete sa`. https://upcloud.com/docs/guides/creating-restricted-revocable-kubeconfigs/
- OIDC guide: "the static admin kubeconfig from `GET /kubernetes/{uuid}/kubeconfig` keeps working" and "If it has been shared, contact UpCloud support to rotate it." https://upcloud.com/docs/guides/managed-kubernetes-oidc-authentication/
- Service account tokens: TokenRequest tokens default to 1 hour and are recommended; a non-expiring token is a Secret of type `kubernetes.io/service-account-token` annotated `kubernetes.io/service-account.name`; "Only create long-lived API tokens if the token request mechanism is not suitable"; unused legacy tokens are invalidated after one year by default (stable since 1.30). https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/

**Missing**
- Lockout hazard: `PATCH` replaces the list; a cluster created with `null` (open) that Warden sets to `[<egress>/32]` cuts off the customer's own operators. Warden must read the current list, append, and show the resulting list for confirmation; if the current value is `null` the console must say that it is switching the cluster from open to restricted.
- Admin credential lifetime: not documented; rotation only via UpCloud support. Warden "uses it once and discards" but cannot prove to the customer that it is gone; the console should say the credential can only be rotated by UpCloud support.
- Whether a sub-account permission on `managed_kubernetes` allows `GET .../kubeconfig`: not documented (see verification point 1).
- Long-lived SA token: tie it to a Secret so it can be revoked by deleting the Secret, and state its lifetime (non-expiring). Alternative: grant the SA `create` on `serviceaccounts/token` for itself and renew with TokenRequest.
- OIDC authentication is now available (public beta, Kubernetes 1.34+, issuers managed at `/1.3/kubernetes/{uuid}/authentication`, issuer URL must be HTTPS, changes take effect in about a minute): Warden's OIDC issuer could authenticate without the admin kubeconfig, though a RoleBinding still has to be created by an admin. https://upcloud.com/docs/guides/managed-kubernetes-oidc-authentication/
- Kubernetes RBAC verb for exec is `create` on `pods/exec`; for logs `get` on `pods/log` (write Role must list them). https://kubernetes.io/docs/reference/access-authn-authz/rbac/

**Wrong**
- `provider-access-and-control.md`: "no OIDC integration documented" is out of date (OIDC guide dated 2026-09-21). https://upcloud.com/docs/guides/managed-kubernetes-oidc-authentication/
- "which is a cluster-admin client certificate": not documented; the user is named `-admin` and vendor guides call it the admin kubeconfig, but no page states the certificate's groups or privileges.

**Corrections**
- Customer side: "The tenant egress IP appended to `control_plane_ip_filter` (Warden reads the current list, adds the IP, shows the full result; a `null` list is shown as 'open to the internet' before the change)."
- Warden side: add "stores the SA token as a `kubernetes.io/service-account-token` Secret in `warden-system` so the customer can revoke it by deleting the Secret; records that the admin kubeconfig was fetched at time T and can be rotated only by UpCloud support; prefers OIDC on clusters at 1.34+ once out of beta."

### Level 2, probe

**Verified**
- "The worker nodes require Internet access to operate correctly."; private node groups: "the selected SDN network has to have a SDN router and a NAT Gateway configured"; `private_node_groups` defaults to `false`; Cilium CNI; Debian-based nodes (auto-update text). https://upcloud.com/docs/products/managed-kubernetes/data-plane/, https://developers.upcloud.com/1.3/20-managed-kubernetes/
- Same collector RBAC and volume requirements as AKS Level 2. https://opentelemetry.io/docs/platforms/kubernetes/collector/components/

**Missing**
- Private node groups: outbound 443 goes through the customer's NAT Gateway; its state and the gateway plan's connection limit (10,000 on the free Development plan) are prerequisites. https://upcloud.com/docs/products/managed-kubernetes/data-plane/
- metrics-server presence and kubelet TLS/auth on UKS nodes: not documented; the kubeletstats `insecure_skip_verify: true` example on opentelemetry.io is what the probe would use.
- The bootstrap ClusterRole for the probe must be created by the admin kubeconfig step (it is), and the Level 1 SA needs `create` on `daemonsets`, `deployments`, `serviceaccounts`, `configmaps`, `secrets`, `clusterrolebindings` in/for `warden-system` to apply the manifests; the document's "read ClusterRole + write Role per permitted namespace" does not include `clusterrolebindings` create unless `warden-system` is a permitted namespace and the binding is created at bootstrap.

**Wrong**
- Nothing contradicted.

**Corrections**
- Customer side: "Nodes with outbound 443 to the tenant hostname; for private node groups, a running NAT Gateway on the cluster's SDN network."
- Warden side: "Bootstrap creates the ClusterRoleBinding for the probe's ServiceAccount; the Level 1 SA only creates the namespaced objects."

## Answers to the six open verification points

1. **UpCloud metrics, activity log, UKS kubeconfig by sub-account.** No metrics endpoint for servers; the only metrics route is `GET /1.3/database/{uuid}/metrics` for managed databases (https://developers.upcloud.com/1.3/16-managed-database/, https://developers.upcloud.com/1.3/8-servers/). An activity log exists: `GET /1.3/audit-logs` with `resource_types` incl. `server` and `uks`, `account_username`, `ip_address`, 180 days (https://developers.upcloud.com/1.3/22-audit-logs/). Whether a sub-account holding `managed_kubernetes` can call `GET /1.3/kubernetes/{uuid}/kubeconfig`: not documented; the permissions page defines access per resource type without an endpoint table (https://developers.upcloud.com/1.3/18-permissions/). Test empirically.
2. **UKS kubeconfig identity and expiry.** The kubeconfig user is `<cluster>-admin` with `client-certificate-data`/`client-key-data`; privileges are not documented, but UpCloud calls it the "static admin kubeconfig" and the "primary kubeconfig ... master keys". No expiry, no rotation endpoint; "If it has been shared, contact UpCloud support to rotate it." (https://developers.upcloud.com/1.3/20-managed-kubernetes/, https://upcloud.com/docs/guides/managed-kubernetes-oidc-authentication/, https://upcloud.com/docs/guides/creating-restricted-revocable-kubeconfigs/).
3. **AKS ClusterRole creation and `pods/exec`.** Creating a ClusterRole needs data action `Microsoft.ContainerService/managedClusters/rbac.authorization.k8s.io/clusterroles/write` and, by the Kubernetes escalation rule, every permission the ClusterRole contains. `RBAC Admin` has `managedClusters/*` minus `namespaces/write|delete` and `resourcequotas/write|delete`, so it can create the ClusterRole but not the `warden-system` namespace; `RBAC Cluster Admin` can do both. No Azure data action exists for `nodes/stats`, so the escalation check for that rule is not documented. `RBAC Writer` at namespace scope includes `managedClusters/pods/*`, and the data action `managedClusters/pods/exec/action` exists; Azure wildcards match nested action paths, so exec is covered (https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/containers, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/containers, https://learn.microsoft.com/en-us/azure/role-based-access-control/role-definitions, https://kubernetes.io/docs/reference/access-authn-authz/rbac/).
4. **Azure `Monitoring Reader` and Resource Health.** `Monitoring Reader` = `*/read` + `Microsoft.OperationalInsights/workspaces/search/action` + `Microsoft.Support/*`; Azure Monitor states metrics need "read access to the resource type and scope"; `Reader` (`*/read`) therefore suffices for platform metrics, the Activity Log (`Microsoft.Insights/eventtypes/values/Read`) and Resource Health (`Microsoft.ResourceHealth/AvailabilityStatuses/read`). `Monitoring Reader` is not required (https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/monitor, https://learn.microsoft.com/en-us/azure/azure-monitor/roles-permissions-security, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/monitor, https://learn.microsoft.com/en-us/azure/role-based-access-control/permissions/management-and-governance).
5. **OpenSSH Match block.** The current sshd_config(5) lists both `TrustedUserCAKeys` and `AuthorizedPrincipalsFile` among the keywords allowed after `Match`, and `Match All` is a valid criterion (https://man.openbsd.org/sshd_config). `AuthorizedPrincipalsFile` has been Match-able since 5.6 (https://www.openssh.org/txt/release-5.6); no release note from 5.4 to 6.5 records when `TrustedUserCAKeys` became Match-able, so the floor is not documented. Per-distribution confirmation: run `sshd -T -C user=warden,addr=<egress> | grep -Ei 'trustedusercakeys|authorizedprincipalsfile'` on each target image; any distro with OpenSSH ≥ 8.2 also supports the `Include` drop-in (https://www.openssh.org/txt/release-8.2). Note the 10.6 fix for `none` inside Match (https://www.openssh.org/txt/release-10.6).
6. **Ansible Python floor and compose.** Managed-node Python by ansible-core: 2.18/2.19 → 3.8-3.13; 2.20/2.21 → 3.9-3.14; 2.22 → 3.9-3.15; 2.23 → 3.10-3.15 (https://docs.ansible.com/projects/ansible-core/devel/reference_appendices/release_and_maintenance.html). `community.docker.docker_compose_v2` needs "Docker CLI with Docker compose plugin 2.18.0 or later" on the host that runs the module (the managed node) and PyYAML only when `definition` is used; no Docker SDK; check mode full, diff mode none; the v1 module is removed since community.docker 4.0.0 (https://docs.ansible.com/projects/ansible/latest/collections/community/docker/docker_compose_v2_module.html).

## Top five risks, ranked

1. **AKS Level 2 install path does not work as written.** `RBAC Admin` cannot create `warden-system` and no Azure data action covers `nodes/stats`; Warden would need `RBAC Cluster Admin`, which is cluster-wide super-user, contradicting the "read-only install" intent. Make the customer-applied manifest the default and Cluster Admin an explicit, time-boxed opt-in.
2. **AKS write identity is much larger than the catalogue.** `managedClusters/write` can re-enable local accounts and rewrite the API access profile; `agentPools/write` upgrades as well as scales; `RBAC Writer` reads every Secret and impersonates every ServiceAccount in its namespaces. The gate is the only control; the cell page must say so, or custom roles must replace the built-ins.
3. **UKS admin kubeconfig has no documented lifetime and no customer-side rotation.** Warden touches a credential that only UpCloud support can rotate, and a `PATCH` of `control_plane_ip_filter` can lock the customer out of their own cluster (default `null` = open; the call replaces the list). Append-only edits with confirmation, a recorded fetch time, and OIDC (1.34+) as the forward path.
4. **AKS Level 1 read identity cannot deliver the promised reads.** `RBAC Reader` lacks `nodes/read`, and the `pods/log` data action is undocumented; `command invoke` for private clusters is explicitly "Not for programmatic access". Add a custom read role and verify logs on a live cluster before the console shows "node status" or "pod logs".
5. **Host-side prerequisites are under-specified and partly wrong.** OpenSSH floor is 8.2 (drop-in) not 6.2, a `Match` block in a drop-in needs a closing `Match All`, Python must be ≥ 3.9 for current ansible-core (RHEL 8 and Ubuntu 20.04 fail), new Azure VNets have no default outbound route, the UpCloud firewall default rule is customer-set and takes two minutes, and UpCloud API tokens expire within a year. Each is a silent onboarding failure unless validation checks it.
