// Sparklines on canvas, fed from the metrics ring buffer.
export function spark(canvas, data, max, warn) {
  if (!canvas) return;
  const dpr = window.devicePixelRatio || 1; const w = canvas.clientWidth || 200, h = 36;
  canvas.width = w * dpr; canvas.height = h * dpr;
  const ctx = canvas.getContext('2d'); ctx.scale(dpr, dpr); ctx.clearRect(0, 0, w, h);
  const css = getComputedStyle(document.documentElement);
  const col = css.getPropertyValue(warn ? '--warning' : '--beam').trim(); const grid = css.getPropertyValue('--line').trim();
  ctx.strokeStyle = grid; ctx.lineWidth = 1; ctx.beginPath(); ctx.moveTo(0, h - 0.5); ctx.lineTo(w, h - 0.5); ctx.stroke();
  if (data.length < 2) return;
  const n = 89;
  ctx.strokeStyle = col; ctx.lineWidth = 1.5; ctx.beginPath();
  data.forEach((v, i) => { const x = i / n * w, y = h - 2 - Math.min(1, v / max) * (h - 6); if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y); });
  ctx.stroke();
  const last = data[data.length - 1]; const lx = (data.length - 1) / n * w, ly = h - 2 - Math.min(1, last / max) * (h - 6);
  ctx.fillStyle = col; ctx.beginPath(); ctx.arc(lx, ly, 2.5, 0, Math.PI * 2); ctx.fill();
}
