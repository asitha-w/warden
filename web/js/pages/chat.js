import { el, Button } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { toast } from '../shell.js';
import { toolLine } from './investigations.js';

export function pageChat() {
  const s = store.state; const userName = s.user ? s.user.name : 'you';
  const orch = sel.agent('orchestrator') || {};
  const items = s.chat.map((m) => {
    if (m.role === 'tool') { const g = el('div', { class: 'tools' }); toolLine(m.tool).forEach((n) => g.appendChild(n)); return el('div', { class: 'msg' }, [el('div', { class: 'who orch', text: 'O' }), el('div', { class: 'body2' }, [el('div', { class: 'meta' }, [el('b', { text: 'Orchestrator' }), el('span', { text: 'tool' })]), g])]); }
    const isUser = m.role === 'user';
    return el('div', { class: 'msg' }, [el('div', { class: 'who ' + (isUser ? 'user' : 'orch'), text: isUser ? userName.slice(0, 1).toUpperCase() : 'O' }), el('div', { class: 'body2' }, [el('div', { class: 'meta' }, [el('b', { text: isUser ? userName : 'Orchestrator' })]), el('p', { 'data-msg': m.id, text: m.text || '…' })])]);
  });
  const empty = el('div', { class: 'empty' }, [el('div', { class: 'heading', text: 'Talk to the orchestrator' }), el('p', { class: 'sm muted', text: 'It knows the estate, the findings and the runs, and it starts the right agent for what you ask. Try: "what is open?", "check edge-01", or "investigate why aks-shop is slow".' })]);
  const input = el('input', { id: 'chat-input', placeholder: 'Ask the orchestrator…', autocomplete: 'off' });
  const form = el('form', { onSubmit: (e) => { e.preventDefault(); const q = input.value.trim(); if (!q) return; input.value = ''; api.chat(q).catch((x) => toast(x.message)); } }, [el('div', { class: 'ask' }, [input, Button({ variant: 'primary', label: 'Send', type: 'submit' })])]);
  return el('div', { class: 'page' }, [
    el('div', null, [el('h1', { class: 'display', text: 'Ask Warden' }), el('p', { class: 'muted', text: `Orchestrator · ${orch.model || 'no model'} · effort ${orch.effort || '-'} · ${s.reasoner === 'claude' ? 'live' : 'scripted mode, keyword routing'}` })]),
    el('div', { class: 'card lg chat' }, [el('div', { class: 'thread' }, items.length ? items : [empty]), form]),
  ]);
}
