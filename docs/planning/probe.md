# The probe, 2026-10-31

The probe is what Warden puts on a probe-tier cell. It is an unmodified OpenTelemetry Collector (contrib distribution) that dials out to the tenant runtime. It collects; it never executes. Command execution is "hands" and is covered in `provider-access-and-control.md`.

## Two shapes, one contract

| | Host probe | Cluster probe |
|---|---|---|
| Cell kind | Azure VM, UpCloud server | AKS, UKS |
| Runs as | systemd service: the OpAMP supervisor, which starts and watches `otelcol-contrib` | DaemonSet (one collector per node) plus a single-replica Deployment (cluster-level) in `warden-system` |
| Collects | hostmetrics (cpu, memory, disk, filesystem, network, load), process scraper, filelog per enabled log source, optional receivers (nginx, postgresql, redis, ...), OTLP in on localhost for the application | DaemonSet: kubeletstats, hostmetrics of the node, filelog of container logs. Deployment: k8s_cluster metrics, k8sobjects events |
| Configured by | OpAMP remote config through the supervisor | ConfigMap written through the Kubernetes API plus a rollout; the `opamp` extension in each collector reports status back |
| Installed by | Azure: run command or Custom Script Extension. UpCloud: `user_data` on creation or the operator-run install line | Warden applies the manifests through the Kubernetes API, or the customer applies them |
| Identity to Warden | Per-cell bootstrap token at install, exchanged on first OpAMP connect for a per-instance credential | Per-cell token mounted as a Secret; one instance UID per pod |

Both shapes speak the same contract to the runtime: OpAMP for status and config, OTLP for data. The Kubernetes shape does not run the supervisor. Warden already has API access to the cluster to install the probe, so config is a ConfigMap and the collector's read-only `opamp` extension reports health. That avoids a supervisor process inside each pod.

## What the runtime provides

Per tenant: an OpAMP server endpoint and an OTLP ingest endpoint, both on 443 behind the tenant's hostname, both accepting only the tenant's probe credentials. The natural ingest is a gateway collector per tenant: OTLP receiver with the auth extension, resource processor stamping tenant and cell, ClickHouse exporter. The signal loop and the investigator tools read ClickHouse, never the probe.

## Config rendering

The `Telemetry` struct on a cell renders to collector YAML. Each field maps to a block.

| Setting | Collector |
|---|---|
| `hostMetrics` | `hostmetrics` receiver with cpu, memory, disk, filesystem, network, load scrapers, 30 s |
| `processes` | `process` scraper on hostmetrics, names and cpu/memory per process |
| `logs[]` with `enabled` | one `filelog/<name>` receiver per enabled source, start at end, with the cell's multiline and parser preset if known |
| `receivers[]` with `enabled` | the named contrib receiver with its default endpoint on localhost |
| `otlp` | `otlp` receiver on 127.0.0.1:4317 and :4318 |

Always present: `memory_limiter`, `resourcedetection` (cloud provider and host), `resource` (tenant and cell attributes), `batch`, `otlp` exporter to the runtime with the per-instance credential and a persistent sending queue (`file_storage` extension) so a runtime outage does not lose data. The rendered config is validated server-side (`otelcol validate` against the pinned version) before it is pushed; a config that does not validate is never sent.

The investigator can only read what is enabled: `tail_logs` on a path that is not an enabled log source is refused at the toolbox, as the code already does.

## Lifecycle

1. **Onboard** the cell at basic tier. Nothing installed.
2. **Install.** Warden issues a bootstrap token bound to the cell and an install line. Azure VM: Warden runs it through run command. UpCloud server: the operator runs it, or `user_data` does. Cluster: Warden applies the manifests. The cell shows channel `waiting`.
3. **First connect.** The supervisor (or the pod) connects with the bootstrap token, presents its instance UID, receives its per-instance credential and the current rendered config. Channel goes `online`, tier goes to probe. The `Probe.Hosts` list is the set of instance UIDs seen; `Probe.Expected` is what the catalogue says the cell has (one for a VM, the node count plus one for a cluster).
4. **Change telemetry.** Save renders a new config, validates it, pushes it. The supervisor applies it and reports health; the cluster probe gets a new ConfigMap and a rollout restart.
5. **Detach.** Warden pushes an empty pipeline config, then uninstalls through hands if it has them, otherwise tells the operator what to remove. The credential is revoked either way.

## Failure modes and what covers them

| Failure | Behaviour | What Warden does |
|---|---|---|
| Collector crashes | Supervisor restarts it; Kubernetes restarts the pod | Health report shows restarts; after three in ten minutes the channel is marked `degraded` and a finding opens on the cell |
| Bad config pushed | The supervisor caches the last received remote config and re-applies it on restart, so a bad config persists. There is no built-in rollback. | Validation before push, and the runtime keeps the last config that reported healthy per cell. When health reports failed after a push, Warden pushes the last healthy config back and marks the telemetry change as rejected. |
| Runtime unreachable | Exporter retries with backoff; the persistent queue holds data on disk up to its size limit | Channel goes `offline` after three missed heartbeats. Provider polls continue, so the basic tier keeps working and the signal loop still sees provider rules. |
| Node lost in a cluster | That node's DaemonSet pod is gone; the others are unaffected | Expected versus seen instance count on the cell page |
| Cluster-level collector lost | Single replica; Kubernetes reschedules it | A gap in cluster metrics and events, visible as a hole; no duplication risk, which is why it is one replica |
| Credential leaked | A per-instance credential only authorises writing telemetry for one cell | Revoke the instance, re-issue a bootstrap token, reinstall |
| Collector version drift | Warden pins one contrib version per runtime release | Host: upgrade through the supervisor's package capability or by re-running the installer. Cluster: image tag in the manifests. |

There is no second probe to fail over to on a host; a VM has one collector, and its resilience is the supervisor plus the persistent queue. On a cluster, redundancy comes from the DaemonSet. The failover that matters is at the runtime side: the ingest and OpAMP endpoints must be the highly available part, not the probe.

## Open questions, in order

1. **Supervisor maturity.** The OpAMP supervisor in collector-contrib is still young. Its stability level, its behaviour on a bad config, and its package upgrade path must be tested on a real UpCloud server before the host probe design is final. If it is not ready, the host probe falls back to the `opamp` extension for status and the installer for config changes, which is the same model as the cluster probe.
2. **Which contrib components** go into the pinned build: hostmetrics, filelog, the application receivers in the first telemetry preset list, otlp, opamp, file_storage, resourcedetection. A custom distribution built with the collector builder keeps the binary small.
3. **Credential exchange.** OpAMP has a certificate provisioning flow for mTLS; a bearer token is simpler for the PoC. Decide at implementation.
4. **Private clusters.** Whether the cluster probe should carry a reverse tunnel for the Kubernetes API, which would turn it into something more than a collector.
5. **Log parsing presets** per log source: syslog, nginx access, journald, container logs. The presets are what make `tail_logs` output readable for the investigator.

## Sources

- Collector Helm chart modes and presets: https://opentelemetry.io/docs/platforms/kubernetes/helm/collector/
- OpAMP at scale, supervisor and bridge: https://www.cncf.io/blog/2026/07/13/operating-opentelemetry-at-scale-with-opamp/
- Supervisor re-applies the last received config: https://help.splunk.com/en/splunk-observability-cloud/manage-data/manage-otel-agents-and-collectors/remote-configuration-of-opentelemetry-collectors/opamp-supervisor-for-collectors-on-standalone-hosts/troubleshoot-opamp-supervisor
- OpAMP gateway extension: https://bindplane.com/blog/opamp-for-opentelemetry-managing-collector-fleets-and-introducing-the-new-opamp-gateway-extension
