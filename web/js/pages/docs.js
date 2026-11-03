// Documentation: the model, one page per provider with the levels of reach,
// the grant, the probe, security, and the how-to list. Pages are Markdown
// files served from the binary; the "::layers" directive draws the layer
// graphic for a cell kind.
import { el } from '../ds.js';
import { markdown } from '../markdown.js';
import { store } from '../store.js';

export const DOC_PAGES = [
  ['model', 'How Warden reaches your infrastructure'],
  ['azure', 'Azure'],
  ['upcloud', 'UpCloud'],
  ['grant', 'The Warden grant'],
  ['probe', 'The probe'],
  ['security', 'Security and data'],
  ['howto', 'How-to'],
];

const cache = {}; const loading = {};
function load(page) {
  if (cache[page] || loading[page]) return;
  loading[page] = true;
  fetch('/docs/' + page + '.md').then((r) => r.ok ? r.text() : Promise.reject(new Error(r.statusText))).then((t) => { cache[page] = t; }).catch((e) => { cache[page] = '# Not found\n\nThis page could not be loaded: ' + e.message; }).then(() => { loading[page] = false; dispatchEvent(new HashChangeEvent('hashchange')); });
}

const NS = 'http://www.w3.org/2000/svg';
function sv(tag, attrs, children) {
  const n = document.createElementNS(NS, tag);
  Object.keys(attrs || {}).forEach((k) => { if (k === 'text') n.textContent = attrs[k]; else n.setAttribute(k, attrs[k]); });
  (children || []).forEach((c) => { if (c) n.appendChild(c); });
  return n;
}
const W = 500; // band width; the gutter to the right carries the arrows
function band(y, h, label, sub, cls, level, levelCls) {
  const g = sv('g', { class: 'band ' + (cls || '') }, [
    sv('rect', { x: 0, y, width: W, height: h, rx: 8 }),
    sv('text', { x: 16, y: y + 24, class: 'band-title', text: label }),
    sv('text', { x: 16, y: y + 44, class: 'band-sub', text: sub }),
  ]);
  if (level) { const w = level.length * 6.6 + 20; g.appendChild(sv('g', { class: 'pill ' + (levelCls || '') }, [sv('rect', { x: W - w - 12, y: y + 12, width: w, height: 22, rx: 11 }), sv('text', { x: W - w / 2 - 12, y: y + 27, 'text-anchor': 'middle', text: level })])); }
  return g;
}
function arrow(x, y1, y2, label, cls) {
  const down = y2 > y1; const ym = (y1 + y2) / 2;
  return sv('g', { class: 'arrow ' + (cls || '') }, [
    sv('line', { x1: x, y1, x2: x, y2: down ? y2 - 6 : y2 + 6, 'marker-end': 'url(#' + (down ? 'ah-down' : 'ah-up') + ')' }),
    label.length <= 10 ? sv('text', { x: x + 8, y: ym + 4, text: label }) : sv('text', { x: x + 6, y: ym, transform: 'rotate(-90 ' + (x + 6) + ' ' + ym + ')', 'text-anchor': 'middle', text: label }),
  ]);
}
function defs() {
  const m = (id, path) => sv('marker', { id, viewBox: '0 0 10 10', refX: 5, refY: 5, markerWidth: 7, markerHeight: 7, orient: 'auto' }, [sv('path', { d: path })]);
  return sv('defs', null, [m('ah-down', 'M0,0 L10,5 L0,10 z'), m('ah-up', 'M0,0 L10,5 L0,10 z')]);
}

// layers draws the stack for a VM or a cluster: the tenant runtime at the
// top, the provider API, then your cell, with the levels on the bands and the
// connections between the layers in the gutter on the right.
export function layers(kind) {
  const s = store.state; const egress = s && s.tenant && s.tenant.egressIp ? s.tenant.egressIp : 'one static address';
  const k8s = kind === 'k8s';
  const H = k8s ? 330 : 270;
  const svg = sv('svg', { viewBox: '0 0 720 ' + H, class: 'layers', role: 'img', 'aria-label': 'Layers of reach for a ' + (k8s ? 'cluster' : 'VM') });
  svg.appendChild(defs());
  svg.appendChild(band(0, 60, 'Warden tenant runtime', 'agents, policy gate, SSH certificate authority, telemetry store · egress ' + egress, 'runtime'));
  svg.appendChild(band(90, 60, 'Provider API', k8s ? 'cluster and node pool state, activity log · scale, start, stop' : 'state, hypervisor metrics, activity log · start, stop, restart', 'provider', 'Level 0 · connected', 'l0'));
  if (!k8s) {
    svg.appendChild(band(180, 90, 'Your VM', 'sshd with the Warden trust · warden user · the probe', 'cell', 'Level 1 · granted', 'l1'));
    svg.appendChild(arrow(540, 60, 90, 'connection', 'down'));
    svg.appendChild(arrow(600, 60, 180, 'SSH 22 from egress', 'down'));
    svg.appendChild(arrow(670, 180, 60, 'telemetry 443 out', 'up'));
  } else {
    svg.appendChild(band(180, 60, 'Kubernetes API server', 'workloads, events, logs · rollout undo, restart, scale, exec', 'api', 'Level 1 · API access', 'l1'));
    svg.appendChild(band(270, 60, 'Nodes', 'the probe as a DaemonSet and a cluster collector', 'cell', 'Level 2 · probe', 'l2'));
    svg.appendChild(arrow(540, 60, 90, 'connection', 'down'));
    svg.appendChild(arrow(600, 60, 180, 'kube identity from egress', 'down'));
    svg.appendChild(arrow(670, 270, 60, 'telemetry 443 out', 'up'));
  }
  return el('div', { class: 'layers-wrap' }, [svg]);
}

export function pageDocs(page, anchor) {
  const cur = DOC_PAGES.some((p) => p[0] === page) ? page : 'model';
  const nav = el('nav', { class: 'docs-nav', 'aria-label': 'Documentation' }, DOC_PAGES.map((p) => el('a', { href: '#docs/' + p[0], class: p[0] === cur ? 'active' : '', text: p[1] })));
  let body;
  if (cache[cur]) {
    body = markdown(cache[cur], { docs: true, directive: (name, args) => name === 'layers' ? layers(args[0]) : null });
    if (anchor) requestAnimationFrame(() => { const t = document.getElementById(anchor); if (t) t.scrollIntoView({ block: 'start' }); });
  } else { load(cur); body = el('div', { class: 'empty', text: 'Loading…' }); }
  return el('div', { class: 'page docs-page' }, [el('div', { class: 'docs' }, [nav, el('article', { class: 'docs-body' }, [body])])]);
}
