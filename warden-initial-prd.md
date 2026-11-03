# Warden --- Initial Product Requirements Document (PRD)

**Document status:** Initial draft v0.1\
**Product name:** Warden (working name)\
**Product category:** Agentic Reliability Operations\
**Initial cloud providers:** Microsoft Azure and UpCloud\
**Primary product:** SRE / reliability monitoring and bounded first-line
remediation

------------------------------------------------------------------------

## 1. Executive Summary

Warden is a multi-tenant, agentic reliability operations platform that
helps engineering teams keep cloud-hosted services reliable without
requiring them to build and operate their own AI SRE platform.

Warden connects to customer cloud environments, discovers resources and
available telemetry, evaluates reliability signals, investigates
important anomalies, and produces evidence-backed findings. For a
defined set of low-risk first-line tasks, Warden may execute approved
remediation procedures within explicit permissions and safety
boundaries. It verifies outcomes and escalates complex, risky, or
uncertain cases to humans.

The initial prototype supports **Azure and UpCloud as first-class
providers** through a common, provider-neutral connector interface.
Neither provider should be treated as a temporary mock or designed as a
one-off integration.

Warden should consume existing cloud telemetry where possible rather
than attempting to replace a customer's observability stack.

## 2. Product Vision

**Make reliable cloud operations accessible to teams that do not have a
large, dedicated SRE function.**

Warden should evolve toward a dependable digital operations team: always
observing, investigating, handling routine operational work safely, and
involving humans when judgment or authority is required.

### Product principles

1.  **Outcome over chatbot:** Warden must perform useful operational
    workflows, not merely answer questions.
2.  **Proactive by default:** Detect and investigate relevant issues
    without waiting for a user prompt.
3.  **Evidence before conclusions:** Findings and diagnoses must be
    grounded in actual telemetry or resource state.
4.  **Bounded autonomy:** Agents may act only within explicit
    permissions, policies, and task boundaries.
5.  **Provider-neutral core:** Provider-specific behaviour belongs
    behind connector interfaces.
6.  **Human escalation:** High-risk, ambiguous, or failed tasks must be
    escalated clearly.
7.  **Tenant isolation by design:** Customer boundaries must be enforced
    by the platform, not by prompts.
8.  **Durable execution:** Workflows must survive worker restarts and
    must not depend on a model conversation being alive.
9.  **Incremental adoption:** Begin read-only; introduce narrowly scoped
    remediation only after it can be safely verified.
10. **Replaceable AI components:** Avoid coupling core product logic to
    one model provider or agent framework.

## 3. Problem Statement

Cloud teams often have monitoring tools and alerts but still need people
to correlate signals, identify likely causes, decide what to do, execute
routine fixes, and verify the result. Smaller teams may not have
dedicated SRE capacity. Existing monitoring can report symptoms without
completing the operational workflow.

Warden aims to reduce the manual effort between detecting a reliability
issue and reaching a verified resolution, while retaining human control
over consequential changes.

### Problem hypotheses to validate

-   Target customers spend meaningful engineering time investigating
    recurring operational issues.
-   Some first-line tasks are sufficiently repetitive and bounded to
    automate safely.
-   Customers value a managed service that operates across their cloud
    providers.
-   Evidence-backed findings and clear audit history increase trust in
    agent-led operations.
-   Customers will grant narrowly scoped cloud access if the
    permissions, controls, and value are clear.

These are hypotheses, not validated market facts. Customer discovery and
prototype testing must challenge them.

## 4. Target Customers and Users

### Initial target segment hypothesis

Small and mid-sized engineering organizations that run production
workloads on cloud infrastructure but lack the capacity for a mature,
dedicated SRE team.

### Primary personas

**Engineering / platform lead** - Wants to improve reliability without
adding significant operational overhead. - Needs visibility into service
health, recurring problems, and reliability risks. - Requires confidence
that automation will not make uncontrolled changes.

**DevOps / platform engineer** - Investigates alerts, resource issues,
deployment failures, and recurring incidents. - Needs evidence,
practical recommendations, and safe automation for repetitive tasks. -
Needs the ability to review actions and understand why they occurred.

**Service owner** - Needs to know whether a service is healthy, what
risks exist, and which actions matter most. - May not need to understand
every underlying cloud detail.

A later market may include managed service providers operating Warden
for multiple customer environments. Do not optimize the first prototype
for this segment until the initial customer and delivery model are
validated.

## 5. Value Proposition

**Connect your cloud environments. Warden continuously assesses
reliability, investigates problems, handles eligible first-line tasks
safely, and escalates the rest---with evidence for every important
conclusion and action.**

Potential differentiators to validate:

-   A managed reliability outcome rather than another tool customers
    must operate.
-   One operational model across Azure and UpCloud.
-   Proactive investigation, not just interactive incident Q&A.
-   Bounded first-line remediation with post-action verification.
-   Evidence-backed findings, transparent uncertainty, and auditable
    decisions.

"AI-powered", "multi-cloud", and "self-healing" alone are not sufficient
differentiators.

## 6. Product Scope

### 6.1 Prototype / MVP must include

1.  **Tenant and environment management**
    -   Create and manage tenants.
    -   Register multiple cloud connections per tenant.
    -   Keep all resources, credentials, jobs, evidence, and findings
        scoped to the correct tenant.
2.  **Azure connector**
    -   Discover supported resources and resource metadata.
    -   Retrieve supported Azure Monitor metrics and selected logs.
    -   Retrieve relevant resource health and activity signals where
        available.
    -   Use scoped, read-only identities for the initial observation
        phase.
3.  **UpCloud connector**
    -   Discover resources through supported UpCloud APIs.
    -   Retrieve available service/resource health and telemetry
        signals.
    -   Support logs and metrics only where the provider exposes them or
        a supported external telemetry source is configured.
    -   Document capability gaps explicitly; do not fabricate parity
        with Azure.
4.  **Common provider-neutral connector contract**
    -   Normalize common resource identity and metadata.
    -   Expose provider capabilities and unsupported operations.
    -   Normalize telemetry query results where practical.
    -   Preserve provider-specific identifiers and source links.
    -   Allow future providers to be added without rewriting the
        reliability engine.
5.  **Service inventory**
    -   Associate cloud resources with customer-facing or internal
        services.
    -   Allow users to set service criticality and ownership.
    -   Allow manual service grouping in the prototype; advanced
        dependency discovery is optional.
6.  **Reliability evaluation**
    -   Run deterministic health checks.
    -   Support initial SLI/SLO definitions and evaluations where the
        required telemetry exists.
    -   Record evaluation windows, inputs, and results.
    -   Do not ask an LLM to calculate authoritative SLO values when
        deterministic computation is possible.
7.  **Proactive monitoring and investigation**
    -   Support scheduled assessments.
    -   Support investigation triggered by configured alerts or detected
        threshold violations.
    -   Use agents to investigate signals through approved tools.
    -   Store workflow state durably outside model context.
8.  **Findings and evidence**
    -   Record severity, affected service/resources, observed facts,
        hypotheses, confidence, evidence references, recommended next
        steps, and status.
    -   Distinguish verified facts from inferred explanations.
    -   Link back to source telemetry or provider resources where
        possible.
9.  **First-line task handling**
    -   Define an explicit registry of supported operational tasks.
    -   Initially operate in observe/recommend mode.
    -   Add execution only for narrowly scoped, low-risk tasks with
        explicit policy, permission, preconditions, and verification.
    -   If safe remediation cannot be implemented and tested for the
        prototype, demonstrate the workflow in recommendation-only mode.
10. **Human escalation and notifications**
    -   Surface issues requiring approval or human investigation.
    -   Provide a basic web console and at least one notification path,
        such as email or webhook.
    -   Record why an issue was escalated.
11. **Governance and operations**
    -   Audit agent tool calls and consequential decisions.
    -   Enforce execution time, token/cost, retry, and concurrency
        limits.
    -   Support cancellation and timeout.
    -   Track run status, failures, and retries.

### 6.2 Explicitly out of scope for the initial prototype

-   Unrestricted autonomous production remediation.
-   Broad write permissions to customer cloud accounts.
-   Replacing Azure Monitor, UpCloud telemetry, or a general-purpose
    observability platform.
-   Full FinOps optimization, security posture management, compliance
    auditing, or general IT operations.
-   Full multi-cloud dependency graph and automatic topology
    reconstruction.
-   Support for every Azure or UpCloud resource and API.
-   A general-purpose agent marketplace.
-   Customer-defined arbitrary code execution.
-   Complex multi-region data lake or graph database unless validated
    requirements demand it.
-   Guaranteeing root cause or zero incidents.

## 7. Core User Journeys

### Journey A: Connect a cloud environment

1.  An authorized user creates a tenant and environment.
2.  The user establishes an Azure or UpCloud connection using the
    supported authentication method.
3.  Warden validates access and displays granted capabilities.
4.  Warden discovers supported resources.
5.  The user reviews discovered resources and groups them into services.
6.  Warden begins configured reliability evaluations.

**Acceptance criteria** - Connection status and last successful
validation are visible. - Missing permissions and unsupported
capabilities are clearly reported. - Credentials are not exposed in
logs, prompts, or the UI. - Discovery is tenant-scoped and auditable.

### Journey B: Detect and investigate a reliability issue

1.  A scheduled evaluation or alert identifies a relevant anomaly.
2.  Warden creates a durable investigation workflow.
3.  The investigation retrieves only authorized telemetry and resource
    data.
4.  The agent correlates signals and forms one or more hypotheses.
5.  A verification step checks that each material claim is supported by
    evidence.
6.  Warden creates a finding with impact, evidence, uncertainty, and
    next steps.
7.  The finding is shown in the console and notifications are sent
    according to policy.

**Acceptance criteria** - The investigation has a durable run ID and
lifecycle status. - Every material finding has evidence references or is
explicitly labelled as unverified. - The system can express
"insufficient evidence" rather than inventing a diagnosis. - A failed
investigation is visible and can be retried safely. - The workflow
remains isolated to the tenant and environment that initiated it.

### Journey C: Resolve an eligible first-line task

1.  Warden identifies a task from the supported task registry.
2.  Policy checks task type, tenant configuration, permissions, resource
    scope, preconditions, and risk.
3.  If approval is required, Warden requests it and waits.
4.  If policy permits automatic execution, the task runs with narrowly
    scoped credentials and bounded tools.
5.  Warden verifies the outcome using independent signals where
    possible.
6.  Warden records the action, evidence, result, and any follow-up.
7.  If verification fails or uncertainty exceeds the policy threshold,
    Warden stops and escalates.

**Acceptance criteria** - No agent can bypass the policy engine by
requesting a tool directly. - Every action has an initiating event,
policy decision, input parameters, result, and audit record. - Actions
are idempotent where possible or protected against duplicate
execution. - A failed or uncertain action is never reported as a
successful resolution. - Rollback or recovery instructions are
documented for any write-capable task.

## 8. Autonomy and Safety Model

Warden should use explicit autonomy levels, configurable by task and
environment.

-   **Level 0 --- Observe:** Collect signals and evaluate reliability.
-   **Level 1 --- Investigate:** Run read-only diagnostic workflows.
-   **Level 2 --- Recommend:** Propose actions for a human to perform.
-   **Level 3 --- Approved execution:** Execute an action after a human
    approves the specific action.
-   **Level 4 --- Bounded autonomous execution:** Automatically perform
    allowlisted low-risk actions when preconditions and policies are
    satisfied.

The prototype should establish Levels 0--2 first. Level 3 may be
implemented for a carefully selected task. Level 4 should be enabled
only for a task with test coverage, explicit policy, limited scope,
reliable verification, and a defined failure path.

### Required safety controls

-   Read-only cloud access by default.
-   Separate observation credentials from remediation credentials.
-   Least privilege and resource-level scoping where supported.
-   A deterministic policy/authorization service outside the LLM.
-   Tool allowlists and input validation.
-   Per-tenant and per-environment execution boundaries.
-   Restricted worker network egress.
-   Protection against prompt injection from logs, tags, resource names,
    tickets, and other customer-controlled data.
-   Approval gates for high-impact operations.
-   Rate limits, budgets, timeouts, retry limits, and cancellation.
-   Durable audit history for investigations, policy decisions,
    approvals, and actions.
-   Post-action verification and explicit escalation on uncertainty.

## 9. Functional Requirements

  -----------------------------------------------------------------------
  ID                      Requirement             Priority
  ----------------------- ----------------------- -----------------------
  FR-01                   Manage tenants and      P0
                          isolate tenant data and 
                          execution               

  FR-02                   Connect to Azure and    P0
                          discover supported      
                          resources               

  FR-03                   Connect to UpCloud and  P0
                          discover supported      
                          resources               

  FR-04                   Define a common         P0
                          connector contract with 
                          declared capabilities   

  FR-05                   Normalize common        P0
                          resource identity while 
                          retaining provider      
                          identifiers             

  FR-06                   Ingest/query supported  P0
                          metrics and selected    
                          telemetry               

  FR-07                   Configure services and  P0
                          criticality             

  FR-08                   Run deterministic       P0
                          reliability checks and  
                          supported SLO           
                          evaluations             

  FR-09                   Trigger investigations  P0
                          from schedules and      
                          configured alerts       

  FR-10                   Persist workflow state  P0
                          and support             
                          retries/timeouts        

  FR-11                   Produce evidence-backed P0
                          findings and            
                          investigation history   

  FR-12                   Expose status and       P0
                          findings in a basic web 
                          console                 

  FR-13                   Notify users through a  P1
                          basic integration       

  FR-14                   Support a registry of   P1
                          eligible first-line     
                          tasks                   

  FR-15                   Enforce policies and    P0 before any write
                          human approval for      action
                          consequential actions   

  FR-16                   Execute one narrowly    P1
                          scoped remediation      
                          safely, if feasible     

  FR-17                   Track usage, execution  P1
                          costs, and per-tenant   
                          budgets                 

  FR-18                   Add future providers    P1
                          without changing core   
                          workflow logic          
  -----------------------------------------------------------------------

## 10. Non-Functional Requirements

### Security

-   Tenant isolation is enforced in APIs, persistence, background jobs,
    caches, retrieval, and agent tools.
-   Secrets are stored in a suitable secret manager and never placed in
    model prompts.
-   Authentication and authorization decisions are made by deterministic
    application code.
-   All sensitive operations are authenticated, authorized, and audited.

### Reliability

-   Long-running investigations survive worker restarts.
-   Retries use bounded backoff and do not create duplicate actions.
-   Connector failures are isolated and surfaced as provider-specific
    errors.
-   The system degrades gracefully when telemetry is unavailable.

### Observability

-   Emit structured logs, metrics, and traces for platform workflows.
-   Track connector health, discovery duration, evaluation failures,
    investigation duration, model/tool usage, action outcomes, and
    notification delivery.
-   Correlate events using tenant, environment, service, workflow, and
    run identifiers without leaking secrets.

### Performance and cost

-   Establish baseline latency and cost budgets during prototype testing
    rather than inventing unsupported production SLAs.
-   Bound every agent run by time, tool calls, model usage, and
    concurrency.
-   Avoid sending large raw log volumes to an LLM; retrieve focused
    evidence windows and summarize deterministically where appropriate.

### Maintainability

-   Provider-specific logic stays inside connectors or provider
    adapters.
-   The core workflow depends on internal contracts, not provider SDK
    types.
-   Model providers and agent frameworks should be replaceable behind
    narrow interfaces.
-   APIs and internal contracts are versioned and tested.

## 11. Initial Architecture Direction

This is a starting hypothesis, not a mandate to use a specific
framework.

### Logical planes

**Control plane** - Tenant and identity management - Cloud connection
registry - Policies, approvals, and task registry - Scheduling and
orchestration - Product API, UI, and usage controls

**Execution plane** - Durable workflow engine - Isolated agent workers -
Specialist investigation tasks - Deterministic policy enforcement and
verification

**Data plane** - Resource inventory - Service and SLO configuration -
Evaluation results and findings - Workflow/action records and evidence
references - Telemetry access through provider APIs or
customer-configured telemetry sources

### Architectural flow

`Azure / UpCloud → Provider Connectors → Normalized Resource & Telemetry Contracts → Reliability Evaluation → Durable Investigation Workflow → Evidence Verification → Finding / Approved Action → Outcome Verification`

### Implementation guidance

-   Use a relational database for tenant, configuration, inventory, SLO,
    workflow metadata, findings, and evidence references.
-   Use a durable workflow engine or equivalent persistence-backed
    orchestration; do not rely on an agent conversation as the workflow
    state store.
-   Keep raw telemetry in the provider's or customer's telemetry system
    where possible.
-   Build a provider-neutral connector contract and contract tests
    against both Azure and UpCloud early.
-   Use a model gateway or adapter to avoid hard-coding a single model
    provider.
-   Keep authorization and policy evaluation deterministic and outside
    the model.
-   Do not introduce a graph database or custom telemetry platform
    without demonstrated need.

## 12. Initial Domain Model

Candidate core entities:

-   **Tenant** --- customer security and billing boundary.
-   **Environment** --- logical cloud environment such as production or
    staging.
-   **Connection** --- provider account/project and authentication
    configuration.
-   **ProviderCapability** --- operations and telemetry available
    through a connection.
-   **Resource** --- normalized resource record plus provider-native
    identifiers.
-   **Service** --- customer-defined grouping of resources with an owner
    and criticality.
-   **SLO** --- reliability objective and its measurement configuration.
-   **SLOEvaluation** --- deterministic evaluation result over a defined
    time window.
-   **Trigger** --- schedule, alert, or evaluation event that starts
    work.
-   **AgentRun** --- durable investigation or task execution.
-   **Finding** --- prioritized issue or risk with status and impact.
-   **EvidenceReference** --- pointer to supporting telemetry, events,
    resource state, or other source material.
-   **TaskDefinition** --- allowlisted operational task, permissions,
    preconditions, and verification rules.
-   **ActionRecord** --- approval, policy decision, execution result,
    and post-action verification.
-   **AuditEvent** --- immutable or tamper-evident record of
    consequential activity.

The prototype should avoid over-modelling before the initial workflows
are validated.

## 13. Connector Contract --- Initial Design Requirements

Both Azure and UpCloud connectors must implement the same conceptual
contract. Exact method signatures should be designed during
implementation.

Each connector should support, where the provider permits:

-   Validate connection and identity.
-   Discover resources within an authorized scope.
-   Retrieve a resource by normalized or provider-native identity.
-   Query metrics for a resource and time range.
-   Query logs or events where available.
-   Retrieve resource/service health signals where available.
-   Report supported capabilities and limitations.
-   Provide source references or URLs for evidence.
-   Return normalized errors for authorization, rate limiting,
    unavailable services, invalid queries, and transient failures.

### Connector design rules

-   Never assume that both providers offer identical telemetry or
    operations.
-   Capability discovery must be explicit and machine-readable.
-   Keep provider-specific identifiers, timestamps, units, and source
    links.
-   Normalize units and timestamps carefully; preserve original values
    where transformations occur.
-   Apply tenant and connection scopes before requests are made.
-   Include contract tests that run against fixtures and, where
    practical, sandbox/test accounts for both providers.
-   No connector should expose arbitrary cloud API access directly to an
    agent.

## 14. Findings and Evidence Format

Every finding should include, where applicable:

-   Finding ID and lifecycle status.
-   Tenant, environment, service, and affected resource references.
-   Severity and impact statement.
-   Detection time and relevant observation window.
-   Observed facts.
-   Hypotheses and supporting/contradicting evidence.
-   Confidence or uncertainty explanation.
-   Evidence references with timestamps and source links.
-   Recommended next step.
-   Whether an action is available, requires approval, or is prohibited.
-   Investigation run reference.
-   Resolution status and verification evidence, if action was taken.

Avoid presenting model-generated confidence as a calibrated probability
unless it has been validated. Use clear qualitative confidence
categories initially and explain their basis.

## 15. Success Metrics

The prototype should collect evidence for the following measures:

**Product value** - Time from trigger to useful finding. - Time to
diagnosis versus a documented human baseline. - Percentage of seeded
issues detected. - Percentage of findings supported by traceable
evidence. - Diagnostic correctness against known scenarios. -
False-positive and unsupported-claim rates. - Percentage of eligible
tasks successfully completed and verified. - Percentage of cases
correctly escalated instead of acted upon.

**Platform quality** - Connector success and failure rates by
provider. - Resource discovery coverage for the explicitly supported
resource set. - Investigation completion, timeout, retry, and
cancellation rates. - Cost per investigation and per tenant. - Duplicate
action rate. - Cross-tenant isolation test results.

**Commercial validation** - Customer willingness to connect a test
environment. - Customer trust in evidence and autonomy controls. -
Frequency and perceived value of findings. - Willingness to pay for
ongoing reliability operations.

Do not set numerical targets until test scenarios and a baseline are
agreed. Establish target thresholds before running the final prototype
evaluation to avoid choosing success criteria after seeing results.

## 16. Prototype Validation Plan

Use controlled scenarios in both Azure and UpCloud. Select only
scenarios supported by each provider's available signals.

For each scenario: 1. Define the known fault or reliability condition.
2. Define expected observable evidence. 3. Define what a correct
diagnosis and safe next step look like. 4. Run the same core workflow
against each provider where comparable. 5. Record detection, evidence
completeness, diagnostic correctness, execution cost, and time. 6. Test
unavailable telemetry, permission denial, rate limits, worker restart,
and malformed/untrusted log content. 7. If a remediation is tested, use
a disposable environment and verify both successful and failed outcomes.

Compare the same product workflow across both providers, while
acknowledging differences in telemetry and resource capabilities.

## 17. Key Risks and Open Questions

1.  **Market differentiation:** Will customers pay for managed
    reliability outcomes, or do they prefer features inside existing
    monitoring products?
2.  **Initial customer:** Small/mid-sized direct customers, platform
    engineering teams, or managed service providers?
3.  **Delivery model:** Fully managed SaaS, SRE copilot, or a hybrid of
    proactive service plus interactive investigation?
4.  **Telemetry availability:** Which signals can reliably be accessed
    from Azure and UpCloud, and which require external instrumentation?
5.  **Remediation scope:** Which single first-line task is sufficiently
    common, low-risk, reversible, and verifiable for an initial
    demonstration?
6.  **Customer trust:** What permissions, data handling, deployment, and
    residency controls are required?
7.  **Cost model:** How should usage, telemetry queries, model calls,
    and agent execution be budgeted?
8.  **Service definition:** Should users group resources into services
    manually at first, or is automated mapping necessary for the first
    useful outcome?
9.  **Alert integration:** Which alert sources should trigger
    investigations in the prototype?
10. **Product name:** Warden is a working name; trademark and domain
    availability have not been checked.

## 18. Suggested Delivery Milestones

### Milestone 1 --- Cross-provider foundation

-   Tenant and environment model.
-   Common connector contract.
-   Azure and UpCloud connection validation and resource discovery.
-   Capability reporting and contract tests.

### Milestone 2 --- Reliability baseline

-   Service grouping and criticality.
-   Selected deterministic health checks.
-   Initial SLI/SLO evaluation where data supports it.
-   Basic dashboard and findings persistence.

### Milestone 3 --- Durable investigations

-   Scheduled and event-triggered workflows.
-   Read-only agent tools.
-   Evidence-backed findings and verifier.
-   Execution history, retry, timeout, and cost controls.

### Milestone 4 --- Bounded first-line operations

-   Choose one low-risk task based on customer discovery.
-   Add deterministic policy, narrow permissions, preconditions, and
    verification.
-   Add approval or escalation paths.
-   Test failure and duplicate-execution scenarios in disposable
    environments.

### Milestone 5 --- Pilot validation

-   Run seeded scenarios on both providers.
-   Compare against a human baseline.
-   Review customer trust, perceived value, operating cost, and
    willingness to pay.
-   Decide whether to expand remediation, provider coverage, or product
    scope.

## 19. Instructions for the Implementation Agent

Use this PRD as an initial product direction, not as permission to
invent missing requirements or implement the entire roadmap in one pass.

Before major implementation: 1. Inspect the existing repository, project
conventions, tests, and documentation. 2. Identify assumptions and
conflicts with the current codebase. 3. Produce a concise implementation
plan mapped to the PRD and milestones. 4. Propose the common connector
interface and show how both Azure and UpCloud will implement it. 5.
Define the first supported resource and telemetry capabilities for each
provider based on verified APIs and available credentials. 6. Identify
security boundaries, threat scenarios, and tests before introducing any
write-capable operation. 7. Prefer a small vertical slice that works end
to end over broad scaffolding.

Implementation constraints: - Do not add autonomous production write
operations without explicit task scope and safety design. - Do not place
secrets in source code, fixtures, prompts, or logs. - Do not make
provider-specific assumptions in the core. - Do not use an LLM for
deterministic calculations or authorization decisions. - Do not claim a
finding is verified without supporting evidence. - Add tests for tenant
isolation, connector capabilities, workflow recovery, policy
enforcement, and evidence integrity. - Document unknowns and propose
options rather than silently choosing consequential behaviour.

## 20. Immediate Next Step

The first implementation decision should be a small, testable vertical
slice:

**Connect to one test Azure environment and one test UpCloud environment
→ discover a deliberately limited set of resources → normalize them
through the same contract → run one deterministic reliability check →
display a finding with source evidence.**

Once that works end to end, add durable agent investigation. Introduce a
first-line remediation only after selecting a real customer-relevant
task with a clear safety and verification story.
