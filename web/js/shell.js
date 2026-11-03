// The app frame: rail, top bar, activity panel, toasts and the demo bar.
import { el, Button } from './ds.js';
import { store, sel } from './store.js';
import { route, go } from './router.js';
import { api } from './api.js';
import { icon } from './icons.js';

export const NAV = [
  ['warden', 'Warden', 'warden'], ['findings', 'Findings', 'activity'], ['investigations', 'Investigations', 'search'], ['approvals', 'Approvals', 'shield'],
  ['sep', 'Configure'],
  ['cells', 'Cells', 'server'], ['connections', 'Connections', 'plug'], ['runbooks', 'Runbooks', 'book'], ['watches', 'Watches', 'activity'], ['agents', 'Agents', 'bot'], ['responders', 'Responders', 'zap'], ['consumption', 'Consumption', 'gauge'],
  ['sep', 'Help'],
  ['docs', 'Documentation', 'layers'],
];
// docsButton sits top right on every page: the documentation, outlined in the brand green.
function docsButton() {
  return el('a', { class: 'docsbtn', href: '#docs', title: 'Documentation: the model, Azure and UpCloud step by step, the grant, the probe' }, [icon('layers'), 'Documentation']);
}
function themeToggle() {
  const dark = (document.documentElement.dataset.theme || (matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark')) === 'dark';
  return el('button', { class: 'iconbtn', title: dark ? 'Switch to light' : 'Switch to dark', onClick: () => { const next = dark ? 'light' : 'dark'; document.documentElement.dataset.theme = next; try { localStorage.setItem('warden-theme', next); } catch (e) {} dispatchEvent(new HashChangeEvent('hashchange')); } }, [icon(dark ? 'sun' : 'moon')]);
}

export function shell(contentNode, opts = {}) {
  const s = store.state; const r = route().split('/')[0];
  const pending = sel.pendingApprovals().length;
  const step = sel.nextStep();
  const rail = el('nav', { class: 'rail', 'aria-label': 'Warden' }, [
    el('div', { class: 'brand' }, [el('i', null, [icon('warden')]), 'Warden']),
    el('div', { class: 'tenant' }, [el('span', { text: s.tenant ? s.tenant.name : 'No tenant' }), el('span', { class: 'label', text: s.tenant ? s.tenant.env : '' })]),
    el('div', { class: 'nav' }, NAV.map((n) => n[0] === 'sep' ? el('div', { class: 'sep', text: n[1] }) : el('a', { href: '#' + n[0], class: (r === n[0] || (n[0] === 'investigations' && r === 'inv') ? 'active' : '') + (step && step.nav === n[0] && r !== n[0] ? ' hint' : '') }, [icon(n[2]), n[1], n[0] === 'approvals' && pending ? el('span', { class: 'count', text: String(pending) }) : null, step && step.nav === n[0] && r !== n[0] ? el('span', { class: 'tip', role: 'status', text: step.tip }) : null]))),
    el('div', { class: 'me' }, [el('span', { class: 'avatar', text: (s.user ? s.user.name : 'U').slice(0, 1).toUpperCase() }), el('div', { class: 'who' }, [el('span', { text: s.user ? s.user.name : '' }), el('span', { class: 'sm', text: s.user ? s.user.email : '' })]), el('a', { href: '#', title: 'Sign out', onClick: (e) => { e.preventDefault(); api.logout(); } }, [icon('logout')])]),
  ]);
  const crumbs = (opts.crumb || '').split(' / ');
  const top = el('div', { class: 'topbar' }, [el('span', { class: 'crumb' }, crumbs.flatMap((c, i) => [i ? el('span', { text: '/' }) : null, i === crumbs.length - 1 ? el('b', { text: c }) : el('span', { text: c })])), el('span', { class: 'spacer' }), el('span', { class: 'sm muted', text: probeSummary() }), el('span', { class: 'badge', text: s.reasoner === 'claude' ? s.model : 'scripted mode' }), docsButton(), themeToggle()]);
  const body = el('div', { class: 'body' + (opts.aside ? '' : ' noaside') }, [el('div', { class: 'content' }, [contentNode]), opts.aside ? el('aside', { class: 'aside', 'aria-label': 'Activity' }, [activityPanel()]) : null]);
  return el('div', { class: 'app' }, [rail, el('div', { class: 'main' }, [top, body])]);
}
export function setupShell(contentNode) {
  const s = store.state;
  return el('div', { class: 'app', style: 'grid-template-columns:1fr' }, [el('div', { class: 'main' }, [el('div', { class: 'topbar' }, [el('div', { class: 'brand', style: 'padding:0' }, [el('i', null, [icon('warden')]), 'Warden']), el('span', { class: 'spacer' }), el('span', { class: 'sm muted', text: s.user ? s.user.email : '' }), docsButton(), themeToggle()]), el('div', { class: 'body noaside' }, [el('div', { class: 'content' }, [contentNode])])])]);
}
function probeSummary() {
  const s = store.state; const cells = s.cells.length; const probes = s.probe.hosts.length;
  if (!cells) return s.connections.some((c) => c.status === 'connected') ? 'Connected · no cells yet' : 'Not connected';
  return `${cells} cell${cells === 1 ? '' : 's'} · ${probes} probe${probes === 1 ? '' : 's'} online`;
}
export function activityPanel() {
  const s = store.state;
  const items = s.activity.length ? s.activity.map((a) => {
    const n = el('div', { class: 'act' + (a.task ? ' task' : '') }, [el('span', { class: 't', text: fmtTime(a.at) + ' · ' + a.kind }), el('span', { text: a.text })]);
    if (a.runId && sel.run(a.runId)) n.appendChild(el('div', { class: 'bc-row', style: 'margin-top:6px' }, [Button({ variant: a.task ? 'primary' : 'quiet', label: a.task ? 'Open approval' : 'Open run', onClick: () => go('inv/' + a.runId) })]));
    return n;
  }) : [el('div', { class: 'empty sm', text: 'Quiet. Signals, runs, approvals and notifications appear here as they happen.' })];
  return el('div', { class: 'activity' }, [el('div', { class: 'label', text: 'Activity' })].concat(items));
}
export function fmtTime(iso) { const d = new Date(iso); return isNaN(d) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }); }
export function ago(iso) {
  const ms = Date.now() - new Date(iso).getTime(); if (isNaN(ms)) return '';
  const s = Math.max(0, Math.round(ms / 1000)); if (s < 60) return s + 's'; const m = Math.round(s / 60); if (m < 60) return m + 'm'; const h = Math.round(m / 60); if (h < 48) return h + 'h'; return Math.round(h / 24) + 'd';
}
export function duration(r) {
  const end = r.endedAt ? new Date(r.endedAt) : new Date(); const s = Math.max(0, Math.round((end - new Date(r.startedAt)) / 1000));
  return s < 60 ? s + 's' : Math.floor(s / 60) + 'm ' + String(s % 60).padStart(2, '0') + 's';
}
export function money(v) { return '$' + (v || 0).toFixed(2); }
export function tokens(n) { return n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k' : String(n || 0); }
export function toast(text) {
  const box = document.getElementById('toasts'); const t = el('div', { class: 'toast', text }); box.appendChild(t); setTimeout(() => t.remove(), 4200);
}
export function demobar() {
  const s = store.state; const bar = document.getElementById('demobar'); bar.innerHTML = '';
  const ready = s.user && s.tenant && s.probe.status === 'online';
  bar.appendChild(el('span', { class: 'label', text: 'Demo controls' }));
  bar.appendChild(el('span', { text: ready ? 'Inject a fault into the simulated estate:' : 'Finish sign-in, tenant and connections to enable scenarios.' }));
  bar.appendChild(el('span', { class: 'scen' }, s.scenarios.map((sc) => Button({ variant: sc.status === 'idle' ? 'primary' : 'quiet', label: sc.status === 'running' ? sc.name + ' (running)' : sc.status === 'done' ? sc.name + ' (done)' : sc.name, title: sc.description, disabled: !ready || sc.status !== 'idle', onClick: () => api.startScenario(sc.id).then(() => { if (route().split('/')[0] !== 'warden') go('findings'); }).catch((e) => toast(e.message)) }))));
  bar.appendChild(Button({ label: 'Reset tenant', onClick: () => api.reset().then(() => { location.hash = '#login'; }) }));
}
