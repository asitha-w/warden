// A small Markdown renderer for runbooks and the documentation: headings,
// lists, paragraphs, fenced code, tables, links, bold and inline code. Builds
// DOM, never HTML strings. In docs mode headings get ids so links can target
// sections, and a line "::name arg" is handed to opts.directive to render a
// component in place.
import { el } from './ds.js';

export function slug(text) { return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, ''); }

function inline(text) {
  const out = [];
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\([^)]+\))/g;
  let last = 0; let m;
  while ((m = re.exec(text))) {
    if (m.index > last) out.push(document.createTextNode(text.slice(last, m.index)));
    const t = m[0];
    if (t.startsWith('`')) out.push(el('code', { text: t.slice(1, -1) }));
    else if (t.startsWith('**')) out.push(el('b', { text: t.slice(2, -2) }));
    else { const lm = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(t); out.push(el('a', { href: lm[2], text: lm[1] })); }
    last = m.index + t.length;
  }
  if (last < text.length) out.push(document.createTextNode(text.slice(last)));
  return out;
}

function cells(line) { return line.replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim()); }

export function markdown(src, opts = {}) {
  const root = el('div', { class: 'md' + (opts.docs ? ' md-docs' : '') });
  const lines = (src || '').split('\n');
  let list = null; let para = []; let i = 0;
  const flushPara = () => { if (para.length) { root.appendChild(el('p', null, inline(para.join(' ')))); para = []; } };
  const flushList = () => { list = null; };
  const heading = (level, text) => {
    const tag = opts.docs ? (level === 1 ? 'h2' : level === 2 ? 'h3' : 'h4') : (level === 1 ? 'h3' : 'h4');
    const attrs = opts.docs ? { id: slug(text) } : null;
    return el(tag, attrs, inline(text));
  };
  while (i < lines.length) {
    const line = lines[i].trimEnd();
    if (line.startsWith('```')) {
      flushPara(); flushList();
      const buf = []; i++;
      while (i < lines.length && !lines[i].startsWith('```')) { buf.push(lines[i]); i++; }
      root.appendChild(el('pre', { class: 'code', text: buf.join('\n') }));
      i++; continue;
    }
    if (line.startsWith('|') && i + 1 < lines.length && /^\|?\s*:?-{2,}/.test(lines[i + 1])) {
      flushPara(); flushList();
      const head = cells(line); i += 2;
      const rows = [];
      while (i < lines.length && lines[i].trimEnd().startsWith('|')) { rows.push(cells(lines[i].trimEnd())); i++; }
      root.appendChild(el('div', { class: 'tablewrap' }, [el('table', { class: 'table compact' }, [
        el('thead', null, [el('tr', null, head.map((h) => el('th', null, inline(h))))]),
        el('tbody', null, rows.map((r) => el('tr', null, r.map((c) => el('td', null, inline(c)))))),
      ])]));
      continue;
    }
    if (line.startsWith('::') && opts.directive) {
      flushPara(); flushList();
      const parts = line.slice(2).trim().split(/\s+/);
      const node = opts.directive(parts[0], parts.slice(1));
      if (node) root.appendChild(node);
      i++; continue;
    }
    const h = /^(#{1,3})\s+(.*)$/.exec(line);
    const ul = /^[-*]\s+(.*)$/.exec(line);
    const ol = /^\d+\.\s+(.*)$/.exec(line);
    if (!line) { flushPara(); flushList(); i++; continue; }
    if (h) { flushPara(); flushList(); root.appendChild(heading(h[1].length, h[2])); i++; continue; }
    if (ul || ol) { flushPara(); const tag = ul ? 'ul' : 'ol'; if (!list || list.tagName.toLowerCase() !== tag) { list = el(tag); root.appendChild(list); } list.appendChild(el('li', null, inline((ul || ol)[1]))); i++; continue; }
    flushList(); para.push(line); i++;
  }
  flushPara();
  return root;
}
