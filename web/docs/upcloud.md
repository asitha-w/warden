# UpCloud

What must exist in your UpCloud account for each level, and the exact steps. Read [the model](#docs/model) first if the levels are new to you.

## The connection

A sub-account with an API token, and permissions granted by your main account.

The connection form asks for the sub-account, its API token and validity, and the permissions you granted, and shows every command below filled in with your values.

1. **Create a sub-account** for Warden in the control panel under People.
2. **Create an API token** for it. Tokens are valid at most 365 days; set `allowed_ip_ranges` to the tenant egress address. Basic authentication is being deprecated and is not accepted. The form shows the `POST /1.3/account/tokens` call.
3. **Grant permissions** from the main account with `POST /1.3/permission/grant`: target type `server` with the server UUID (or `*`), `managed_kubernetes` with the cluster UUID. Add `storage: yes` if Warden should read storage details.
4. **Validate.** Warden lists what the token can reach and fills the catalogue. UpCloud permissions are per resource and carry no read and write split, so a server permission allows stop as well as read; Warden's policy gate is the only split, and the connection page says so.

Warden tracks the token's expiry and takes the connection offline with a finding before it lapses.

::layers vm

## Server Level 0: connected

What exists: the connection above with a permission on the server.

What Warden reads: state, plan, zone, IP addresses, tags, storage, firewall and metadata flags; audit log events for the server (actor, source IP, 180 days). UpCloud has no metrics endpoint for servers; state is what Level 0 gives.

What Warden can do under policy: start, stop, restart, add and remove firewall rules.

Revoke: revoke the permission, delete the token, or delete the sub-account.

## Server Level 1: granted

What must exist on the server:

- Linux with OpenSSH 8.2 or later, Python 3.9 or later, systemd, outbound 443 to your tenant hostname.
- A public IPv4 address, or the reverse tunnel from [the grant](#docs/grant).
- `metadata: yes` on the server, so the grant can read the server UUID. API-created servers have it off by default; enable it with a server modify.
- Inbound TCP 22 from the tenant egress address. Warden adds the firewall rule itself through the API when the server firewall is enabled; the rule takes one to two minutes to apply. The firewall's **default rule** is set by you in the control panel: an allow rule only restricts anything when the default is drop, and the cell page shows which it is. The firewall does not cover the utility network interface.

Steps:

1. Open the cell page and copy the **grant line**.
2. Let Warden add the firewall rule, or add it yourself: allow TCP 22 from `<tenant egress>/32`.
3. Run the grant line on the server as a user with sudo. It creates the `warden` user, installs the tenant's SSH certificate authority public key, installs the probe, and reports the host key and server UUID back.
4. Watch the cell page: the channel goes online and the level to 1.

What Warden reads and does: the same as an Azure VM at Level 1, see [Azure](#docs/azure/vm-level-1-granted).

Revoke: delete the `warden` user and the sshd drop-in, or remove the firewall rule in the control panel.

::layers k8s

## UKS Level 0: connected

What exists: the connection above with a `managed_kubernetes` permission on the cluster.

What Warden reads: state, version, zone, plan, node groups with plans and counts, the control plane IP filter, private node group setting, network; audit log events for the cluster.

What Warden can do under policy: scale a node group, modify the IP filter. **Denied without exception**, because the same permission allows them: cluster delete, node group delete, node delete, upgrade.

Revoke: revoke the permission.

## UKS Level 1: API access

What must exist: the tenant egress address in the cluster's `control_plane_ip_filter`.

Steps:

1. On the cell page choose **Enable API access**. Warden reads the current IP filter, appends the tenant egress address, and shows the full resulting list for your confirmation. A cluster created without a filter is open to the internet, and Warden says so before changing it: the modify call replaces the whole list, and a wrong list locks you out.
2. Warden fetches the cluster kubeconfig from the API. It is a client certificate for the user `<cluster>-admin` with no documented expiry; it can be rotated only by UpCloud support, and Warden records when it fetched it.
3. Warden uses that kubeconfig once to create, in `warden-system`: two ServiceAccounts `warden-read` and `warden-write`, a read-only ClusterRole bound to the first, a write Role in each permitted namespace bound to the second (`create` on `pods/exec`, `get` on `pods/log`), the probe's ClusterRole and binding for Level 2, and a token Secret per ServiceAccount so you can revoke by deleting the Secret. It stores the two tokens and discards the admin kubeconfig.
4. The level goes to 1.

On clusters at Kubernetes 1.34 or later, UKS OIDC authentication (beta) against the tenant runtime's issuer avoids the admin kubeconfig entirely; Warden offers it when available.

What Warden reads and does: the same as AKS at Level 1, see [Azure](#docs/azure/aks-level-1-api-access).

Revoke: delete the two ServiceAccounts or their token Secrets, or remove the egress address from the IP filter.

## UKS Level 2: probe

What must exist: nodes with outbound 443 to your tenant hostname. For private node groups, a running NAT gateway on the cluster's SDN network.

Steps: on the cell page choose **Install probe**. Warden applies the probe manifest with `warden-write`; the ClusterRole and binding already exist from Level 1. Or download the manifest and apply it yourself.

What Warden reads: node and kubelet metrics, container logs with history, retained events, cluster-level metrics.

Revoke: delete the `warden-system` namespace.
