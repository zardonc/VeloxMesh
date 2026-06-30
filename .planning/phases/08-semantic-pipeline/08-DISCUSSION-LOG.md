# Phase 8: Semantic Pipeline - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-06-29
**Phase:** 8-Semantic Pipeline
**Areas discussed:** Rule scope, configuration granularity, handler order, failure handling, Caveman/Ponytail request behavior

---

## Rule Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Define rule scope first | Confirm what RTK, Headroom, PII, Rewrite, Caveman, Ponytail, and Filter can touch before planning order | yes |
| Jump straight to implementation order | Decide order before conflict analysis | |

**User's choice:** Confirm rule scope and mutual conflicts first.
**Notes:** Rules must expose switches and default to disabled.

---

## Configuration Granularity

| Option | Description | Selected |
|--------|-------------|----------|
| Per-user plus global default | User config overrides global defaults; no provider/model/route layers yet | yes |
| Provider/model/route overrides | More expressive but broader and more complex | |

**User's choice:** Per-user configuration only, with global defaults underneath.
**Notes:** Each user's rule configuration must be isolated from other users. Avoid over-design for now.

---

## Handler Order

| Option | Description | Selected |
|--------|-------------|----------|
| Safety/privacy first | Filter and PII run before rewrite/compression; PII restore runs late | yes |
| Style first | Caveman/Ponytail or Rewrite before PII | |

**User's choice:** Safety/privacy first.
**Notes:** Proposed default request order is Filter(pre), PII Redaction, Rewrite, RTK, Headroom. Proposed default response order is Caveman or Ponytail, Filter(post), PII Restore after provider output.

---

## Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Skip failed handler and log | Continue pipeline while recording the exception | yes |
| Fail closed | Stop the request when a handler fails | |

**User's choice:** Skip failed handlers, but record handler exceptions.
**Notes:** Logs must not leak prompts, secrets, or sensitive content.

---

## Caveman/Ponytail Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Response-only | Never affect request text | |
| Style hint on request, transform response | Default request side uses only a style hint | yes |
| Explicit request rewrite option | User can choose to rewrite request text, default off | yes |

**User's choice:** Caveman/Ponytail are mutually exclusive by default. On request side, they normally provide only style hints, with a special user-enabled option to rewrite original request text. That option defaults off.
**Notes:** This avoids accidental changes to user intent while still allowing opt-in behavior.

---

## the agent's Discretion

- Keep the first implementation small.
- Do not add provider/model/route override layers in Phase 8.
- Defer full Admin Console UI to Phase 10 while preserving backend toggle capability.

## Deferred Ideas

- Provider/model/route/API-key override layers.
- Full Admin Console rule management UI.
- Complex conflict-resolution policy beyond Caveman/Ponytail mutual exclusion.
