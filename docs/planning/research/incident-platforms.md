# Research report: how PagerDuty, incident.io, Rootly and FireHydrant reach into customer infrastructure

Vendor docs only, fetched 2026-10-31.

Method note: every fact below comes from the vendor's own docs/trust pages fetched directly; marketing claims are labelled as such. ServiceNow docs are JS-rendered and could not be fetched as text — see §4. Blameless: www.blameless.com returns HTTP 302 → https://firehydrant.com/, so "Blameless" is covered under FireHydrant; nothing ServiceNow-related was found for Blameless.

---

## 1. PAGERDUTY — Paige (SRE Agent), PD Automation Runner (EA), Runbook Automation (Rundeck) Enterprise Runner

### Deployment form
- Paige is SaaS only (web app, Slack, MS Teams EA); PD has US and EU service regions (OAuth callback hosts api.pagerduty.com / api.eu.pagerduty.com; `--set runner.region=eu`) (https://docs.pagerduty.com/ai-automation/advance/connectors-tools-and-skills.md, https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner/pd-runner-installation.md).
- Runbook Automation ships as a "fully managed SaaS" ("dedicated Rundeck" per customer, "hosted in several AWS US regions") and as Runbook Automation Self-Hosted; both use the Enterprise Runner for remote execution (https://docs.rundeck.com/docs/about/cloud/). PD Automation Runner is Early Access, PD Reliability Platform accounts only (https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner.md).

### What is installed in the customer environment
- Paige itself: nothing; it reaches tools via SaaS API/MCP connectors (https://docs.pagerduty.com/ai-automation/advance/connectors-tools-and-skills.md).
- PD Automation Runner: Helm chart `oci://ghcr.io/pagerduty/pagerduty-automation-runner-helm-charts/pd-runner` (or K8s YAML / Docker run / Compose), namespace `pagerduty-automation-runner`, ServiceAccount `pagerduty-automation-runner`; credentials RUNNER_ID, RUNNER_SECRET (shown once), RUNNER_PDTOKEN (a PD REST API key with read/write). The chart creates NO RBAC; docs publish a ClusterRole: core `configmaps, namespaces, nodes, persistentvolumes, persistentvolumeclaims, secrets, services` [get,list]; `pods` [get,list,create,delete]; `pods/log` [get]; `pods/exec` [create]; apps `daemonsets, deployments, replicasets, statefulsets` [get,list]; batch `cronjobs, jobs` [get,list]; `ingresses` [get,list]; `poddisruptionbudgets` [list]; `storageclasses` [get,list]; `customresourcedefinitions` [list]. Docs warn Secrets read is cluster-wide with a ClusterRoleBinding and suggest RoleBindings per namespace (https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner/pd-runner-installation.md).
- Enterprise Runner (Rundeck): Java program (Java 17 JRE, 164 MB JAR), systemd/Windows service or Docker image `rundeckpro/runner`; configured with RUNNER_RUNDECK_CLIENT_ID / RUNNER_RUNDECK_SERVER_TOKEN / RUNNER_RUNDECK_SERVER_URL; "Runner Replicas" for HA; bundles remote plugins (Ansible, Kubernetes, WinRM…) (https://docs.rundeck.com/docs/administration/runner/runner-installation/creating-runners.html, https://docs.rundeck.com/docs/administration/runner/runner-installation/runner-install.html, https://docs.rundeck.com/docs/administration/runner/concepts.html).

### Cloud identity model
- Incident Workflows AWS connection: PD's AWS account assumes a customer IAM role via `sts:AssumeRole` with External ID (trust principal `arn:aws:iam::<PD account>:root`); example attaches `ReadOnlyAccess`; per-action permission table (https://docs.pagerduty.com/ai-automation/automation/incident-workflows/workflow-integrations.md). The "AWS: Invoke an SSM Command" action needs `ssm:SendCommand` scoped to the `AWS-RunShellScript` document and the instance, plus `ssm:GetCommandInvocation`; the instance needs `AmazonSSMManagedInstanceCore` (https://docs.pagerduty.com/workflow-actions/aws/aws-invoke-an-ssm-command.md).
- Runbook Automation AWS: SaaS → IAM role with trust relationship + External ID (system- or project-level); self-hosted on EC2 → instance role; ECS → task role; access key/secret "least recommended". SSM node executor policy: `ssm:SendCommand` on `AWS-RunShellScript`, `AWS-RunPowerShellScript`, `AWS-RunRemoteScript` docs + target instances; `ssm:ListCommands`, `ssm:ListCommandInvocations`, `ssm:GetCommandInvocation`, `logs:GetLogEvents`; `s3:PutObject` for RBA and `s3:GetObject`/`s3:ListBucket` for instances on a staging bucket (https://docs.rundeck.com/docs/manual/plugins/aws-plugins-overview.html, https://docs.rundeck.com/docs/manual/projects/node-execution/aws-ssm.html).
- Kubernetes: PD Automation Runner authenticates with the pod's own ServiceAccount token, so it must run in-cluster (https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner.md). Rundeck Kubernetes plugins (Create/Delete/Describe/List/Update Object, Object Logs, Run Command, Run Script, EKS/AKS/GKE node sources) are listed but the overview page does not state the auth model (https://docs.rundeck.com/docs/manual/plugins/kubernetes-plugins-overview.html).
- Paige connectors: API keys or OAuth; GitHub via PagerDuty GitHub App with Contents Read&Write, Pull requests Read&Write, Metadata Read (to open revert PRs); Atlassian/Notion via OAuth 2.1 PKCE read-only MCP (https://docs.pagerduty.com/integrations/github-cloud-connector.md, https://docs.pagerduty.com/ai-automation/advance/connectors-tools-and-skills.md).

### Network path
- Enterprise Runner: outbound-only HTTPS to `<subdomain>.runbook.pagerduty.cloud` on TCP 443, polls every 5 s, HTTP proxy supported; "no need to open additional ports". IP allowlisting: no static list — "IP addresses are unlikely to change, but may… query the A records for your subdomain using dig or nslookup". SaaS disables local command execution on the instance (https://docs.rundeck.com/docs/administration/runner/runner-overview.html, https://docs.rundeck.com/docs/about/cloud/, https://docs.rundeck.com/docs/administration/runner/runner-config.html).
- PD Automation Runner: "All communication is outbound HTTPS from the runner to the PagerDuty API"; no inbound endpoints (https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner.md).

### How actions execute
- Runbook Automation: server queues tasks (job execution, node discovery, secrets retrieval, command execution); Runner replicas poll and execute plugins locally or dispatch to nodes. Node execution is SSH by default (key/password/sudo-password from Key Storage), WinRM, AWS SSM (RBA → SSM service → SSM agent pulls; scripts via S3), ECS exec, Ansible (https://docs.rundeck.com/docs/administration/runner/concepts.html, https://docs.rundeck.com/docs/manual/projects/node-execution/ssh.html, https://docs.rundeck.com/docs/manual/projects/node-execution/aws-ssm.html).
- PD Automation Runner: PD delivers the job over the runner's outbound connection; six K8s actions incl. "Run Pod Command" (= `kubectl exec`, arbitrary shell command, chaining allowed), Create Pod, Delete Pod; 10-min default timeout (https://docs.pagerduty.com/ai-automation/automation/pd-automation-runner/kubernetes-workflow-actions.md).
- Incident Workflows SSM action: PD SaaS assumes the role and calls SSM directly; only three pre-approved operations (Collect Logs, Restart Services, Run Diagnostics); "does not accept free-text or arbitrary shell commands"; Linux only (https://docs.pagerduty.com/workflow-actions/aws/aws-invoke-an-ssm-command.md).
- Paige: retrieves logs/metrics/runbooks/code; its listed PD capabilities are get_incident_details, list_incidents, add_incident_note, get_service_details, get_related_services; connector tools are grouped "Read Only Tools" vs "Write/delete Tools", each set to Always Allow or Blocked; "Paige uses only the allowed tools, automatically or via user request" (https://docs.pagerduty.com/ai-automation/advance/sre-agent.md, https://docs.pagerduty.com/ai-automation/advance/connectors-tools-and-skills.md). Paige recommends an Incident Workflow; a human clicks "Run recommended workflow"; once per incident; logged in Timeline (https://docs.pagerduty.com/ai-automation/advance/paige-recommended-workflows.md).

### Approval / autonomy posture
- GA Paige "recommend[s] diagnostic and remediation steps"; it can be auto-triggered as a virtual responder via Incident Workflow or Escalation Policy (EA) and investigates in parallel with humans (https://support.pagerduty.com/docs/sre-agent, https://support.pagerduty.com/docs/trigger-sre-agent-via-escalation-policy).
- SRE Workbench (PD Labs): remediation appears as an inline card; "may let you approve it with one click, or ask you to confirm you have reviewed the relevant evidence first"; approve / deny / ask for alternative; Paige confirms outcome afterwards; role-based permissions "not yet available" (https://docs.pagerduty.com/ai-automation/pd-labs/workbench-user-guide.md).
- GenAI guidelines: AI features are opt-in and can be disabled at any time (https://pagerduty.com/security/generative-ai-guidelines).

### Tenant isolation and data handling
- Paige memory artifacts are scoped per PD service; Memory API lets customers view/redact; only first 2,000 chars of custom_details/notes analysed (https://support.pagerduty.com/docs/sre-agent).
- GenAI guidelines: customer data used only for that customer's functionality, not to train models benefiting others; training uses artificial datasets; GenAI excluded from FedRAMP boundary until approved (https://pagerduty.com/security/generative-ai-guidelines). Full "PagerDuty Advance AI Disclosure" is gated at assurance.pagerduty.com (https://www.pagerduty.com/security/).
- Runbook Automation SaaS: secrets retrieval can be delegated to the Runner for self-hosted secret stores (https://docs.rundeck.com/docs/about/cloud/).

### Doc URLs used
Listed inline above plus https://docs.rundeck.com/docs/learning/getting-started/runners-overview.html, https://support.pagerduty.com/docs/pagerduty-automation-trial, https://docs.pagerduty.com/ai-automation/pd-labs/workbench-overview.md.

### Not published
Paige's egress IPs; which LLM providers Paige uses; Paige tenant-isolation architecture; whether Paige can invoke Runbook Automation / PD Automation Runner actions directly (only via human-run workflows is documented); approval levels for GA Paige (only the Labs Workbench describes them); Rundeck Kubernetes plugin credential model; a static IP allowlist for Runbook Automation.

---

## 2. INCIDENT.IO — Investigations / Nexus

### Deployment form
- SaaS only, on GCP; transactional data in europe-west1 with hot standby in europe-west4, "We currently don't send any data outside of Europe"; whitepaper says EU customers get a dedicated EU region and data residency is configured per tenant at onboarding; customer data is "logically isolated" (https://docs.incident.io/admin/security-faqs.md, https://incident.io/blog/incident-io-security-whitepaper). No BYO model/key ("Not currently") (https://docs.incident.io/investigations/trust-and-safety.md).

### What is installed in the customer environment
- Optional Proxy (UI calls it "Connector"): Docker image `incidentio/connector-proxy`, opens an outbound SSH connection on port 443 to `relay.incident.io`, authenticates with a one-time-shown token, builds a reverse tunnel; `INCIDENT_CONNECTOR_FORWARD_DEST_ALLOWLIST` restricts host:port destinations; sample K8s Deployment runs as UID 65532, drop ALL caps, no RBAC needed for the proxy itself (https://docs.incident.io/integrations/proxy.md).
- Kubernetes data source: read-only; kubeconfig bearer token needing get/list on namespaces, nodes, pods, services, events, endpoints, PV/PVC, apps workloads, batch jobs/cronjobs, ingresses, network policies; ConfigMaps/ServiceAccounts optional; namespace-scoped mode generates a discovery ClusterRole + a read ClusterRole bound by RoleBinding per named namespace (max 250) (https://docs.incident.io/nexus/telemetry/kubernetes.md).

### Cloud identity model
- AWS: recommended OIDC IAM role assumed with `sts:AssumeRoleWithWebIdentity`, trust federates `accounts.google.com` pinned on `accounts.google.com:sub` (incident.io's service account ID) and `accounts.google.com:oaud` = `incident-io-telemetry`; static access keys as alternative. Per-service permissions: CloudWatch `cloudwatch:ListMetrics`, `cloudwatch:GetMetricData`, `logs:DescribeLogGroups`, `logs:StartQuery`, `logs:GetQueryResults`, `logs:StopQuery`; EKS `eks:ListClusters`, `eks:DescribeCluster` + EKS access entry with `AmazonEKSViewPolicy`; OpenSearch `es:ListDomainNames`, `es:DescribeDomains`, `es:ListTags`, `es:ESHttpGet/Post/Head`; RDS `rds:DescribeDBInstances`, `rds:DescribeDBClusters`, `rds:DescribeBlueGreenDeployments`, DB auth via `rds-db:connect` or password (https://docs.incident.io/nexus/telemetry/aws.md).
- GitHub: GitHub App with short-lived installation tokens; Contents and Pull requests Read (read-only) or Write; GitLab: service-account PAT, Reporter + `read_api`,`read_repository` or Developer + `api` (https://docs.incident.io/nexus/code/overview.md).

### Network path
- Egress IPs published: 34.91.219.113, 34.32.171.26, 146.148.114.148, 104.155.67.8, plus for Investigations 34.52.216.178, 34.140.142.231, 35.204.31.148, 35.204.114.23; machine-readable at `GET /v1/ip_ranges`; 2 weeks' notice of changes (https://docs.incident.io/integrations/ip-allowlist.md). Proxy is outbound-only to relay.incident.io:443, no inbound ports. Public EKS endpoints restricted by CIDR are not reachable — use a private endpoint + proxy (https://docs.incident.io/nexus/telemetry/kubernetes.md).

### How actions execute
- incident.io SaaS queries telemetry/cluster/SQL directly or through the proxy tunnel; Kubernetes access "never writes to the cluster"; SQL sources are read-only queries (https://docs.incident.io/nexus/telemetry/kubernetes.md, https://docs.incident.io/nexus/telemetry/overview.md). The only write path is opening a draft pull request; code analysis runs in an isolated container with no inbound and tightly restricted outbound network (https://docs.incident.io/nexus/code/overview.md). Connectors = remote MCP servers or HTTP APIs the customer attaches, with allowed tools (https://docs.incident.io/nexus/extensions/overview.md). No VM-level command execution documented.

### Approval / autonomy posture
- "Investigations don't take action on your behalf without your request or approval"; Investigations default Off; triggers: every incident, conditional, via workflow, or on demand; "Proposing code fixes" unprompted is Off by default and still yields a draft PR a human merges; nothing is auto-merged or deployed; responders can steer/pause (https://docs.incident.io/investigations/trust-and-safety.md, https://docs.incident.io/admin/ai-governance.md).

### Tenant isolation and data handling
- Zero Data Retention agreements with OpenAI, Anthropic, Google Vertex; subprocessors can be opted out (opting out of Vertex disables code analysis); AI data redaction (cards/SSN/phones) on request; AES-256 at rest, TLS in transit; SOC 2 Type II, ISO 27001 in progress; per-data-source "Restricted" query mode and team-scoped management (https://docs.incident.io/admin/ai-governance.md, https://docs.incident.io/admin/ai-usage.md, https://incident.io/blog/incident-io-security-whitepaper, https://docs.incident.io/nexus/telemetry/access-controls.md).

### Doc URLs used
Above plus https://docs.incident.io/investigations/getting-started.md, https://docs.incident.io/investigations/triggering.md.

### Not published
Any dedicated/single-tenant instance offering (only regional residency + logical isolation); IPs of relay.incident.io; whether MCP/HTTP connectors may be write-capable and how tools are allowlisted (connectors page not read); on-prem/BYOC options (none mentioned).

---

## 3. ROOTLY — AI SRE and Private Agent

### Deployment form
- SaaS; LLM inference "runs in US-based infrastructure", no region routing; Private Agent is an Early Preview in-network component for approved customers; marketing security page says "Rootly supports isolated tenants" for InfoSec separation (marketing claim) (https://docs.rootly.com/ai/data-privacy-for-rootly-ai.md, https://docs.rootly.com/private-agent.md, https://rootly.com/security).

### What is installed in the customer environment
- Private Agent: Helm `helm upgrade --install rootly-private-agent rootly/rootly-private-agent --version 0.1.0` from https://rootlyhq.github.io/helm-charts, image `rootlyhub/rootly-private-agent` (cosign-signed, digest-pinned), runs as UID/GID 65532, read-only rootfs, PVC for rotating credentials; one-time enrollment token (24 h); bootstrap mode for ECS/Fargate/Nomad. Local YAML policy: `allowed_namespaces` (empty = deny all), `allow_cluster_scoped: false`, `allow_pod_logs: false` defaults, result/log byte caps. RBAC guidance: `get`,`list`,`watch` on needed resources via Role/RoleBinding per namespace or read-only ClusterRole + per-namespace RoleBindings; `get` on `pods/log` only if logs enabled; `create` on `selfsubjectaccessreviews` and `selfsubjectrulesreviews`; "Do not grant Secrets, workload write verbs, RBAC administration, exec, attach, port-forward, impersonation, or service-account token creation". Uses Kubernetes Go clients, "does not shell out to kubectl"; rejects kubeconfig `exec`/`auth-provider` stanzas (https://docs.rootly.com/private-agent.md, https://docs.rootly.com/private-agent-kubernetes.md). The separate kubewatch-based Kubernetes integration is event ingestion only.

### Cloud identity model
- AWS connector: customer IAM role with External ID via CloudFormation, Terraform module or manual; selecting services → policy limited to Get/List/Describe for those services; all services → AWS managed `ReadOnlyAccess`; EKS option adds `AmazonEKSMCPReadOnlyAccess` + EKS access entry with `AmazonEKSViewPolicy`; Rootly "passes through the AWS MCP tool catalog" and "does not enforce a separate read-only tool-name allowlist" (https://docs.rootly.com/ai/connectors/aws.md).
- Kubernetes (Private Agent): in-cluster ServiceAccount, or mounted kubeconfig with static token/cert for extra clusters (https://docs.rootly.com/private-agent-kubernetes.md). Other connectors: OAuth or API keys per provider; Atlassian, Notion, Linear, Braintrust, Honeycomb, AWS, Cloudflare pass through the provider's full tool catalog, so provider permissions are the boundary (https://docs.rootly.com/ai/connectors/overview.md).

### Network path
- Private Agent: outbound-only gRPC over TLS/HTTP2 to `https://connect.rootly.com` TCP 443, long-lived bidirectional streams, no HTTP polling fallback, "no inbound connection from Rootly to your network", health endpoints local on 8080 (https://docs.rootly.com/private-agent-kubernetes.md). For SaaS connectors reaching IP-restricted EKS endpoints, Rootly's outbound IPs are "listed on the connection modal" (in-app, not in public docs) (https://docs.rootly.com/ai/connectors/aws.md).

### How actions execute
- Rootly control plane queues a typed invocation → agent accepts only a registered capability/version → local policy check → provider calls the local service → bounded result returned. Capability tiers: Safe read (K8s discovery/get/list/watch, self-reviews), Sensitive read (Pod logs, DB diagnostics, HTTP GET/HEAD/OPTIONS, Prometheus/Loki/Tempo/etc.), Write (internal HTTP POST/PUT/PATCH/DELETE, database `execute_sql`). "It does not provide arbitrary script execution"; Kubernetes mutations/exec unsupported. Edge Connector (separate product) runs event-triggered HTTP/script actions (https://docs.rootly.com/private-agent.md). No VM/SSH-level execution documented.

### Approval / autonomy posture
- "No Private Agent capability pauses for interactive approval, including those in the write tier." Responder-started investigations use the initiating user's role (owners/admins all tiers, users safe reads, observers none). Automatic investigations run as the `ai-sre` system actor "without applying an individual user's role" and "have no interactive confirmation step"; account-level automatic investigation is switched on by the Rootly account team; Auto-run rules per alert class; outcome can be "Inconclusive — needs human" (https://docs.rootly.com/private-agent.md, https://docs.rootly.com/ai/ai-sre/running-an-investigation.md, https://docs.rootly.com/ai/data-privacy-for-rootly-ai.md). Note: the marketing page's "every change requires explicit human sign-off" (https://rootly.com/ai-sre, via search snippet) conflicts with the docs' unattended write-tier statement.

### Tenant isolation and data handling
- Default model Claude Sonnet 4.6, fallback GPT-5 via managed gateway; not used to fine-tune; PII scrubbed from prompts; AI session messages deleted after 90 days (no per-tenant retention config); Private Agent transport records app-layer encrypted, eligible for deletion 7 days after last update, but downstream copies in model context/history persist; BYOK "on the roadmap"; within SOC 2 Type II scope; per-tenant token budget (~250k TPM); investigation report visible to anyone who can read the alert/incident (https://docs.rootly.com/ai/data-privacy-for-rootly-ai.md).

### Doc URLs used
Above plus https://docs.rootly.com/ai/ai-settings.md, https://docs.rootly.com/ai/ai-sre/evidence-sources.md.

### Not published
Tenant-isolation architecture beyond the marketing "isolated tenants" line; public egress IP list; on-prem/BYOC (none); GA date/limits for Private Agent; the Helm chart's actual RBAC template contents (docs give guidance; chart exposes `rbac.create`).

---

## 4. FIREHYDRANT (incl. Blameless) and ServiceNow

### FireHydrant
- Deployment form: SaaS; SOC 2 Type II, SAML/SCIM (https://firehydrant.com/security/).
- Installed in customer env: nothing. The Runbook "Script" step only displays a shell script plus a `curl` command for an engineer to run manually and report status back — FireHydrant does not execute it (https://docs.firehydrant.com/docs/runbook-step-script.md).
- Cloud identity model: none documented for AI; AI features are incident summaries, meeting transcripts, suggested related incidents, drafted retrospectives, status-page drafts, Slack triage channels (https://docs.firehydrant.com/docs/ai-powered-incident-management.md, https://docs.firehydrant.com/docs/triage-channels-in-slack.md).
- Network path: not published for AI (no infra connectors).
- How actions execute: AI produces text only; no infrastructure actions; no VM-level execution.
- Approval/autonomy: per-feature toggles under a global "All AI functionality" switch; outputs are drafts/suggestions (https://docs.firehydrant.com/docs/ai-settings.md).
- Tenant/data: "We only send data in the prompt"; never used to train FireHydrant's or upstream models; providers OpenAI, Anthropic, Recall.ai, AssemblyAI; "tenancy at the lowest level"; users see AI content only for incidents they can access (https://docs.firehydrant.com/docs/firehydrant-ai-security-practices.md).
- Blameless: www.blameless.com 302-redirects to firehydrant.com; no Blameless-specific agentic docs exist.
- Not published: any agentic operations / infrastructure access model — FireHydrant does not offer one.

### ServiceNow
- Primary doc pages (servicenow.com/docs) are JavaScript-only and returned no text to fetch, so nothing below is verified against page text. Search snippets attributed to servicenow.com docs say: Now Assist for ITOM has a "Remediation Agent" that works with the AIOps AI Specialist "to automatically identify and execute the appropriate remediation subflow for an alert"; standalone ITOM AIOps agents "run autonomously on a schedule or in response to a trigger"; MID Server Guardian "offers to initiate" remediation such as re-keying MID Servers or restarting services (https://www.servicenow.com/docs/r/C17ZqvSaMwUVpMBg8Nc5sg/wRwnLBwZMxEU0ZAenjfqPA, https://www.servicenow.com/docs/r/zurich/servicenow-platform/mid-guardian.html, https://www.servicenow.com/docs/r/it-operations-management/explore-ai-in-itom-aiops.html). Deployment form, identity model, network path, approval defaults and tenant statements for these agents: not obtained from primary sources.

---

## Cross-vendor takeaways
- In-network component: PD (Enterprise Runner, Java/Docker; PD Automation Runner, Helm, EA), Rootly (Private Agent, Helm, early preview), incident.io (proxy, Docker, tunnel only). FireHydrant: none.
- All in-network components are outbound-only on 443 (HTTPS polling for Rundeck; HTTPS for PD runner; gRPC/HTTP2 for Rootly; SSH-over-443 for incident.io). Only incident.io publishes a static egress IP list.
- VM-level execution exists only in PagerDuty (SSH/WinRM/SSM via Rundeck; pre-approved SSM ops via Workflows; kubectl exec via PD Automation Runner). Rootly and incident.io are read-only against Kubernetes; Rootly's only writes are allowlisted internal HTTP methods and SQL; incident.io's only write is a draft PR.
- Autonomy: incident.io is recommendation-first with no infra writes; Rootly runs unattended investigations (incl. write-tier) with no approval pause; PagerDuty's human-approval UX is documented only in the PD Labs SRE Workbench, while workflows run on a human click.
