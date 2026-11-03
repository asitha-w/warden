import { el, Button } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { toast } from '../shell.js';
import { markdown } from '../markdown.js';

function parseSelector(text) { const out = {}; text.split(',').map((p) => p.trim()).filter(Boolean).forEach((p) => { const i = p.indexOf('='); if (i > 0) out[p.slice(0, i).trim()] = p.slice(i + 1).trim(); }); return out; }
function selectorText(s) { return Object.keys(s || {}).map((k) => k + '=' + s[k]).join(', '); }
function matches(rb) { return sel.cellsMatching(rb.selector); }

const TEMPLATE = `# App name\n\nWhat this app is and what depends on it.\n\n## Health signals\n- Which metric matters and at what level.\n\n## Known fixes, in order\n1. First thing to try, and when it is safe.\n2. Second thing.\n\n## Never\n- What agents must not do here.\n\n## Escalation\nWho to contact, and when to page.\n`;

// editor returns a card with the runbook form; rb is null for a new one.
function editor(rb, onClose) {
  const name = el('input', { id: 'rb-name', value: rb ? rb.name : '', placeholder: 'Shop web on VMs' });
  const selInput = el('input', { id: 'rb-sel', value: rb ? selectorText(rb.selector) : '', placeholder: 'app=shop, tier=web' });
  const body = el('textarea', { id: 'rb-body', rows: '16', spellcheck: 'false' }); body.value = rb ? rb.body : TEMPLATE;
  const save = () => api.saveRunbook({ id: rb ? rb.id : '', name: name.value, selector: parseSelector(selInput.value), body: body.value }).then(onClose).catch((e) => toast(e.message));
  return el('div', { class: 'card' }, [
    el('div', { class: 'card-title', text: rb ? 'Edit runbook' : 'New runbook' }),
    el('div', { class: 'form', style: 'max-width:none;margin-top:12px' }, [
      el('div', { class: 'split' }, [el('div', { class: 'field' }, [el('label', { for: 'rb-name', text: 'Name' }), name]), el('div', { class: 'field' }, [el('label', { for: 'rb-sel', text: 'Applies to (label selector)' }), selInput, el('span', { class: 'sm muted', text: 'Same labels as apps. The investigator gets every runbook whose selector matches the resource, most specific first.' })])]),
      el('div', { class: 'field' }, [el('label', { for: 'rb-body', text: 'Runbook (Markdown)' }), body]),
      el('div', { class: 'bc-row' }, [Button({ variant: 'primary', label: rb ? 'Save changes' : 'Create runbook', onClick: save }), Button({ label: 'Cancel', onClick: onClose }), rb ? Button({ variant: 'danger', label: 'Delete', onClick: () => api.deleteRunbook(rb.id).then(onClose).catch((e) => toast(e.message)) }) : null]),
    ]),
  ]);
}

let editing = null; // runbook id, 'new', or null

export function pageRunbooks() {
  const s = store.state;
  const rerender = () => dispatchEvent(new HashChangeEvent('hashchange'));
  const close = () => { editing = null; rerender(); };
  const cards = s.runbooks.map((rb) => {
    if (editing === rb.id) return editor(rb, close);
    const m = matches(rb);
    const used = s.runs.filter((r) => (r.runbooks || []).includes(rb.id)).length;
    return el('div', { class: 'card' }, [
      el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('div', { class: 'card-title', text: rb.name }), el('div', { class: 'sm muted', text: `Updated ${new Date(rb.updatedAt).toLocaleDateString()} by ${rb.updatedBy || 'unknown'} · used in ${used} run${used === 1 ? '' : 's'}` })]), Button({ label: 'Edit', onClick: () => { editing = rb.id; rerender(); } })]),
      el('div', { class: 'policy', style: 'flex-wrap:wrap;margin-top:8px' }, Object.keys(rb.selector).map((k) => el('span', { text: k + '=' + rb.selector[k] }))),
      el('div', { class: 'sm muted', style: 'margin-top:4px', text: m.length ? 'Applies to ' + m.map((r) => r.id).join(', ') : 'No cell matches this selector yet.' }),
      el('div', { class: 'md-wrap' }, [markdown(rb.body)]),
    ]);
  });
  return el('div', { class: 'page' }, [
    el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('h1', { class: 'display', text: 'Runbooks' }), el('p', { class: 'muted', text: 'Tenant-written guidance the agents follow. A runbook attaches to a label selector, like an app. It guides the investigator; the responder policy still decides what may run.' })]), Button({ variant: 'primary', label: 'New runbook', onClick: () => { editing = 'new'; rerender(); } })]),
    editing === 'new' ? editor(null, close) : null,
    cards.length ? el('div', { class: 'grid2' }, cards) : el('div', { class: 'empty', text: 'No runbooks yet. Write one for each app so agents know how you want it handled.' }),
  ]);
}
