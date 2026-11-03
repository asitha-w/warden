# How cloud providers' AI SRE products reach into customer infrastructure

Sources: vendor docs only (learn.microsoft.com, docs.aws.amazon.com, aws.amazon.com/blogs, docs.cloud.google.com / cloud.google.com), read 2026-10-31. Page dates noted where the docs show them.

---

## 1. Azure SRE Agent (preview)

### Deployment form
A Microsoft-managed agent resource created in the customer's subscription/resource group; the hosting resource is a Container App (`Microsoft.App/containerApps`), and the `Microsoft.App` provider must be registered (https://learn.microsoft.com/en-us/azure/sre-agent/agent-identity; https://learn.microsoft.com/en-us/azure/sre-agent/faq). Deployable regions: Sweden Central, East US 2, Australia East; it manages resources in any region regardless of where it is hosted; deployable via Bicep/CLI (https://learn.microsoft.com/en-us/azure/sre-agent/faq; https://learn.microsoft.com/en-us/azure/sre-agent/overview).

### What is installed in the customer environment
Nothing on VMs or clusters. The docs describe only the agent resource, two managed identities (a UAMI `id-*` resource plus a system-assigned identity), and optionally a dedicated delegated subnet (https://learn.microsoft.com/en-us/azure/sre-agent/agent-identity; https://learn.microsoft.com/en-us/azure/sre-agent/network-integration). No VM extension, Run Command, DaemonSet or cluster component is mentioned anywhere.

### Cloud identity model and permission names
- Identity: an auto-created user-assigned managed identity (UAMI) does all Azure resource operations (ARM, CLI, diagnostics), the ADX and Azure DevOps connectors; the system-assigned identity is "for internal operations only" (https://learn.microsoft.com/en-us/azure/sre-agent/agent-identity).
- Always-assigned core roles: **Reader**, **Log Analytics Reader**, **Monitoring Reader** at resource-group scope, plus **Monitoring Contributor at subscription scope** (to acknowledge/close Azure Monitor alerts) (https://learn.microsoft.com/en-us/azure/sre-agent/permissions).
- **Reader level** = core roles + resource-type-specific *reader/operator* roles; the agent must request OBO elevation to act. **Privileged level** = core roles + resource-type-specific *contributor* roles detected in the managed resource groups (examples named: Container App Contributor; AKS Cluster User appears in the portal text). No full resource-type→role table is published (https://learn.microsoft.com/en-us/azure/sre-agent/permissions; https://learn.microsoft.com/en-us/azure/sre-agent/manage-permissions).
- Scoping: roles are assigned per **resource group** you select; adding a group assigns the roles, removing it revokes them; "You can't remove individual permissions, only entire resource groups"; resource groups can be picked "across your subscriptions"; optional subscription-wide Reader; assigning requires Owner or User Access Administrator on the groups (https://learn.microsoft.com/en-us/azure/sre-agent/permissions; https://learn.microsoft.com/en-us/azure/sre-agent/manage-permissions).
- **On-behalf-of**: when the UAMI lacks permission, the agent uses the Entra OAuth2 OBO flow with the *user's* token; only work/school accounts, and only users with the **SRE Agent Administrator** role can authorise; permissions are not retained after the operation; applies to chat, incident threads, scheduled runs and autonomous operations (https://learn.microsoft.com/en-us/azure/sre-agent/permissions).
- Connectors: Log Analytics / Application Insights connectors use the managed identity, read-only; the portal assigns Log Analytics Reader and Monitoring Reader on save (https://learn.microsoft.com/en-us/azure/sre-agent/connect-telemetry-source; https://learn.microsoft.com/en-us/azure/sre-agent/create-and-set-up). GitHub uses OAuth (user consent from a GitHub org member) or a PAT with `repo`/`public_repo`; GitHub Enterprise supports a bring-your-own GitHub App; no managed identity involved (https://learn.microsoft.com/en-us/azure/sre-agent/setup-github-connector; https://learn.microsoft.com/en-us/azure/sre-agent/connectors; https://learn.microsoft.com/en-us/azure/sre-agent/overview).
- User roles (Azure data-plane roles on the agent resource): SRE Agent Reader, Standard User, Author, Administrator; only Administrator approves actions, manages connectors, authorises OBO; Azure Owner/Contributor do not substitute (https://learn.microsoft.com/en-us/azure/sre-agent/user-roles).
- Lighthouse / other tenants: **not stated** anywhere in the SRE Agent docs; the data-privacy page says data is isolated "by tenant and Azure subscription".

### Network path
None required by default: "Unrestricted" egress mode works over the public internet and reaches anything with public access (https://learn.microsoft.com/en-us/azure/sre-agent/faq). Three egress modes: Unrestricted (default), Limited (wildcard URL allow-list), Azure VNet (all non-platform outbound traffic through a customer subnet: /27+, delegated to `Microsoft.App/environments`, same region, dedicated; customer NSG/firewall/DNS apply). In VNet mode Azure infrastructure (Log Analytics, App Insights, AKS, databases, Key Vault) routes through the VNet; platform traffic (orchestration, model endpoints) always goes through Microsoft's infra network; connector traffic stays on the public internet; inbound/private endpoint to the agent is not supported (https://learn.microsoft.com/en-us/azure/sre-agent/network-integration).

### How actions execute
- Azure: via **Azure CLI** ("If you can run it in `az`, your agent can run it too") through tools `RunAzCliReadCommands` / `RunAzCliWriteCommands` under the UAMI; also ARM operations; every `az` command logged as an `AgentAzCliExecution` event in Application Insights (https://learn.microsoft.com/en-us/azure/sre-agent/execute-mitigations; https://learn.microsoft.com/en-us/azure/sre-agent/skills).
- VMs: only control-plane CLI examples (`az vm list`, `az vm resize`). **No VM Run Command, extension, SSH or in-guest execution is documented.**
- AKS: "Run Azure CLI and kubectl commands against public endpoints"; tool names `RunKubectlReadCommand` / `RunKubectlWriteCommand` (e.g. `kubectl get *`, `kubectl apply *`); private clusters need VNet integration. The Kubernetes auth mechanism (Azure RBAC for Kubernetes vs. command invoke) is **not stated**; the only AKS role named is AKS Cluster User (https://learn.microsoft.com/en-us/azure/sre-agent/faq; https://learn.microsoft.com/en-us/azure/sre-agent/tool-access-policies; https://learn.microsoft.com/en-us/azure/sre-agent/manage-permissions).
- Other tools: `RunInTerminal`, `RunShellCommand`, `ExecutePythonCode`, `RunPsqlReadCommand`, Kusto queries, MCP tools (https://learn.microsoft.com/en-us/azure/sre-agent/tool-access-policies).

### Approval / autonomy posture and guardrails
- Run modes **Review** (default; approval before Azure CLI/ARM write actions) and **Autonomous**; set per response plan and per scheduled task (whose table default is Autonomous); only SRE Agent Administrators approve (https://learn.microsoft.com/en-us/azure/sre-agent/run-modes).
- Default permission level is Reader ("can't take actions"); recommended scope progression: single resource → resource group → subscription (dev/test only) (https://learn.microsoft.com/en-us/azure/sre-agent/execute-mitigations).
- Hard guardrails: never runs `delete`/`remove`; blocks all `az keyvault` commands; checks Azure management locks (ReadOnly locks block regardless of mode) (https://learn.microsoft.com/en-us/azure/sre-agent/execute-mitigations).
- Tool access policies: allow/ask/deny per tool-glob with argument patterns (e.g. `bash(az * delete *)`); only the global scope (Admin) can deny; precedence Hooks > tool policies > connector defaults > run modes; a hook returning allow can override a global deny (audit-logged) (https://learn.microsoft.com/en-us/azure/sre-agent/tool-access-policies).

### Tenant isolation and data handling
"isolates your data by tenant and Azure subscription"; Microsoft engineers have no default access; Customer Lockbox not supported; conversation history, uploaded files, tool results stored in the selected agent region (some search-index regions differ); kept until you delete; not used to train models. Model providers: Anthropic (default outside EU/EFTA/UK, processed globally, outside EU Data Boundary) and Azure OpenAI (default in EU/EFTA/UK and sovereign clouds, inference inside EU Data Boundary for EU/EFTA deployments) (https://learn.microsoft.com/en-us/azure/sre-agent/data-privacy).

### Doc URLs used
permissions, manage-permissions, user-roles, roles-permissions-overview, agent-identity, connectors, connect-telemetry-source, setup-github-connector, create-and-set-up, network-integration, execute-mitigations, run-modes, tool-access-policies, skills, faq, overview, data-privacy — all under https://learn.microsoft.com/en-us/azure/sre-agent/.

### Not published
- Whether it can target resources in another Entra tenant / via Azure Lighthouse.
- The full resource-type → RBAC role table behind Reader and Privileged levels (VM, AKS, App Service roles).
- How kubectl authenticates to AKS (Azure RBAC for Kubernetes vs. `az aks command invoke` vs. kubeconfig) and how a private cluster is reached beyond "VNet integration".
- Any in-guest VM execution path (Run Command / extensions).
- OBO token lifetime and whether consent is per action.
- Which LLM model versions; compliance certifications.
- Internal doc inconsistencies: Reader-level default (one page says default, one doesn't), run-mode defaults (Review default vs. Autonomous per-plan table), who may configure the GitHub connector (Admin only vs. Admin or Standard User).

---

## 2a. AWS DevOps Agent

### Deployment form
An AWS-hosted service organised in **Agent Spaces** ("the primary security boundary"); "operates globally with regional processing capabilities"; data stored in the region of the Agent Space; inference via Bedrock within the geography (EU spaces use an EU cross-region inference profile) (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html).

### What is installed in the customer environment
Nothing on EC2 or EKS. Customer-side artefacts are IAM roles (primary account role, secondary account roles, web-app role, optional elevated role) and the service-linked role `AWSServiceRoleForAIDevOps`, which may create VPC Lattice resource gateways tagged `AWSAIDevOpsManaged` for private connections (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-devops-agent-iam-permissions.html; https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-limiting-agent-access-in-an-aws-account.html). For EKS, the AWS blog instructs creating an EKS **access entry** for the agent's primary role with the `AmazonAIOpsAssistantPolicy` access policy (https://aws.amazon.com/blogs/containers/diagnose-kubernetes-control-plane-performance-issues-with-aws-devops-agent/).

### Cloud identity model and permission names
- Agent assumes customer IAM roles trusting service principal `aidevops.amazonaws.com` (sts:AssumeRole, SetSourceIdentity, TagSession) with `aws:SourceAccount`/`aws:SourceArn` confused-deputy conditions (https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html).
- Managed policies: **AIDevOpsAgentAccessPolicy** (read-only investigation policy on the agent role), **AIDevOpsAgentActionsPolicy** (elevated role for directed actions; excludes `account:*, cognito-identity:*, iam:*, identitystore:*, organizations:*, ram:*, rolesanywhere:*, sso:*, sts:*`), **AIDevOpsAgentFullAccess** / **AIDevOpsAgentReadOnlyAccess** (console users), **AIDevOpsOperatorAppAccessPolicy** (operator web app, scoped by `aws:PrincipalTag/AgentSpaceId`) (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-devops-agent-iam-permissions.html; https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html).
- Service-side **permission guardrail**: a session policy applied to every session, ceiling = ReadOnlyAccess ∪ AIDevOpsAgentAccessPolicy + `athena:StartQueryExecution`, `athena:StopQueryExecution`, `kms:Decrypt`; effective permissions are the intersection with the role (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-limiting-agent-access-in-an-aws-account.html).
- Cross-account: primary account (where the Agent Space lives) plus secondary accounts, each with its own role trusting `aidevops.amazonaws.com` directly; registering one needs `iam:PassRole` with `iam:PassedToService=aidevops.amazonaws.com` (https://docs.aws.amazon.com/devopsagent/latest/userguide/configuring-integrations-and-knowledge-connecting-multiple-aws-accounts.html).
- Elevated role for directed actions is optional per account, registered as `agentElevatedRoleArn`, "the ceiling of what the agent can ever do… not a standing grant" (https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html).

### Network path
No VPC change needed to reach AWS resources (API calls). For privately hosted third-party tools: "private connections" via VPC Lattice resource gateways; for publicly hosted tools: allow published per-region source IP ranges; VPC endpoints only for customer→agent traffic such as webhooks (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html).

### How actions execute
Direct AWS SDK/API operations under the assumed role; tools are classified READ_ONLY / MUTATIVE / DESTRUCTIVE; "Directed actions" are mutating operations run under the elevated role on explicit operator request (https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html). Kubernetes: via the EKS access entry (read-only `get`/`list` through AmazonAIOpsAssistantPolicy) (https://aws.amazon.com/blogs/containers/diagnose-kubernetes-control-plane-performance-issues-with-aws-devops-agent/; https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/EKS-Integration.html). No SSM Run Command, CloudFormation or Lambda execution path is documented; ticket/support-case creation is the one write allowed by default (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html).

### Approval / autonomy posture and guardrails
Read-only by default; "Limited write capabilities – the tools … are not able to mutate resources, with the exception of opening tickets and support cases". Directed actions are "disabled by default and require explicit, layered opt-in plus per-action operator approval"; approvals happen only in chat, single-use or reusable for up to 4 hours; "never executes a mutating tool without approval". Guardrails refuse delete-class operations, permissions-boundary mutations and `iam:PassRole`-requiring operations by default. No named autonomy levels (https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html; https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html).

### Tenant isolation and data handling
"Agent Spaces maintain strict isolation"; encrypted at rest with AWS-managed keys and in transit; data not used to train models; deleted within 30 days of Agent Space deletion; PII is not filtered in summaries; inference within geography (https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html).

### Doc URLs used
- https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security.html
- https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-devops-agent-iam-permissions.html
- https://docs.aws.amazon.com/devopsagent/latest/userguide/aws-devops-agent-security-limiting-agent-access-in-an-aws-account.html
- https://docs.aws.amazon.com/devopsagent/latest/userguide/working-with-devops-agent-working-with-directed-actions.html
- https://docs.aws.amazon.com/devopsagent/latest/userguide/configuring-integrations-and-knowledge-connecting-multiple-aws-accounts.html
- https://aws.amazon.com/blogs/containers/diagnose-kubernetes-control-plane-performance-issues-with-aws-devops-agent/

### Not published
- Whether any directed action uses SSM Run Command / in-instance execution (no in-guest path documented at all).
- Kubernetes write path (EKS access policy is read-only; no `eks:MutateViaKubernetesApi` guidance).
- Whether it opens pull requests; CloudFormation/IaC execution.
- AWS Organizations-wide onboarding; list of service regions.
- Customer-managed KMS keys; retention beyond deletion; explicit multi-tenant isolation architecture.
- The "What is" landing page returned no content, so the formal Agent Space definition is from the security page only.

## 2b. Amazon CloudWatch investigations (the former Amazon Q Developer operational investigations)

### Deployment form
Account-level **investigation group** (one per account, 2 concurrent, 150 enhanced investigations/month) created in CloudWatch "AI Operations"; a no-config default mode uses the caller's own permissions and creates no resources (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations.html; https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-Security.html).

### What is installed in the customer environment
Nothing; for EKS, an access entry associating the EKS access policy `AmazonAIOpsAssistantPolicy` (get/list only across core, apps, batch, networking, storage, metrics APIs, incl. `pods/log`) with the investigation role (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/EKS-Integration.html).

### Cloud identity model and permission names
IAM role trusting `aiops.amazonaws.com` with recommended `aws:SourceAccount`/`aws:SourceArn` conditions; agent policy **AIOpsAssistantPolicy** (optionally plus ReadOnlyAccess, or a scoped-down custom policy), **AIOpsAssistantIncidentReportPolicy** (auto-added after 2025-10-10); user policies **AIOpsConsoleAdminPolicy**, **AIOpsOperatorAccess**, **AIOpsReadOnlyAccess**. Cross-account: monitoring account links up to 25 source accounts via `AIOps-CrossAccountInvestigationRole` in each source account, trust requiring `sts:ExternalId` = investigation-group ARN; policy `AIOpsAssistantCrossAccountPolicy-${guid}` (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-GetStarted-Group.html; https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-Security.html; https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-cross-account.html).

### Network path
No VPC/endpoint configuration documented.

### How actions execute
Read-only API scanning ("periodically scan resources in your account… regardless of whether there is an active investigation"); may *suggest* a Systems Manager Automation runbook; "Execute suggested runbook remediations" is listed as a capability (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations.html).

### Approval / autonomy posture and guardrails
User decides per suggestion to accept or discard; operators (AIOpsOperatorAccess) start investigations; CloudWatch alarms can create investigations; all actions logged to CloudTrail (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations.html; https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-Security.html).

### Tenant isolation and data handling
At rest with AWS-owned key or optional customer-managed symmetric KMS key; default-mode investigations deleted after 24 h; cross-Region inference may move data within the geography (EU→Frankfurt/Ireland/Paris/Stockholm) (https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-Security.html).

### Doc URLs used
- https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations.html
- https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-Security.html
- https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-GetStarted-Group.html
- https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Investigations-cross-account.html
- https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/EKS-Integration.html

### Not published
Who runs the suggested runbook and whether it can run automatically; default retention period; model-training statement; tenant-isolation statement; the exact contents of AIOpsAssistantPolicy; any rename statement from "Amazon Q Developer".

---

## 3. Google Cloud — Gemini Cloud Assist investigations (preview) and Proactive Mode

### Deployment form
Google-hosted service used from the Cloud console (Investigations page, Logs Explorer, Monitoring alerts, chat panel, mobile app), enabled per project via `geminicloudassist.googleapis.com` (also enables `cloudaicompanion.googleapis.com`) plus `cloudasset`, `designcenter`, `appoptimize`, `apphub` APIs; investigation results stored in the project (or App Hub management project) (https://docs.cloud.google.com/cloud-assist/set-up-gemini; https://docs.cloud.google.com/cloud-assist/investigations). Since 2026-04-10, creating/running/editing investigations requires a Premium Support contract or account-team access (https://docs.cloud.google.com/cloud-assist/create-investigation).

### What is installed in the customer environment
Nothing on VMs or GKE clusters; only API enablement and, for Proactive Mode, a provisioned **agent identity** (dedicated service account, principal `agents.global.org-ORG.system.id.goog/resources/geminicloudassist/projects/PROJECT/locations/global/agents/cloud`) (https://docs.cloud.google.com/cloud-assist/proactive-agents-setup).

### Cloud identity model and permission names
- Interactive: runs with the **caller's OAuth 2.0 credentials** — "the investigation's access is limited to the access that you have"; "The OAuth 2.0 token is never used for mutating data" (https://docs.cloud.google.com/cloud-assist/create-investigation; https://docs.cloud.google.com/cloud-assist/investigations).
- User roles: `roles/geminicloudassist.user` (+ `roles/cloudasset.viewer` recommended), `.viewer`, editor, admin; investigation roles `roles/geminicloudassist.investigationCreator`, `.investigationOwner` (auto-granted to creator), `.investigationViewer`, `.investigationEditor`, `.investigationAdmin` (https://docs.cloud.google.com/cloud-assist/iam-requirements; https://docs.cloud.google.com/gemini/docs/cloud-assist/manage-investigations).
- Proactive agent identity roles: `roles/iam.supportUser` and `roles/serviceusage.serviceUsageConsumer` (troubleshooting), plus `roles/cloudhub.operator` and `roles/appoptimize.admin` for cost optimisation; "limited to read-only access for services like Cloud Monitoring, Cloud Logging, and Cloud Asset Inventory" and cannot reach databases/buckets unless granted (https://docs.cloud.google.com/cloud-assist/proactive-agents-setup).
- MCP access requires `roles/geminicloudassist.user|editor|admin` and uses the user's Application Default Credentials (https://docs.cloud.google.com/cloud-assist/configure-mcp).

### Network path
None documented. Only note: VPC Service Controls perimeters break console setup; use gcloud and exclude `appoptimize.googleapis.com` (https://docs.cloud.google.com/cloud-assist/set-up-gemini).

### How actions execute
- Investigations are read-only analysis of logs, metrics, configurations and Cloud Asset Inventory; where remediation is supported, "Generate plan" produces `gcloud` commands or Kubernetes manifests and the user clicks **Run** to perform them (https://docs.cloud.google.com/cloud-assist/create-investigation).
- Via MCP (private preview): "Write operations in Gemini Cloud Assist can only be executed through the invocation of this tool" — `invoke_operation`, functional only when "Agent Actions are enabled"; the page's operation types are `GKE_APPLY`/`GKE_PATCH` (Kubernetes API), with the skill instructed to ask "Would you like to proceed with applying this configuration?" first (https://docs.cloud.google.com/cloud-assist/configure-mcp).
- No Compute Engine in-guest execution path is documented. Proactive agents emit Eventarc events `google.cloud.geminicloudassist.task.v1.completed`; any "automated remediation" is the customer's own Cloud Run / Workflows code wired to that event (https://docs.cloud.google.com/cloud-assist/proactive-agents-tasks).

### Approval / autonomy posture and guardrails
Human-in-the-loop by design: investigations recommend; Run is a user click; MCP writes require Agent Actions enablement plus confirmation; Proactive Mode must be explicitly enabled by a project administrator (Gemini Cloud Assist Admin), runs autonomously but read-only, with citations on every insight; investigations are scoped to a single project or App Hub application (https://docs.cloud.google.com/cloud-assist/proactive-agents-setup; https://docs.cloud.google.com/cloud-assist/investigations; https://docs.cloud.google.com/cloud-assist/overview).

### Tenant isolation and data handling
"Gemini Cloud Assist can store data in any Google Cloud data center globally" — not for projects with data residency or CMEK requirements; "By default, Google Cloud does not examine either the inputs or outputs associated with your investigations"; prompt/answer sharing is opt-in; proactive actions are audit-logged (https://docs.cloud.google.com/cloud-assist/set-up-gemini; https://docs.cloud.google.com/cloud-assist/investigations; https://docs.cloud.google.com/cloud-assist/overview).

### Doc URLs used
- https://docs.cloud.google.com/cloud-assist/iam-requirements
- https://docs.cloud.google.com/cloud-assist/investigations
- https://docs.cloud.google.com/cloud-assist/create-investigation
- https://docs.cloud.google.com/cloud-assist/proactive-agents-setup
- https://docs.cloud.google.com/cloud-assist/proactive-agents-tasks
- https://docs.cloud.google.com/cloud-assist/proactive-cost-optimization
- https://docs.cloud.google.com/cloud-assist/set-up-gemini
- https://docs.cloud.google.com/cloud-assist/overview
- https://docs.cloud.google.com/cloud-assist/configure-mcp
- https://docs.cloud.google.com/gemini/docs/cloud-assist/manage-investigations

### Not published
- What identity and permissions the "Run" button on a remediation plan uses, and whether it needs additional roles.
- Whether proactive agents can ever apply changes themselves (docs show only read-only roles and event emission).
- Explicit "not used for training" and tenant-isolation statements (the overview only links to the general Gemini for Google Cloud data-governance page).
- Retention periods for investigations.
- Any Compute Engine in-guest execution; how GKE writes authenticate beyond "Kubernetes APIs" under the user's credentials.
- Network requirements (none stated); the product marketing page could not be read.

---

## Cross-vendor summary
- **Azure** is the only one with a documented write path into Kubernetes (`RunKubectlWriteCommand`) and generic shell/CLI execution, runs under a managed identity the customer scopes per resource group, and has an OBO elevation fallback; nothing in-guest on VMs is documented.
- **AWS** keeps the agent read-only by a service-side session-policy ceiling, with mutating "directed actions" gated behind a separate elevated role plus per-action chat approval; Kubernetes access is a read-only EKS access entry.
- **Google** runs under the user's own token ("never used for mutating data"), with remediation plans executed only by a user click or via an opt-in MCP `invoke_operation` (GKE apply/patch), and background agents limited to read-only roles.
