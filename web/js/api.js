// Thin client for the Warden API. Every call returns the parsed JSON body or throws.
async function send(method, path, body) {
  const res = await fetch(path, { method, headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
export const api = {
  state: () => send('GET', '/api/state'),
  setup: () => send('GET', '/api/setup'),
  login: (email, name) => send('POST', '/api/login', { email, name }),
  logout: () => send('POST', '/api/logout'),
  createTenant: (t) => send('POST', '/api/tenant', t),
  createConnection: (body) => send('POST', '/api/connections', body),
  deleteConnection: (id) => send('DELETE', `/api/connections/${id}`),
  setTelemetry: (id, telemetry) => send('POST', `/api/cells/${id}/telemetry`, telemetry),
  saveRunbook: (rb) => send('POST', '/api/runbooks', rb),
  workflow: (id, patch) => send('POST', `/api/workflows/${id}`, patch),
  deleteWorkflow: (id) => send('DELETE', `/api/workflows/${id}`),
  deleteRunbook: (id) => send('DELETE', `/api/runbooks/${id}`),
  setLabels: (id, labels) => send('POST', `/api/cells/${id}/labels`, { labels }),
  onboard: (connId, ids) => send('POST', `/api/connections/${connId}/onboard`, { ids }),
  detachCell: (id) => send('DELETE', `/api/cells/${id}`),
  installProbe: () => send('POST', '/api/probe/install'),
  grant: (id, body) => send('POST', `/api/cells/${id}/grant`, body || {}),
  apiAccess: (id) => send('POST', `/api/cells/${id}/api-access`),
  cellProbe: (id) => send('POST', `/api/cells/${id}/probe`),
  decide: (id, decision) => send('POST', `/api/approvals/${id}`, { decision }),
  chat: (question) => send('POST', '/api/chat', { question }),
  newSession: () => send('POST', '/api/chat/new'),
  ask: (runId, question) => send('POST', `/api/runs/${runId}/ask`, { question }),
  responderMode: (id, mode) => send('POST', `/api/responders/${id}`, { mode }),
  agent: (id, patch) => send('POST', `/api/agents/${id}`, patch),
  startScenario: (id) => send('POST', `/api/scenarios/${id}/start`),
  reset: () => send('POST', '/api/reset'),
};
// Opens the event stream. onEvent gets every decoded event; reconnects on drop.
export function connect(onEvent, onStatus) {
  let es;
  function open() {
    es = new EventSource('/api/events');
    es.onopen = () => onStatus && onStatus('open');
    es.onmessage = (m) => { try { onEvent(JSON.parse(m.data)); } catch (e) { console.error('bad event', e); } };
    es.onerror = () => { onStatus && onStatus('reconnecting'); };
  }
  open();
  return () => es && es.close();
}
