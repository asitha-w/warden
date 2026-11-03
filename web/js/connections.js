// Connection forms and cards shared by setup and the Connections page. A
// connection is an identity at a scope; its catalogue is what it could manage.
// The forms carry the manual steps (consent, role assignments, tokens,
// permissions) so nothing has to be looked up elsewhere.
import { el, Button, StatusChip, CapabilityList } from './ds.js';
import { store, sel } from './store.js';
import { api } from './api.js';
import { toast } from './shell.js';

export function connChip(s) { return s === 'connected' ? StatusChip({ status: 'verified', label: 'Connected' }) : s === 'validating' ? StatusChip({ status: 'investigating', label: 'Validating' }) : StatusChip({ status: 'unverified', label: 'Not connected' }); }
export function accessChip(a, provider) {
  if (provider === 'upcloud' && a) return el('span', { class: 'bc-pill bc-pill-missing', text: 'all operations · no read/write split' });
  return a === 'write' ? el('span', { class: 'bc-pill bc-pill-missing', text: 'read + write' }) : a === 'read' ? el('span', { class: 'bc-pill bc-pill-available', text: 'read only' }) : el('span', { class: 'bc-pill bc-pill-none', text: 'no access' });
}
export function levelName(c) { if (c.level === 0) return 'Level 0 · connected'; if (isCluster(c)) return c.level === 1 ? 'Level 1 · API access' : 'Level 2 · probe'; return 'Level 1 · granted'; }
export function levelChip(c) { return el('span', { class: 'bc-pill ' + (c.level > 0 ? 'bc-pill-available' : 'bc-pill-none'), text: levelName(c) }); }
export function channelChip(c) { const m = { online: ['verified', 'Online'], connected: ['verified', 'Connected'], waiting: ['investigating', 'Waiting'], offline: ['unverified', 'Offline'] }[c.channel] || ['unverified', c.channel]; return StatusChip({ status: m[0], label: m[1] }); }
export function isCluster(c) { return /cluster/.test(c.kind || ''); }
export function maxLevel(c) { return isCluster(c) ? 2 : 1; }
export function egress() { const s = store.state; return s && s.tenant ? s.tenant.egressIp : '<tenant egress>'; }
export function runtimeHost() { const s = store.state; return (s && s.tenant ? s.tenant.runtime : 'runtime') + '.warden.example'; }

function field(id, label, input, hint) { return el('div', { class: 'field' }, [el('label', { for: id, text: label }), input, hint ? el('span', { class: 'sm muted', text: hint }) : null]); }
function select(id, options, value, onChange) { return el('select', { id, onChange }, options.map((o) => el('option', { value: o[0], text: o[1], selected: o[0] === value }))); }

// code renders a command block with a copy button.
export function code(text) {
  const pre = el('pre', { class: 'code', text });
  const btn = el('button', { class: 'copy', title: 'Copy', text: 'Copy', onClick: (e) => { e.preventDefault(); navigator.clipboard && navigator.clipboard.writeText(text).then(() => toast('Copied')); } });
  return el('div', { class: 'codewrap' }, [pre, btn]);
}
// steps renders numbered manual steps: each has a title, a line of text and optional code.
export function steps(items, title) {
  return el('details', { class: 'details steps', open: true }, [el('summary', { text: title || 'Manual steps' }),
    el('ol', { class: 'steps-list' }, items.filter(Boolean).map((st) => el('li', null, [el('div', { class: 'heading', text: st.title }), st.text ? el('div', { class: 'sm muted' }, typeof st.text === 'string' ? [st.text] : st.text) : null, st.code ? code(st.code) : null, st.download ? el('a', { class: 'doclink sm', href: 'data:application/json;charset=utf-8,' + encodeURIComponent(st.download.body), download: st.download.name, text: 'Download ' + st.download.name }) : null])))]);
}

const WARDEN_APP_ID = '0b3a6c1e-beac-4a11-9d0e-5e4c0ffee001';
function vmOperatorRole(sub) { return JSON.stringify({ Name: 'Warden VM operator', IsCustom: true, Description: 'Power actions Warden may take on VMs, under policy.', Actions: ['Microsoft.Compute/virtualMachines/start/action', 'Microsoft.Compute/virtualMachines/restart/action', 'Microsoft.Compute/virtualMachines/deallocate/action', 'Microsoft.Compute/virtualMachines/powerOff/action'], NotActions: [], DataActions: [], AssignableScopes: ['/subscriptions/' + sub] }, null, 2); }
function aksOperatorRole(sub) { return JSON.stringify({ Name: 'Warden AKS operator', IsCustom: true, Description: 'Node pool scale, start and stop. agentPools/write also allows upgrades and other pool changes; the approval shows the full diff.', Actions: ['Microsoft.ContainerService/managedClusters/agentPools/write', 'Microsoft.ContainerService/managedClusters/start/action', 'Microsoft.ContainerService/managedClusters/stop/action'], NotActions: [], DataActions: [], AssignableScopes: ['/subscriptions/' + sub] }, null, 2); }
function aksReaderRole(sub) { return JSON.stringify({ Name: 'Warden AKS reader', IsCustom: true, Description: 'Lets Warden list nodes; RBAC Reader does not.', Actions: [], NotActions: [], DataActions: ['Microsoft.ContainerService/managedClusters/nodes/read'], AssignableScopes: ['/subscriptions/' + sub] }, null, 2); }
function aksWriterRole(sub) { return JSON.stringify({ Name: 'Warden AKS namespace writer', IsCustom: true, Description: 'Rollout undo, restart, scale, pod delete and exec in the permitted namespaces. No secrets.', Actions: [], NotActions: [], DataActions: ['Microsoft.ContainerService/managedClusters/apps/deployments/read', 'Microsoft.ContainerService/managedClusters/apps/deployments/write', 'Microsoft.ContainerService/managedClusters/apps/replicasets/read', 'Microsoft.ContainerService/managedClusters/pods/read', 'Microsoft.ContainerService/managedClusters/pods/delete', 'Microsoft.ContainerService/managedClusters/pods/exec/action'], AssignableScopes: ['/subscriptions/' + sub] }, null, 2); }

// azureForm collects what a real Azure connection needs and shows every manual step next to it.
export function azureForm(opts = {}) {
  const dir = el('input', { id: 'az-dir', value: '2c1a7b90-3f4e-4d2a-9b1c-7e5f6a8d9c01', placeholder: 'Entra tenant id' });
  const sub = el('input', { id: 'az-sub', value: '7d2e9c44-1a5b-4f3c-8e6d-2b9a0c1f7e55', placeholder: 'Subscription id' });
  const clientId = el('input', { id: 'az-client', placeholder: 'Application (client) id' });
  const secret = el('input', { id: 'az-secret', type: 'password', placeholder: 'Client secret value', autocomplete: 'off' });
  const credHint = el('span', { class: 'sm muted' });
  const cred = select('az-cred', [['federated', 'Federated credential (no secret, recommended)'], ['certificate', 'Certificate'], ['secret', 'Client secret (fallback)']], 'federated', () => refresh());
  const byoBox = el('div', { class: 'form hidden', style: 'max-width:none' }, [field('az-client', 'Application (client) id', clientId), field('az-cred', 'Credential', cred), credHint, el('div', { class: 'field hidden', id: 'az-secret-box' }, [el('label', { for: 'az-secret', text: 'Client secret' }), secret])]);
  const consentBox = el('p', { class: 'sm muted', text: 'A tenant administrator grants consent by opening the link in step 1 below. Warden finds the resulting service principal when you test the connection.' });
  const auth = select('az-auth', [['consent', 'Consent to the Warden app (recommended)'], ['byo', 'Bring your own app registration']], 'consent', () => refresh());
  const readLevel = select('az-rl', [['resource-group', 'Resource group'], ['subscription', 'Whole subscription']], 'resource-group', () => refresh());
  const readTarget = el('input', { id: 'az-rt', value: 'rg-shop-prod', placeholder: 'resource group name', onInput: () => refresh() });
  const writeLevel = select('az-wl', [['resource-group', 'Resource group'], ['resource', 'Single resource'], ['none', 'None, read only']], 'resource-group', () => refresh());
  const writeTarget = el('input', { id: 'az-wt', value: 'rg-shop-prod', placeholder: 'resource group or resource id', onInput: () => refresh() });
  const name = el('input', { id: 'az-name', placeholder: 'Shop production (optional)' });
  const stepsBox = el('div');
  function scopeVar(level, target) { if (level === 'subscription') return '/subscriptions/$SUB'; if (level === 'resource') return target || '/subscriptions/$SUB'; return '/subscriptions/$SUB/resourceGroups/' + (target || '<rg>'); }
  function assign(role, scopeName) { return `az role assignment create --assignee-object-id "$OID" --assignee-principal-type ServicePrincipal --role "${role}" --scope "$${scopeName}"`; }
  function refresh() {
    const byo = auth.value === 'byo';
    byoBox.classList.toggle('hidden', !byo); consentBox.classList.toggle('hidden', byo);
    document.getElementById('az-secret-box') && document.getElementById('az-secret-box').classList.toggle('hidden', cred.value !== 'secret');
    credHint.textContent = cred.value === 'federated' ? `Add a federated credential on your app: issuer https://${runtimeHost()}/oidc, subject tenant:${store.state.tenant ? store.state.tenant.id : '<tenant>'}, audience api://AzureADTokenExchange. No secret is stored anywhere.` : cred.value === 'certificate' ? 'Warden generates a key pair in the tenant secret store and shows you the public certificate to upload to the app registration.' : 'Stored in the tenant secret store. Rotate it before it expires; federation avoids this.';
    readTarget.parentElement.classList.toggle('hidden', readLevel.value === 'subscription');
    writeTarget.parentElement.classList.toggle('hidden', writeLevel.value === 'none');
    const subId = sub.value.trim() || '<subscription>'; const dirId = dir.value.trim() || '<directory>';
    const appId = byo ? (clientId.value.trim() || '<client id>') : WARDEN_APP_ID;
    const vars = `SUB=${subId}\nOID=$(az ad sp show --id ${appId} --query id -o tsv)   # the service principal in your directory\nREAD_SCOPE=${scopeVar(readLevel.value, readTarget.value.trim())}` + (writeLevel.value !== 'none' ? `\nWRITE_SCOPE=${scopeVar(writeLevel.value, writeTarget.value.trim())}` : '');
    const items = [
      byo ? { title: 'Your app registration', text: `Application ${appId} in directory ${dirId}, with a ${cred.value} credential. Its service principal must exist in the directory.` }
        : { title: 'Grant admin consent', text: 'A tenant administrator opens this link, reads what the Warden app asks for, and accepts. That creates the Warden service principal in your directory. No secret changes hands and no resource access is granted by this step.', code: `https://login.microsoftonline.com/${dirId}/adminconsent?client_id=${WARDEN_APP_ID}` },
      { title: 'Set the variables once', text: 'In a shell signed in with az login as Owner, User Access Administrator or Role Based Access Control Administrator at the scope.', code: vars },
      { title: 'Assign the read roles', text: 'Reader covers discovery, platform metrics, the Activity Log and Resource Health. The two AKS roles and the custom reader role give Kubernetes reads on clusters in scope.', code: [`az role definition create --role-definition @warden-aks-reader.json`, assign('Reader', 'READ_SCOPE'), assign('Azure Kubernetes Service RBAC Reader', 'READ_SCOPE'), assign('Azure Kubernetes Service Cluster User Role', 'READ_SCOPE'), assign('Warden AKS reader', 'READ_SCOPE')].join('\n'), download: { name: 'warden-aks-reader.json', body: aksReaderRole(subId) } },
      writeLevel.value !== 'none' ? { title: 'Create and assign the write roles', text: 'Three custom roles with only the actions the catalogue uses, assigned at the write scope. The namespace writer goes on each permitted namespace of each cluster.', code: [`az role definition create --role-definition @warden-vm-operator.json`, `az role definition create --role-definition @warden-aks-operator.json`, `az role definition create --role-definition @warden-aks-namespace-writer.json`, assign('Warden VM operator', 'WRITE_SCOPE'), assign('Warden AKS operator', 'WRITE_SCOPE'), assign('Warden AKS namespace writer', 'WRITE_SCOPE/providers/Microsoft.ContainerService/managedClusters/<cluster>/namespaces/<namespace>')].join('\n'), download: { name: 'warden-roles.json', body: '[' + [vmOperatorRole(subId), aksOperatorRole(subId), aksWriterRole(subId)].join(',\n') + ']' } } : { title: 'No write roles', text: 'Read only: Warden recommends and never acts on Azure through this connection. Add a write scope later to enable actions.' },
      { title: 'Test and save the connection', text: 'Warden signs in, finds the service principal, lists the role assignments it holds, derives its capabilities, fills the catalogue, and refuses to save if the read identity carries write actions.' },
    ];
    stepsBox.replaceChildren(steps(items, 'Manual steps for this connection'));
  }
  const submit = () => {
    api.createConnection({ provider: 'azure', name: name.value, directory: dir.value.trim(), subscriptionId: sub.value.trim(), authMode: auth.value, clientId: clientId.value.trim(), credentialKind: auth.value === 'byo' ? cred.value : 'federated',
      readScope: { level: readLevel.value, target: readLevel.value === 'subscription' ? sub.value.trim() : readTarget.value.trim() }, writeScope: { level: writeLevel.value, target: writeLevel.value === 'none' ? '' : (writeLevel.value === 'subscription' ? sub.value.trim() : writeTarget.value.trim()) } })
      .then(() => opts.onSubmit && opts.onSubmit()).catch((e) => toast(e.message));
  };
  const form = el('div', { class: 'form', style: 'max-width:none' }, [
    el('div', { class: 'split' }, [field('az-dir', 'Directory (Entra tenant id)', dir), field('az-sub', 'Subscription id', sub, 'Role assignments live in a subscription.')]),
    field('az-auth', 'Identity', auth, 'Consent creates a service principal for Warden in your directory. Bring your own app when your policy requires it.'),
    consentBox, byoBox,
    el('div', { class: 'split' }, [
      el('div', { class: 'form' }, [el('span', { class: 'label', text: 'Read scope' }), field('az-rl', 'Level', readLevel), field('az-rt', 'Resource group', readTarget, 'Reader and the AKS read roles are assigned here. What Warden can see.')]),
      el('div', { class: 'form' }, [el('span', { class: 'label', text: 'Write scope' }), field('az-wl', 'Level', writeLevel), field('az-wt', 'Target', writeTarget, 'The custom operator roles are assigned here. What Warden can change; keep it narrow.')]),
    ]),
    stepsBox,
    field('az-name', 'Connection name', name),
    el('div', { class: 'bc-row' }, [Button({ variant: 'primary', label: 'Test and save connection', onClick: submit }), opts.onCancel ? Button({ label: 'Cancel', onClick: opts.onCancel }) : null, el('a', { class: 'doclink sm', href: '#docs/azure/the-connection', text: 'Read the Azure page' })]),
  ]);
  requestAnimationFrame(refresh);
  return form;
}

// upcloudForm collects the sub-account, its token and the permissions the main account granted.
export function upcloudForm(opts = {}) {
  const sub = el('input', { id: 'uc-sub', value: 'warden', placeholder: 'sub-account username' });
  const tok = el('input', { id: 'uc-token', type: 'password', value: 'ucat_01J9…redacted', autocomplete: 'off' });
  const days = el('input', { id: 'uc-days', type: 'number', min: '1', max: '365', value: '365', onInput: () => refresh() });
  const name = el('input', { id: 'uc-name', placeholder: 'Edge production (optional)' });
  const perms = [{ targetType: 'server', targetIdentifier: '*' }, { targetType: 'managed_kubernetes', targetIdentifier: '*' }];
  const permBox = el('div', { class: 'conn' });
  const stepsBox = el('div');
  function renderPerms() {
    permBox.replaceChildren(...perms.map((p, i) => el('div', { class: 'bc-row' }, [
      select('uc-pt-' + i, [['server', 'server'], ['managed_kubernetes', 'managed_kubernetes'], ['managed_database', 'managed_database']], p.targetType, (e) => { p.targetType = e.target.value; refresh(); }),
      el('input', { value: p.targetIdentifier, placeholder: 'UUID or *', style: 'flex:1', onInput: (e) => { p.targetIdentifier = e.target.value; refresh(); } }),
      Button({ label: 'Remove', onClick: () => { perms.splice(i, 1); renderPerms(); refresh(); } }),
    ])), el('div', null, [Button({ label: 'Add permission', onClick: () => { perms.push({ targetType: 'server', targetIdentifier: '*' }); renderPerms(); refresh(); } })]));
  }
  function refresh() {
    const exp = new Date(Date.now() + (parseInt(days.value, 10) || 365) * 86400000).toISOString().slice(0, 10);
    const grants = perms.map((p) => `curl -s -X POST https://api.upcloud.com/1.3/permission/grant -H "Authorization: Bearer <main account token>" \\\n  -H "Content-Type: application/json" -d '{"permission":{"target_type":"${p.targetType}","target_identifier":"${p.targetIdentifier || '*'}","user":"${sub.value.trim() || '<sub-account>'}"}}'`).join('\n');
    stepsBox.replaceChildren(steps([
      { title: 'Create a sub-account', text: 'In the control panel under People, add a sub-account for Warden. It starts with no access to existing resources.' },
      { title: 'Create an API token for it', text: `Tokens live at most 365 days; this one expires ${exp}. Pin it to the tenant egress address so it is useless elsewhere.`, code: `curl -s -X POST https://api.upcloud.com/1.3/account/tokens -u "${sub.value.trim() || '<sub-account>'}:<password>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"name":"warden","expires_at":"${exp}T00:00:00Z","allowed_ip_ranges":["${egress()}/32"]}'` },
      { title: 'Grant permissions from the main account', text: 'Only the main account can grant. A permission allows every operation on its target, read and write alike; the Warden policy gate is the only split.', code: grants },
      { title: 'Test and save the connection', text: 'Warden lists what the token can reach and fills the catalogue. It tracks the expiry and takes the connection offline with a finding before the token lapses.' },
    ], 'Manual steps for this connection'));
  }
  const submit = () => api.createConnection({ provider: 'upcloud', name: name.value, subAccount: sub.value.trim(), token: tok.value, tokenDays: parseInt(days.value, 10) || 365, permissions: perms.map((p) => ({ targetType: p.targetType, targetIdentifier: p.targetIdentifier || '*' })) })
    .then(() => opts.onSubmit && opts.onSubmit()).catch((e) => toast(e.message));
  renderPerms(); refresh();
  return el('div', { class: 'form', style: 'max-width:none' }, [
    el('div', { class: 'split' }, [field('uc-sub', 'Sub-account', sub), field('uc-days', 'Token validity, days', days, 'UpCloud API tokens live at most a year.')]),
    field('uc-token', 'API token', tok, 'Stored in the tenant secret store, pinned to the egress address ' + egress() + '.'),
    el('div', { class: 'field' }, [el('label', { text: 'Permissions granted by the main account' }), permBox, el('span', { class: 'sm muted', text: 'Per resource, no read and write split. server and managed_kubernetes with * cover everything Warden can manage.' })]),
    stepsBox,
    field('uc-name', 'Connection name', name),
    el('div', { class: 'bc-row' }, [Button({ variant: 'primary', label: 'Test and save connection', onClick: submit }), opts.onCancel ? Button({ label: 'Cancel', onClick: opts.onCancel }) : null, el('a', { class: 'doclink sm', href: '#docs/upcloud/the-connection', text: 'Read the UpCloud page' })]),
  ]);
}

function scopeText(sc) { if (!sc || !sc.level || sc.level === 'none') return 'none, read only'; return sc.level.replace('-', ' ') + ' ' + sc.target; }

// catalogue is the discovery grid: what the connection can manage, with a
// checkbox per supported entry and one button to onboard the selection.
export function catalogue(c) {
  const cat = c.catalogue || [];
  if (!cat.length) return el('p', { class: 'sm muted', text: 'Nothing discovered in scope.' });
  const boxes = {};
  const rows = cat.map((k) => {
    const cell = sel.cell(k.id); const can = k.supported && !cell;
    const box = el('input', { type: 'checkbox', checked: can, disabled: !can });
    if (can) boxes[k.id] = box;
    return el('tr', null, [el('td', null, [box]), el('td', { class: 'mono', text: k.id }), el('td', null, [el('div', { text: k.kind }), el('div', { class: 'sm muted', text: k.detail + (k.resourceGroup ? ' · ' + k.resourceGroup : '') })]), el('td', null, [accessChip(k.access, c.provider)]), el('td', null, [cell ? el('a', { href: '#cell/' + cell.id, class: 'bc-pill bc-pill-available', text: 'cell · ' + levelName(cell) }) : k.supported ? el('span', { class: 'bc-pill bc-pill-none', text: 'manageable' }) : el('span', { class: 'bc-pill bc-pill-none', text: 'not supported yet' })])]);
  });
  const onboard = () => { const ids = Object.keys(boxes).filter((id) => boxes[id].checked); if (!ids.length) return toast('Select something to onboard'); api.onboard(c.id, ids).catch((e) => toast(e.message)); };
  const n = Object.keys(boxes).length;
  return el('div', { class: 'conn' }, [
    el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('span', { class: 'heading', text: 'Discovered in scope' }), n ? Button({ variant: 'primary', label: 'Onboard selected as cells', onClick: onboard }) : el('span', { class: 'sm muted', text: 'Everything manageable is onboarded' })]),
    el('div', { class: 'tablewrap' }, [el('table', { class: 'table compact' }, [el('thead', null, [el('tr', null, [el('th'), el('th', { text: 'Id' }), el('th', { text: 'Kind' }), el('th', { text: 'Access' }), el('th', { text: 'State' })])]), el('tbody', null, rows)])]),
  ]);
}

// connectionCard shows one validated connection: identity, scopes, roles, capabilities, catalogue.
export function connectionCard(c, opts = {}) {
  const s = store.state;
  const head = el('div', { class: 'conn-head' }, [el('span', { class: 'name', text: c.name }), connChip(c.status), el('span', { class: 'spacer', style: 'flex:1' }), opts.removable ? Button({ label: 'Remove', onClick: () => api.deleteConnection(c.id).catch((e) => toast(e.message)) }) : null]);
  if (c.status === 'validating') return el('div', { class: 'card conn' }, [head, el('p', { class: 'muted', text: c.provider === 'azure' ? 'Validating identity in the directory, listing role assignments at each scope, discovering what is in scope…' : 'Validating the token, reading its permissions, listing servers and clusters…' })]);
  const kv = c.provider === 'azure'
    ? [['Directory', c.directory], ['Subscription', c.subscriptionId], ['Identity', c.identity], ['Object id', c.objectId], ['Read scope', scopeText(c.readScope)], ['Write scope', scopeText(c.writeScope)]]
    : [['Identity', c.identity], ['Permissions', (c.permissions || []).map((p) => p.targetType + ' ' + p.targetIdentifier).join(', ')], ['Token expires', c.tokenExpiresAt ? new Date(c.tokenExpiresAt).toLocaleDateString() : '']];
  if (s.tenant) kv.push(['Egress', s.tenant.egressIp + ' (static)']);
  const details = el('details', { class: 'details' }, [el('summary', { text: 'Roles and capabilities' }),
    c.roles && c.roles.length ? el('div', { class: 'tablewrap', style: 'margin:8px 0' }, [el('table', { class: 'table compact' }, [el('thead', null, [el('tr', null, [el('th', { text: 'Identity' }), el('th', { text: 'Role' }), el('th', { text: 'Scope' })])]), el('tbody', null, c.roles.map((r) => el('tr', null, [el('td', null, [el('span', { class: 'bc-pill ' + (r.identity === 'write' ? 'bc-pill-missing' : 'bc-pill-available'), text: r.identity })]), el('td', { class: 'sm', text: r.role }), el('td', { class: 'mono sm', text: scopeText(r.scope) })])))])]) : null,
    CapabilityList({ items: c.capabilities || [] })]);
  return el('div', { class: 'card conn' }, [head, el('dl', { class: 'kv' }, kv.flatMap((p) => [el('dt', { text: p[0] }), el('dd', { class: ['Directory', 'Subscription', 'Object id', 'Egress'].includes(p[0]) ? 'mono' : '', text: p[1] || '' })])), opts.catalogue === false ? null : catalogue(c), details]);
}
