// Entry point: connects the event stream, applies events to the store and
// renders the current route. Metrics and streamed text update in place; every
// other event re-renders the page with scroll and input state preserved.
import { el } from './ds.js';
import { store, apply } from './store.js';
import { connect } from './api.js';
import { route, go } from './router.js';
import { shell, setupShell, demobar, toast } from './shell.js';
import { pageLogin, pageSetupTenant } from './pages/setup.js';
import { pageFindings, drawTiles } from './pages/findings.js';
import { pageInvestigations, pageInv } from './pages/investigations.js';
import { pageChat } from './pages/chat.js';
import { pageAgents } from './pages/agents.js';
import { pageRunbooks } from './pages/runbooks.js';
import { pageCells, pageCell } from './pages/cells.js';
import { pageApprovals, pageConnections, pageResponders, pageConsumption } from './pages/admin.js';
import { pageDocs } from './pages/docs.js';
import { pageHome, openForm } from './pages/home.js';
import { pageWatches } from './pages/watches.js';
import { api } from './api.js';

const PAGES = {
  warden: { page: pageHome, crumb: 'Warden' },
  findings: { page: pageFindings, crumb: 'Findings', aside: true },
  investigations: { page: pageInvestigations, crumb: 'Investigations' },
  approvals: { page: pageApprovals, crumb: 'Approvals', aside: true },
  chat: { page: pageChat, crumb: 'Ask Warden', aside: true },
  agents: { page: pageAgents, crumb: 'Agents' },
  runbooks: { page: pageRunbooks, crumb: 'Runbooks' },
  watches: { page: pageWatches, crumb: 'Watches' },
  cells: { page: pageCells, crumb: 'Cells' },
  connections: { page: pageConnections, crumb: 'Connections' },
  responders: { page: pageResponders, crumb: 'Responders' },
  consumption: { page: pageConsumption, crumb: 'Consumption' },
};

let renderQueued = false; let lastRoute = '';
function render() {
  if (!store.state) return;
  const r = route(); const app = document.getElementById('app');
  const active = document.activeElement; const activeId = active && active.id; const activeVal = active && 'value' in active ? active.value : null;
  const scrollEl = app.querySelector('.content'); const sc = r === lastRoute && scrollEl ? scrollEl.scrollTop : 0;
  // scroll containers that must not jump when the page is rebuilt: keep their
  // position by id, and keep a thread pinned to its end when it was there
  const kept = {};
  if (r === lastRoute) app.querySelectorAll('[data-keep-scroll]').forEach((n) => { kept[n.id] = { top: n.scrollTop, atEnd: n.scrollHeight - n.scrollTop - n.clientHeight < 48 }; });
  lastRoute = r;
  let out;
  if (r === 'login') out = pageLogin();
  else if (r === 'setup-tenant') out = setupShell(pageSetupTenant());
  else {
    const parts = r.split('/');
    if (parts[0] === 'docs') { const d = pageDocs(parts[1] || 'model', parts[2]); out = store.state.tenant ? shell(d, { crumb: 'Documentation' }) : setupShell(d); }
    else if (parts[0] === 'inv') out = shell(pageInv(parts[1]), { crumb: 'Investigations / ' + parts[1], aside: true });
    else if (parts[0] === 'cell') out = shell(pageCell(parts[1]), { crumb: 'Cells / ' + parts[1] });
    else { const p = PAGES[parts[0]] || PAGES.warden; out = shell(p.page(), { crumb: p.crumb, aside: !!p.aside }); }
  }
  app.replaceWith(out); out.id = 'app';
  const ns = out.querySelector('.content'); if (ns) ns.scrollTop = sc;
  out.querySelectorAll('[data-keep-scroll]').forEach((n) => { const k = kept[n.id]; if (!k) { if (n.dataset.keepScroll === 'end') n.scrollTop = n.scrollHeight; return; } n.scrollTop = k.atEnd && n.dataset.keepScroll === 'end' ? n.scrollHeight : k.top; });
  if (activeId) { const again = document.getElementById(activeId); if (again) { if (activeVal !== null && 'value' in again && again.value === '') again.value = activeVal; again.focus(); } }
  drawTiles(); demobar();
}
let setupTimer = null;
const lastFired = {};
function refreshSetup() {
  if (setupTimer) return;
  setupTimer = setTimeout(() => { setupTimer = null; api.setup().then((o) => { store.setup = o; scheduleRender(); }).catch(() => {}); }, 80);
}
function scheduleRender() {
  if (renderQueued) return; renderQueued = true;
  requestAnimationFrame(() => { renderQueued = false; render(); });
}
let watchTimer = null;
function onEvent(ev) {
  const hint = apply(ev);
  if (hint === 'full' && ev.type === 'workflow' && !(ev.data && ev.data.fired > (lastFired[ev.data.id] || 0))) {
    // a watch tick changes only its last run and result: redraw at most every few seconds
    if (!watchTimer) watchTimer = setTimeout(() => { watchTimer = null; scheduleRender(); }, 2500);
  } else if (hint === 'full') scheduleRender();
  if (ev.type === 'workflow' && ev.data) lastFired[ev.data.id] = ev.data.fired;
  else if (hint === 'metrics') drawTiles();
  else if (hint && hint.kind === 'delta') {
    const n = document.querySelector(`[data-msg="${hint.id}"]`);
    if (!n) scheduleRender();
    else {
      // streaming text grows the thread: keep it pinned to the end when it was there
      const box = n.closest('[data-keep-scroll="end"]'); const atEnd = box && box.scrollHeight - box.scrollTop - box.clientHeight < 48;
      n.textContent = hint.text;
      if (box && atEnd) box.scrollTop = box.scrollHeight;
    }
  }
  else if (hint && hint.kind === 'toast') toast(hint.text);
  else if (hint && hint.kind === 'ui') { if (hint.data && hint.data.open) { openForm(hint.data.open); go('warden'); } }
  if (ev.type === 'snapshot' || ev.type === 'user' || ev.type === 'tenant') {
    // keep the route valid after sign-in, sign-out and reset
    const s = store.state; const h = location.hash.replace(/^#/, '');
    if (!s.user && h !== 'login') location.hash = '#login';
    else if (s.user && !s.tenant && h.indexOf('setup') !== 0) location.hash = '#setup-tenant';
    else if (s.user && s.tenant && (h === 'login' || h === 'setup-tenant')) location.hash = '#warden';
  }
  if (['snapshot', 'tenant', 'connection', 'connections', 'cells', 'probe', 'approval'].includes(ev.type)) refreshSetup();
}
try { const t = localStorage.getItem('warden-theme'); if (t) document.documentElement.dataset.theme = t; } catch (e) {}
window.addEventListener('hashchange', render);
connect(onEvent, (status) => { store.connection = status; if (status === 'reconnecting') toast('Connection to Warden lost, reconnecting…'); });
