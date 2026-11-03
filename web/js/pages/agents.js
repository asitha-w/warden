import { el, StatusChip } from '../ds.js';
import { store } from '../store.js';
import { api } from '../api.js';
import { money, tokens, toast } from '../shell.js';

const kindWords = { orchestrator: 'Orchestrator', specialist: 'Specialist', deterministic: 'Deterministic, no model' };

export function pageAgents() {
  const s = store.state;
  const total = Object.values(s.usage).reduce((a, u) => a + (u.costUsd || 0), 0);
  const cards = s.agents.map((a) => {
    const u = s.usage[a.id] || { calls: 0, inputTokens: 0, outputTokens: 0, costUsd: 0, runs: 0 };
    const running = s.runs.filter((r) => r.agent === a.id && !r.endedAt && r.status !== 'done' && r.status !== 'failed').length;
    const effort = el('select', { class: 'sm', disabled: !a.model, onChange: (e) => api.agent(a.id, { effort: e.target.value }).catch((x) => toast(x.message)) }, ['low', 'medium', 'high', 'xhigh', 'max'].map((o) => el('option', { text: o, selected: o === a.effort })));
    const toggle = el('label', { class: 'switch' }, [el('input', { type: 'checkbox', checked: a.enabled, disabled: a.kind === 'orchestrator', onChange: (e) => api.agent(a.id, { enabled: e.target.checked }).catch((x) => toast(x.message)) }), a.enabled ? 'Enabled' : 'Disabled']);
    return el('div', { class: 'card agent-card' + (a.enabled ? '' : ' off') }, [
      el('div', { class: 'head' }, [el('span', { class: 'name', text: a.name }), StatusChip({ status: running ? 'running' : (a.enabled ? 'idle' : 'unverified'), label: running ? running + ' running' : (a.enabled ? 'Idle' : 'Disabled') }), el('span', { class: 'spacer', style: 'flex:1' }), toggle]),
      el('div', { class: 'role', text: a.role }),
      el('p', { class: 'sm', text: a.description }),
      el('dl', { class: 'kv' }, [el('dt', { text: 'Kind' }), el('dd', { text: kindWords[a.kind] || a.kind }), el('dt', { text: 'Model' }), el('dd', { class: 'mono', text: a.model || 'none' }), el('dt', { text: 'Effort' }), el('dd', null, [a.model ? effort : el('span', { class: 'muted', text: 'n/a' })]), el('dt', { text: 'Tool budget' }), el('dd', { text: a.maxToolCalls ? a.maxToolCalls + ' calls per run' : 'n/a' })]),
      el('div', { class: 'policy', style: 'flex-wrap:wrap' }, a.tools.map((t) => el('span', { text: t }))),
      el('div', { class: 'usage' }, [
        el('div', null, [el('span', { class: 'label', text: 'Runs' }), el('b', { text: String(u.runs || 0) })]),
        el('div', null, [el('span', { class: 'label', text: 'Model calls' }), el('b', { text: String(u.calls || 0) })]),
        el('div', null, [el('span', { class: 'label', text: 'Tokens in / out' }), el('b', { text: tokens(u.inputTokens) + ' / ' + tokens(u.outputTokens) })]),
        el('div', null, [el('span', { class: 'label', text: 'Cost' }), el('b', { text: money(u.costUsd) })]),
      ]),
    ]);
  });
  return el('div', { class: 'page' }, [
    el('div', null, [el('h1', { class: 'display', text: 'Agents' }), el('p', { class: 'muted', text: 'The orchestrator is the agent you talk to. It starts specialists for signals and requests; deterministic agents do the parts that must not depend on a model. Usage is per agent, this tenant, this month.' })]),
    el('div', { class: 'split' }, [
      el('div', { class: 'card' }, [el('span', { class: 'label', text: 'Model spend, all agents' }), el('div', { class: 'val', style: 'margin-top:4px' }, [el('span', { class: 'numeral', text: money(total) })]), el('p', { class: 'sm muted', text: s.reasoner === 'claude' ? `Live usage from the API, model ${s.model}.` : 'Scripted mode: token counts are simulated at typical sizes so the views have numbers.' })]),
      el('div', { class: 'card' }, [el('span', { class: 'label', text: 'How work flows' }), el('p', { class: 'sm', style: 'margin:4px 0 0', text: 'Signal loop → Orchestrator applies the responder table → Investigator reads and proposes → Policy gate (allow, ask, deny) → Remediator executes → Verifier re-reads the signal.' })]),
    ]),
    el('div', { class: 'split' }, cards),
  ]);
}
