// Home: the orchestrator beside the setup panel. The operator creates the
// tenant and adds a credential in a form; everything after that is the
// orchestrator's. The panel renders the backend's setup overview (three
// bands: tenant, connections, cells) and lights the band that holds the next
// step. Every action in the panel is either a form only a human can fill or
// a message to the orchestrator, so the agent stays in the loop.
import { el, Button, StatusChip } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { toast, ago } from '../shell.js';
import { toolLine } from './investigations.js';
import { azureForm, upcloudForm, connChip, levelChip, channelChip, code, isCluster } from '../connections.js';
import { markdown } from '../markdown.js';
import { icon } from '../icons.js';

let formOpen = null; // 'azure' | 'upcloud' | null, survives re-renders
export function openForm(name) { formOpen = name; rerender(); }

function say(q) { api.chat(q).catch((e) => toast(e.message)); }

// orchText renders an orchestrator answer: command lines become code with a
// copy button, the rest is light Markdown.
function orchText(text) {
  const out = []; let buf = [];
  const flush = () => { if (buf.length) { out.push(markdown(buf.join('\n'))); buf = []; } };
  (text || '').split('\n').forEach((line) => {
    if (/^(curl|kubectl|az|sudo|ssh|helm)\s/.test(line.trim())) { flush(); out.push(code(line.trim())); } else buf.push(line);
  });
  flush();
  return out;
}

function thread() {
  const s = store.state; const userName = s.user ? s.user.name : 'you';
  const items = s.chat.map((m) => {
    if (m.role === 'tool') { const g = el('div', { class: 'tools' }); toolLine(m.tool).forEach((n) => g.appendChild(n)); return el('div', { class: 'msg' }, [el('div', { class: 'who orch', text: 'W' }), el('div', { class: 'body2' }, [el('div', { class: 'meta' }, [el('b', { text: 'Warden' }), el('span', { text: 'tool' })]), g])]); }
    const isUser = m.role === 'user';
    const body = isUser ? [el('p', { 'data-msg': m.id, text: m.text || '…' })] : [el('div', { 'data-msg': m.id, class: 'orchtext' }, m.text ? orchText(m.text) : [el('p', { text: '…' })])];
    return el('div', { class: 'msg' }, [el('div', { class: 'who ' + (isUser ? 'user' : 'orch'), text: isUser ? userName.slice(0, 1).toUpperCase() : 'W' }), el('div', { class: 'body2' }, [el('div', { class: 'meta' }, [el('b', { text: isUser ? userName : 'Warden' })]), ...body])]);
  });
  const empty = el('div', { class: 'empty' }, [el('div', { class: 'heading', text: 'Warden speaks first' }), el('p', { class: 'sm muted', text: 'Add a cloud provider credential in the panel and Warden tells you what it found and what comes next.' })]);
  const input = el('input', { id: 'home-input', placeholder: 'Ask Warden, or tell it what to do…', autocomplete: 'off' });
  const form = el('form', { onSubmit: (e) => { e.preventDefault(); const q = input.value.trim(); if (!q) return; input.value = ''; say(q); } }, [el('div', { class: 'ask' }, [input, Button({ variant: 'primary', label: 'Send', type: 'submit' })])]);
  const node = el('div', { class: 'card lg chat home-chat' }, [el('div', { class: 'thread', id: 'home-thread' }, items.length ? items : [empty]), form]);
  requestAnimationFrame(() => { const t = document.getElementById('home-thread'); if (t) t.scrollTop = t.scrollHeight; });
  return node;
}

// band is one of the three setup bands. hot marks the band with the next step.
function band(title, hot, children, hint) {
  return el('section', { class: 'band' + (hot ? ' hot' : '') }, [
    el('div', { class: 'band-head' }, [el('span', { class: 'heading', text: title }), hot ? el('span', { class: 'band-next', text: 'Next' }) : null]),
    hint ? el('p', { class: 'sm muted', text: hint }) : null,
    ...children,
  ]);
}

function tenantBand(o) {
  const t = store.state.tenant;
  return band('Tenant', o.stage === 'tenant', [
    el('dl', { class: 'kv' }, [el('dt', { text: 'Name' }), el('dd', { text: t.name }), el('dt', { text: 'Runtime' }), el('dd', { class: 'mono', text: o.runtimeHost || '' }), el('dt', { text: 'Egress' }), el('dd', { class: 'mono', text: (o.egressIp || '') + ' (static, allowlist this)' })]),
  ]);
}

function connectionsBand(o) {
  const s = store.state;
  const rows = s.connections.map((c) => {
    const cs = (o.connections || []).find((x) => x.id === c.id) || {};
    return el('div', { class: 'band-row' }, [
      el('div', { class: 'band-main' }, [el('div', { class: 'bc-row' }, [el('b', { text: c.name }), connChip(c.status)]), el('div', { class: 'sm muted', text: c.status === 'validating' ? 'Validating identity and discovering what is in scope…' : `Found ${cs.found || 0} · manageable ${cs.manageable || 0} · onboarded ${cs.onboarded || 0}` })]),
      c.status === 'connected' && (cs.onboarded || 0) < (cs.manageable || 0) ? Button({ variant: 'primary', label: 'Ask Warden to onboard', onClick: () => say('onboard everything from ' + c.id) }) : null,
    ]);
  });
  const adders = el('div', { class: 'bc-row' }, [
    Button({ variant: formOpen ? 'quiet' : 'primary', label: 'Add Azure credential', onClick: () => { formOpen = formOpen === 'azure' ? null : 'azure'; rerender(); } }),
    Button({ variant: formOpen ? 'quiet' : 'primary', label: 'Add UpCloud token', onClick: () => { formOpen = formOpen === 'upcloud' ? null : 'upcloud'; rerender(); } }),
  ]);
  const close = () => { formOpen = null; rerender(); };
  const form = formOpen === 'azure' ? el('div', { class: 'band-form' }, [azureForm({ onCancel: close, onSubmit: close })]) : formOpen === 'upcloud' ? el('div', { class: 'band-form' }, [upcloudForm({ onCancel: close, onSubmit: close, simple: true })]) : null;
  const hot = o.stage === 'connection' || o.stage === 'onboard';
  return band('Connections', hot, [...rows, adders, form, el('p', { class: 'sm muted', text: 'The credential goes in this form and nowhere else. Warden never sees it in the chat.' })],
    !s.connections.length ? 'A credential is an identity at a scope on a provider. Warden discovers what it covers; nothing is managed until Warden onboards it.' : null);
}

function cellsBand(o) {
  const s = store.state;
  if (!s.cells.length) return band('Cells', false, [el('p', { class: 'sm muted', text: 'Cells appear here when Warden onboards what a connection found. A cell is one VM, one server or one cluster.' })]);
  const pending = sel.pendingApprovals().filter((a) => !a.runId);
  const rows = s.cells.map((c) => {
    const cs = (o.cells || []).find((x) => x.id === c.id) || {};
    const next = cs.next; const ap = pending.find((a) => a.args && a.args.cell === c.id);
    const actions = [];
    if (ap) actions.push(el('div', { class: 'band-approval' }, [el('div', { class: 'sm' }, [el('b', { text: ap.task }), ' · ', ap.blastRadius]), el('div', { class: 'bc-row' }, [Button({ variant: 'primary', label: 'Approve', onClick: () => api.decide(ap.id, 'approve').catch((e) => toast(e.message)) }), Button({ label: 'Deny', onClick: () => api.decide(ap.id, 'deny').catch((e) => toast(e.message)) })])]));
    else if (next && next.kind === 'grant' && c.grant && c.channel === 'waiting') actions.push(el('div', null, [el('div', { class: 'sm muted', text: 'Run this on the host as a user with sudo. The level rises when it reports back.' }), code(next.detail.replace(/^.*Run on the host: /, ''))]));
    else if (next && next.kind === 'grant') actions.push(Button({ variant: 'primary', label: 'Ask Warden for the grant', onClick: () => say('grant ' + c.id) }));
    else if (next && next.kind === 'api-access') actions.push(Button({ variant: 'primary', label: 'Ask Warden for API access', onClick: () => say('enable api access on ' + c.id) }));
    else if (next && next.kind === 'probe') actions.push(Button({ variant: 'primary', label: 'Ask Warden to install the probe', onClick: () => say('install the probe on ' + c.id) }));
    return el('div', { class: 'band-row' + (next && o.next && o.next[0] && o.next[0].target === c.id ? ' hot' : '') }, [
      el('div', { class: 'band-main' }, [el('div', { class: 'bc-row' }, [el('a', { href: '#cell/' + c.id, class: 'mono', text: c.id }), levelChip(c), channelChip(c)]), el('div', { class: 'sm muted', text: c.kind + (next ? ' · next: ' + next.title : ' · at its highest level') })]),
      ...actions,
    ]);
  });
  return band('Cells', o.stage === 'reach', rows, null);
}

function approvalsBand() {
  const pending = sel.pendingApprovals();
  if (!pending.length) return null;
  return band('Needs you', true, pending.map((ap) => el('div', { class: 'band-approval' }, [
    el('div', { class: 'sm' }, [el('b', { text: ap.task }), ap.command ? el('div', { class: 'mono sm muted', text: ap.command }) : null, el('div', { class: 'sm muted', text: ap.blastRadius || '' })]),
    el('div', { class: 'bc-row' }, [Button({ variant: 'primary', label: ap.runId ? 'Approve fix' : 'Approve', onClick: () => api.decide(ap.id, 'approve').catch((e) => toast(e.message)) }), Button({ label: 'Deny', onClick: () => api.decide(ap.id, 'deny').catch((e) => toast(e.message)) }), ap.runId ? el('a', { href: '#inv/' + ap.runId, class: 'doclink sm', text: 'Open the run' }) : null]),
  ])), null);
}

function runbooksBand() {
  const s = store.state;
  const rows = s.runbooks.map((rb) => el('div', { class: 'band-row' }, [el('div', { class: 'band-main' }, [el('div', { class: 'bc-row' }, [el('a', { href: '#runbooks', text: rb.name }), el('span', { class: 'sm muted', text: Object.keys(rb.selector || {}).map((k) => k + '=' + rb.selector[k]).join(', ') })]), el('div', { class: 'sm muted', text: `applies to ${sel.cellsMatching(rb.selector).length} cell(s) · by ${rb.updatedBy || 'default'}` })])]));
  return band('Runbooks', false, rows, 'What an app is and how to handle it. Say "add a runbook for edge-01" and Warden writes one from your words.');
}

function workflowsBand() {
  const s = store.state;
  const rows = (s.workflows || []).map((wf) => el('div', { class: 'band-row' + (wf.fired ? ' hot' : '') }, [
    el('div', { class: 'band-main' }, [
      el('div', { class: 'bc-row' }, [el('b', { text: wf.name }), el('span', { class: 'bc-pill ' + (wf.enabled ? 'bc-pill-available' : 'bc-pill-none'), text: wf.enabled ? 'every ' + wf.every + 's' : 'paused' }), wf.fired ? el('span', { class: 'bc-pill bc-pill-missing', text: 'fired ' + wf.fired }) : null]),
      el('div', { class: 'sm muted', text: `${wf.collect} on ${Object.keys(wf.selector || {}).map((k) => k + '=' + wf.selector[k]).join(', ')} · ${wf.condition} · ${wf.trigger === 'wake-warden' ? 'wakes Warden' : 'starts ' + wf.trigger.replace('responder:', '')}` }),
      wf.allow && wf.allow.length ? el('div', { class: 'sm', style: 'color:var(--brand)', text: 'Standing order: ' + wf.allow.join(', ') + ' runs without asking' }) : null,
      wf.lastResult ? el('div', { class: 'sm muted mono', text: 'last: ' + wf.lastResult }) : null,
    ]),
    el('div', { class: 'bc-row' }, [Button({ label: wf.enabled ? 'Pause' : 'Resume', onClick: () => api.workflow(wf.id, { enabled: !wf.enabled }).catch((e) => toast(e.message)) }), wf.createdBy !== 'default' ? Button({ label: 'Remove', onClick: () => api.deleteWorkflow(wf.id).catch((e) => toast(e.message)) }) : null]),
  ]));
  return band('Watches', false, rows, 'Loops that run in the runtime and cost no tokens; a model runs only when one fires. A watch with a pre-approved action is a standing order. Say "watch the logs on edge-01 and wake me on errors" or "add disk alerts for production and rotate the logs, pre-approved".');
}

function agentsBand() {
  const s = store.state;
  const rows = s.agents.filter((a) => a.enabled).map((a) => el('div', { class: 'band-row' }, [el('div', { class: 'band-main' }, [el('div', { class: 'bc-row' }, [el('a', { href: '#agents', text: a.name }), el('span', { class: 'sm muted', text: a.kind === 'deterministic' ? 'no model' : a.kind })]), el('div', { class: 'sm muted', text: a.role + (a.scope && Object.keys(a.scope).length ? ' · scope ' + Object.keys(a.scope).map((k) => k + '=' + a.scope[k]).join(', ') : '') })])]));
  return band('Agents', false, rows, 'Warden starts these; it adds specialists from templates only. Say "add an nginx specialist for the edge servers".');
}

function rerender() { dispatchEvent(new HashChangeEvent('hashchange')); }

export function pageHome() {
  const o = store.setup || { stage: 'tenant', next: [], connections: [], cells: [] };
  const first = o.next && o.next[0];
  const panel = el('aside', { class: 'setup-panel', 'aria-label': 'Setup' }, [
    el('div', { class: 'panel-head' }, [icon('warden'), el('span', { class: 'heading', text: o.stage === 'ready' ? 'Your estate' : 'Setup' }), el('span', { class: 'spacer' }), el('span', { class: 'badge', text: 'stage · ' + o.stage })]),
    first ? el('div', { class: 'panel-next' }, [el('span', { class: 'label', text: 'Next' }), el('div', { text: first.title }), el('div', { class: 'sm muted', text: first.who === 'operator' ? 'Yours to do.' : first.who === 'both' ? 'Warden prepares it, you run it.' : first.tool ? 'Warden does this; ask it or click the button.' : 'In progress.' })]) : null,
    approvalsBand(),
    tenantBand(o), connectionsBand(o), cellsBand(o),
    ...(store.state.cells.length ? [runbooksBand(), workflowsBand(), agentsBand()] : []),
  ]);
  return el('div', { class: 'home' }, [thread(), panel]);
}
