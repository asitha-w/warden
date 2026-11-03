// Client state: the server snapshot plus reducers for each event type, and a
// small metrics ring buffer for the sparklines. Pages read from here only.
const SERIES = 90;
export const store = {
  state: null,
  metrics: { hosts: {}, series: {}, probes: 0, online: false, at: null },
  connection: 'connecting',
  setup: null, // the server's setup overview, refreshed after setup events
};
function upsert(list, item, key = 'id', prepend = false) {
  const i = list.findIndex((x) => x[key] === item[key]);
  if (i >= 0) list[i] = item; else if (prepend) list.unshift(item); else list.push(item);
}
// apply returns a hint for the renderer: 'full', 'metrics', a delta, a toast, or null.
export function apply(ev) {
  const s = store.state;
  switch (ev.type) {
    case 'snapshot': store.state = ev.data; return 'full';
    case 'user': s.user = ev.data; return 'full';
    case 'tenant': s.tenant = ev.data; return 'full';
    case 'connection': upsert(s.connections, ev.data); return 'full';
    case 'connections': s.connections = ev.data; return 'full';
    case 'cells': s.cells = ev.data; return 'full';
    case 'runbooks': s.runbooks = ev.data; return 'full';
    case 'workflow': upsert(s.workflows, ev.data); return 'full';
    case 'workflows': s.workflows = ev.data; return 'full';
    case 'ui': return { kind: 'ui', data: ev.data };
    case 'probe': s.probe = ev.data; return 'full';
    case 'finding': upsert(s.findings, ev.data, 'id', true); return 'full';
    case 'run': upsert(s.runs, ev.data, 'id', true); return 'full';
    case 'run.delta': {
      const r = s.runs.find((x) => x.id === ev.data.runId);
      const m = r && r.messages.find((x) => x.id === ev.data.messageId);
      if (m) m.text = (m.text || '') + ev.data.delta;
      return { kind: 'delta', id: ev.data.messageId, text: m ? m.text : '' };
    }
    case 'approval': upsert(s.approvals, ev.data); return 'full';
    case 'activity': s.activity.unshift(ev.data); if (s.activity.length > 60) s.activity.length = 60; return 'full';
    case 'usage': s.usage = ev.data; return 'full';
    case 'chat': upsert(s.chat, ev.data); return 'full';
    case 'chat.insert': { const i = s.chat.findIndex((x) => x.id === ev.data.before); if (i >= 0) s.chat.splice(i, 0, ev.data.message); else s.chat.push(ev.data.message); return 'full'; }
    case 'chat.delta': {
      const m = s.chat.find((x) => x.id === ev.data.messageId);
      if (m) m.text = (m.text || '') + ev.data.delta;
      return { kind: 'delta', id: ev.data.messageId, text: m ? m.text : '' };
    }
    case 'scenario': upsert(s.scenarios, ev.data); return 'full';
    case 'responder': upsert(s.responders, ev.data); return 'full';
    case 'agent': upsert(s.agents, ev.data); return 'full';
    case 'metrics': {
      const m = store.metrics;
      m.hosts = ev.data.hosts; m.probes = ev.data.probes; m.online = ev.data.online; m.at = Date.now();
      Object.keys(ev.data.hosts).forEach((h) => {
        const hs = (m.series[h] = m.series[h] || {});
        Object.keys(ev.data.hosts[h]).forEach((k) => { const arr = (hs[k] = hs[k] || []); arr.push(ev.data.hosts[h][k]); if (arr.length > SERIES) arr.shift(); });
      });
      return 'metrics';
    }
    case 'toast': return { kind: 'toast', text: ev.data };
    default: return null;
  }
}
function matchesSel(selector, cell) { const keys = Object.keys(selector || {}); return keys.length > 0 && keys.every((k) => (cell.labels || {})[k] === selector[k]); }
export const sel = {
  run: (id) => store.state.runs.find((r) => r.id === id),
  finding: (id) => store.state.findings.find((f) => f.id === id),
  approval: (id) => store.state.approvals.find((a) => a.id === id),
  pendingApprovals: () => store.state.approvals.filter((a) => a.status === 'pending'),
  agent: (id) => store.state.agents.find((a) => a.id === id),
  conn: (id) => store.state.connections.find((c) => c.id === id),
  conns: (provider) => store.state.connections.filter((c) => c.provider === provider),
  connected: (provider) => store.state.connections.some((c) => c.provider === provider && c.status === 'connected'),
  // nextStep is the one setup step the console points at, or null when the estate is set up.
  nextStep: () => {
    const s = store.state; if (!s || !s.tenant) return null;
    if (!s.connections.some((c) => c.status === 'connected')) return { nav: 'warden', tip: 'Start by adding a credential', detail: 'A connection is an identity at a scope. It discovers what Warden could manage; you choose what becomes a cell.', doc: '#docs/model' };
    if (!s.cells.length) return { nav: 'warden', tip: 'Let Warden onboard what it found', detail: 'Each connection lists what it found. Pick what Warden should manage; each becomes a cell at Level 0.', doc: '#docs/model/levels-of-reach' };
    if (s.cells.some((c) => c.probeCapable && c.level < (/cluster/.test(c.kind) ? 2 : 1))) return { nav: 'warden', tip: 'Raise your cells with Warden', detail: 'Each cell page has the manual steps for its next level: the grant on a VM, API access and the probe on a cluster.', doc: '#docs/model/levels-of-reach' };
    return null;
  },
  cell: (id) => store.state.cells.find((c) => c.id === id),
  cellsMatching: (selector) => store.state.cells.filter((c) => matchesSel(selector, c)),
  sharedWith: (cell) => store.state.cells.filter((c) => c.id !== cell.id && c.connectionId === cell.connectionId).length,
  metric: (host, k) => (store.metrics.hosts[host] || {})[k],
  series: (host, k) => ((store.metrics.series[host] || {})[k] || []),
};
