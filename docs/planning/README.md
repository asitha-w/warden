# Product planning

Working notes that shape Warden before code is written. Each file is one topic. `RESUME.md` at the repo root stays the short list of what to do next; these files hold the reasoning.

| File | Topic |
|---|---|
| `technical-gaps.md` | The areas where the prototype stands in for a real design, and the order to settle them. |
| `provider-access-and-control.md` | How far Warden can really reach into Azure and UpCloud: what a cell must have on its side, what identity and network path Warden needs, and what comparable products ask for. |
| `probe.md` | The probe per cell kind: what it is, how it is installed, how it is configured, and how it fails. |
| `cell-access-model.md` | The reference: what must exist on the customer's side and Warden's side at each level of reach, per cell kind, for Azure and UpCloud. |
| `hands-ssh.md` | Settled: how Warden acts on a VM. SSH from the static egress, reverse tunnel fallback, Ansible on top, per-tenant SSH CA. |
| `apps-and-cells.md` | Settled: the cell is the unit of reach, a connection finds cells, the application is discovered on a cell by the agent and recognised by runbooks; no app object. |
| `warden-skills-and-workflows.md` | Settled 2026-11-03: Warden is the only contact, capabilities are skills, runbooks are written by conversation, monitoring workflows are tenant objects that wake Warden, the log digest is the cheap tier, specialists come from templates. |
| `competitor-access-models.md` | Twenty operations agents compared on what they install, what identity they need, how they act and what they publish; what Warden takes from it. |
| `research/` | The five vendor-documentation reports behind the comparison, and the independent review of the access model, with every source URL. |

The customer-facing copy of the access model is `web/docs/` in the console (Documentation page, `#docs`). When `cell-access-model.md` changes, change `web/docs/azure.md` and `web/docs/upcloud.md` with it.
