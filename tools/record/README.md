# Recording the walkthrough

`record.py` starts its own Warden server on port 8091 in scripted mode, drives the installed Chrome headless with Playwright, records a WebM, and converts it to MP4 with ffmpeg.

```sh
sudo apt install -y ffmpeg python3-venv          # once
python3 -m venv tools/record/.venv
tools/record/.venv/bin/pip install playwright
# Playwright wants its own ffmpeg build for video; the system one works:
mkdir -p ~/.cache/ms-playwright/ffmpeg-1013 && ln -sf "$(command -v ffmpeg)" ~/.cache/ms-playwright/ffmpeg-1013/ffmpeg-linux

tools/record/.venv/bin/python tools/record/record.py
```

Output lands in `tools/record/out/warden-walkthrough.mp4`; copy it to `docs/recordings/`. The script uses the system Chrome (`channel="chrome"`); set `WARDEN_RECORD_CHANNEL=chromium` after `playwright install chromium` to use Playwright's own build instead.
