# Configuration Application Verification Design

## Goal

Turn Provider and Routing saves into a traceable closed loop: persist the change, activate it in the live Gateway, read it back, verify it with a minimal data-plane request, and record a secret-free audit event.

## Status model

Every configuration mutation returns an `application` object:

- `applied`: the persisted revision was activated in the Gateway runtime.
- `verified`: activation was read back and a live request confirmed the expected provider or route.
- `warning`: persistence and activation succeeded, but live verification was unavailable or inconclusive.
- `failed`: persistence, activation, or readback failed. The UI must not show success.

The object also carries `revision`, `request_id`, observed `provider`/`route`, and a safe diagnostic message. It never contains credentials, prompt bodies, or upstream response bodies.

## Runtime application

Provider and Routing services synchronously rebuild the `RuntimeProviderManager` snapshot after persistence. Routing activation uses the stored default provider and strategy. Cross-instance configuration events are still published after local activation. If activation fails, the previous atomic runtime snapshot remains active and the mutation is explicitly reported as not applied.

## Closed-loop verification

The BFF performs the operation in this order:

1. Send the authenticated Admin API mutation with an operation request ID and actor.
2. Read the resource back and require the expected revision.
3. Call `/v1/models` for Provider verification or a minimal `/v1/chat/completions` request for Routing verification.
4. Compare the response headers (`X-Provider`, `X-Routing-Strategy`, `X-Request-ID`) with the expected target.
5. Return the application status to the Dashboard and write the corresponding audit metadata.

A separate verification endpoint supports retrying verification without rewriting configuration. The BFF uses a dedicated data-plane API key; the Admin API key is never reused or returned to the browser.

## Audit and identity

Trusted actor and request ID values are placed in typed request context after authentication. Audit records include actor, action, target, outcome, revision, and verification request ID. Secrets and raw request content are excluded.

## User experience

The System Management page renders verified, warning, and failed results distinctly. A successful HTTP write alone is not enough to show a success notice. Warning and failure messages include actionable, non-sensitive diagnostics.

## Testing

- Unit tests prove deterministic default-provider routing and snapshot replacement.
- Service tests prove Routing persistence activates the live runtime and preserves the old snapshot on failure.
- BFF tests prove mutation, readback, live verification, warning, failure, auth forwarding, and secret redaction.
- Frontend tests prove the application state is rendered correctly.
- E2E tests prove a route change changes the provider observed on a subsequent real Gateway request and can be found in Audit by actor and request ID.

