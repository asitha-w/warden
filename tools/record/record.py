"""Record one continuous walkthrough video of the Warden console.

Starts its own server on port 8091 in scripted mode, drives a headless
Chromium with Playwright, records WebM, then converts to MP4 with ffmpeg.

    tools/record/.venv/bin/python tools/record/record.py

Output: tools/record/out/warden-walkthrough.mp4 (copy to docs/recordings/).
"""
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tools" / "record" / "out"
PORT = int(os.environ.get("WARDEN_RECORD_PORT", "8091"))
BASE = f"http://localhost:{PORT}"
W, H = 1600, 900

# A visible cursor and a part title overlay, injected into every page.
INIT_JS = """
(() => {
  const c = document.createElement('div');
  c.id = '__cursor';
  c.style.cssText = 'position:fixed;left:0;top:0;width:18px;height:18px;z-index:99999;pointer-events:none;transform:translate(-2px,-2px);transition:transform .05s';
  c.innerHTML = '<svg viewBox="0 0 24 24" width="18" height="18"><path d="M5 3l14 8.5-6.5 1.5L10 20z" fill="#fff" stroke="#000" stroke-width="1.5" stroke-linejoin="round"/></svg>';
  document.addEventListener('DOMContentLoaded', () => document.body.appendChild(c));
  window.addEventListener('mousemove', (e) => { c.style.left = e.clientX + 'px'; c.style.top = e.clientY + 'px'; });
  window.addEventListener('mousedown', () => { c.style.transform = 'translate(-2px,-2px) scale(.8)'; });
  window.addEventListener('mouseup', () => { c.style.transform = 'translate(-2px,-2px) scale(1)'; });
  window.__title = (t, sub) => {
    let o = document.getElementById('__title');
    if (!o) { o = document.createElement('div'); o.id = '__title'; document.body.appendChild(o); }
    o.style.cssText = 'position:fixed;inset:0;z-index:99998;display:grid;place-items:center;background:#09090b;color:#fafafa;font-family:Geist,Inter,system-ui,sans-serif;transition:opacity .4s';
    o.innerHTML = '<div style="text-align:center;max-width:720px"><div style="font-size:14px;letter-spacing:.2em;color:#a1a1aa;text-transform:uppercase;margin-bottom:16px">' + sub + '</div><div style="font-size:44px;font-weight:600;letter-spacing:-.02em">' + t + '</div></div>';
    o.style.opacity = '1';
  };
  window.__untitle = () => { const o = document.getElementById('__title'); if (o) { o.style.opacity = '0'; setTimeout(() => o.remove(), 450); } };
})();
"""


class Demo:
    def __init__(self, page):
        self.page = page
        self.x, self.y = W / 2, H / 2

    def pause(self, s):
        time.sleep(s)

    def title(self, text, sub, hold=2.8):
        self.page.evaluate("([t, s]) => window.__title(t, s)", [text, sub])
        self.pause(hold)
        self.page.evaluate("() => window.__untitle()")
        self.pause(0.6)

    def glide(self, x, y, steps=28):
        self.page.mouse.move(x, y, steps=steps)
        self.x, self.y = x, y
        self.pause(0.25)

    def click(self, locator, pause=1.2):
        # glide the visible cursor there, then let Playwright click: it retries
        # if the console re-renders the element between mouse down and up.
        target = locator.first
        target.scroll_into_view_if_needed()
        self.pause(0.3)
        box = target.bounding_box()
        if box:
            self.glide(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
        try:
            target.click(timeout=8000)
        except Exception:
            target.dispatch_event("click")
        self.pause(pause)

    def type(self, locator, text, delay=45):
        self.click(locator, pause=0.3)
        self.page.keyboard.type(text, delay=delay)
        self.pause(0.5)

    def go(self, hash_, pause=1.8):
        self.page.goto(f"{BASE}/#{hash_}")
        self.pause(pause)

    def scroll(self, dy, pause=1.4):
        self.page.mouse.wheel(0, dy)
        self.pause(pause)

    def button(self, name):
        return self.page.get_by_role("button", name=name)

    def hover_read(self, locator, s=2.0):
        # a changed sentence must never stall a recording: skip quietly if absent
        try:
            locator.first.scroll_into_view_if_needed(timeout=4000)
            box = locator.first.bounding_box(timeout=4000)
        except Exception:
            box = None
        if box:
            self.glide(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
        self.pause(s)


def part1(d: Demo):
    d.title("Onboarding", "Part 1 of 4 · sign in, tenant, connections, cells, probes, agents")
    d.go("login")
    d.type(d.page.get_by_placeholder("you@company.example"), "asitha@shop.example")
    d.type(d.page.get_by_placeholder("Password"), "demo-password", delay=30)
    d.click(d.button("Sign in"), pause=2)
    d.hover_read(d.page.get_by_text("The tenant gets its own runtime"), 2)
    d.click(d.button("Create tenant"), pause=2.5)
    # the console points at the next step: a connection
    d.hover_read(d.page.get_by_text("Start by creating a connection").first, 2.5)
    d.click(d.button("Go to connections"), pause=2)
    # UpCloud first: sub-account, token, grants
    d.click(d.button("Add UpCloud"), pause=1.5)
    d.hover_read(d.page.get_by_text("Restrict the sub-account to the servers"), 2)
    d.click(d.button("Validate token"), pause=3)
    # Azure: directory, identity, read scope, write scope
    d.click(d.button("Add Azure"), pause=1.5)
    d.hover_read(d.page.get_by_text("Consent creates a service principal"), 2.2)
    d.scroll(380, 1.5)
    d.hover_read(d.page.get_by_text("Reader roles are assigned here"), 2.2)
    d.hover_read(d.page.get_by_text("Contributor roles are assigned here"), 2.2)
    d.click(d.button("Consent and validate"), pause=3.5)
    d.scroll(-900, 1.5)
    # discovery: what each connection found, what is manageable, then onboard
    d.hover_read(d.page.get_by_text("Discovered in scope").first, 2.5)
    d.hover_read(d.page.get_by_text("not supported yet").first, 2.2)
    d.click(d.button("Onboard selected as cells").first, pause=2.5)
    d.click(d.button("Onboard selected as cells").first, pause=2.5)
    # the next step moves to cells: install the probe
    d.go("cells", 2.5)
    d.hover_read(d.page.get_by_text("can be raised").first, 2.5)
    d.click(d.button("Raise all cells (demo)"), pause=2)
    d.pause(14)
    d.hover_read(d.page.get_by_text("one isolated channel per cell"), 3)
    d.hover_read(d.page.get_by_text("identity shared with").first, 2.5)
    d.hover_read(d.page.get_by_text("dedicated identity").first, 2.5)
    # one cell in depth: provider sensors and probe telemetry
    d.click(d.page.locator("tr.row").first, pause=2.5)
    d.hover_read(d.page.get_by_text("Provider sensors"), 3)
    d.hover_read(d.page.get_by_text("Probe telemetry"), 3)
    d.hover_read(d.page.get_by_text("/var/log/nginx/access.log"), 2.5)
    d.click(d.page.get_by_text("Collector config the probe receives"), pause=3)
    d.go("agents", 2.5)
    d.hover_read(d.page.get_by_text("Orchestrator", exact=True).first, 2)
    d.hover_read(d.page.get_by_text("Investigator", exact=True).first, 2)
    d.scroll(600, 2)
    d.hover_read(d.page.get_by_text("Cost advisor", exact=True).first, 1.5)
    card = d.page.locator(".agent-card").filter(has_text="Cost advisor")
    d.click(card.locator("input[type=checkbox]"), pause=2.5)


def part2(d: Demo):
    d.title("Runbooks", "Part 2 of 4 · tenant guidance the agents follow")
    d.go("runbooks", 2.5)
    d.hover_read(d.page.get_by_text("Shop web on VMs").first, 2)
    d.scroll(300, 2)
    d.scroll(-300, 1)
    d.click(d.button("New runbook"), pause=1.5)
    d.type(d.page.locator("#rb-name"), "Shop data on managed PostgreSQL")
    d.type(d.page.locator("#rb-sel"), "app=shop, tier=data")
    d.click(d.page.locator("#rb-body"), pause=0.3)
    d.page.keyboard.press("Control+A")
    body = (
        "# Shop data on managed PostgreSQL\n\n"
        "UpCloud managed PostgreSQL 16 behind the shop API. Nightly backup 01:00 to 02:00 CET.\n\n"
        "## Health signals\n- cpu_percent above 80 during the backup window is normal; outside it, investigate.\n"
        "- connections above 80 means the API is leaking connections.\n\n"
        "## Known fixes, in order\n1. High connections: ask the API owner to restart shop-api. Never kill sessions on the database.\n"
        "2. Slow queries: capture pg_stat_statements and hand it to data@shop.example.\n\n"
        "## Never\n- Never resize or restart the managed database from Warden.\n\n"
        "## Escalation\ndata@shop.example. Page for Critical only.\n"
    )
    d.page.keyboard.type(body, delay=12)
    d.pause(1)
    d.click(d.button("Create runbook"), pause=2)
    d.scroll(900, 2)
    d.hover_read(d.page.get_by_text("No cell matches this selector yet"), 2.5)


def part3(d: Demo):
    d.title("An incident, end to end", "Part 3 of 4 · signal, investigation, approval, verification")
    d.go("findings", 2.5)
    d.click(d.button("Ship a bad version of shop-api"), pause=1)
    d.hover_read(d.page.get_by_text("Error rate, aks-shop"), 6)
    d.pause(5)
    d.click(d.page.locator("tr.row").first, pause=3)
    d.hover_read(d.page.get_by_text("Runbooks handed to the investigator"), 3)
    d.pause(4)
    d.scroll(500, 3)
    d.scroll(500, 3)
    d.scroll(600, 3)
    d.hover_read(d.page.get_by_text("Blast radius").first, 3)
    d.click(d.button("Approve fix"), pause=3)
    d.scroll(500, 4)
    d.scroll(500, 4)
    d.pause(5)
    d.scroll(400, 3)
    d.go("findings", 3)
    d.hover_read(d.page.locator("tr.row").first, 3)


def part4(d: Demo):
    d.title("Working with the agents", "Part 4 of 4 · the operator and the orchestrator resolve an outage")
    d.go("chat", 2)
    d.click(d.button("Crash nginx on edge-01"), pause=1)
    d.go("chat", 2)
    d.pause(5)
    d.type(d.page.locator("#chat-input"), "customers say the edge is down, what is open?")
    d.page.keyboard.press("Enter")
    d.pause(5)
    d.type(d.page.locator("#chat-input"), "why is edge-01 failing?")
    d.page.keyboard.press("Enter")
    d.pause(6)
    d.click(d.button("Open approval").first, pause=3)
    d.scroll(900, 3)
    d.scroll(900, 2)
    d.type(d.page.locator("#ask-input"), "is restarting nginx safe right now, any downtime?")
    d.page.keyboard.press("Enter")
    d.pause(5)
    d.click(d.button("Approve fix"), pause=3)
    d.scroll(600, 4)
    d.pause(6)
    d.scroll(600, 3)
    d.go("consumption", 3)
    d.hover_read(d.page.get_by_text("By agent"), 3)
    d.title("Warden", "agentic reliability operations · prototype, scripted mode", hold=3)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for f in OUT.glob("*.webm"):
        f.unlink()
    server = subprocess.Popen(
        ["go", "run", "./cmd/warden"], cwd=ROOT,
        env={**os.environ, "WARDEN_ADDR": f":{PORT}", "WARDEN_PACE_MS": "900", "WARDEN_WEB_DIR": ""},
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, preexec_fn=os.setsid,
    )
    try:
        for _ in range(60):
            try:
                import urllib.request
                urllib.request.urlopen(f"{BASE}/api/state", timeout=1).read()
                break
            except Exception:
                time.sleep(1)
        else:
            raise SystemExit("server did not start")
        with sync_playwright() as p:
            browser = p.chromium.launch(channel=os.environ.get("WARDEN_RECORD_CHANNEL", "chrome"))
            ctx = browser.new_context(viewport={"width": W, "height": H}, record_video_dir=str(OUT), record_video_size={"width": W, "height": H}, color_scheme="dark")
            ctx.add_init_script(INIT_JS)
            page = ctx.new_page()
            d = Demo(page)
            page.goto(f"{BASE}/#login")
            d.pause(1.5)
            part1(d); part2(d); part3(d); part4(d)
            video = page.video
            ctx.close()
            webm = Path(video.path())
            browser.close()
        mp4 = OUT / "warden-walkthrough.mp4"
        subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-i", str(webm), "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(mp4)], check=True)
        print("wrote", mp4, mp4.stat().st_size // 1024, "KB")
    finally:
        os.killpg(os.getpgid(server.pid), signal.SIGTERM)


if __name__ == "__main__":
    main()
