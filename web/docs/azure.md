# Azure

What must exist in your Azure subscription for each level, and the exact steps. Read [the model](#docs/model) first if the levels are new to you.

## The connection

One service principal for Warden in your Entra directory, with roles at a subscription or resource group scope.

The connection form asks for the directory (Entra tenant id), the subscription id, the identity, a read scope and an optional write scope, and shows every command below filled in with your values.

1. **Create the identity.** Either a tenant administrator opens the consent link the form shows and accepts, which creates the Warden service principal in your directory and gives you its object id; or you bring your own app registration with a **federated credential** that trusts the tenant runtime's OIDC issuer (issuer, subject and audience `api://AzureADTokenExchange` are shown on the form), a certificate, or a client secret as the fallback.
2. **Assign the read roles.** `Reader` at the scope covers discovery through Resource Graph, instance views, platform metrics, the Activity Log and Resource Health; `Monitoring Reader` is not needed. For clusters in scope: `Azure Kubernetes Service RBAC Reader`, `Azure Kubernetes Service Cluster User Role`, and the custom `Warden AKS reader` role (data action `managedClusters/nodes/read`), whose definition the form offers for download.
3. **Create and assign the write roles, optional.** Three custom roles with only the catalogue's actions: `Warden VM operator`, `Warden AKS operator` and `Warden AKS namespace writer`. The form offers the JSON and the assignment commands. Assigning roles needs Owner, User Access Administrator or Role Based Access Control Administrator at the scope.
4. **Validate.** Warden lists the role assignments it finds, derives its capabilities from them, fills the catalogue of what it could manage, and refuses to connect if the read identity carries write actions.

::layers vm

## VM Level 0: connected

What exists: the connection above, with the VM inside its read scope.

What Warden reads: power state and VM Agent status from the instance view; platform metrics without any agent: `Percentage CPU`, `Available Memory Bytes`, `Network In Total`, `Network Out Total`, `Disk Read Bytes`, `Disk Write Bytes`, `VmAvailabilityMetric`; the Activity Log (90 days); Resource Health.

What Warden can do, with the write identity and under policy:

```
Microsoft.Compute/virtualMachines/start/action
Microsoft.Compute/virtualMachines/restart/action
Microsoft.Compute/virtualMachines/deallocate/action
Microsoft.Compute/virtualMachines/powerOff/action
```

Revoke: remove the role assignments or delete the service principal.

## VM Level 1: granted

What must exist on the VM:

- Linux with OpenSSH 8.2 or later and systemd.
- Python 3.9 or later (Ansible runs on it; RHEL 8 and Ubuntu 20.04 are below the floor).
- Outbound HTTPS (443) to your tenant hostname. A VM in a virtual network created after March 2026 has no default outbound route and needs a NAT gateway, a load balancer outbound rule or a public IP.
- Inbound TCP 22 from the tenant egress address only: a network security group rule, the host firewall if enabled, and a public IP or a routed path. If you refuse inbound, use the reverse tunnel described in [the grant](#docs/grant).
- For the container image catalogue: Docker CLI with the compose plugin 2.18 or later.

Steps:

1. Open the cell page and copy the **grant line**. It carries a one-time token valid for one hour.
2. Add the NSG rule:

```
az network nsg rule create -g <rg> --nsg-name <nsg> -n warden-ssh \
  --priority 300 --direction Inbound --access Allow --protocol Tcp \
  --source-address-prefixes <tenant egress>/32 --destination-port-ranges 22
```

3. Run the grant line on the VM as a user with sudo. It creates the `warden` user, installs the tenant's SSH certificate authority public key, installs the probe, and reports the host key and the VM id back. Warden cross-checks the VM id against Azure.
4. Watch the cell page: the channel goes from waiting to online and the level to 1.

What Warden reads: what the cell's telemetry settings enable (host metrics, processes, named log files, named receivers, OTLP from your application), plus Ansible facts on demand.

What Warden can do: catalogue actions as Ansible roles, run through SSH with a certificate valid for ten minutes; free-form commands only under an `ask` policy with the exact command in the approval. The `warden` user has full sudo because Ansible cannot run under a command-restricted sudo; the catalogue and the policy gate are what limit what runs.

Revoke: delete the `warden` user and the sshd drop-in, or remove the NSG rule.

::layers k8s

## AKS Level 0: connected

What exists: the connection above, with the cluster inside its read scope. `Reader` covers the cluster and node pool reads.

What Warden reads: cluster and node pool state, Kubernetes version, API server access profile, Entra and Azure RBAC settings, platform metrics, the Activity Log, Resource Health.

What Warden can do, with the write identity and under policy:

```
Microsoft.ContainerService/managedClusters/agentPools/write     # scale; also allows any node pool change
Microsoft.ContainerService/managedClusters/start/action
Microsoft.ContainerService/managedClusters/stop/action
```

The approval for a scale shows the full node pool diff because the permission allows more than scaling. `managedClusters/write` is never requested: it would allow re-enabling local accounts and rewriting the API server access profile. Cluster upgrade is not in the catalogue.

## AKS Level 1: API access

What must exist on the cluster:

- Managed Entra integration with **Azure RBAC for Kubernetes** enabled, in the same Entra tenant as the subscription.
- A public API server with the tenant egress address appended to the authorized IP ranges (keep the cluster's own egress IP in the list; changes take up to two minutes). A **private cluster** reaches Level 1 only through the tunnel pod described below.

Role assignments on the cluster resource (up to five minutes to propagate):

- Read identity: `Azure Kubernetes Service RBAC Reader` at cluster scope, `Azure Kubernetes Service Cluster User Role` so Warden can fetch the kubeconfig, and a custom role with the data action `Microsoft.ContainerService/managedClusters/nodes/read`, because RBAC Reader cannot list nodes.
- Write identity: a custom role at the namespaces Warden may act in, with these data actions and no `secrets/*`:

```
Microsoft.ContainerService/managedClusters/apps/deployments/read
Microsoft.ContainerService/managedClusters/apps/deployments/write
Microsoft.ContainerService/managedClusters/apps/replicasets/read
Microsoft.ContainerService/managedClusters/pods/read
Microsoft.ContainerService/managedClusters/pods/delete
Microsoft.ContainerService/managedClusters/pods/exec/action
```

The built-in `Azure Kubernetes Service RBAC Writer` is accepted instead, but it reads every Secret and can act as any ServiceAccount in those namespaces, and the cell page will say so.

Steps:

1. Confirm Entra integration and Azure RBAC on the cluster: `az aks show -g <rg> -n <cluster> --query "aadProfile.enableAzureRbac"`.
2. Append the tenant egress address: `az aks update -g <rg> -n <cluster> --api-server-authorized-ip-ranges <existing>,<tenant egress>/32`.
3. Assign the roles above to Warden's service principal, scoped to the cluster resource id, and the write role to `<cluster id>/namespaces/<namespace>`.
4. On the cell page, choose **Enable API access**. Warden fetches the user kubeconfig, exchanges its credential for an Entra token, lists nodes and namespaces, and sets the level to 1.

What Warden reads: workloads, pods, events, node status, metrics from the metrics API. Pod logs appear once Warden has confirmed them on your cluster.

What Warden can do: rollout undo and restart, scale, delete a pod, exec, in the permitted namespaces, under policy.

**Private clusters.** Apply the tunnel manifest from the cell page: one pod in `warden-system` running a plain OpenSSH client that dials out to the tenant runtime on 443 and forwards the API server through it. Outbound only, nothing exposed. The same roles apply. A jump VM in the virtual network carrying [the grant](#docs/grant) is the fallback. Azure's `command invoke` is not used; Microsoft documents it as not for programmatic access.

Revoke: remove the Kubernetes role assignments, or remove the egress address from the authorized ranges, or delete the tunnel pod.

## AKS Level 2: probe

What must exist: nodes with outbound 443 to your tenant hostname, and a Pod Security or Azure Policy configuration that allows hostPath mounts of `/var/log/pods`, `/var/lib/docker/containers` and `/` in `warden-system`.

Steps, default path: download the probe manifest from the cell page and apply it once with your own cluster-admin. This keeps Warden's identities unchanged. It creates the namespace, a ServiceAccount, a read-only ClusterRole (get, list, watch), a DaemonSet with the node collector and a single-replica Deployment with the cluster collector, and a Secret with the cell's enrollment token.

Alternative: grant `Azure Kubernetes Service RBAC Cluster Admin` to Warden's write identity for the install and remove it afterwards. `RBAC Admin` is not enough: it cannot create the namespace.

What Warden reads: node and kubelet metrics, container logs as far as the kubelet keeps them (10 MiB times 5 files per container by default), retained events, cluster-level metrics.

Revoke: delete the `warden-system` namespace and the ClusterRoleBinding.
