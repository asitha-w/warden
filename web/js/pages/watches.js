// Watches: every monitoring loop the tenant owns, shown running. A watch runs
// in the runtime on its cadence and costs no tokens; a model runs only when
// one fires. A watch with a pre-approved catalogue action is a standing order.
import { el, Button, StatusChip } from '../ds.js';
import { store, sel } from '../store.js';
import { api } from '../api.js';
import { toast, ago } from '../shell.js';
import { go } from '../router.js';

function selText(s) { return Object.keys(s || {}).map((k) => k + '=' + s[k]).join(', ') || 'every cell'; }

export function pageWatches() {
  const s = store.state;
  const watches = s.workflows || [];
  const cards = watches.map((wf) => {
    const cells = sel.cellsMatching(wf.selector);
    const trigger = wf.trigger === 'wake-warden' ? 'wakes Warden, who picks the responder' : 'starts responder ' + wf.trigger.replace('responder:', '');
    const status = wf.enabled ? el('span', { class: 'bc-chip bc-chip-ok live' }, [el('i', { class: 'dot' }), 'Running · every ' + wf.every + 's']) : StatusChip({ status: 'idle', label: 'Paused' });
    return el('div', { class: 'card watch' + (wf.enabled ? '' : ' off') }, [
      el('div', { class: 'head' }, [el('span', { class: 'name', text: wf.name }), status, wf.fired ? el('span', { class: 'bc-pill bc-pill-missing', text: 'fired ' + wf.fired + '×' }) : null, el('span', { class: 'spacer', style: 'flex:1' }), el('span', { class: 'mono sm muted', text: wf.id })]),
      el('dl', { class: 'kv' }, [
        el('dt', { text: 'Scope' }), el('dd', null, [el('span', { class: 'mono', text: selText(wf.selector) }), el('span', { class: 'sm muted', text: ' · ' + cells.length + ' cell' + (cells.length === 1 ? '' : 's') + (cells.length ? ': ' + cells.map((c) => c.id).join(', ') : '') })]),
        el('dt', { text: 'Collects' }), el('dd', { text: wf.collect === 'log-digest' ? 'log digest of ' + (wf.source === '*' ? 'every log on the cell' : wf.source) + ' (new lines, error count, error signatures; a few hundred bytes)' : wf.collect === 'metric' ? 'metric ' + wf.source : 'provider state' }),
        el('dt', { text: 'Fires when' }), el('dd', { text: wf.condition }),
        el('dt', { text: 'Then' }), el('dd', { text: trigger }),
        wf.allow && wf.allow.length ? el('dt', { text: 'Standing order' }) : null, wf.allow && wf.allow.length ? el('dd', { style: 'color:var(--brand)', text: wf.allow.join(', ') + ' runs without asking; anything else still asks; delete-class actions are denied in every mode' }) : null,
        el('dt', { text: 'Last run' }), el('dd', { class: 'sm' }, [wf.lastRun ? el('span', { text: ago(wf.lastRun) + ' ago' }) : el('span', { class: 'muted', text: 'not yet' }), wf.lastResult ? el('span', { class: 'mono muted', text: ' · ' + wf.lastResult }) : null]),
        el('dt', { text: 'Created' }), el('dd', { class: 'sm muted', text: (wf.createdBy === 'default' ? 'default watch' : 'by Warden for ' + wf.createdBy) + (wf.createdAt ? ' · ' + new Date(wf.createdAt).toLocaleDateString() : '') }),
      ]),
      el('div', { class: 'bc-row' }, [Button({ label: wf.enabled ? 'Pause' : 'Resume', onClick: () => api.workflow(wf.id, { enabled: !wf.enabled }).catch((e) => toast(e.message)) }), wf.createdBy !== 'default' ? Button({ label: 'Remove', onClick: () => api.deleteWorkflow(wf.id).catch((e) => toast(e.message)) }) : null]),
    ]);
  });
  const running = watches.filter((w) => w.enabled).length;
  return el('div', { class: 'page' }, [
    el('div', null, [el('h1', { class: 'display', text: 'Watches' }), el('p', { class: 'muted', text: `${running} of ${watches.length} running. A watch is a loop in the runtime: it collects a small payload from the cells in its scope on a cadence, checks a deterministic condition, and either starts a responder or wakes Warden. It costs no tokens; a model runs only when a watch fires. Warden adds watches from your words.` })]),
    el('div', { class: 'card', style: 'border-color:var(--brand)' }, [el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('div', { class: 'heading', text: 'Add a watch by telling Warden' }), el('div', { class: 'sm muted', text: '"watch the logs on edge-01 every 10 seconds and wake me on errors" · "add disk space alerts for production, and if disk is low clean up the logs, already approved"' })]), Button({ variant: 'primary', label: 'Open Warden', onClick: () => go('warden') })])]),
    el('div', { class: 'grid2' }, cards.length ? cards : [el('div', { class: 'empty', text: 'No watches yet.' })]),
  ]);
}
