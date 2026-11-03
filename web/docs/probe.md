# The probe

The probe is what Warden runs on a Level 1 VM and a Level 2 cluster to collect telemetry. It is an unmodified **OpenTelemetry Collector** that dials out to the tenant runtime. It collects; it never executes a command.

## Two shapes

| | Host probe | Cluster probe |
|---|---|---|
| Runs on | Azure VM, UpCloud server | AKS, UpCloud Managed Kubernetes |
| Runs as | A systemd service: the OpAMP supervisor, which starts and watches the collector | A DaemonSet (one collector per node) and a single-replica Deployment in `warden-system` |
| Collects | Host metrics, processes, the log files you enable, application receivers you enable (nginx, PostgreSQL, Redis and others), OTLP from your application on localhost | Node and kubelet metrics, container logs, cluster metrics and events |
| Configured by | Remote configuration over OpAMP | A ConfigMap written through the Kubernetes API |
| Installed by | [The grant](#docs/grant) | You, with the manifest from the cell page, or Warden through its Level 1 identity |

## What it sends and where

Metrics, logs and events go over OTLP on port 443 to your tenant hostname, authenticated with a per-cell credential that can only write telemetry for that one cell. The runtime stores them in the tenant's own telemetry store. The investigator reads from that store, never from the probe directly.

## What the investigator can read

Only what you enabled on the cell page. A log file that is not listed cannot be tailed; a receiver that is off produces nothing. The cell page is the contract.

## When things fail

- Collector crashes: the supervisor or Kubernetes restarts it. After repeated restarts the channel shows degraded and a finding opens on the cell.
- Bad configuration: Warden validates every configuration before pushing it and keeps the last one that reported healthy; if a push fails, the last healthy one goes back.
- Runtime unreachable: the collector queues on disk and retries. The channel shows offline after three missed heartbeats. Provider polling continues, so Level 0 keeps working.
- A node is lost in a cluster: only that node's collector is gone.

## Removing it

On a host, revoking [the grant](#docs/grant) removes it. On a cluster, delete the `warden-system` namespace.
