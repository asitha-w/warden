import { el, Button, SeverityChip, StatusChip } from '../ds.js';
import { store, sel } from '../store.js';
import { go } from '../router.js';
import { spark } from '../charts.js';
import { ago } from '../shell.js';

const TILES = [
  { id: 'probes', label: 'Cells · probes online', sub: 'isolated channels' },
  { id: 'rate', label: 'Metrics per minute', sub: 'into the tenant cell', host: 'web-01', metric: 'requests_per_second', max: 500, scale: 4 },
  { id: 'disk', label: 'Disk, web-01', sub: 'disk_used_percent · threshold 90%', host: 'web-01', metric: 'disk_used_percent', max: 100, warn: 90, pct: true },
  { id: 'err', label: 'Error rate, aks-shop', sub: 'error_rate_percent · threshold 5%', host: 'aks-shop', metric: 'error_rate_percent', max: 25, warn: 5, pct: true },
  { id: 'cpu', label: 'CPU, edge-01', sub: 'hostmetrics · UpCloud', host: 'edge-01', metric: 'cpu_percent', max: 100, pct: true },
];

export function pageFindings() {
  const s = store.state;
  const tiles = el('div', { class: 'tiles' }, TILES.map((t) => el('div', { class: 'tile' }, [el('span', { class: 'label', text: t.label }), el('div', { class: 'val' }, [el('span', { class: 'numeral', id: 'v-' + t.id, text: '—' })]), t.metric ? el('canvas', { id: 'sp-' + t.id, height: '36' }) : el('div', { style: 'height:36px' }), el('span', { class: 'sm muted', text: t.sub })])));
  const rows = s.findings.map((f) => el('tr', { class: 'row', onClick: () => go('inv/' + f.runId) }, [
    el('td', null, [SeverityChip({ severity: f.severity })]), el('td', null, [el('div', { text: f.title }), el('div', { class: 'mono muted', text: f.id + ' · ' + f.resource })]),
    el('td', null, [f.service ? el('span', { class: 'badge', text: 'app=' + f.service }) : el('span', { class: 'muted sm', text: '—' })]), el('td', null, [StatusChip({ status: f.status })]), el('td', { class: 'mono muted', text: ago(f.createdAt) })]));
  const table = el('div', { class: 'tablewrap' }, [el('table', { class: 'table' }, [el('thead', null, [el('tr', null, [el('th', { text: 'Severity' }), el('th', { text: 'Finding' }), el('th', { text: 'Group' }), el('th', { text: 'Status' }), el('th', { text: 'Age' })])]), el('tbody', null, rows.length ? rows : [el('tr', null, [el('td', { colspan: '5', class: 'muted', text: 'No findings. The estate is quiet; inject a fault from the demo bar to see a run.' })])])])]);
  const step = sel.nextStep();
  const banner = step ? el('div', { class: 'card', style: 'border-color:var(--brand)' }, [el('div', { class: 'bc-row', style: 'justify-content:space-between' }, [el('div', null, [el('div', { class: 'heading', text: step.tip }), el('div', { class: 'sm muted' }, [step.detail + ' ', el('a', { href: step.doc, class: 'doclink', text: 'Read how it works' })])]), Button({ variant: 'primary', label: step.nav === 'cells' ? 'Go to cells' : 'Go to connections', onClick: () => go(step.nav) })])]) : null;
  return el('div', { class: 'page' }, [el('div', null, [el('h1', { class: 'display', text: 'Findings' }), el('p', { class: 'muted', id: 'v-last', text: 'waiting for telemetry' })]), banner, tiles, table]);
}

// drawTiles runs on every metrics event without a page re-render.
export function drawTiles() {
  const s = store.state; if (!s || !document.getElementById('v-probes')) return;
  const m = store.metrics;
  document.getElementById('v-probes').textContent = `${s.cells.length} · ${s.probe.hosts.length}`;
  TILES.filter((t) => t.metric).forEach((t) => {
    const v = sel.metric(t.host, t.metric); const node = document.getElementById('v-' + t.id);
    const series = sel.series(t.host, t.metric);
    if (!m.online || v === undefined) { node.textContent = '—'; node.style.color = ''; spark(document.getElementById('sp-' + t.id), [], t.max, false); return; }
    const shown = t.scale ? v * t.scale : v;
    node.textContent = t.pct ? shown.toFixed(shown < 10 ? 1 : 0) + '%' : Math.round(shown).toLocaleString();
    const warn = t.warn !== undefined && v > t.warn;
    node.style.color = warn ? 'var(--critical)' : '';
    spark(document.getElementById('sp-' + t.id), series, t.max, warn);
  });
  const la = document.getElementById('v-last');
  if (la) la.textContent = m.online ? 'telemetry flowing · last sample ' + Math.max(0, Math.round((Date.now() - m.at) / 1000)) + 's ago' : 'waiting for telemetry';
}
