// Warden design system components as an ES module. Source of truth is
// design-system/project/components/bundle.js; keep the two in step.
export function el(tag, attrs, children) {
  const n = document.createElement(tag);
  if (attrs) Object.keys(attrs).forEach((k) => {
    const v = attrs[k];
    if (k === 'class') n.className = v;
    else if (k === 'text') n.textContent = v;
    else if (k === 'html') n.innerHTML = v;
    else if (k.indexOf('on') === 0 && typeof v === 'function') n.addEventListener(k.slice(2).toLowerCase(), v);
    else if (v === true) n.setAttribute(k, '');
    else if (v !== false && v != null) n.setAttribute(k, v);
  });
  (children || []).forEach((c) => { if (c == null || c === false) return; n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c); });
  return n;
}
export function Button(p = {}) {
  return el('button', { class: 'bc-btn bc-btn-' + (p.variant || 'quiet'), type: p.type || 'button', disabled: !!p.disabled, onClick: p.onClick, id: p.id, title: p.title }, [p.label || '']);
}
const sevWords = { critical: 'Critical', warning: 'Warning', info: 'Info', ok: 'Healthy' };
export function SeverityChip(p = {}) { const s = p.severity || 'info'; return el('span', { class: 'bc-chip bc-chip-' + s, text: p.label || sevWords[s] || s }); }
const statusTone = { open: 'critical', investigating: 'info', 'awaiting-approval': 'approve', resolved: 'ok', verified: 'ok', escalated: 'warning', unverified: 'neutral', executing: 'info', verifying: 'info', running: 'info', waiting: 'approve', done: 'ok', failed: 'warning', pending: 'approve', approved: 'ok', denied: 'warning', idle: 'neutral' };
const statusWords = { open: 'Open', investigating: 'Investigating', 'awaiting-approval': 'Awaiting approval', resolved: 'Resolved', verified: 'Verified', escalated: 'Escalated', unverified: 'Unverified', executing: 'Executing', verifying: 'Verifying', running: 'Running', waiting: 'Waiting', done: 'Done', failed: 'Failed', pending: 'Pending', approved: 'Approved', denied: 'Denied', idle: 'Idle' };
export function StatusChip(p = {}) { const s = p.status || 'open'; return el('span', { class: 'bc-chip bc-chip-' + (statusTone[s] || 'neutral'), text: p.label || statusWords[s] || s }); }
export function StatRow(p = {}) {
  return el('div', { class: 'bc-stat' }, (p.items || []).map((it) => typeof it === 'string' ? el('span', { text: it }) : el('span', null, [el('b', { text: it.value }), ' ' + (it.label || '')])));
}
export function EvidenceTable(p = {}) {
  const head = el('thead', null, [el('tr', null, [el('th', { text: 'Source' }), el('th', { text: 'Observed' }), el('th', { text: 'Pointer' })])]);
  const body = el('tbody', null, (p.rows || []).map((r) => el('tr', null, [el('td', { class: 'bc-kind', text: r.kind || '' }), el('td', { text: r.claim || '' }), el('td', { class: 'bc-ptr', text: r.pointer || '' })])));
  return el('table', { class: 'bc-evidence' }, [head, body]);
}
function arrow() {
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.setAttribute('viewBox', '0 0 16 16'); s.setAttribute('class', 'bc-chain-arrow'); s.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  p.setAttribute('d', 'M2 8h10M8 4l4 4-4 4'); p.setAttribute('fill', 'none'); p.setAttribute('stroke', 'currentColor'); p.setAttribute('stroke-width', '1.5');
  s.appendChild(p); return s;
}
export function CausalChain(p = {}) {
  const out = [];
  (p.steps || []).forEach((s, i) => { if (i > 0) out.push(arrow()); out.push(el('div', { class: 'bc-chain-step' + (s.severity ? ' bc-sev-' + s.severity : '') }, [el('time', { text: s.time || '' }), s.text || ''])); });
  return el('div', { class: 'bc-chain', role: 'list' }, out);
}
export function ApprovalCard(p = {}) {
  return el('div', { class: 'bc-approval', role: 'group', 'aria-label': 'Approval request' }, [
    el('h3', { text: p.finding || '' }),
    el('span', { class: 'bc-eyebrow', text: 'Proposed action' }), el('div', { class: 'bc-action', text: p.action || '' }),
    el('span', { class: 'bc-eyebrow', text: 'Blast radius' }), el('p', { class: 'bc-blast', text: p.blastRadius || '' }),
    el('div', { class: 'bc-buttons' }, [
      Button({ variant: 'primary', label: p.approveLabel || 'Approve', disabled: !p.canApprove, onClick: p.onApprove }),
      Button({ variant: 'quiet', label: 'Deny', disabled: !p.canApprove, onClick: p.onDeny }),
      el('span', { class: 'bc-who', text: p.canApprove ? 'You can approve this.' : 'Administrators approve this.' }),
    ]),
  ]);
}
const pillWords = { available: 'Available', missing: 'Missing permission', none: 'Not offered', note: 'Note' };
export function CapabilityList(p = {}) {
  return el('ul', { class: 'bc-caps' }, (p.items || []).map((it) => { const st = it.state || 'none'; return el('li', null, [el('span', { class: 'bc-cap-name', text: it.name || '' }), it.detail ? el('span', { class: 'bc-cap-detail', text: it.detail }) : null, el('span', { class: 'bc-pill bc-pill-' + st, text: pillWords[st] || st })]); }));
}
