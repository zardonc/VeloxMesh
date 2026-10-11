---
schema_version: 1
open_count: 3
waived_count: 0
fixed_count: 3
total_count: 6
last_updated: 2026-09-25T01:02:18.223Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 28 | deviation | .planning/phases/28-tool-calling-protocol-completion/28-01-SUMMARY.md |  | ToolProtocolRequirements propagation was added because the initial handler wiring discarded later routing preflight data. | fixed |  | 2026-09-24T20:13:25.506Z | 2026-09-24T20:14:07.075Z |
| 2 | 28 | deviation | internal/http/handlers/chat.go |  | Handler work was split into helpers to meet the AGENTS.md 50-line function limit. | fixed |  | 2026-09-24T20:13:26.151Z | 2026-09-24T20:14:07.668Z |
| 3 | 28 | deviation | internal/providers/capabilities.go |  | Added authoritative normalized protocol predicate and explicit assistant-history capability to fail closed. | fixed |  | 2026-09-24T20:59:42.804Z | 2026-09-24T21:00:19.150Z |
| 4 | 28 | deviation | internal/providers/toolstream/state.go |  | Later tool fragments preserve the incoming wire shape while shadow state retains identity. | open |  | 2026-09-25T01:02:17.173Z |  |
| 5 | 28 | deviation | internal/providers/toolstream/state.go |  | Unfinished opaque argument buffers are capped per tool call. | open |  | 2026-09-25T01:02:17.710Z |  |
| 6 | 28 | deviation | internal/providers/adaptertest/tool_contract.go |  | The harness permits valid first-turn tool requests and validates supplied tool-result history. | open |  | 2026-09-25T01:02:18.223Z |  |

````json
[
  {
    "id": 1,
    "kind": "deviation",
    "phase": "28",
    "file": ".planning/phases/28-tool-calling-protocol-completion/28-01-SUMMARY.md",
    "line": null,
    "description": "ToolProtocolRequirements propagation was added because the initial handler wiring discarded later routing preflight data.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-24T20:13:25.506Z",
    "resolved_at": "2026-09-24T20:14:07.075Z",
    "milestone": "v7.9"
  },
  {
    "id": 2,
    "kind": "deviation",
    "phase": "28",
    "file": "internal/http/handlers/chat.go",
    "line": null,
    "description": "Handler work was split into helpers to meet the AGENTS.md 50-line function limit.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-24T20:13:26.151Z",
    "resolved_at": "2026-09-24T20:14:07.668Z",
    "milestone": "v7.9"
  },
  {
    "id": 3,
    "kind": "deviation",
    "phase": "28",
    "file": "internal/providers/capabilities.go",
    "line": null,
    "description": "Added authoritative normalized protocol predicate and explicit assistant-history capability to fail closed.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-24T20:59:42.804Z",
    "resolved_at": "2026-09-24T21:00:19.150Z",
    "milestone": "v7.9"
  },
  {
    "id": 4,
    "kind": "deviation",
    "phase": "28",
    "file": "internal/providers/toolstream/state.go",
    "line": null,
    "description": "Later tool fragments preserve the incoming wire shape while shadow state retains identity.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-25T01:02:17.173Z",
    "resolved_at": null,
    "milestone": "v7.9"
  },
  {
    "id": 5,
    "kind": "deviation",
    "phase": "28",
    "file": "internal/providers/toolstream/state.go",
    "line": null,
    "description": "Unfinished opaque argument buffers are capped per tool call.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-25T01:02:17.710Z",
    "resolved_at": null,
    "milestone": "v7.9"
  },
  {
    "id": 6,
    "kind": "deviation",
    "phase": "28",
    "file": "internal/providers/adaptertest/tool_contract.go",
    "line": null,
    "description": "The harness permits valid first-turn tool requests and validates supplied tool-result history.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-25T01:02:18.223Z",
    "resolved_at": null,
    "milestone": "v7.9"
  }
]
````
