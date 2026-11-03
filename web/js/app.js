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
import { api } from './api.js';

const PAGES = {
  warden: { page: pageHome, crumb: 'Warden' },
  findings: { page: pageFindings, crumb: 'Findings', aside: true },
  investigations: { page: pageInvestigations, crumb: 'Investigations' },
  approvals: { page: pageApprovals, crumb: 'Approvals', aside: true },
  chat: { page: pageChat, crumb: 'Ask Warden', aside: true },
  agents: { page: pageAgents, crumb: 'Agents' },
  runbooks: { page: pageRunbooks, crumb: 'Runbooks' },
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
  const scrollEl = app.querySelector('.content'); const sc = r === lastRoute && scrollEl ? scrollEl.scrollTop : 0; lastRoute = r;
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
  if (activeId) { const again = document.getElementById(activeId); if (again) { if (activeVal !== null && 'value' in again && again.value === '') again.value = activeVal; again.focus(); } }
  drawTiles(); demobar();
}
let setupTimer = null;
function refreshSetup() {
  if (setupTimer) return;
  setupTimer = setTimeout(() => { setupTimer = null; api.setup().then((o) => { store.setup = o; scheduleRender(); }).catch(() => {}); }, 80);
}
function scheduleRender() {
  if (renderQueued) return; renderQueued = true;
  requestAnimationFrame(() => { renderQueued = false; render(); });
}
function onEvent(ev) {
  const hint = apply(ev);
  if (hint === 'full') scheduleRender();
  else if (hint === 'metrics') drawTiles();
  else if (hint && hint.kind === 'delta') { const n = document.querySelector(`[data-msg="${hint.id}"]`); if (n) n.textContent = hint.text; else scheduleRender(); }
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
