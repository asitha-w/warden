# Recordings

Light theme, scripted mode with no model, 1600 by 900. `tools/record/record.py` regenerates all of them in one run; see `tools/record/README.md`.

| File | What |
|---|---|
| `2026-11-03-warden-walkthrough.mp4` | All five steps in one take, about 5 minutes. |
| `2026-11-03-warden-step-1-connect.mp4` | Sign in, create the tenant, say "upcloud". Warden opens the token form, onboards what it found, hands over the grant line for edge-01 and raises the cluster through two approvals in the panel. Ends on the Cells page. 99 s. |
| `2026-11-03-warden-step-2-runbook.mp4` | Fresh session. "Add a runbook for edge-01", answer in one message, Warden writes and saves it. Ends on the Runbooks page. 41 s. |
| `2026-11-03-warden-step-3-watch.mp4` | Fresh session. "Watch the logs on edge-01 every 10 seconds and wake me on errors." The Watches page shows it running. nginx crashes, the watch fires, Warden starts the responder, the fix is approved in the panel, the finding resolves. 72 s. |
| `2026-11-03-warden-step-4-standing-order.mp4` | Fresh session. "Add disk space alerts for production, and if disk is low clean up the logs, already approved." The disk fills, the standing order fires, the logs rotate with no approval, the run shows the policy line. 62 s. |
| `2026-11-03-warden-step-5-specialist.mp4` | Fresh session. "Add an nginx specialist for the edge servers." Warden adds it from the template; the Agents page shows it. 30 s. |

Each step starts with "New session" on the home page: the thread clears, the estate stays.
