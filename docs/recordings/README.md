# Recordings

| File | What |
|---|---|
| `2026-11-03-warden-walkthrough.mp4` | The show-and-tell line, light theme, 4 min 26 s, scripted mode with no model. Part 1, three steps: sign in, tenant, "upcloud", then Warden onboards, hands over the grant line and raises the cluster through two approvals in the panel. Part 2, teach Warden: a runbook from the operator's words, a log watch, a standing order for disk pressure. Part 3, it happens: nginx crashes and the log watch wakes Warden, who starts the responder and the fix is approved in the panel; the disk fills and the standing order rotates the logs with no approval. Part 4, a specialist from a template. |

`tools/record/record.py` regenerates it; see `tools/record/README.md`. The Beacon walkthrough from 2026-10-31 stays in the Beacon repo.
