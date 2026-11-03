import { el, Button, SeverityChip, StatusChip, StatRow, EvidenceTable, CausalChain, ApprovalCard } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { go } from '../router.js';
import { fmtTime, ago, duration, money, tokens, toast } from '../shell.js';

export function pageInvestigations() {
  const s = store.state;
  const rows = s.runs.map((r) => { const f = sel.finding(r.findingId) || {}; const a = sel.agent(r.agent) || {}; return el('tr', { class: 'row', onClick: () => go('inv/' + r.id) }, [el('td', { class: 'mono', text: r.id }), el('td', { text: f.title || r.task }), el('td', { text: a.name || r.agent }), el('td', null, [StatusChip({ status: r.status, label: r.outcome })]), el('td', { class: 'mono', text: duration(r) }), el('td', { class: 'mono', text: money(r.usage.costUsd) }), el('td', { class: 'mono muted', text: ago(r.startedAt) })]); });
  return el('div', { class: 'page' }, [el('h1', { class: 'display', text: 'Investigations' }), el('p', { class: 'muted', text: 'Every agent run: what triggered it, which agent did the work, what it cost and how it ended.' }), el('div', { class: 'tablewrap' }, [el('table', { class: 'table' }, [el('thead', null, [el('tr', null, [el('th', { text: 'Run' }), el('th', { text: 'Task' }), el('th', { text: 'Agent' }), el('th', { text: 'Outcome' }), el('th', { text: 'Duration' }), el('th', { text: 'Cost' }), el('th', { text: 'When' })])]), el('tbody', null, rows.length ? rows : [el('tr', null, [el('td', { colspan: '7', class: 'muted', text: 'No runs yet.' })])])])])]);
}

function who(role, agent, userName) {
  if (role === 'user') return el('div', { class: 'who user', text: (userName || 'U').slice(0, 1).toUpperCase() });
  if (role === 'signal') return el('div', { class: 'who', text: 'S' });
  if (role === 'orchestrator') return el('div', { class: 'who orch', text: 'O' });
  if (role === 'error') return el('div', { class: 'who err', text: '!' });
  return el('div', { class: 'who warden', text: 'B' });
}
function msg(role, agent, meta, nodes, userName) {
  return el('div', { class: 'msg' }, [who(role, agent, userName), el('div', { class: 'body2' }, [el('div', { class: 'meta' }, meta.filter(Boolean).map((m, i) => i === 0 ? el('b', { text: m }) : el('span', { text: m })))].concat(nodes))]);
}
function argText(input) {
  return Object.keys(input || {}).filter((k) => !['summary', 'blast_radius', 'claim', 'detail'].includes(k)).map((k) => { const v = String(input[k]); return k + '=' + (v.length > 60 ? v.slice(0, 57) + '…' : v); }).join(' ');
}
export function toolLine(t) {
  const line = el('div', { class: 't ' + (t.status || ''), title: 'Show result' }, [el('i', { class: t.status === 'running' ? 'run' : '' }), el('span', { text: t.name + ' ' + argText(t.input) })]);
  const out = el('div', { class: 'out hidden', text: t.output || '' });
  line.addEventListener('click', () => out.classList.toggle('hidden'));
  return [line, out];
}
const agentTitle = { investigator: 'Investigator', orchestrator: 'Orchestrator', remediator: 'Remediator', verifier: 'Verifier' };

export function pageInv(id) {
  const s = store.state; const r = sel.run(id);
  if (!r) return el('div', { class: 'page' }, [el('h1', { class: 'display', text: id }), el('div', { class: 'empty', text: 'No such run in this tenant.' })]);
  const f = sel.finding(r.findingId) || { title: r.task, severity: 'info', status: r.status, resource: '', service: '' };
  const userName = s.user ? s.user.name : 'you';
  const items = []; let toolGroup = null; let inserted = false;
  const insertAnalysis = () => {
    if (inserted) return; inserted = true;
    if (r.evidence.length) items.push(msg('agent', r.agent, ['Evidence', r.evidence.length + ' rows with source pointers'], [el('div', { class: 'tablewrap' }, [EvidenceTable({ rows: r.evidence })])]));
    if (r.hypotheses.length || r.chain.length) items.push(msg('agent', r.agent, ['Hypotheses'], r.hypotheses.map((h) => el('div', { class: 'hyp ' + (h.verdict === 'supported' ? 'v' : 'u') }, [el('div', { class: 'heading', text: (h.verdict === 'supported' ? 'Likely cause: ' : 'Considered and rejected: ') + h.title }), el('p', { class: 'sm', text: h.detail })])).concat(r.chain.length ? [CausalChain({ steps: r.chain })] : [])));
  };
  r.messages.forEach((m) => {
    if (m.role === 'tool') {
      if (m.tool.name === 'propose_action') insertAnalysis();
      if (!toolGroup) { toolGroup = el('div', { class: 'tools' }); items.push(msg('agent', m.agent, [agentTitle[m.agent] || m.agent, 'tools'], [toolGroup], userName)); }
      toolLine(m.tool).forEach((n) => toolGroup.appendChild(n));
      return;
    }
    toolGroup = null;
    const title = m.title || agentTitle[m.agent] || (m.role === 'agent' ? 'Investigator' : m.role);
    let body;
    if (m.role === 'executor') body = [el('div', { class: 'code', text: m.text })];
    else body = [el('p', { 'data-msg': m.id, text: m.text || (m.role === 'agent' ? '…' : '') })];
    if (m.role === 'policy') {
      const ap = sel.approval(r.approvalId);
      if (ap && ap.status === 'pending') body.push(ApprovalCard({ finding: f.title, action: ap.command, blastRadius: ap.blastRadius, approveLabel: 'Approve fix', canApprove: true, onApprove: () => api.decide(ap.id, 'approve').catch((e) => toast(e.message)), onDeny: () => api.decide(ap.id, 'deny').catch((e) => toast(e.message)) }));
    }
    if (m.role === 'verification') body.push(StatRow({ items: [{ value: duration(r), label: 'trigger to verified' }, { value: String(r.evidence.length), label: 'sources' }, { value: money(r.usage.costUsd), label: 'cost' }, { value: tokens(r.usage.inputTokens + r.usage.outputTokens), label: 'tokens' }, r.id] }));
    items.push(msg(m.role, m.agent, [title.split(' · ')[0], title.split(' · ')[1], fmtTime(m.at)], body, userName));
  });
  insertAnalysis();
  if (r.summary && r.status === 'done') items.push(msg('agent', r.agent, ['Summary'], [el('p', { text: r.summary })]));
  const thread = el('div', { class: 'thread' }, items);
  const head = el('div', null, [el('div', { class: 'bc-row', style: 'margin-bottom:8px' }, [SeverityChip({ severity: f.severity }), StatusChip({ status: f.status }), el('span', { class: 'badge', text: (sel.agent(r.agent) || {}).name || r.agent })]), el('h1', { class: 'display', text: f.title }),
    el('div', { style: 'margin-top:8px' }, [StatRow({ items: [{ value: duration(r), label: r.endedAt ? 'total' : 'running' }, { value: String(r.evidence.length), label: 'evidence rows' }, { value: tokens(r.usage.inputTokens + r.usage.outputTokens), label: 'tokens' }, { value: money(r.usage.costUsd), label: 'cost' }, f.resource + (f.service ? ' · app=' + f.service : ''), r.id] })])]);
  const rbs = (r.runbooks || []).map((id) => (s.runbooks || []).find((x) => x.id === id)).filter(Boolean);
  if (rbs.length) head.appendChild(el('div', { class: 'sm muted', style: 'margin-top:6px' }, ['Runbooks: ', ...rbs.flatMap((rb, i) => [i ? ', ' : '', el('a', { href: '#runbooks', text: rb.name })])]));
  return el('div', { class: 'page' }, [head, el('div', { class: 'card lg' }, [thread]), askBox(r, userName)]);
}

function askBox(r, userName) {
  const input = el('input', { id: 'ask-input', placeholder: 'Ask Warden about this investigation…' });
  const out = el('div', { class: 'thread', id: 'ask-out' }, r.questions.map((q) => el('div', null, [msg('user', '', [userName], [el('p', { text: q.question })], userName), msg('agent', 'orchestrator', ['Warden'], [el('p', { text: q.answer })])])));
  const form = el('form', { onSubmit: (e) => {
    e.preventDefault(); const q = input.value.trim(); if (!q) return; input.value = '';
    out.appendChild(msg('user', '', [userName], [el('p', { text: q })], userName));
    const ans = el('p', { text: 'Thinking…' }); out.appendChild(msg('agent', 'orchestrator', ['Warden'], [ans]));
    api.ask(r.id, q).then((d) => { ans.textContent = d.answer; }).catch((x) => { ans.textContent = 'Could not answer: ' + x.message; });
  } }, [el('div', { class: 'ask' }, [input, Button({ variant: 'primary', label: 'Ask', type: 'submit' })])]);
  return el('div', { class: 'card' }, [el('div', { class: 'label', text: 'Ask Warden about this run' }), el('div', { style: 'height:8px' }), out, form]);
}
