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
  c.innerHTML = '<svg viewBox="0 0 24 24" width="18" height="18"><path d="M5 3l14 8.5-6.5 1.5L10 20z" fill="#000" stroke="#fff" stroke-width="1.5" stroke-linejoin="round"/></svg>';
  document.addEventListener('DOMContentLoaded', () => document.body.appendChild(c));
  window.addEventListener('mousemove', (e) => { c.style.left = e.clientX + 'px'; c.style.top = e.clientY + 'px'; });
  window.addEventListener('mousedown', () => { c.style.transform = 'translate(-2px,-2px) scale(.8)'; });
  window.addEventListener('mouseup', () => { c.style.transform = 'translate(-2px,-2px) scale(1)'; });
  window.__title = (t, sub) => {
    let o = document.getElementById('__title');
    if (!o) { o = document.createElement('div'); o.id = '__title'; document.body.appendChild(o); }
    o.style.cssText = 'position:fixed;inset:0;z-index:99998;display:grid;place-items:center;background:#ffffff;color:#09090b;font-family:Geist,Inter,system-ui,sans-serif;transition:opacity .4s';
    o.innerHTML = '<div style="text-align:center;max-width:720px"><div style="font-size:14px;letter-spacing:.2em;color:#71717a;text-transform:uppercase;margin-bottom:16px">' + sub + '</div><div style="font-size:44px;font-weight:600;letter-spacing:-.02em">' + t + '</div></div>';
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


def wait_for_warden(d: Demo, text, timeout=40):
    # wait until the orchestrator's latest message contains the text, then read it
    loc = d.page.locator("#home-thread .orchtext").last
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            if text.lower() in loc.inner_text(timeout=1000).lower():
                break
        except Exception:
            pass
        time.sleep(0.4)
    d.hover_read(loc, 2.5)


def ask(d: Demo, text, wait=None, pause=3):
    d.type(d.page.locator("#home-input"), text, delay=28)
    d.page.keyboard.press("Enter")
    if wait:
        wait_for_warden(d, wait)
    d.pause(pause)


def wait_panel_approval(d: Demo, timeout=40):
    deadline = time.time() + timeout
    while time.time() < deadline and d.page.locator(".band-approval").count() == 0:
        time.sleep(0.5)


def wait_resolved(d: Demo, timeout=90):
    import json, urllib.request
    deadline = time.time() + timeout
    while time.time() < deadline:
        st = json.loads(urllib.request.urlopen(f"{BASE}/api/state", timeout=2).read())
        if any(f["status"] == "resolved" for f in st["findings"]):
            return
        time.sleep(1)


MARKS = []  # (name, seconds from the start of the recording)
T0 = [None]


def mark(name):
    MARKS.append((name, time.time() - T0[0]))


def new_session(d: Demo):
    d.go("warden", 1.5)
    d.click(d.button("New session"), pause=1.5)


def step1(d: Demo):
    mark("1-connect")
    d.title("Step 1 · Connect", "sign in, create the tenant, name your provider; Warden connects the cells")
    d.go("login")
    d.type(d.page.get_by_placeholder("you@company.example"), "asitha@shop.example")
    d.type(d.page.get_by_placeholder("Password"), "demo-password", delay=30)
    d.click(d.button("Sign in"), pause=2)
    d.hover_read(d.page.get_by_text("The tenant gets its own runtime"), 2)
    d.click(d.button("Create tenant"), pause=1)
    wait_for_warden(d, "Which provider")
    ask(d, "upcloud", wait="opened the token form", pause=1)
    d.hover_read(d.page.get_by_text("Stored in the tenant secret store"), 2.5)
    d.click(d.button("Test and save connection"), pause=1)
    wait_for_warden(d, "Your connection is validated")
    d.hover_read(d.page.locator(".band.hot").first, 2)
    ask(d, "onboard everything", wait="Next: grant")
    d.hover_read(d.page.locator(".band").filter(has_text="Cells"), 2)
    ask(d, "grant edge-01", wait="Grant issued")
    d.hover_read(d.page.locator("#home-thread .codewrap").last, 3)
    wait_for_warden(d, "reported back")
    d.click(d.button("Ask Warden for API access"), pause=1)
    wait_panel_approval(d)
    d.hover_read(d.page.locator(".band-approval").first, 2.5)
    d.click(d.page.locator(".band-approval").get_by_role("button", name="Approve"), pause=1)
    wait_for_warden(d, "has API access")
    d.click(d.button("Ask Warden to install the probe"), pause=1)
    wait_panel_approval(d)
    d.click(d.page.locator(".band-approval").get_by_role("button", name="Approve"), pause=1)
    wait_for_warden(d, "nothing left")
    d.hover_read(d.page.locator(".panel-head"), 2.5)
    d.go("cells", 2.5)
    d.hover_read(d.page.locator("tr.row").first, 2.5)


def step2(d: Demo):
    mark("2-runbook")
    d.title("Step 2 · Runbooks", "a fresh session: tell Warden about the app in your words")
    new_session(d)
    ask(d, "add a runbook for edge-01", wait="Tell me in one message", pause=1)
    ask(d, "edge-01 runs nginx as a caching proxy in front of the shop. Restarting nginx is safe at any hour. Rotating the logs is safe. Never delete the cache under /var/cache/nginx. Page ops@shop.example for critical only.", wait="saved")
    d.hover_read(d.page.locator(".band").filter(has_text="Runbooks"), 2.5)
    d.go("runbooks", 2)
    d.hover_read(d.page.get_by_text("Applies when").first, 3)
    d.scroll(400, 2.5)


def step3(d: Demo):
    mark("3-watch")
    d.title("Step 3 · A watch", "a fresh session: add a watch, see it running, then something happens")
    new_session(d)
    ask(d, "watch the logs on edge-01 every 10 seconds and wake me on errors", wait="Created watch")
    d.go("watches", 2.5)
    d.hover_read(d.page.get_by_text("Logs on edge-01", exact=True).first, 2.5)
    d.hover_read(d.page.get_by_text("Running · every 10s").first, 2)
    d.pause(6)
    d.hover_read(d.page.get_by_text("Last run").first, 2.5)
    d.go("warden", 1.5)
    d.click(d.button("Crash nginx on edge-01"), pause=1)
    wait_for_warden(d, "fired on edge-01")
    wait_panel_approval(d)
    d.hover_read(d.page.locator(".band-approval").first, 3.5)
    d.click(d.page.locator(".band-approval").get_by_role("button", name="Approve fix"), pause=2)
    wait_resolved(d, 60)
    d.go("findings", 2.5)
    d.hover_read(d.page.locator("tr.row").first, 3)


def step4(d: Demo):
    mark("4-standing-order")
    d.title("Step 4 · A standing order", "a fresh session: pre-approve one action; Warden acts without asking")
    new_session(d)
    ask(d, "add disk space alerts for production, and if disk is low clean up the logs, already approved", wait="Created watch")
    d.go("watches", 2.5)
    d.hover_read(d.page.get_by_text("Standing order").first, 3)
    d.go("warden", 1.5)
    d.click(d.button("Fill the disk on edge-01"), pause=1)
    wait_for_warden(d, "pre-approves rotate_logs", timeout=90)
    d.hover_read(d.page.locator("#home-thread .orchtext").last, 3)
    wait_resolved(d, 90)
    d.pause(1.5)
    d.go("investigations", 2.5)
    d.click(d.page.locator("tr.row").first, pause=3)
    d.hover_read(d.page.get_by_text("standing order", exact=False).first, 3.5)
    d.scroll(700, 3)
    d.scroll(700, 3)


def step5(d: Demo):
    mark("5-specialist")
    d.title("Step 5 · A specialist", "a fresh session: Warden adds an agent from a template")
    new_session(d)
    ask(d, "add an nginx specialist for the edge servers", wait="Added")
    d.hover_read(d.page.locator(".band").filter(has_text="Agents"), 3)
    d.go("agents", 2.5)
    d.hover_read(d.page.get_by_text("nginx specialist", exact=True).first, 3)
    d.title("Warden", "the agent that tends the estate · prototype, scripted mode, no model", hold=3)
    mark("end")


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
            ctx = browser.new_context(viewport={"width": W, "height": H}, record_video_dir=str(OUT), record_video_size={"width": W, "height": H}, color_scheme="light")
            ctx.add_init_script(INIT_JS)
            page = ctx.new_page()
            d = Demo(page)
            page.goto(f"{BASE}/#login")
            T0[0] = time.time()
            d.pause(1.5)
            step1(d); step2(d); step3(d); step4(d); step5(d)
            video = page.video
            ctx.close()
            webm = Path(video.path())
            browser.close()
        mp4 = OUT / "warden-walkthrough.mp4"
        subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-i", str(webm), "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(mp4)], check=True)
        print("wrote", mp4, mp4.stat().st_size // 1024, "KB")
        # one clip per step, cut at the title cards
        for i in range(len(MARKS) - 1):
            name, start = MARKS[i]
            _, end = MARKS[i + 1]
            clip = OUT / f"warden-step-{name}.mp4"
            subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-ss", f"{start:.2f}", "-to", f"{end:.2f}", "-i", str(mp4), "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(clip)], check=True)
            print("wrote", clip, f"{end - start:.0f}s")
    finally:
        os.killpg(os.getpgid(server.pid), signal.SIGTERM)


if __name__ == "__main__":
    main()
