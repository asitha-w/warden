# AI SRE vendors — how they reach into customer infrastructure

Sources: vendor docs, trust/security pages, and vendors' own public Helm-chart repos only. No third-party sites used. Fetched 2026-10-31. Where a fact comes from a vendor's public GitHub chart rather than prose docs, the chart file is cited.

---

## 1. Resolve.ai

**Deployment form**
Cloud-hosted SaaS, optionally with a "Satellite" (containerized agent) deployed in customer clusters (https://docs.resolve.ai/security-and-administration.md). Marketing page lists two models: "Shared — secure multi-tenant environment with satellite deployment" and "Dedicated — single tenant cloud deployment with bring your own keys" (https://resolve.ai/security). No on-prem control plane offered.

**What is installed**
Resolve Satellite: Helm chart `oci://registry-1.docker.io/resolveaihq/satellite-chart`, a StatefulSet (since v1.1.0), authenticated by an ingest token generated in the Resolve UI; alternative ECS Fargate deployment via CloudFormation (https://docs.resolve.ai/resolve-satellite.md, https://docs.resolve.ai/satellite-on-ecs.md). RBAC: ClusterRole `resolve-satellite`, read-only `get/list/watch` on ConfigMaps, Events, Namespaces, Nodes, Pods, Services; DaemonSets, Deployments, ReplicaSets, StatefulSets; PV/PVC/StorageClasses; Jobs/CronJobs; CRDs ArgoRollout, Istio VirtualService/Gateway — "No exec, create, delete, or patch permissions are required" (https://docs.resolve.ai/resolve-satellite.md, https://resolve.ai/integrations/kubernetes). Namespace scoping via `namespaceIncludeList`. Optional DNS Tap component for runtime dependency discovery. Resource ask: 1 core, 8 GB RAM. One Satellite can serve multiple clusters via a kubeconfig in a Secret.

**Cloud identity model**
AWS: cross-account IAM role `resolve-access-role` trusting account `590183716134` with ExternalId; managed policies `SecurityAudit`, `CloudWatchReadOnlyAccess`, `AmazonRDSReadOnlyAccess`, `IAMReadOnlyAccess`; optional CloudWatch Log Group allow-list; via Satellite it instead uses a static access key/secret in a K8s Secret (https://docs.resolve.ai/aws.md). Azure: app registration + client secret, roles `Monitoring Reader` and `Log Analytics Reader`; secret can stay in-cluster when connected via Satellite (https://docs.resolve.ai/azure.md). GCP: SA key JSON, or Workload Identity Federation from Resolve's AWS workload, or WIF from the Satellite's Kubernetes ServiceAccount (no key material anywhere); roles `roles/logging.viewer`, `roles/monitoring.viewer` (https://docs.resolve.ai/gcp.md). Observability tools: customer API keys stored in K8s Secrets mounted at `/etc/secrets/<name>/`, "raw secret values never leave your infrastructure" (https://docs.resolve.ai/resolve-satellite.md).

**Network path**
Satellite is outbound-only: long-lived (30+ min) bidirectional gRPC/HTTP2 stream to Resolve; published allowlist CIDR `18.97.138.16/29`; HTTP proxy supported if it allows long-lived HTTP/2 (https://docs.resolve.ai/resolve-satellite.md). ECS variant: egress 443 to Resolve SaaS and integration endpoints (https://docs.resolve.ai/satellite-on-ecs.md). Satellite-side `domainAllowList` restricts which integration domains it may call; `configOverride: false` (default) blocks remote configuration changes from the SaaS UI; optional per-request audit logging of outbound integration calls.

**How actions execute**
Investigation is read-only queries, proxied through the Satellite for on-prem sources or direct from SaaS for cloud sources. Write actions ("Mitigation Actions": alert silences/downtimes in Grafana/Datadog/AlertManager/Sumo Logic/Kloudfuse; Git PR creation; GitHub Actions dispatch/retry/cancel) are executed by a separate "execution engine" using customer credentials — "the AI model that generates proposals never has direct access to write APIs" (https://docs.resolve.ai/mitigation-actions.md, https://docs.resolve.ai/security-and-administration.md). Required write scopes listed per platform, e.g. Grafana `alert.provisioning:write`, Datadog `monitors_downtime`, AlertManager `/api/v2/silences`. No VM-level command execution or `kubectl exec` is documented anywhere.

**Approval / autonomy posture**
Read-only by default; every mitigation proposal requires explicit human Approve in UI or Slack; proposals auto-expire; silences revocable; all action types default disabled and are enabled per org with team overrides ("Team override > Organization default > Disabled") (https://docs.resolve.ai/mitigation-actions.md). Exception: GitHub Actions workflow tools default to approval but an admin can opt an individual tool into automatic execution, optionally under per-user auth rather than shared credentials (https://docs.resolve.ai/security-and-administration.md). Mitigation Activity dashboard audits who proposed/approved/rejected/revoked.

**Tenant isolation and data handling**
"Customer data is siloed"; raw telemetry queried live and not retained; summaries/metadata cached "in customer-specific environments"; AES-256 at rest, TLS 1.2+; regex PII redaction (email, phone, SSN, card, IP, address…) plus satellite-side `redactionConfig` JSON paths and an optional redaction sidecar; learning signals generalized and stripped before reuse; SOC 2 Type II, HIPAA, GDPR; SAML/OIDC SSO (https://docs.resolve.ai/security-and-administration.md, https://docs.resolve.ai/sensitive-data-redaction.md).

**Doc URLs used**
https://docs.resolve.ai/resolve-satellite.md · https://docs.resolve.ai/satellite-on-ecs.md · https://docs.resolve.ai/security-and-administration.md · https://docs.resolve.ai/mitigation-actions.md · https://docs.resolve.ai/aws.md · https://docs.resolve.ai/azure.md · https://docs.resolve.ai/gcp.md · https://docs.resolve.ai/sensitive-data-redaction.md · https://resolve.ai/integrations/kubernetes · https://resolve.ai/security

**Not published**
Exact ClusterRole YAML (docs say inspect with `kubectl describe clusterrole resolve-satellite`); whether the "Dedicated" single-tenant tier is a separate account/VPC; data-residency regions; details of the GitHub Actions "automatic execution" audit model; nothing on VM/SSH access (none exists per docs).

---

## 2. Traversal

**Deployment form**
Three options: multi-tenant SaaS; single-tenant SaaS (dedicated Traversal-owned AWS account, own domain, optional PrivateLink); BYOC on AWS (dedicated customer-owned account, EKS-on-EC2, fully managed by Traversal via cross-account AssumeRole plus Tailscale VPN for operators). Web app and auth remain external even in BYOC (https://docs.traversal.com/architecture/intro.md, https://docs.traversal.com/architecture/byoc.md, https://docs.traversal.com/architecture/single-tenant-saas.md).

**What is installed**
"Agent-less by design… no sidecars or background processes" for direct integrations (https://docs.traversal.com/responsible-use/security.md). Two optional components: **Traversal Connector** (open source at github.com/InteractionLabs/traversal-connector, image `traversalext/traversal-connector`, Helm chart `oci://registry-1.docker.io/traversalext/traversal-connector-charts`, Cosign-signed, SBOM; also runnable as a plain container) — a stateless outbound mTLS tunnel used to reach private sources incl. Kubernetes API; and **Traversal Processor** (private image) for log ingestion/redaction at the edge (https://docs.traversal.com/setup/connector.md, https://docs.traversal.com/setup/processor.md). The Connector itself needs no Kubernetes RBAC. For the Kubernetes integration the customer creates a target ServiceAccount with read RBAC plus a separate requester identity allowed only `create` on `serviceaccounts/token` for that named SA (TokenRequest flow), or a static bearer token (https://docs.traversal.com/integrations/kubernetes.md). Docs advise against granting secrets, `pods/exec`, port-forward, write verbs, wildcards, impersonation.

**Cloud identity model**
AWS Account: CloudFormation-created cross-account role, ExternalId `traversal:<org_id>`, attaching AWS managed `AIDevOpsAgentAccessPolicy` — docs warn it is "predominantly investigative, but… not exclusively read-only" (https://docs.traversal.com/integrations/aws-cli.md). CloudWatch: separate role with only CloudWatch/Logs discovery+query actions, `Resource: "*"` (https://docs.traversal.com/integrations/cloudwatch.md). EKS: CloudFormation creates role + EKS Access Entry with AWS managed `AmazonAIOpsAssistantPolicy`; Traversal assumes the role and mints short-lived EKS tokens (https://docs.traversal.com/integrations/kubernetes.md). Azure: Microsoft Entra Workload Identity Federation (secretless) to a customer-owned managed identity/app registration trusted to a Traversal cluster OIDC issuer; roles `Reader` + `Log Analytics Reader`; Bicep template provided (https://docs.traversal.com/integrations/azure-resource-manager.md). STS AssumeRole per request, no stored long-lived keys.

**Network path**
Direct SaaS→customer integrations originate from `50.112.45.78`, `35.162.155.1`, `52.43.83.98`. Connector is outbound-only to `edge.traversal.com` (`44.254.181.73`, `16.148.183.36`, `54.71.194.238`) and `telemetry.traversal.com` (`184.33.54.68`, `44.230.130.223`, `100.22.80.68`, port 4317); Processor to `ingest.traversal.com` (`52.13.31.181`, `16.146.208.114`, `44.241.232.92`). Only listening port is a local health check; HTTP CONNECT proxy supported; PrivateLink optional (https://docs.traversal.com/responsible-use/security.md, https://docs.traversal.com/architecture/connector.md). BYOC publishes a full egress domain list (auth0, rudderstack, pagerduty, tailscale, okta, github, docker, pypi, anthropic/openai) and requires a /20 VPC CIDR (https://docs.traversal.com/architecture/byoc.md).

**How actions execute**
Platform → Edge Controller → mTLS tunnel → Connector → HTTPS to internal service; Connector "executes HTTP requests" only, stores/logs nothing (https://docs.traversal.com/architecture/connector.md). Kubernetes: Traversal's agent passes credentials to "one bare, recognized read-only `kubectl` command"; rejects mutating verbs, watch/stream, plugins, wrappers, pipes, redirects, shell chains, credential/destination overrides (https://docs.traversal.com/integrations/kubernetes.md). AWS Account integration runs AWS CLI read commands with STS creds. Code: may check out a repo in an "ephemeral sandbox"; writes to repos rejected by default (https://docs.traversal.com/responsible-use/data-privacy.md). No VM/SSH execution documented.

**Approval / autonomy posture**
"Read-only by default… cannot modify your systems or data unless you explicitly authorize it" (https://docs.traversal.com/responsible-use/security.md). Workers (proactive alert/incident agents) "read, reason, explain, and recommend… never modifies your alerting, paging, or routing, and it does not take destructive action" (https://docs.traversal.com/using-traversal/workers.md, https://docs.traversal.com/using-traversal/incident-workers.md). No approval workflow for infra actions documented because no infra write actions are offered.

**Tenant isolation and data handling**
Multi-tenant SaaS uses logical isolation; single-tenant/BYOC dedicated infra and keys; AES-256 / TLS 1.3; zero-data-retention agreements with LLM providers; BYOC can route LLMs via Bedrock or a private gateway; customer-configured regex redaction at the Connector (TOML rules, hot-reload, fails closed on invalid rules) and Processor (rules pushed by Traversal); SOC 2 Type II, GDPR/HIPAA aligned, EU/UK GDPR representatives named (https://docs.traversal.com/responsible-use/data-privacy.md, https://docs.traversal.com/setup/redaction.md).

**Doc URLs used**
https://docs.traversal.com/architecture/intro.md · https://docs.traversal.com/architecture/connector.md · https://docs.traversal.com/architecture/single-tenant-saas.md · https://docs.traversal.com/architecture/byoc.md · https://docs.traversal.com/setup/connector.md · https://docs.traversal.com/setup/processor.md · https://docs.traversal.com/setup/redaction.md · https://docs.traversal.com/integrations/kubernetes.md · https://docs.traversal.com/integrations/aws-cli.md · https://docs.traversal.com/integrations/cloudwatch.md · https://docs.traversal.com/integrations/azure-resource-manager.md · https://docs.traversal.com/responsible-use/security.md · https://docs.traversal.com/responsible-use/data-privacy.md · https://docs.traversal.com/using-traversal/workers.md · https://docs.traversal.com/using-traversal/incident-workers.md

**Not published**
A sample read ClusterRole for the target SA (customer writes it); GCP integration details (not in the fetched index); how the SaaS-side kubectl allowlist is technically enforced; retention period for investigation data; where the ephemeral code sandbox runs in multi-tenant SaaS.

---

## 3. Cleric

**Deployment form**
SaaS only ("Is Cleric available for self-hosting? No."); single-tenant: dedicated frontend/backend instance, dedicated PostgreSQL, customer domain `<company>.app.cleric.ai`, GCP `us-east4` default, EU `europe-west1` option (https://docs.cleric.ai/reference/faq.md, https://docs.cleric.ai/security/security-data-privacy.md).

**What is installed**
Nothing by default: "No agents, sidecars, or daemons run in your clusters or cloud accounts." Optional **Connector** (VM or K8s) acting as a network proxy for private endpoints; deployment is "guided by the Cleric team" — no public chart or manifest (https://docs.cleric.ai/reference/faq.md, https://docs.cleric.ai/integrations/private-resources.md). Kubernetes RBAC: customer applies the published ClusterRole `cleric-role`: `get/list/watch` on core resources incl. `pods/log`, `resources: ["*"]` in ~50 listed API groups (apps, batch, rbac, policy, argoproj.io, cert-manager, istio, keda, flux, external-secrets, plus GKE/EKS/AKS add-on groups), `list` only on secrets (no `get`), `create` on `selfsubjectaccessreviews`/`selfsubjectrulesreviews`; bound to SA `cleric-sa` in namespace `cleric` with a long-lived `kubernetes.io/service-account-token` Secret (https://docs.cleric.ai/integrations/supported-integrations.md#kubernetes).

**Cloud identity model**
AWS: STS AssumeRole into a customer role with Cleric's trust policy; a "validation policy" (`iam:GetRole`, `iam:ListAttachedRolePolicies`, `iam:ListRolePolicies`, `iam:GetRolePolicy`, `iam:GetPolicy`, `iam:GetPolicyVersion`) lets Cleric verify at save time that the role grants no write permissions; recommended managed policies `CloudWatchReadOnlyAccess`, `CloudWatchLogsReadOnlyAccess`, `ElasticLoadBalancingReadOnly`, `AmazonEC2ReadOnlyAccess`, `AWSCloudTrail_ReadOnlyAccess`, plus customer policies `ecs:Describe*/List*`, `eks:Describe*/List*` (https://docs.cleric.ai/integrations/supported-integrations.md#amazon-web-services). EKS: the same IAM role mapped to group `cleric` via EKS Access Entry or aws-auth, bound to `cleric-role` (https://docs.cleric.ai/integrations/supported-integrations.md#elastic-kubernetes-service). GitHub: GitHub App with Contents/Issues/Pull requests read-write, Actions/Checks/Metadata read, short-lived installation tokens (https://docs.cleric.ai/integrations/supported-integrations.md#github). SaaS tools: API keys. GCP: documented on the same page (not extracted in detail).

**Network path**
Cleric calls customer endpoints directly from its hosted instance; "dedicated static egress IP per instance for allowlisting"; for public K8s control planes customers can request the CIDR to allowlist; private resources via the outbound Connector ("No public IPs, VPN appliances, or special firewall rules required… end-to-end encrypted") (https://docs.cleric.ai/security/security-data-privacy.md, https://docs.cleric.ai/integrations/supported-integrations.md#kubernetes, https://docs.cleric.ai/integrations/private-resources.md).

**How actions execute**
"Cleric uses standard CLIs and APIs (`kubectl`, `gh`, `aws`, etc.) to query your infrastructure" from its hosted instance; per-investigation "ephemeral sandbox" for GitHub work using `gh` with a short-lived installation token (https://docs.cleric.ai/integrations/overview.md, https://docs.cleric.ai/integrations/supported-integrations.md#github). No infrastructure writes; no VM/SSH execution documented.

**Approval / autonomy posture**
"Cleric does not make changes to customer infrastructure but may propose them. All remediation actions are performed by your team." Writes limited to proposing: PR/branch/issue creation on GitHub, Jira issue creation if the Atlassian user is granted "Create Issues". Guardrail is instructional plus recommended repo controls: "Cleric is instructed that it cannot merge PRs or push directly to your default branch… We recommend configuring branch protection rules" (https://docs.cleric.ai/security/security-data-privacy.md, https://docs.cleric.ai/reference/faq.md). No in-product approval gate is described; the AWS integration refuses roles that carry write permissions at save time.

**Tenant isolation and data handling**
Single-tenant compute, network boundary and DB; raw logs/metrics/code queried at runtime and not persisted; stores queries, reasoning traces, evidence extracts, responses; system logs 30 d, application logs 120 d (configurable), deletion on request within 2 business days; per-tenant memory never shared; one-way execution telemetry to a central US profiler; LLMs Anthropic/Gemini/OpenAI under ZDR enterprise terms; EU hosting still sends operational logs/traces/analytics to the US; SOC 2 Type II, annual pentests, AES-256, TLS 1.2+, no SSH to prod (https://docs.cleric.ai/security/security-data-privacy.md, https://docs.cleric.ai/security/eu-hosting-data-processing.md).

**Doc URLs used**
https://docs.cleric.ai/meet-cleric/how-cleric-works.md · https://docs.cleric.ai/integrations/overview.md · https://docs.cleric.ai/integrations/supported-integrations.md · https://docs.cleric.ai/integrations/private-resources.md · https://docs.cleric.ai/security/security-data-privacy.md · https://docs.cleric.ai/security/eu-hosting-data-processing.md · https://docs.cleric.ai/reference/faq.md

**Not published**
Connector deployment artifacts, image, RBAC or exact egress endpoints (vendor-guided only); the egress IP range (assigned per customer, on request); any in-product approval step for PR/issue creation (none described — instruction-based); token rotation guidance for the long-lived Kubernetes SA token the docs prescribe.

---

## 4a. Parity (tryparity.com)

**Deployment form**
Hosted SaaS (`remediator.api.tryparity.com`); "you can self-host the entire Parity platform and bring your own model providers" on request to founders (https://docs.tryparity.com/security.md).

**What is installed**
`remediator` pod via Helm chart `parity/remediator` from `https://parity-ai.github.io/helm-charts`, image `parityai/remediator:latest` (https://docs.tryparity.com/quickstart.md). From the public chart source: ClusterRole on `apiGroups: ["*"], resources: ["*"]` with `verbs: ["get","list","watch"]` when `parity.readonly=true` (default), or `verbs: ["*"]` when `--set parity.readonly=false` — i.e. cluster-admin-equivalent in write mode (https://github.com/parity-ai/helm-charts/blob/main/remediator/templates/clusterrole.yaml, https://github.com/parity-ai/helm-charts/blob/main/remediator/values.yaml). The Parity API key is passed as a plain env var `API_KEY` rendered from Helm values, not from a Secret (https://github.com/parity-ai/helm-charts/blob/main/remediator/templates/deployment.yaml).

**Cloud identity model**
Kubernetes ServiceAccount + ClusterRoleBinding only; Parity API key for the remediator→Parity channel. No AWS/Azure/GCP IAM integration documented (https://docs.tryparity.com/quickstart.md).

**Network path**
"Egress-only access strictly to our API" over TLS to `remediator.api.tryparity.com:443`; optional NetworkPolicy (`egress.restricted: true`) pins egress to `34.168.135.128/32` (https://docs.tryparity.com/quickstart.md, https://github.com/parity-ai/helm-charts/blob/main/remediator/templates/networkpolicy.yaml).

**How actions execute**
"All access to your clusters are controlled via the remediator service. This means there is no way for an LLM to execute dangerous or arbitrary commands on your cluster as the access is controlled via RBAC on the remediator" — the in-cluster pod performs Kubernetes API calls on behalf of the hosted agent (https://docs.tryparity.com/security.md). No VM-level execution documented.

**Approval / autonomy posture**
Read-only by default; write mode is a single Helm flag (`parity.readonly=false`). No approval workflow, action levels, or audit trail is documented (https://docs.tryparity.com/quickstart.md, https://docs.tryparity.com/security.md).

**Tenant isolation and data handling**
Only: TLS between remediator and Parity's hosted services; "our underlying model providers do not train on data from our users"; self-hosting available for strict requirements (https://docs.tryparity.com/security.md). No SOC 2, encryption-at-rest, retention or tenant-isolation statements.

**Doc URLs used**
https://docs.tryparity.com/quickstart.md · https://docs.tryparity.com/security.md · https://docs.tryparity.com/llms.txt · https://github.com/parity-ai/helm-charts (README, `remediator/templates/clusterrole.yaml`, `networkpolicy.yaml`, `deployment.yaml`, `values.yaml`)

**Not published**
Any approval/human-in-the-loop model for write mode; what actions the agent performs in write mode; audit logging; compliance certifications; tenant isolation; data retention; cloud-provider access. The marketing site (tryparity.com / www.tryparity.com) could not be fetched (redirect loop / no response), so only the three doc pages plus the chart repo exist as sources. The docs site has just three pages (Quickstart, Slack, Security).

---

## 4b. Metoro

**Deployment form**
Managed Cloud (`us-east.metoro.io`), BYOC (Metoro-managed in your cloud, on request), and On-Prem (`metoro-onprem` hub Helm chart; marketed as "airgapped or not, complete isolation") (https://metoro.io/docs/getting-started/getting-started.md, https://metoro.io/docs/on-premises/overview.md).

**What is installed**
Per monitored cluster: **Node Agent** DaemonSet (eBPF; `hostPID: true`, `privileged: true`, hostPath mounts for `/sys/fs/cgroup`, `/sys/kernel/tracing`, `/sys/kernel/debug`, `/sys`, containerd/docker/crio/podman sockets, `/var/lib/kubelet`, `/var/lib/containerd`, `/var/lib/docker`, `/var/lib/containers`), **Exporter** Deployment with HPA (min 2 replicas), **Redis** (sentinel), optional OTel Collector + Target Allocator for ServiceMonitor scraping (https://metoro.io/docs/on-premises/11.x/architecture/metoro-exporter/components/node-agent.md, https://metoro.io/docs/on-premises/11.x/installation/connect-monitored-clusters.md, https://github.com/metoro-io/metoro-helm-charts/blob/main/charts/metoro-exporter/templates/node_agent_daemonset.yaml). Exporter ClusterRole `metoro-exporter-cluster-role`: `get/list/watch` on pods, nodes, services, endpoints, namespaces, configmaps, events, serviceaccounts, PV/PVC, apps/*, batch/*, HPAs, ingresses, networkpolicies, monitoring.coreos.com/*, Metoro CRDs; `get/list/create/update/delete` on `coordination.k8s.io/leases` (leader election); opt-in `get` on secrets via per-namespace Roles by default or a global ClusterRole rule (https://github.com/metoro-io/metoro-helm-charts/blob/main/charts/metoro-exporter/templates/exporter_service_account.yaml, https://github.com/metoro-io/metoro-helm-charts/blob/main/charts/metoro-exporter/templates/exporter_secret_reader_rbac.yaml). Not supported on Fargate-style nodes or GKE Autopilot. On-prem AI additionally deploys an Agent Sandbox controller, sandbox router and Guardian runner `SandboxTemplate` in the hub namespace (https://metoro.io/docs/on-premises/11.x/installation/metoro-ai.md).

**Cloud identity model**
Exporter authenticates to the hub with an environment-specific bearer token held in a K8s Secret (`metoro-exporter-token`, generated in Settings → Data Ingestion) (https://metoro.io/docs/on-premises/11.x/installation/connect-monitored-clusters.md). GitHub via GitHub App: repository contents/metadata read, pull requests/issues write (https://metoro.io/docs/integrations/github.md). Optional customer Bedrock credentials (cloud) or any OpenAI-compatible endpoint with API key in a Secret (on-prem) for the model (https://metoro.io/docs/ai-sre/overview.md, https://metoro.io/docs/on-premises/11.x/installation/metoro-ai.md). No AWS/Azure/GCP cloud-API integration (IAM role, managed identity, SA) is documented — Metoro is Kubernetes-telemetry-centric.

**Network path**
Exporter sends telemetry outbound to the hub ingester and reads configuration from the hub apiserver; node agents talk only to the in-cluster exporter and Redis; troubleshooting notes reference outbound network policy and private-CA ConfigMaps (https://metoro.io/docs/on-premises/11.x/architecture/metoro-exporter/components/metoro-exporter.md, https://metoro.io/docs/on-premises/11.x/installation/connect-monitored-clusters.md). No IP allowlist or endpoint list published for Managed Cloud.

**How actions execute**
Guardian reasons over telemetry already ingested into the Metoro backend (ClickHouse/Postgres/Temporal), not by calling into the cluster at investigation time (https://metoro.io/docs/getting-started/architecture.md). Code fixes: the Guardian runner checks out repos in a temporary workspace inside an isolated Docker/agent-sandbox runner (non-root, capabilities dropped, unprivileged Linux namespaces, file writes confined, shell network blocked); GitHub installation credentials stay on the Metoro server; the runner has fetch-only repo access and "no publication tool" (https://metoro.io/docs/ai-sre/github-drafts.md). A webhook event "Agent action approval requested — sent when Guardian requests approval to take an action" exists, implying an action-approval mechanism (https://metoro.io/docs/api-reference/webhooks/agent-action-approval-requested.md). No VM/SSH or `kubectl` write execution documented.

**Approval / autonomy posture**
AI SRE features (Guardian AI, AI Deployment Verification, Autonomous Issue Detection) are opt-in toggles under Settings → Features (https://metoro.io/docs/ai-sre/overview.md). PRs: "Metoro can create a pull request… but your team reviews, approves, and merges it" / "will never merge them automatically"; chat drafts are published by the user with the user's own permissions and carry a "Checks not run" label (https://metoro.io/docs/ai-sre/github-integration.md, https://metoro.io/docs/ai-sre/github-drafts.md). Beyond the webhook name, there is no documentation of which infrastructure actions Guardian can request approval for or how approval is granted.

**Tenant isolation and data handling**
Trace redaction via re2 regex rules before storage (https://metoro.io/docs/ingestion-settings/trace-redaction.md). On-prem: model API key in a K8s Secret; stateless Responses API mode for Zero-Data-Retention gateways (https://metoro.io/docs/on-premises/11.x/installation/metoro-ai.md). BYO Bedrock keys for "complete control over AI processing and billing" (https://metoro.io/docs/ai-sre/overview.md). No SOC 2, encryption, retention or multi-tenant isolation statements were found in the docs; `metoro.io/security`, `metoro.io/trust` and `trust.metoro.io` do not exist (404 / no DNS), and a web search found no Metoro trust center.

**Doc URLs used**
https://metoro.io/docs/getting-started/getting-started.md · https://metoro.io/docs/getting-started/requirements.md · https://metoro.io/docs/getting-started/architecture.md · https://metoro.io/docs/ai-sre/overview.md · https://metoro.io/docs/ai-sre/github-integration.md · https://metoro.io/docs/ai-sre/github-drafts.md · https://metoro.io/docs/integrations/github.md · https://metoro.io/docs/on-premises/overview.md · https://metoro.io/docs/on-premises/11.x/installation/connect-monitored-clusters.md · https://metoro.io/docs/on-premises/11.x/installation/metoro-ai.md · https://metoro.io/docs/on-premises/11.x/architecture/metoro-exporter/components/node-agent.md · https://metoro.io/docs/on-premises/11.x/architecture/metoro-exporter/components/metoro-exporter.md · https://metoro.io/docs/ingestion-settings/trace-redaction.md · https://metoro.io/docs/api-reference/webhooks/agent-action-approval-requested.md · https://github.com/metoro-io/metoro-helm-charts

**Not published**
Any compliance certification or trust center; Managed Cloud tenant isolation, encryption, retention; egress IPs/endpoints for Managed Cloud; what "agent actions" exist and the approval UX behind the webhook; cloud-provider (AWS/Azure/GCP) API access model; BYOC architecture (contact sales only).

---

## Cross-vendor observations (facts only)

- **Only Parity** ships a component whose RBAC can be flipped to cluster-wide `verbs: ["*"]`; every other vendor's in-cluster footprint is read-only with no write path to Kubernetes.
- **Metoro** is the only one requiring a privileged, hostPID DaemonSet (eBPF collector); the others' in-cluster components are unprivileged proxies/pollers (Resolve documents a non-root, drop-ALL securityContext).
- **Resolve** is the only vendor with a documented infra-side write action (alert silences, GitHub Actions dispatch) and a formal proposal → human approve → separate execution engine model with an audit dashboard.
- **Traversal** and **Cleric** both run `kubectl`/`aws`/`gh` CLIs inside the vendor's hosted environment using customer-granted credentials (Traversal through its Connector tunnel; Cleric directly or through a vendor-guided Connector). Traversal is the only one that documents a kubectl command allowlist/denylist.
- Published IP allowlists: Resolve (`18.97.138.16/29`), Traversal (full per-endpoint lists), Parity (`34.168.135.128/32` in chart). Cleric: per-instance static IP on request. Metoro: none.
- Cloud IAM: Resolve and Cleric use AWS managed read-only policies; Traversal uses AWS's `AIDevOpsAgentAccessPolicy` / `AmazonAIOpsAssistantPolicy` and flags that the former is not strictly read-only. Traversal is the only one with secretless Azure WIF; Resolve the only one with GCP WIF from a Kubernetes SA.
- Deployment breadth: Traversal (SaaS / single-tenant / BYOC) and Metoro (cloud / BYOC / on-prem) are the only ones offering customer-cloud control planes; Cleric is SaaS-only; Resolve is SaaS with a shared/dedicated tier; Parity offers self-hosting on request.
- None documents VM-level command execution (SSH/SSM) in any form.

Local copies of every fetched page: `/tmp/claude-1000/-home-asithaw-work-asitha-Warden/0f37e04a-e59e-4eb3-b589-ac5ca923685d/scratchpad/fetch/`.
