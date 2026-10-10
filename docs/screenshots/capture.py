"""Capture the README screenshots of the tadl web UI.

Serves the real app/ui/index.html with a mocked /health (a UVR42 with named sensors, one sensor
out of range, one output on; the frame counter advances on every request, so the receive LED
flashes) and photographs it with headless Chromium. Run from the project root, on demand only:

    docker run --rm -v "$PWD":/src -w /src mcr.microsoft.com/playwright/python:v1.52.0-noble \
        python3 docs/screenshots/capture.py

Writes docs/screenshots/web-ui*.png and docs/social-preview.png.
"""

import base64
import json
import threading
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[2]
PAGE = (ROOT / "app/ui/index.html").read_bytes()
OUT = ROOT / "docs/screenshots"
TZ = timezone(timedelta(hours=2))
PORT = 8766
UPTIME = 3 * 86400 + 4 * 3600 + 12 * 60

# Sample sensors: key, label, value (None = out of range), bar range and 15 min trend.
TEMPERATURES = [
    ("temperature1", "Collector", 118.6, -20, 150, 2.4),
    ("temperature2", "Tank top", 52.8, 0, 100, 0.6),
    ("temperature3", "Tank bottom", None, 0, 100, None),
    ("temperature4", "Boiler room", 18.2, -5, 25, -0.1),
]
OUTPUTS = [("out1", "Solar pump", True), ("out2", "", False)]

frames = 48213


def health():
    global frames
    frames += 1
    now = datetime.now(TZ).replace(microsecond=0)
    return {
        "app": "tadl", "appVersion": "1.7.0", "hostname": "pi-heating", "os": "linux",
        "uptimeSeconds": UPTIME, "mqtt": "connected",
        "mqttBroker": "192.168.1.5:1883", "mqttTopic": "tadl",
        "datalogger": {
            "type": "uvr42", "current": True,
            "lastFrame": (now - timedelta(seconds=2)).isoformat(), "lastFrameAgeSeconds": 2,
            "temperatures": [
                {"key": k, "label": l, "value": v, "min": lo, "max": hi, "trend15m": tr}
                for k, l, v, lo, hi, tr in TEMPERATURES
            ],
            "outputs": [{"key": k, "label": l, "on": on} for k, l, on in OUTPUTS],
        },
        "bus": {
            "signal": "receiving", "bitRateHz": 50.0, "invertedLine": True,
            "framesReceived": frames, "rejectedFrames": 0, "droppedFrames": 0,
            "protocolErrors": 3, "droppedEdges": 0,
            "errors24h": {"rejectedFrames": 0, "droppedFrames": 0, "protocolErrors": 3,
                          "droppedEdges": 0, "total": 3},
            "lastError": (now - timedelta(hours=2, minutes=14)).isoformat(),
            "lastErrorAgeSeconds": 2 * 3600 + 14 * 60,
            "decoder": "Decoder state: decoding data, Frequency: 50.00 Hz, Buffer overflow count: 0, Resync count: 0",
        },
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/":
            body, ctype = PAGE, "text/html; charset=utf-8"
        elif self.path == "/health":
            body, ctype = json.dumps(health()).encode(), "application/json"
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", ctype)
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


def shoot(browser, path, width, height, scheme, scale=1, full_page=True):
    ctx = browser.new_context(viewport={"width": width, "height": height},
                              color_scheme=scheme, device_scale_factor=scale)
    ctx.add_init_script("localStorage.setItem('tadl.apiKey', 'demo')")
    page = ctx.new_page()
    page.goto(f"http://localhost:{PORT}/")
    # The second poll brings the first frame difference; catch a moment with a lit receive LED.
    page.wait_for_function("document.querySelector('.rx .led.on') !== null", polling="raf", timeout=10000)
    page.screenshot(path=str(path), full_page=full_page)
    ctx.close()
    print("wrote", path.relative_to(ROOT))


def social(browser):
    shot = base64.b64encode((OUT / "web-ui.png").read_bytes()).decode()
    html = f"""<!doctype html><meta charset="utf-8">
<style>
  body {{ margin: 0; width: 1280px; height: 640px; background: #f3f5f7; font-family: system-ui, sans-serif;
         display: flex; align-items: center; gap: 56px; padding: 0 0 0 96px; box-sizing: border-box; overflow: hidden; }}
  .text {{ display: flex; flex-direction: column; gap: 22px; width: 470px; flex: none; }}
  .brand {{ display: flex; align-items: center; gap: 22px; }}
  svg {{ width: 84px; height: 84px; color: #2563a8; }}
  h1 {{ margin: 0; font-size: 84px; letter-spacing: -.02em; color: #17202b; }}
  h1 b {{ color: #2563a8; }}
  p {{ margin: 0; font-size: 30px; line-height: 1.3; color: #3d4a58; }}
  .tags {{ font-size: 21px; color: #5d6b7a; }}
  img {{ height: 520px; border-radius: 14px; box-shadow: 0 20px 50px rgba(23, 32, 43, .18);
         border: 1px solid #dde2e8; object-fit: cover; object-position: left top; width: 900px; }}
</style>
<div class="text">
  <div class="brand"><svg viewBox="0 0 34 34" fill="none" stroke-linecap="round">
    <path d="M9 4.5a3.5 3.5 0 0 1 7 0v16.2a6.5 6.5 0 1 1-7 0Z" stroke="currentColor" stroke-width="2.2"/>
    <circle cx="12.5" cy="26" r="3.6" fill="#d2452f"/><rect x="11.2" y="9" width="2.6" height="15" rx="1.3" fill="#d2452f"/>
    <path d="M21 8h10M21 14h7M21 20h9" stroke="#5d6b7a" stroke-width="2.2"/></svg>
    <h1>ta<b>dl</b></h1></div>
  <p>DL-Bus data logger for Technische Alternative UVR42 and UVR31 on the Raspberry Pi.</p>
  <span class="tags">MQTT · REST API · live web page</span>
</div>
<img src="data:image/png;base64,{shot}" alt="">"""
    page = browser.new_page(viewport={"width": 1280, "height": 640})
    page.set_content(html)
    path = ROOT / "docs/social-preview.png"
    page.screenshot(path=str(path))
    page.close()
    print("wrote", path.relative_to(ROOT))


def main():
    server = ThreadingHTTPServer(("localhost", PORT), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    OUT.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        browser = p.chromium.launch()
        shoot(browser, OUT / "web-ui.png", 1280, 520, "light")
        shoot(browser, OUT / "web-ui-dark.png", 1280, 520, "dark")
        # Phone: the first screen only.
        shoot(browser, OUT / "web-ui-phone.png", 390, 760, "light", scale=2, full_page=False)
        social(browser)
        browser.close()
    server.shutdown()


if __name__ == "__main__":
    main()
