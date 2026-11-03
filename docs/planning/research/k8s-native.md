# How SaaS "AI SRE" products reach customer infrastructure — vendor-published facts only

Date of research: 2026-10-31. Sources are the vendors' own docs, Helm charts/GitHub, and official blogs, plus NVIDIA's docs for Shoreline (now part of NVIDIA). No third-party comparison sites were used.

**Access caveats:** `docs.kubiya.ai`, `kubiya.ai` and `docs.shoreline.io` did not resolve (DNS) from the research environment, and `help.komodor.com` / `komodor.com` returned HTTP 403. For those, facts come from search-engine snippets of the vendors' own pages (marked "via snippet") and from the vendors' GitHub repositories. `shoreline.io` now 301-redirects to nvidia.com, so Shoreline marketing pages are indexed copies only. `docs.komodor.com` (KAOP docs) was read directly.

---

## 1. Komodor (Klaudia AI, `komodor-agent` Helm chart, KAOP)

Komodor now has two surfaces: the classic Kubernetes platform agent (`komodor-agent` chart, used by Klaudia) and the newer Komodor Agentic Operations Platform (KAOP, docs.komodor.com), which runs catalog agents including Klaudia-driven remediation.

### Deployment form
- Classic agent: SaaS control plane; chart value `site: "us" | "eu"` selects the Komodor-hosted region (https://raw.githubusercontent.com/komodorio/helm-charts/master/charts/komodor-agent/values.yaml).
- KAOP: agents can be Komodor-hosted or self-hosted; the control plane is Komodor-hosted, with a self-hosted control plane (umbrella Helm chart) available "under agreement", marked Beta (https://docs.komodor.com/get-started/deployment-methods).

### What is installed in the customer environment
- `komodor-agent` chart components: watcher (Deployment), metrics (Telegraf), node-enricher DaemonSet, OpenTelemetry collector, admission controller; optional kubectl-proxy and GPU DaemonSet (https://github.com/komodorio/helm-charts/tree/master/charts/komodor-agent/templates).
- Watcher ClusterRole (`clusterrole-k8s-watcher.yaml`), chart defaults:
  - `allowedResources.allowReadAll=true` → `apiGroups:["*"] resources:["*"] verbs:[get,list,watch]`.
  - `capabilities.actions=true` ("Allow users to perform actions on the cluster, granular access control is defined in the application") → `patch` on deployments/statefulsets/replicasets/daemonsets and `deployments/scale`, `statefulsets/scale`; `create/update/patch/delete` on pods, persistentvolumeclaims, configmaps, services, persistentvolumes, storageclasses, jobs, cronjobs, ingresses, networkpolicies, horizontalpodautoscalers; `patch` on nodes; `create` on `pods/exec`, `pods/portforward`, `pods/eviction`.
  - `capabilities.crActions=true` → `apiGroups:["*"] resources:["*"] verbs:[get,list,watch,patch,delete]` (cluster-wide wildcard patch/delete).
  - `capabilities.helm.enabled=true`, `readonly=false` → secrets `get/list/watch/create/update/delete` plus `apiGroups:["*"] resources:["*"] verbs:[delete,list,get]` "Allow to uninstall charts".
  - `capabilities.rbac=true` → `verbs:["*"]` on serviceaccounts, secrets, roles, rolebindings, clusterroles, clusterrolebindings; `rbacTempTokens=true` → `serviceaccounts/token create`.
  - `capabilities.impersonation.enabled` default **false**, values comment "SECURITY: this flag grants impersonate-any-user" → `users` and `groups` `impersonate` (`clusterrole-impersonation.yaml`).
  - `capabilities.cost.hpa=true` → patch/update on HPAs and KEDA scaledobjects/scaledjobs; metrics.k8s.io reads, `nodes/stats`, `nodes/proxy` get; pod logs get/list.
  - `profile: cost` renders "a read-only, cost-only agent" (chart README).
  (Sources: https://raw.githubusercontent.com/komodorio/helm-charts/master/charts/komodor-agent/templates/clusterrole-k8s-watcher.yaml, .../clusterrole-impersonation.yaml, .../values.yaml, .../README.md)
- KAOP in-cluster investigators (Kubernetes RCA, Cluster Investigator): ServiceAccount bound to the aggregated `view` ClusterRole plus a supplemental cluster-scoped read role, and `pods/exec` cluster-wide (narrowable via `rbac.exec.namespaces` or removable); Secrets reads off by default; self-hosted only, one per cluster (https://docs.komodor.com/manage-your-agents/build/catalog/kubernetes).
- KAOP "configuration clusters": an operator in the cluster reconciles KAOP resources from manifests, credential held in a Kubernetes Secret, shown once (https://docs.komodor.com/security-and-governance/identity-and-access/configuration-clusters).

### Cloud identity model
- Classic agent authenticates with a Komodor `apiKey` (or `apiKeySecret` referencing a Kubernetes Secret); no cloud IAM required for the cluster agent (values.yaml).
- KAOP integrations: AWS via access keys or a cross-account IAM role created by a CloudFormation stack ("Prefer the AWS IAM-role variant over access keys… the role can be scoped read-only at creation time"); Azure via client secret + application (client) ID + directory (tenant) ID + subscription ID; Google Cloud via base64 service-account key. No specific IAM policy/role names are published (https://docs.komodor.com/manage-your-agents/build/built-in-integrations).
- KAOP workers authenticate with a per-agent revocable worker token (`AGENTOPS_WORKER_TOKEN`, `AGENTOPS_URL`); creation/rotation/revocation audited (https://docs.komodor.com/security-and-governance/identity-and-access/agent-identity).

### Network path
- Outbound only: "Agents need outbound HTTPS to the control plane; inbound access to an agent is never required"; "No inbound firewall rule, public endpoint, or VPN is needed" (https://docs.komodor.com/get-started/architecture; https://docs.komodor.com/security-and-governance/architecture-considerations/network-egress-control).
- Outpost relay for private services: outbound-only, deny-by-default allowlist on scheme/host/port/path prefix, exact or single-wildcard-label host match, refuses `CONNECT`, loopback/link-local/metadata IPs, redirects off the allowlist; size caps, timeouts, concurrency caps (https://docs.komodor.com/security-and-governance/architecture-considerations/outposts).
- Classic chart: `capabilities.tunnel.enabled=true` ("Enable the network tunnel for remote WebSocket connections"), `tunnel.whitelist` empty = allow all, `tunnel.kubeapiserver.enabled=true`; proxy settings (`proxy.komodorOnly=true`); admission-controller webhook listens in-cluster on 8443; local health ports 8089/8090 (values.yaml, README). No endpoint hostnames, IP allowlist or outbound port list published.

### How actions execute
- Classic: the in-cluster watcher's ServiceAccount performs actions (restart, scale, rollback, edit, delete, exec, port-forward) against the Kubernetes API with the ClusterRole above; "granular access control is defined in the application" (clusterrole-k8s-watcher.yaml comment). Requests arrive over the agent's outbound WebSocket tunnel; the relay protocol is not described.
- KAOP: control plane dispatches a job to the worker over the worker's outbound connection "carrying the prompt, the input, correlation IDs, and the run-scoped credentials"; tool calls go through built-in integrations or the MCP Gateway "which adds scoped credentials the agent never sees" (https://docs.komodor.com/get-started/architecture). Remediation Executor acts on live resources by "briefing Klaudia" through a Komodor MCP server, or opens a GitHub PR; it is code-denied from answering approvals (https://docs.komodor.com/manage-your-agents/build/catalog/remediation-executor).
- No VM/SSH/node-shell command execution is documented. In-pod `pods/exec` in KAOP investigators is restricted by the agent to "a short list of read-only diagnostics", interactive sessions refused, no database client (kubernetes catalog page). Classic chart: node-enricher DaemonSet mounts host paths for logs; OTel init container runs as root; GPU accessor privileged but off by default (values.yaml).

### Approval / autonomy posture and guardrails
- Standards pack default guardrail posture: "read-only work proceeding on its own, privileged diagnostics asking first, and mutations always asking"; also cost budget, ownership, escalation route; every standard except Docs can be waived visibly (https://docs.komodor.com/security-and-governance/controls/standards-guardrails).
- Guardrails are evaluated outside the agent at five gates (agent start, tool call, tool response, model request, model response); outcomes Block (default) / Hold for approval (tool-call gate) / Redact / Record only; most severe outcome wins; account-wide rules cover all agents; starter rules "no destructive Kubernetes actions", "nothing that writes to production", "no payment card numbers reaching a model"; backtest against the last 7 days of tool calls; authoring restricted to Owner/Admin (same page).
- Approvals are human-only in the console ("there is no chat or tool path to answer one"); a grant is single-use and specific to the call and its arguments; pending approvals are not pushed as notifications (https://docs.komodor.com/security-and-governance/controls/approvals).
- Agents are governed principals with a default **Agent** role; permissions checked at the moment of action (https://docs.komodor.com/security-and-governance/identity-and-access/agent-identity).
- Blog (via snippet): Klaudia can run as a co-pilot that recommends and waits for sign-off, with autonomy widened over time; human-in-the-loop option for pod crashes/failed rollouts; company-wide guardrails cannot be overridden by an agent's owner (https://komodor.com/blog/autonomous-self-healing-capabilities-for-cloud-native-infrastructure-and-operations/; https://komodor.com/blog/komodor-introduces-autonomous-self-healing-capabilities-for-cloud-native-infrastructure-and-operations/; https://komodor.com/blog/technical-teardown-komodor-agentic-operations-platform/).

### Tenant isolation and data handling
- Account is the tenancy boundary, "enforced on every read and write"; run ownership derives from the invoked agent, not caller-supplied values; per-agent worker tokens, roles, credential bindings, label-scoped grants; Access explorer to inspect effective access (https://docs.komodor.com/security-and-governance/architecture-considerations/agent-isolation-tenancy).
- Run-scoped secrets masked "before anything leaves the worker"; credentials delivered in memory per run and never persisted; rotation takes effect next run (https://docs.komodor.com/security-and-governance/architecture-considerations/secrets-credential-handling).
- Run evidence (prompts, tool args/results, logs, outputs) is retained and "can contain customer data"; general PII is not auto-classified; detectors exist for card numbers, SSNs, emails, private keys, JWTs, cloud credential formats; retention periods not console-configurable ("contact your Komodor representative"); export and account deletion by request; audit log append-only (https://docs.komodor.com/security-and-governance/architecture-considerations/data-handling-redaction; .../data-retention-deletion-export).
- Classic chart: secret redaction on by default (`capabilities.redaction.secret.enable`), per-kind jq redaction, log/event redaction lists, namespace allow/deny lists for logs; metrics and telemetry sent to Komodor backend by default (values.yaml).
- Compliance (via snippet): SOC 2 Type II and GDPR cited on komodor.com pricing page; Klaudia page says it is "built on AWS Bedrock" and adheres to SOC 2 Type 2 (https://komodor.com/platform/pricing-and-plans/; https://komodor.com/platform/klaudia-ai-powered-troubleshooting/).

### Doc URLs used
- https://github.com/komodorio/helm-charts/tree/master/charts/komodor-agent (README, values.yaml, templates/clusterrole-k8s-watcher.yaml, templates/clusterrole-impersonation.yaml)
- https://docs.komodor.com/get-started/architecture
- https://docs.komodor.com/get-started/deployment-methods
- https://docs.komodor.com/security-and-governance/architecture-considerations/network-egress-control
- https://docs.komodor.com/security-and-governance/architecture-considerations/outposts
- https://docs.komodor.com/security-and-governance/architecture-considerations/agent-isolation-tenancy
- https://docs.komodor.com/security-and-governance/architecture-considerations/data-handling-redaction
- https://docs.komodor.com/security-and-governance/architecture-considerations/data-retention-deletion-export
- https://docs.komodor.com/security-and-governance/architecture-considerations/secrets-credential-handling
- https://docs.komodor.com/security-and-governance/controls/approvals
- https://docs.komodor.com/security-and-governance/controls/standards-guardrails
- https://docs.komodor.com/security-and-governance/identity-and-access/agent-identity
- https://docs.komodor.com/security-and-governance/identity-and-access/configuration-clusters
- https://docs.komodor.com/manage-your-agents/build/catalog/kubernetes
- https://docs.komodor.com/manage-your-agents/build/catalog/remediation-executor
- https://docs.komodor.com/manage-your-agents/build/built-in-integrations
- Blog/marketing via snippet: https://komodor.com/blog/technical-teardown-komodor-agentic-operations-platform/, https://komodor.com/blog/autonomous-self-healing-capabilities-for-cloud-native-infrastructure-and-operations/, https://komodor.com/platform/pricing-and-plans/, https://komodor.com/platform/klaudia-ai-powered-troubleshooting/

### Not published
- Komodor endpoint hostnames, IP allowlist, outbound ports.
- The IAM policy content of the AWS cross-account role; Azure/GCP role names.
- How the classic agent tunnel relays action requests (protocol, authorisation per request).
- Klaudia's per-action approval defaults in the classic product (help.komodor.com unreachable).
- Retention periods; SOC 2 report scope; whether the self-hosted control plane is GA.

---

## 2. Kubiya

### Deployment form
- SaaS control plane with hosted runners or self-hosted runners; self-hosted runners are "the recommended option for production", installed on your own Kubernetes cluster with Helm (via snippet, https://docs.kubiya.ai/concepts/runners). Hosted runners cannot handle Kubernetes use cases (via snippet, https://docs.kubiya.ai/docs/kubiya-resources/local-runners).
- Self-hosted control plane via Kubernetes Operator / OpenShift Operator described as the enterprise option "for private networking or custom policies"; HIPAA support for self-hosted deployments (via snippet, https://docs.kubiya.ai/core-concepts/control-plane/overview; https://docs.kubiya.ai/). Air-gapped: not published.

### What is installed in the customer environment
- Helm chart `kubiya-runner` (repo `https://kubiyabot.github.io/helm-charts`), default namespace `kubiya`; components: Agent Manager, Kubiya Operator, Tool Manager ("Executes tools within the cluster", "Integrates with Dagger for container runtime"), Workflow Engine; subcharts dagger-helm, kube-state-metrics, alloy; prerequisites Kubernetes 1.19+, PV provisioner, NATS credentials (https://github.com/kubiyabot/helm-charts/blob/main/charts/kubiya-runner/README.md).
- RBAC: "Namespace-scoped RBAC permissions for components (except default required cluster-wide PVC management for tool-manager)"; dedicated service accounts per component; optional `adminClusterRole.create` grants tool-manager full cluster access; optional operator full-access toggle (same README). Control-plane docs: install needs rights to create namespaces, deployments, services, PVCs, "plus cluster-wide roles if that option is enabled"; recommendation to keep cluster-wide access off unless a tool needs cross-namespace access (via snippet, control-plane overview). Exact ClusterRole rules: not published.

### Cloud identity model
- AWS: recommended path is an IAM role that trusts Kubiya's AWS account; alternative binds an AWS role to an EKS service account in the customer cluster ("Kubiya can't validate that setup"); the agent's execution environment receives temporary credentials for the role (via snippet, https://docs.kubiya.ai/docs/kubiya-resources/integrations/connect-kubiya-to-aws). Integration config `auth_type: iam_role`, `role_arn`; Kubernetes integration `auth_type: service_account` (via snippet, https://docs.kubiya.ai/concepts/integrations). SDK exposes `integrations.get_integration_credentials(vendor="aws")` (https://docs.kubiya.ai/sdk/client-overview).
- Runner authenticates to Kubiya with a token generated in the UI; NATS JWT credentials for messaging; optional Azure service principal only for Prometheus remote-write telemetry (chart README; runners page).
- No IAM policy names, external-ID details, or Azure/GCP role names published.

### Network path
- "Outbound HTTPS access to Kubiya API" is the stated requirement; "Only necessary network access is allowed, with outbound HTTPS to Kubiya API" (via snippet, runners page). Chart README: outbound to NATS Cloud (Synadia) and optional Prometheus remote-write to Azure Managed Prometheus. No inbound requirement stated; no IP allowlist or port list published.

### How actions execute
- Runner pulls work and "Sends results, logs, and status back to Kubiya"; each workflow step/tool runs in its own container launched in the cluster by the Tool Manager via Dagger; results stream back to the Control Plane (via snippet, runners page; https://docs.kubiya.ai/core-concepts/execution-infrastructure/task-queues; https://github.com/kubiyabot/community-tools "Each tool runs in an isolated Docker container"). Which component calls the Kubernetes/cloud API is the tool container with the integration's credentials. VM-level SSH/SSM execution: not published.

### Approval / autonomy posture and guardrails
- Security page describes an approval workflow: tool execution routed to an approval check, then approver validation, then execution; platform roles Administrator, Developer, Operator, Viewer; least-privilege and containerised isolation recommended (via snippet, https://docs.kubiya.ai/docs/concepts/security). Control-plane docs mention "organization policies and guardrails"; a tool-policy-enforcement use-case page exists (https://docs.kubiya.ai/docs/use-cases/build-a-use-case/tool-policy-enforcement). Default posture and policy engine (OPA or other): not published in the readable material.

### Tenant isolation and data handling
- "SOC 2 Type II, GDPR, CCPA certifications. Multi-tenant isolation at the database level"; "Only metadata and telemetry leave your infrastructure" (via snippet, https://docs.kubiya.ai/concepts/platform-overview). Note: the task-queue page says results stream back to the control plane, so "metadata only" is not fully consistent.
- Encryption at rest and in transit; configurable log and conversation retention; data-sovereignty options for regional processing; VPC connectivity and private deployments for enterprise (via snippet, security page). Alloy and kube-state-metrics "collect only data from same kubernetes namespace where kubiya-runner is deployed" (chart README).

### Doc URLs used
- https://docs.kubiya.ai/concepts/runners
- https://docs.kubiya.ai/docs/kubiya-resources/local-runners
- https://docs.kubiya.ai/core-concepts/control-plane/overview
- https://docs.kubiya.ai/docs/concepts/security
- https://docs.kubiya.ai/concepts/integrations
- https://docs.kubiya.ai/docs/kubiya-resources/integrations/connect-kubiya-to-aws
- https://docs.kubiya.ai/concepts/platform-overview
- https://docs.kubiya.ai/core-concepts/execution-infrastructure/task-queues
- https://docs.kubiya.ai/docs/use-cases/build-a-use-case/tool-policy-enforcement
- https://github.com/kubiyabot/helm-charts/blob/main/charts/kubiya-runner/README.md
- https://github.com/kubiyabot/community-tools
(All docs.kubiya.ai facts are from search-engine snippets of those pages; the host did not resolve from the research environment.)

### Not published
- Exact ClusterRole/Role rules of the runner chart.
- IAM trust-policy details (Kubiya account ID, external ID) and permission names; Azure/GCP identity model.
- Kubiya API endpoints, ports, IP allowlist.
- Approval defaults, who approves, policy language/engine.
- LLM data handling; retention periods; SOC 2 scope; air-gapped support.

---

## 3. Shoreline.io (now NVIDIA; shipped as NVIDIA Mission Control Autonomous Hardware Recovery, AHR)

### Deployment form
- The currently documented form is customer-hosted/on-prem: the AHR backend "runs on the admin Kubernetes nodes in the control plane. It is installed to the control plane via helm", with primary/secondary replicas; agent config sets `FORCE_ON_PREM=true` (https://docs.nvidia.com/mission-control/docs/nmc-software-installation-guide/latest/nmc-ahr-installation.html).
- Shoreline's former SaaS/per-cloud backends: shoreline.io pages now redirect to nvidia.com; the indexed copy of https://www.shoreline.io/architecture says it "supports all major cloud providers" (not verifiable live). No SaaS offering is documented today.

### What is installed in the customer environment
- Agent on every managed node (compute nodes, Kubernetes worker nodes, BCM head nodes), "installed in BCM's image for distribution to the managed computers"; delivered as a Debian package (`shoreline_<version>-enroot.deb`) run as systemd service `shoreline`, executing container image `nvcr.io/nvidia/nv-mission-control/shoreline-agent` via Enroot (`USE_ENROOT=1`); runs as dedicated system user `autonomous-hardware-recovery` (`AGENT_UID/AGENT_GID`), with a sudoers drop-in `/etc/sudoers.d/99-shoreline-user` and `ALLOW_SUDO=true`; Enroot helper binaries get `setcap cap_sys_admin,cap_mknod=ep`; agent files under `/var/lib/shoreline/agent`, `AGENT_MEMORY_LIMIT=1G`, `DISCOVER_GPU=true` (NVIDIA AHR install guide, agent.config block).
- Backend: Helm chart `shoreline-onprem-backend`, namespace `autonomous-hardware-recovery`; StatefulSets `shoreline-backend`, `shoreline-ceph`; Deployments `shoreline-frontend`, `shoreline-ops-tool`, `shoreline-openbao`, `shoreline-system-metadata`, `shoreline-ui` (same guide).
- Legacy Shoreline (indexed https://www.shoreline.io/product): agents "run as a DaemonSet or StatefulSet on Kubernetes or as an installed package on VMs (apt, yum)"; ~250 MB memory / ¼ CPU core (indexed https://www.shoreline.io/how-shoreline-works). Kubernetes ClusterRole rules: not published.

### Cloud identity model
- Legacy blog: "credentials are assigned to the agents using standard Kubernetes service account and cloud provider role based delegation" rather than stored secrets (indexed https://shoreline.io/blog/operations-at-the-edge).
- AHR: the only cloud credential documented is for optional external object storage — an IAM policy with `s3:*` on bucket patterns `onprem-org-shoreline-mdkey-mr-*`, `ss-arc-*-onprem-local*`, `ss-arc-system-metadata-*` plus `s3:ListAllMyBuckets` (NVIDIA AHR install guide). No IAM role/permission names for the agent itself.

### Network path
- Agent → backend outbound: `BACKEND_ADDRESS=$AHR_DISCOVERY_URL:443`; "There is no direct communication to the agents i.e. all usage of NVIDIA Mission Control autonomous hardware recovery is intermediated by the backend" (NVIDIA AHR install guide). Backend exposed on a MetalLB VIP via Kubernetes Gateway API with DNS names `api.`, `discovery.`, `agent-gateway.`, `ceph.` under the AHR domain; agent config lists `SSH_PORT='22'`; package downloads from api.ngc.nvidia.com / nvcr.io.
- Legacy: agents open no inbound ports and hold outbound connections to the backend (indexed shoreline blog). IP allowlist: not applicable/not published (on-prem).

### How actions execute
- Op commands execute on the host by the local agent; Op "can execute anything you can run at the Linux command prompt", operators "can execute unrestricted Linux commands against the target resources" (indexed shoreline.io blog posts). In AHR the agent runs under the `autonomous-hardware-recovery` user inside an Enroot container with sudo allowed (`ALLOW_SUDO=true`, sudoers drop-in) (NVIDIA install guide). Dispatch mechanism (push vs pull) and whether alarm→action bots run locally on the agent without the backend: not published in reachable docs.

### Approval / autonomy posture and guardrails
- Runbooks run from "Alarm triggers", "Time Trigger (cron jobs)", "Other integrations like AlertManager", or manually cell-by-cell; permission policies "determine if the user can view resources and execute actions (named and/or anonymous)"; "Action execution can be limited to a maximum number of affected resources, to specific resources, or to both"; permissions can be attached to runbooks; defaults: "By default, new users are granted administer and configure roles until the privileges are overridden"; "the Autonomous Hardware Recovery Agent user should have only the minimum permissions required for the commands the agent executes" (https://docs.nvidia.com/mission-control/docs/systems-administration-guide/latest/autonomous-hardware-recovery.html).
- Legacy safety page (indexed https://www.shoreline.io/safety): blast radius by role ("max x nodes"), circuit breakers limiting frequency of automated actions ("max x/hour"), RBAC, privilege escalation for "break glass" scenarios.
- No explicit human-approval step feature is documented.

### Tenant isolation and data handling
- Single-tenant on-prem; authentication via BCM LDAP with short-lived JWT and refresh token; TLS protects "all communications"; alarm/action outputs stored in local Ceph or customer-owned S3; optional CALLHOME (default no) posts "the system serial number, a description of the fault, and a diagnostic log" to NVIDIA (NVIDIA install and admin guides). SOC 2 / multi-tenant isolation statements: not published.

### Doc URLs used
- https://docs.nvidia.com/mission-control/docs/nmc-software-installation-guide/latest/nmc-ahr-installation.html
- https://docs.nvidia.com/mission-control/docs/systems-administration-guide/latest/autonomous-hardware-recovery.html
- https://docs.nvidia.com/base-command-manager/product-manuals/nvidia-mission-control/ahr
- https://catalog.ngc.nvidia.com/orgs/nvidia/nv-mission-control/containers/shoreline-agent/-
- Indexed copies (site now redirects to nvidia.com): https://www.shoreline.io/safety, https://www.shoreline.io/product, https://www.shoreline.io/how-shoreline-works, https://shoreline.io/blog/operations-at-the-edge, https://www.shoreline.io/architecture

### Not published
- Kubernetes RBAC rules for the DaemonSet form; contents of the sudoers drop-in; root vs sudo scope.
- IAM roles/instance profiles for cloud-level actions.
- Command dispatch mechanism; local bot execution without backend.
- Human-approval gates; rate-limit configuration syntax.
- Current availability of any SaaS backend; SOC 2 or isolation statements. docs.shoreline.io is unreachable.

---

## 4. Sedai

### Deployment form
- SaaS (`https://yourcompany.sedai.app`); "Each Sedai SaaS instance for customers operates within its dedicated tenant" (https://docs.sedai.io/get-started; https://docs.sedai.io/get-started/onboarding/readme/safety-and-security). Self-hosted / VPC-deployed control plane: not published.

### What is installed in the customer environment
- Agentless by default: "For most cloud resources, Sedai connects securely via IAM permissions, without requiring an agent" (https://sedai.io/). Optional in-cluster Sedai Smart Agent (Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, Deployment `sedai-smart-agent`; Helm, kubectl or GitOps; install commands expire within 30 minutes) (https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/sedai-smart-agent).
- EKS agentless: Sedai IAM role mapped in the `aws-auth` ConfigMap to Kubernetes group `sedai-service-account`; ClusterRole applied from GitHub.
  - Read-write `sedai-autonomous-mode-clusterrole`: `get/list/patch/update/watch` on pods, services, replicationcontrollers (core); daemonsets, deployments, replicasets, statefulsets, `deployments/scale`, `statefulsets/scale` (apps); jobs, cronjobs (batch); poddisruptionbudgets (policy); horizontalpodautoscalers and VerticalPodAutoscaler additionally `create/delete`; `get/list/watch` on ingresses, ingressclasses, networkpolicies, metrics.k8s.io pods/nodes; plus `apiGroups:["*"] resources:["*"] verbs:[get,list,watch]` (https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/eks/sedai-autonomous-mode-clusterrole.yaml).
  - Read-only `sedai-readonly-mode-clusterrole`: get/list/watch only, incl. karpenter.sh / karpenter.k8s.aws resources (https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/eks/sedai-readonly-mode-clusterrole.yaml).
  (https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/eks-agentless-setup)
- AKS agentless: Entra ID group with "Azure Kubernetes Service Cluster User Role" on the cluster; app registration + client secret added to that group; no ClusterRole YAML published for AKS (https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/aks-agentless-setup).

### Cloud identity model
- AWS: cross-account IAM role via CloudFormation (template `https://sedai-onboarding-templates-prod.s3.amazonaws.com/nested/sedai-integration-main.yml`); CLI example role `SedaiAWSIntegrationRole`, policy `SedaiAWSIntegrationIamPolicy`; published trust policy allows `sts:AssumeRole` from AWS principal `504216784688` with an empty Condition — the docs say "Sedai will generate an External ID based on the provided ARN", but the published trust document has no `sts:ExternalId` condition (discrepancy). IAM user with access keys also supported, role recommended (https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-aws-account; https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/aws/sedai-assume-role-policy-document.json).
- Per-service AWS policies published as JSON (RW and R variants, Resource "*" unless noted), under https://sedai-onboarding-templates-prod.s3.amazonaws.com/nested/ linked from the AWS connect page:
  - Common RW: CloudWatch `DescribeAlarms/GetMetricData/ListMetrics`, `cloudtrail:LookupEvents`, `iam:ListInstanceProfiles`, `ec2:DescribeAvailabilityZones/DescribeRegions`, `tag:GetResources`.
  - EKS RW: `eks:Describe*`, `eks:List*` (no eks writes); `autoscaling:CreateAutoScalingGroup/CreateLaunchConfiguration/UpdateAutoScalingGroup/DeleteLaunchConfiguration/DeleteAutoScalingGroup/TerminateInstanceInAutoScalingGroup/PutScalingPolicy`; ELB `CreateRule/CreateTargetGroup/Register*/Deregister*/AddTags`; `iam:PassRole`, `iam:CreateServiceLinkedRole` (conditioned).
  - EC2 RW: `ec2:ModifyInstanceAttribute`, `RunInstances`, `StartInstances`, `StopInstances`, `CreateSnapshot`, `DeleteSnapshot`, `CreateTags`, `Create/DeleteLaunchTemplate*`; autoscaling writes incl. `StartInstanceRefresh`; `kms:DescribeKey/GetKeyPolicy`; IAM simulate/read.
  - ECS RW: `ecs:UpdateService`, `RunTask`, `StartTask`, `StopTask`, `RegisterTaskDefinition`, `DeregisterTaskDefinition`, `Create/Delete/UpdateCapacityProvider`, `PutClusterCapacityProviders`, `UpdateCluster`, `UpdateContainerInstancesState`; `application-autoscaling:Register/DeregisterScalableTarget`, `Put/DeleteScalingPolicy`; `codedeploy:CreateDeployment`, `RegisterApplicationRevision`.
  - Lambda RW: `lambda:UpdateFunctionConfiguration`, `PutFunctionConcurrency`, `PutProvisionedConcurrencyConfig`, `PublishVersion`, `Create/Update/DeleteAlias`; DynamoDB read on tables `SedaiResourceConcurrencyStats`, `SedaiTelemetryLogs`.
  - RDS RW: `rds:ModifyDBInstance`, `rds:ModifyDBCluster` (+ Describe). S3 RW: `s3:PutLifecycleConfiguration`, `PutIntelligentTieringConfiguration`, `PutMetricsConfiguration`. EBS RW: `ec2:ModifyVolume`, `DeleteVolume`, `Create/DeleteSnapshot`.
  - No `autoscaling:*` or `eks:*` wildcard; no SSM/SSH actions.
- Azure: single-tenant app registration (client ID/secret, tenant, subscription) with "Monitor Reader" at subscription or resource-group scope; or Sedai's multitenant service principal assigned "Reader" at subscription scope; separate billing integration (https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-azure-account). The write role used for Autopilot on Azure is not listed.
- GCP: not researched/published in pages read.

### Network path
- Sedai calls cloud APIs from its SaaS via the assumed role; no inbound to the customer. Published egress IPs for EKS public-endpoint allowlisting: `18.204.62.170/32` and `34.238.185.251/32` (Sedai NAT gateways, us-east-1); alternative recommended path is VPC peering from Sedai's VPC with CIDR/security-group details supplied via support@sedai.io (EKS agentless page). AKS network path and Smart Agent egress endpoints: not published.

### How actions execute
- Cloud: the SaaS calls the AWS/Azure APIs listed above (rightsizing, scaling policies, Lambda configuration, RDS modify, storage lifecycle). Kubernetes: patch/update workloads and create/delete HPAs/VPAs via the Kubernetes API using the mapped IAM role (EKS) or the Smart Agent. IaC: "Whenever Sedai optimizes a resource, it automatically opens a pull request in your Git repository … across Terraform, Helm, & Kustomize", plus a Kubernetes admission controller to prevent Argo CD/Flux overwrites (https://sedai.io/ FAQ). No VM-level command execution (no SSM/SSH actions in any policy).

### Approval / autonomy posture and guardrails
- Three modes: Datapilot (default; "Analyzes monitoring data only"), Copilot ("the system will always ask for permission to execute an operation"), Autopilot ("independently, continuously, and safely analyze/execute"); set per account, custom group, resource type or individual resource; read-only policies permit Datapilot only (https://docs.sedai.io/get-started/onboarding/readme/understanding-operation-modes; .../what-to-expect).
- Safety: topology inference; about one month of profiling before autonomous action; impact analysis, efficacy from similar past actions, seasonality-based timing, validation against queued actions; "Changes will be reverted if verification and efficacy criteria are not met"; escalation to humans for recurring issues; audit record of every action (https://docs.sedai.io/get-started/onboarding/readme/safety-and-security). Maintenance windows, pause and instant rollback cited on https://sedai.io/ (marketing).

### Tenant isolation and data handling
- Dedicated tenant per customer, "No data will be shared across customer instances"; collects Golden-Signal metrics ("latency, traffic, errors, and saturation") and CloudWatch metrics; AWS integration "does not … include access to logs"; HTTPS in transit, HMAC on the message bus, AES at rest; credentials in AWS Secrets Manager with rotation; SAML SSO, MFA (safety-and-security page; AWS connect page). SOC 2 Type 2 claimed on https://sedai.io/ only; sedai.io/security returned 404; retention periods not published.

### Doc URLs used
- https://docs.sedai.io/get-started
- https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-aws-account
- https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-azure-account
- https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/eks-agentless-setup
- https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/aks-agentless-setup
- https://docs.sedai.io/get-started/onboarding/autonomous-cloud-management/connect-kubernetes-cluster/sedai-smart-agent
- https://docs.sedai.io/get-started/onboarding/readme/understanding-operation-modes
- https://docs.sedai.io/get-started/onboarding/readme/safety-and-security
- https://docs.sedai.io/get-started/onboarding/readme/what-to-expect
- https://sedai.io/
- https://sedai-onboarding-templates-prod.s3.amazonaws.com/nested/ (16 policy JSON files: common, kubernetes, ec2instance, ecs, lambda, rds, storages3, storageebs; -rw and -r)
- https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/aws/sedai-assume-role-policy-document.json
- https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/eks/sedai-autonomous-mode-clusterrole.yaml
- https://raw.githubusercontent.com/SedaiEngineering/sedai-onboarding/main/eks/sedai-readonly-mode-clusterrole.yaml

### Not published
- Self-hosted option; retention periods; SOC 2 report/scope (security page 404, trust.sedai.io does not resolve).
- Azure write role for Autopilot; AKS network path / IP allowlist; AKS ClusterRole YAML.
- Smart Agent egress endpoints; GCP roles.
- Concrete change-window / numeric-limit configuration; ITSM/ticket path.
- Explanation of the missing `sts:ExternalId` condition in the published trust policy.

---

## 5. Harness AI SRE

### Deployment form
- "AI SRE is SaaS only, and Self-Managed Platform is not supported" (https://developer.harness.io/docs/ai-sre/resources/whats-supported/; same in FAQ).

### What is installed in the customer environment
- Nothing mandatory; optional Harness Delegates for connector-based automation, on-call sync and polling ingestion ("Connectors and delegates call the tool's API"; "Only approved integrations execute via Harness Delegates") (whats-supported; https://developer.harness.io/ai-sre/troubleshooting-and-resources/ai-sre-security).
- Delegate Helm chart `harness-delegate/harness-delegate-ng` from `https://app.harness.io/storage/harness-download/delegate-helm-chart/`, namespace `harness-delegate-ng`; documented role options `cluster-admin`, `cluster-viewer`, `namespace-admin`, or custom cluster roles (https://developer.harness.io/harness-platform/use-harness-platform/delegates/delegate/install-delegates/overview). Delegates "do not require root account access" though Kubernetes and Docker delegates "run as root by default" (https://developer.harness.io/harness-platform/use-harness-platform/delegates/delegate/delegate-concepts/delegate-requirements). AI SRE-specific delegate RBAC: not published.

### Cloud identity model
- "AI SRE uses existing Harness identity and RBAC (no additional API keys needed)"; runbook actions use project-level connectors with dedicated service accounts (Zoom, GitHub, Jira) (https://developer.harness.io/ai-sre/ai-sre-for-administrators/set-up-runbook-management/integrations/source-control/harness-pipelines; https://developer.harness.io/ai-sre/3.0/ai-sre-for-administrators/set-up-runbook-management/integrations/overview).
- No AWS/Azure/GCP/Kubernetes runbook actions are listed for AI SRE. The general Harness connector permission table (e.g. AWS `AmazonEC2FullAccess` + `DescribeRegions`, GCP "Kubernetes Engine Admin", "Storage Object Viewer", Azure client/tenant ID + key) applies to CD connectors, not AI SRE (https://developer.harness.io/harness-ai/use-harness-platform/references/permissions-and-ports-for-harness-connections).

### Network path
- Delegate: "All network connections from your local network to Harness SaaS are outbound-only", "HTTPS port 443 outbound from the delegate to Harness", HTTPS/WSS to Harness Manager (delegate-requirements; https://developer.harness.io/docs/platform/delegates/delegate-concepts/delegate-overview). Endpoints `app.harness.io`, `app3.harness.io`, `accounts.harness.io`, `accounts.eu.harness.io`; optional `logging.googleapis.com` (https://developer.harness.io/harness-platform/use-harness-platform/references/allowlist-harness-domains-and-ips). Harness SaaS egress IPs are published only via the Trust Center (trust.harness.io) after sign-up. No inbound requirement.

### How actions execute
- Runbook actions are API calls to tools via connectors/delegates (whats-supported). "Execute Harness Pipeline" action, fire-and-forget or wait-for-completion (harness-pipelines page). "No, custom scripts cannot be directly integrated into Harness AI SRE Runbooks" — shell work must go through a Harness pipeline (https://developer.harness.io/ai-sre/troubleshooting-and-resources/faq). RCA Change Agent "can invoke runbooks as tools during investigation" (https://developer.harness.io/ai-sre/ai-sre-for-incident-responders/use-ai-agents/rca-change-agent). VM-level command execution via AI SRE itself: not published (only through pipelines, where delegates run steps).

### Approval / autonomy posture and guardrails
- AI Scribe Agent and RCA Change Agent are described as autonomous analysis/documentation components (timelines, root-cause scoring), not remediators (ai-agent and rca-change-agent pages).
- Manual runbook trigger "Recommended for production deployments and rollback operations"; automatic triggers "Use with caution for low-risk, well-tested actions" (harness-pipelines page).
- Executing a pipeline from a runbook requires Edit on Incidents (AI SRE) plus execute permission on the target pipeline; runbook RBAC View/Create/Edit/Delete/"Incident Trigger"; "Ensures that only authorized users execute automated actions" (ai-sre-security page). No approval-step feature documented for AI SRE runbooks.

### Tenant isolation and data handling
- "Data protection: TLS 1.3 transit and AES 256 at rest"; retention "based on your organization's settings" (no period given); AI SRE "runs under its own dedicated Harness project" (ai-sre-security page). Ingests alerts, incident metadata, logs, communication history, Slack/Zoom/Teams content; Scribe keeps a full transcript; Zoom/Teams transcription via Recall.ai and AssemblyAI; LLM model "not published, because models change over time" (https://developer.harness.io/ai-sre/ai-sre-for-incident-responders/use-ai-agents/ai-agent; FAQ). SOC 2/ISO not stated in AI SRE docs (Trust Center referenced).
- Doc inconsistencies: security page says runbooks "require API keys or OAuth authentication for third-party integrations" vs pipelines page "no additional API keys needed"; security page says "Outbound notifications are not permitted" while whats-supported lists outbound Slack messages and status emails; RBAC tables differ between security page and FAQ.

### Doc URLs used
- https://developer.harness.io/ai-sre/troubleshooting-and-resources/ai-sre-security
- https://developer.harness.io/ai-sre/troubleshooting-and-resources/faq
- https://developer.harness.io/docs/ai-sre/resources/whats-supported/
- https://developer.harness.io/ai-sre/ai-sre-for-administrators/get-started/integrate-tools
- https://developer.harness.io/ai-sre/ai-sre-for-administrators/set-up-runbook-management/integrations/source-control/harness-pipelines
- https://developer.harness.io/ai-sre/3.0/ai-sre-for-administrators/set-up-runbook-management/integrations/overview
- https://developer.harness.io/ai-sre/ai-sre-for-administrators/set-up-runbook-management/runbooks
- https://developer.harness.io/ai-sre/ai-sre-for-incident-responders/use-ai-agents/ai-agent
- https://developer.harness.io/ai-sre/ai-sre-for-incident-responders/use-ai-agents/rca-change-agent
- https://developer.harness.io/harness-ai/use-harness-platform/references/permissions-and-ports-for-harness-connections
- https://developer.harness.io/docs/platform/delegates/delegate-concepts/delegate-overview
- https://developer.harness.io/harness-platform/use-harness-platform/delegates/delegate/delegate-concepts/delegate-requirements
- https://developer.harness.io/harness-platform/use-harness-platform/delegates/delegate/install-delegates/overview
- https://developer.harness.io/harness-platform/use-harness-platform/references/allowlist-harness-domains-and-ips

### Not published
- Delegate RBAC needed for AI SRE (cluster-admin vs namespace-scoped).
- Cloud/Kubernetes identity for AI SRE actions (no such actions listed).
- Approval gates for runbook or agent actions.
- LLM vendor/model; retention periods; SOC 2/ISO; tenant-isolation statement; model-training use of data.
- VM-level execution / SSH steps through AI SRE (custom scripts unsupported).
- Exact Harness SaaS egress IPs (gated behind the Trust Center).

---

## Cross-vendor pattern (one line each)
- **Komodor**: broadest in-cluster write RBAC by default (wildcard patch/delete on CRs, secrets, RBAC objects, pods/exec, impersonation opt-in); governance moved to a central policy/approval layer with "mutations always asking" as the default standard; outbound-only agent and Outpost relay.
- **Kubiya**: runner executes tools as containers in your cluster, namespace-scoped by default with opt-in cluster-admin; AWS via cross-account role or IRSA-style binding; approval workflow exists but defaults undocumented.
- **Shoreline / NVIDIA AHR**: host agent on every node running as a dedicated user with sudo, backend self-hosted, RBAC plus resource-count/blast-radius and circuit-breaker limits; no SaaS form currently documented.
- **Sedai**: fully agentless cross-account IAM with explicit per-service RW/R policies and published egress IPs, default read-only (Datapilot) mode, auto-revert guardrail; External-ID gap in the published trust policy.
- **Harness AI SRE**: SaaS only, no infrastructure write path of its own — actions are tool-API calls or Harness pipelines executed through an outbound-only delegate.
