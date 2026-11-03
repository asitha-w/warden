// Cells: the list with levels, and the cell page with the ladder of reach,
// the manual steps for the next level, the sensors, the telemetry and the hands.
import { el, Button, SeverityChip, StatusChip, CapabilityList } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { toast, ago } from '../shell.js';
import { go } from '../router.js';
import { accessChip, levelChip, levelName, channelChip, isCluster, maxLevel, egress, runtimeHost, steps, code } from '../connections.js';
import { markdown } from '../markdown.js';

function parseSelector(text) { const out = {}; text.split(',').map((p) => p.trim()).filter(Boolean).forEach((p) => { const i = p.indexOf('='); if (i > 0) out[p.slice(0, i).trim()] = p.slice(i + 1).trim(); }); return out; }
function selectorText(s) { return Object.keys(s || {}).map((k) => k + '=' + s[k]).join(', '); }
export function labelChips(labels) { return el('div', { class: 'policy', style: 'flex-wrap:wrap' }, Object.keys(labels || {}).map((k) => el('span', { text: k + '=' + labels[k] }))); }
function accessOf(c) { const conn = sel.conn(c.connectionId); return conn ? ((conn.catalogue || []).find((k) => k.id === c.id) || {}).access : ''; }

// channels draws the runtime and one isolated channel per cell.
function channels() {
  const s = store.state;
  const rows = s.cells.map((c) => {
    const conn = sel.conn(c.connectionId); const shared = sel.sharedWith(c);
    return el('div', { class: 'chan ' + (c.channel === 'online' || c.channel === 'connected' ? 'on' : '') }, [
      el('span', { class: 'line' }), el('a', { class: 'mono', href: '#cell/' + c.id, text: c.id }), levelChip(c), el('span', { class: 'sm muted', text: conn ? 'via ' + conn.name + (shared ? ` · identity shared with ${shared} other cell${shared === 1 ? '' : 's'}` : ' · dedicated identity') : 'no connection' }),
    ]);
  });
  return el('div', { class: 'card' }, [
    el('div', { class: 'bc-row', style: 'gap:24px;align-items:flex-start' }, [
      el('div', { class: 'runtime' }, [el('span', { class: 'label', text: 'Tenant runtime' }), el('b', { text: s.tenant ? s.tenant.runtime : '' }), el('span', { class: 'mono sm muted', text: s.tenant ? 'egress ' + s.tenant.egressIp : '' }), el('span', { class: 'sm muted', text: 'one isolated channel per cell' })]),
      el('div', { class: 'chans' }, rows.length ? rows : [el('span', { class: 'sm muted', text: 'No cells yet. Onboard them from a connection.' })]),
    ]),
  ]);
}

function handsText(c) { return { ssh: 'SSH from the egress address · Ansible catalogue', 'ssh-tunnel': 'SSH through the reverse tunnel · Ansible catalogue', 'kube-api': 'Kubernetes API in the permitted namespaces' }[c.hands] || 'none · provider actions only'; }

function cellRow(c) {
  const conn = sel.conn(c.connectionId);
  return el('tr', { class: 'row', onClick: () => go('cell/' + c.id) }, [
    el('td', null, [el('div', { class: 'mono', text: c.id }), el('div', { class: 'sm muted', text: c.kind })]),
    el('td', null, [channelChip(c)]),
    el('td', null, [levelChip(c), el('div', { class: 'sm muted', text: handsText(c) })]),
    el('td', null, [el('div', { class: 'sm', text: conn ? conn.name : '' }), accessChip(accessOf(c), c.provider)]),
    el('td', null, [labelChips(c.labels)]),
  ]);
}

export function pageCells() {
  const s = store.state;
  const pending = s.cells.filter((c) => c.probeCapable && c.level < maxLevel(c));
  const waiting = s.cells.some((c) => c.channel === 'waiting');
  return el('div', { class: 'page' }, [
    el('div', null, [el('h1', { class: 'display', text: 'Cells' }), el('p', { class: 'muted' }, ['A cell is one managed thing: a VM, a cluster, a server. Each has its own isolated channel in the runtime and a level of reach: Level 0 sees it from outside through the connection; the grant or API access raises it. ', el('a', { href: '#docs/model/levels-of-reach', class: 'doclink', text: 'How levels work' })])]),
    channels(),
    pending.length && !waiting ? el('div', { class: 'card', style: 'border-color:var(--brand)' }, [el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('div', { class: 'heading', text: `${pending.length} cell${pending.length === 1 ? '' : 's'} can be raised` }), el('div', { class: 'sm muted', text: 'Open each cell for its manual steps: ' + pending.map((c) => c.id + ' (' + levelName(c) + ')').join(', ') + '. Or let the demo run every step.' })]), Button({ variant: 'primary', label: 'Raise all cells (demo)', onClick: () => api.installProbe().catch((e) => toast(e.message)) })])]) : null,
    s.cells.length ? el('div', { class: 'tablewrap' }, [el('table', { class: 'table' }, [el('thead', null, [el('tr', null, [el('th', { text: 'Cell' }), el('th', { text: 'Channel' }), el('th', { text: 'Level · hands' }), el('th', { text: 'Connection' }), el('th', { text: 'Labels' })])]), el('tbody', null, s.cells.map(cellRow))])]) : el('div', { class: 'empty', text: 'No cells yet. Open a connection and onboard what it discovered.' }),
  ]);
}

// --- cell page ---

const SENSOR = /discovery|metrics|health|Activity|Audit|inventory|Kubernetes API reads|Managed Kubernetes/i;
const ACTION = /power|scale|writes|firewall/i;
const K8S_ONLY = /AKS|Kubernetes/i;
const HOST_ONLY = /VM |Server |Resource discovery/i;
const STATEMENT = /split|carries no write/i;

function sensorsCard(c, conn) {
  const k8s = isCluster(c);
  const caps = ((conn && conn.capabilities) || []).filter((k) => !STATEMENT.test(k.name)).filter((k) => k8s ? !HOST_ONLY.test(k.name) : !K8S_ONLY.test(k.name));
  const sensors = caps.filter((k) => SENSOR.test(k.name)).map((k) => ({ name: k.name, detail: k.detail, state: k.state }));
  const actions = caps.filter((k) => ACTION.test(k.name)).map((k) => ({ name: k.name, detail: k.detail, state: k.state }));
  return el('div', { class: 'card conn' }, [
    el('div', null, [el('div', { class: 'card-title', text: 'Level 0 · provider sensors and actions' }), el('div', { class: 'sm muted', text: 'What the connection gives from outside, nothing on the cell. Through ' + (conn ? conn.name : 'no connection') + '.' })]),
    CapabilityList({ items: sensors }),
    el('div', { class: 'label', text: 'Provider actions, under policy' }), CapabilityList({ items: actions }),
  ]);
}

function configText(c) {
  const t = c.telemetry; const lines = ['receivers:'];
  if (t.hostMetrics) lines.push('  hostmetrics: { scrapers: [cpu, memory, disk, filesystem, network' + (t.processes ? ', process' : '') + '] }');
  t.receivers.filter((r) => r.enabled).forEach((r) => lines.push(`  ${r.name}: {}`));
  const logs = t.logs.filter((l) => l.enabled).map((l) => l.name);
  if (logs.length) lines.push(isCluster(c) ? `  k8s_logs: { workloads: [${logs.join(', ')}] }` : `  filelog: { include: [${logs.join(', ')}] }`);
  if (t.otlp) lines.push('  otlp: { protocols: { grpc: { endpoint: 127.0.0.1:4317 } } }');
  lines.push('exporters:', `  otlp/warden: { endpoint: ${runtimeHost()}:443, auth: { authenticator: bearertokenauth/cell } }`);
  lines.push('extensions:', `  opamp: { server: { ws: { endpoint: wss://${runtimeHost()}/opamp } } }`, '  file_storage: { directory: /var/lib/otelcol/queue }');
  return lines.join('\n');
}

function telemetryCard(c) {
  const t = JSON.parse(JSON.stringify(c.telemetry));
  const sw = (label, get, set, detail) => el('label', { class: 'switch tel' }, [el('input', { type: 'checkbox', checked: get(), onChange: (e) => { set(e.target.checked); preview.textContent = configText({ ...c, telemetry: t }); } }), el('span', null, [el('span', { text: label }), detail ? el('span', { class: 'sm muted', text: ' · ' + detail }) : null])]);
  const preview = el('div', { class: 'code', text: configText(c) });
  const installed = c.level >= maxLevel(c);
  const body = el('div', { class: 'split' }, [
    el('div', { class: 'conn' }, [
      el('div', { class: 'label', text: 'Metrics' }),
      sw('Host metrics', () => t.hostMetrics, (v) => { t.hostMetrics = v; }, 'cpu, memory, disk, filesystem, network'),
      sw('Processes', () => t.processes, (v) => { t.processes = v; }, 'per-process cpu and memory'),
      ...t.receivers.map((r) => sw(r.name, () => r.enabled, (v) => { r.enabled = v; }, r.detail)),
      el('div', { class: 'label', style: 'margin-top:8px', text: 'Application telemetry' }),
      sw('Accept OTLP from the app', () => t.otlp, (v) => { t.otlp = v; }, 'traces and metrics pushed to the probe on the cell'),
    ]),
    el('div', { class: 'conn' }, [
      el('div', { class: 'label', text: isCluster(c) ? 'Logs · workloads' : 'Logs · files' }),
      ...t.logs.map((l) => sw(l.name, () => l.enabled, (v) => { l.enabled = v; })),
      el('div', { class: 'bc-row', style: 'margin-top:4px' }, [el('input', { id: 'new-log', placeholder: isCluster(c) ? 'workload name' : '/var/log/app.log', style: 'flex:1' }), Button({ label: 'Add', onClick: () => { const v = document.getElementById('new-log').value.trim(); if (!v) return; t.logs.push({ name: v, enabled: true }); api.setTelemetry(c.id, t).catch((e) => toast(e.message)); } })]),
    ]),
  ]);
  return el('div', { class: 'card conn' }, [
    el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('div', { class: 'card-title', text: (isCluster(c) ? 'Level 2' : 'Level 1') + ' · probe telemetry' }), el('div', { class: 'sm muted', text: installed ? 'What the probe on this cell collects. Saving pushes a new collector config. The investigator can only read what is enabled here.' : 'The probe is not on this cell yet. This is the configuration it will get.' })]), installed ? Button({ variant: 'primary', label: 'Save and push', onClick: () => api.setTelemetry(c.id, t).then(() => toast('Config pushed to ' + c.id)).catch((e) => toast(e.message)) }) : null]),
    body,
    el('details', { class: 'details' }, [el('summary', { text: 'Collector config the probe receives' }), preview]),
  ]);
}

// ladder lists the levels of reach for this kind with the current one marked.
function ladder(c) {
  const k8s = isCluster(c);
  const rows = k8s
    ? [[0, 'Connected', 'The connection covers the cluster.', 'Cluster and node pool state, versions, the activity or audit log; scale a pool, start, stop under policy.'],
      [1, 'API access', 'A kube identity for Warden and the API server allows the egress address.', 'Workloads, events, logs; rollout undo, restart, scale, exec in the permitted namespaces under policy.'],
      [2, 'Probe', 'The collector runs in the cluster.', 'Node and kubelet metrics, container logs with history, retained events.']]
    : [[0, 'Connected', 'The connection covers the VM.', 'State, hypervisor metrics, the activity or audit log; start, stop, restart under policy.'],
      [1, 'Granted', 'The Warden grant ran on the VM; port 22 allows the egress address or the tunnel is up.', 'In-guest metrics and logs through the probe; actions through SSH as an Ansible catalogue under policy.']];
  return el('div', { class: 'ladder' }, rows.map((r) => el('div', { class: 'rung ' + (r[0] < c.level ? 'done' : r[0] === c.level ? 'current' : 'next') }, [
    el('div', { class: 'rung-level' }, [el('span', { class: 'bc-pill ' + (r[0] <= c.level ? 'bc-pill-available' : 'bc-pill-none'), text: 'Level ' + r[0] })]),
    el('div', null, [el('div', { class: 'heading', text: r[1] + (r[0] === c.level ? ' · now' : '') }), el('div', { class: 'sm muted', text: r[2] }), el('div', { class: 'sm', text: r[3] })]),
  ])));
}

// nextStep renders the manual steps and the action for the level above the current one.
function nextStep(c, conn) {
  const k8s = isCluster(c); const az = c.provider === 'azure'; const ip = egress(); const host = runtimeHost();
  const waiting = c.channel === 'waiting';
  if (c.level >= maxLevel(c)) {
    return el('div', { class: 'conn' }, [el('div', { class: 'heading', text: 'At the highest level for this kind' }), el('div', { class: 'sm muted', text: 'Hands: ' + handsText(c) + '.' }),
      steps(k8s ? [{ title: 'Revoke API access', text: az ? 'Remove the Kubernetes role assignments or the egress address from the authorized ranges.' : 'Delete the warden-read and warden-write ServiceAccounts, or remove the egress address from the IP filter.', code: az ? `az aks update -g <rg> -n ${c.id} --api-server-authorized-ip-ranges <ranges without ${ip}>` : `kubectl -n warden-system delete sa warden-read warden-write` }, { title: 'Remove the probe', code: 'kubectl delete namespace warden-system' }]
        : [{ title: 'Revoke the grant on the host', text: 'Works without Warden.', code: 'sudo userdel -r warden && sudo rm -f /etc/ssh/sshd_config.d/90-warden.conf /etc/sudoers.d/90-warden && sudo systemctl reload ssh\nsudo systemctl disable --now warden-probe' }, { title: 'Or close the port', text: az ? 'Delete the NSG rule warden-ssh.' : 'Remove the firewall rule in the control panel.' }], 'Revoking')]);
  }
  if (!k8s) {
    const tunnel = el('input', { type: 'checkbox', id: 'grant-tunnel', checked: !!(c.grant && c.grant.tunnel) });
    const line = c.grant ? `curl -fsSL "https://${host}/grant/${c.id}?t=${c.grant.token}" | sudo sh${c.grant.tunnel ? ' -s -- --tunnel' : ''}` : `curl -fsSL "https://${host}/grant/${c.id}?t=<token issued below>" | sudo sh`;
    return el('div', { class: 'conn' }, [
      el('div', { class: 'heading', text: 'Raise to Level 1 · run the Warden grant' }),
      el('div', { class: 'sm muted', text: 'One line on the host installs the warden user, the SSH trust and the probe, then reports the host key back. Nothing Warden wrote runs on the host: OpenSSH, Ansible and the OpenTelemetry Collector.' }),
      steps([
        { title: 'Check the host', text: 'OpenSSH 8.2 or later, Python 3.9 or later, systemd, outbound 443 to ' + host + '. Docker CLI with the compose plugin 2.18 or later if containers are in the catalogue.', code: 'ssh -V; python3 --version; systemctl --version | head -1; curl -sI https://' + host + ' | head -1' },
        { title: 'Allow SSH from the tenant egress address', text: az ? 'An NSG rule on the VM or subnet, plus the host firewall if enabled. VMs in virtual networks created after March 2026 also need an explicit outbound route.' : 'Warden adds the server firewall rule when you issue the grant (POST /1.3/server/{uuid}/firewall_rule, never PUT). It takes one to two minutes. The default rule is yours: an allow rule only restricts when the default is drop.',
          code: az ? `az network nsg rule create -g ${c.resourceGroup || '<rg>'} --nsg-name <nsg> -n warden-ssh \\\n  --priority 300 --direction Inbound --access Allow --protocol Tcp \\\n  --source-address-prefixes ${ip}/32 --destination-port-ranges 22` : `curl -s -X POST https://api.upcloud.com/1.3/server/${c.id}/firewall_rule -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \\\n  -d '{"firewall_rule":{"direction":"in","action":"accept","family":"IPv4","protocol":"tcp","source_address_start":"${ip}","source_address_end":"${ip}","destination_port_start":"22","destination_port_end":"22","comment":"warden"}}'` },
        { title: 'Issue the grant and run it on the host as a user with sudo', text: [el('label', { class: 'switch', style: 'display:inline-flex' }, [tunnel, 'Cannot open port 22: use the reverse tunnel instead (the host dials out on 443)'])], code: line },
        { title: 'Wait for the grant to report back', text: 'The host key is pinned, the instance identity is checked against the provider, the probe dials in. The level turns to 1 by itself.' },
      ], 'Manual steps'),
      el('div', { class: 'bc-row' }, [c.grant && !waiting ? null : waiting ? el('span', { class: 'sm muted', text: 'Grant issued ' + ago(c.grant.issuedAt) + ' ago, waiting for the host to report back…' }) : Button({ variant: 'primary', label: 'Issue the grant', onClick: () => api.grant(c.id, { tunnel: tunnel.checked }).catch((e) => toast(e.message)) }), el('a', { class: 'doclink sm', href: '#docs/grant', text: 'Read about the grant' })]),
    ]);
  }
  if (c.level === 0) {
    const items = az ? [
      { title: 'The cluster has Entra integration with Azure RBAC for Kubernetes', code: `az aks show -g ${c.resourceGroup || '<rg>'} -n ${c.id} --query "aadProfile.enableAzureRbac"` },
      { title: 'Append the tenant egress address to the authorized IP ranges', text: 'Keep the existing ranges and the cluster\'s own egress IP. Takes up to two minutes. A private cluster needs the tunnel pod instead; see the Azure page.', code: `az aks update -g ${c.resourceGroup || '<rg>'} -n ${c.id} --api-server-authorized-ip-ranges <existing>,${ip}/32` },
      { title: 'Role assignments on the cluster resource', text: 'Read: AKS RBAC Reader, Cluster User Role and the Warden AKS reader custom role at cluster scope. Write: the Warden AKS namespace writer at each permitted namespace. Up to five minutes to propagate.', code: `CL=$(az aks show -g ${c.resourceGroup || '<rg>'} -n ${c.id} --query id -o tsv)\naz role assignment create --assignee-object-id ${conn && conn.objectId ? conn.objectId : '<object id>'} --assignee-principal-type ServicePrincipal --role "Azure Kubernetes Service RBAC Reader" --scope $CL\naz role assignment create --assignee-object-id ${conn && conn.objectId ? conn.objectId : '<object id>'} --assignee-principal-type ServicePrincipal --role "Azure Kubernetes Service Cluster User Role" --scope $CL\naz role assignment create --assignee-object-id ${conn && conn.objectId ? conn.objectId : '<object id>'} --assignee-principal-type ServicePrincipal --role "Warden AKS reader" --scope $CL\naz role assignment create --assignee-object-id ${conn && conn.objectId ? conn.objectId : '<object id>'} --assignee-principal-type ServicePrincipal --role "Warden AKS namespace writer" --scope $CL/namespaces/<namespace>` },
      { title: 'Enable API access', text: 'Warden fetches the user kubeconfig, exchanges its credential for an Entra token, lists nodes and namespaces, and sets the level to 1.' },
    ] : [
      { title: 'The egress address goes into the control plane IP filter', text: 'Warden reads the current list and appends. The list on this cluster is empty, which means open to the internet; after the change only the addresses below reach the API server. Confirm before enabling.', code: `PATCH https://api.upcloud.com/1.3/kubernetes/${c.id}\n{"cluster":{"control_plane_ip_filter":["${ip}/32"]}}` },
      { title: 'Warden fetches the admin kubeconfig once', text: 'GET /1.3/kubernetes/{uuid}/kubeconfig returns a client certificate for <cluster>-admin with no documented expiry; rotation is through UpCloud support. Warden records the fetch time.' },
      { title: 'Warden creates its own identities and discards the admin kubeconfig', text: 'Namespace warden-system, ServiceAccounts warden-read and warden-write, a read-only ClusterRole, a write Role per permitted namespace (pods/exec create, pods/log get), the probe\'s ClusterRole for Level 2, and a token Secret per ServiceAccount so you can revoke by deleting it.', code: 'kubectl -n warden-system get sa,secret' },
      { title: 'Enable API access', text: 'The level turns to 1. On Kubernetes 1.34 and later Warden offers UKS OIDC authentication instead of the admin kubeconfig.' },
    ];
    return el('div', { class: 'conn' }, [
      el('div', { class: 'heading', text: 'Raise to Level 1 · enable API access' }),
      steps(items, 'Manual steps'),
      el('div', { class: 'bc-row' }, [waiting ? el('span', { class: 'sm muted', text: 'Enabling API access…' }) : Button({ variant: 'primary', label: 'Enable API access', onClick: () => api.apiAccess(c.id).catch((e) => toast(e.message)) }), el('a', { class: 'doclink sm', href: az ? '#docs/azure/aks-level-1-api-access' : '#docs/upcloud/uks-level-1-api-access', text: 'Read the steps' })]),
    ]);
  }
  const manifest = `# Warden probe for ${c.id}, collector ${'0.137.0'}\napiVersion: v1\nkind: Namespace\nmetadata: { name: warden-system }\n---\napiVersion: v1\nkind: Secret\nmetadata: { name: warden-enrollment, namespace: warden-system }\nstringData: { token: bpt_<one-time token for ${c.id}> }\n---\napiVersion: rbac.authorization.k8s.io/v1\nkind: ClusterRole\nmetadata: { name: warden-probe }\nrules:\n- apiGroups: [""]\n  resources: [events, namespaces, nodes, nodes/spec, nodes/stats, pods, pods/status, replicationcontrollers, resourcequotas, services]\n  verbs: [get, list, watch]\n- apiGroups: [apps]\n  resources: [daemonsets, deployments, replicasets, statefulsets]\n  verbs: [get, list, watch]\n- apiGroups: [batch]\n  resources: [jobs, cronjobs]\n  verbs: [get, list, watch]\n- apiGroups: [autoscaling]\n  resources: [horizontalpodautoscalers]\n  verbs: [get, list, watch]\n---\n# ServiceAccount, ClusterRoleBinding, ConfigMap, DaemonSet (node collector) and Deployment (cluster collector, 1 replica) follow.\n`;
  return el('div', { class: 'conn' }, [
    el('div', { class: 'heading', text: 'Raise to Level 2 · install the probe' }),
    steps([
      { title: 'Nodes reach the runtime', text: 'Outbound 443 to ' + host + (az ? '. Pod Security or Azure Policy must allow hostPath mounts of /var/log/pods, /var/lib/docker/containers and / in warden-system.' : '. Private node groups need a running NAT gateway on the cluster\'s SDN network.') },
      az ? { title: 'Apply the manifest once with your own cluster-admin (default)', text: 'Keeps Warden\'s identities as they are. RBAC Admin cannot create the namespace and no Azure data action covers nodes/stats, so the alternative is a time-boxed RBAC Cluster Admin assignment for Warden.', code: `kubectl apply -f warden-probe-${c.id}.yaml`, download: { name: `warden-probe-${c.id}.yaml`, body: manifest } }
        : { title: 'Warden applies the manifest with warden-write', text: 'The ClusterRole and binding already exist from Level 1; only namespaced objects are created now. Or apply it yourself.', code: `kubectl apply -f warden-probe-${c.id}.yaml`, download: { name: `warden-probe-${c.id}.yaml`, body: manifest } },
      { title: 'Wait for the collectors to dial in', text: 'One per node plus the cluster collector. The level turns to 2.' },
    ], 'Manual steps'),
    el('div', { class: 'bc-row' }, [waiting ? el('span', { class: 'sm muted', text: 'Waiting for the collectors…' }) : Button({ variant: 'primary', label: az ? 'I applied it' : 'Let Warden apply it', onClick: () => api.cellProbe(c.id).catch((e) => toast(e.message)) }), el('a', { class: 'doclink sm', href: '#docs/probe', text: 'Read about the probe' })]),
  ]);
}

function reachCard(c, conn) {
  return el('div', { class: 'card conn' }, [
    el('div', null, [el('div', { class: 'card-title', text: 'Reach' }), el('div', { class: 'sm muted', text: 'How far Warden can see and act into this cell. Autonomy is set per responder, on top.' })]),
    ladder(c),
    nextStep(c, conn),
  ]);
}

function handsCard(c) {
  const k8s = isCluster(c);
  const items = c.hands === 'kube-api' ? [{ name: 'Kubernetes API', detail: 'rollout undo, restart, scale, delete a pod, exec, in the permitted namespaces', state: 'available' }, { name: 'Write identity', detail: c.provider === 'azure' ? 'Warden AKS namespace writer, no secrets' : 'ServiceAccount warden-write, per-namespace Role', state: 'available' }]
    : c.hands === 'ssh' || c.hands === 'ssh-tunnel' ? [{ name: c.hands === 'ssh' ? 'SSH 22 from ' + egress() : 'SSH through the reverse tunnel', detail: 'certificate per run, ten minutes, serial = run id', state: 'available' }, { name: 'Ansible catalogue', detail: 'check mode first, assert tasks verify; free-form shell only under ask', state: 'available' }, { name: 'warden user with full sudo', detail: 'Ansible cannot run under a command-restricted sudo; the catalogue and the policy gate limit what runs', state: 'note' }, { name: 'Host key pinned', detail: c.grant && c.grant.hostKey ? c.grant.hostKey : '', state: 'available' }]
    : [{ name: 'No hands yet', detail: k8s ? 'Enable API access to act inside the cluster.' : 'Run the Warden grant to act inside the host.', state: 'none' }, { name: 'Provider actions', detail: 'start, stop, restart through the connection, under policy', state: 'available' }];
  return el('div', { class: 'card conn' }, [el('div', null, [el('div', { class: 'card-title', text: 'Hands' }), el('div', { class: 'sm muted', text: 'How Warden can act inside this cell. Every action still passes the responder policy.' })]), CapabilityList({ items })]);
}

export function pageCell(id) {
  const s = store.state; const c = sel.cell(id);
  if (!c) return el('div', { class: 'page' }, [el('h1', { class: 'display', text: id }), el('div', { class: 'empty', text: 'Not a cell in this tenant. Onboard it from its connection.' })]);
  const conn = sel.conn(c.connectionId); const shared = sel.sharedWith(c);
  const input = el('input', { id: 'labels-' + c.id, value: selectorText(c.labels), style: 'width:100%' });
  const editor = el('div', { class: 'bc-row hidden', style: 'margin-top:6px' }, [input, Button({ variant: 'primary', label: 'Save labels', onClick: () => api.setLabels(c.id, parseSelector(input.value)).then(() => editor.classList.add('hidden')).catch((e) => toast(e.message)) })]);
  const chips = labelChips(c.labels); chips.style.cursor = 'pointer'; chips.title = 'Edit labels'; chips.addEventListener('click', () => editor.classList.toggle('hidden'));
  const head = el('div', null, [
    el('div', { class: 'bc-row', style: 'margin-bottom:8px' }, [channelChip(c), levelChip(c), accessChip(accessOf(c), c.provider), el('span', { class: 'badge', text: c.kind })]),
    el('h1', { class: 'display', text: c.id }),
    el('div', { class: 'sm muted', style: 'margin-top:4px', text: c.detail + ' · via ' + (conn ? conn.name : 'no connection') + (shared ? ` · identity shared with ${shared} other cell${shared === 1 ? '' : 's'}` : ' · dedicated identity') + ' · onboarded ' + ago(c.onboardedAt) + ' ago' }),
    el('div', { style: 'margin-top:10px' }, [chips, editor]),
  ]);
  const rbs = sel.cellsMatching ? s.runbooks.filter((rb) => sel.cellsMatching(rb.selector).some((x) => x.id === c.id)) : [];
  const findings = s.findings.filter((f) => f.resource === c.id);
  const rows = findings.map((f) => el('tr', { class: 'row', onClick: () => go('inv/' + f.runId) }, [el('td', null, [SeverityChip({ severity: f.severity })]), el('td', { text: f.title }), el('td', null, [StatusChip({ status: f.status })]), el('td', { class: 'mono muted', text: ago(f.createdAt) })]));
  return el('div', { class: 'page' }, [
    head,
    reachCard(c, conn),
    el('div', { class: 'grid2' }, [sensorsCard(c, conn), handsCard(c)]),
    telemetryCard(c),
    el('div', { class: 'grid2' }, [
      el('div', { class: 'card conn' }, [el('div', { class: 'card-title', text: 'Runbooks that apply' }), rbs.length ? el('div', { class: 'conn' }, rbs.map((rb) => el('details', { class: 'details' }, [el('summary', { text: rb.name + ' · ' + selectorText(rb.selector) }), markdown(rb.body)]))) : el('p', { class: 'sm muted', text: 'No runbook matches these labels yet.' }), el('div', null, [Button({ label: 'Runbooks', onClick: () => go('runbooks') })])]),
      el('div', { class: 'card conn' }, [el('div', { class: 'card-title', text: 'Findings on this cell' }), rows.length ? el('div', { class: 'tablewrap' }, [el('table', { class: 'table compact' }, [el('tbody', null, rows)])]) : el('p', { class: 'sm muted', text: 'Nothing yet.' }), el('div', { class: 'bc-row' }, [Button({ variant: 'danger', label: 'Detach cell', onClick: () => api.detachCell(c.id).then(() => go('cells')).catch((e) => toast(e.message)) })])]),
    ]),
  ]);
}
