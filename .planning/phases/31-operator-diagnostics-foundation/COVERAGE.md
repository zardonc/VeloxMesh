# Phase 31 — Provider Diagnostics API Coverage

**Date:** 2026-10-17. Planning decisions only; no endpoint or SDK behavior is certified by this matrix.

Full coverage of each existing provider's diagnostic surface is considered below. This phase extends diagnostics on the three current adapter families; it does not add a provider or claim to integrate every product in a vendor SDK. The API detector examined CONTEXT and returned `detected: true`; a matrix is appropriate because real upstream non-generating calls are being introduced.

| capability | decision | reason |
| --- | --- | --- |
| OpenAI-compatible bounded non-generating connectivity probe | INTEGRATE | Verify the configured upstream's supported non-generating operation; unsupported proxies return unsupported/unknown, with no inference fallback. |
| Anthropic bounded non-generating connectivity probe | INTEGRATE | Validate the pinned SDK's candidate model-list operation against its implementation and official contract before use; no unbounded paging or inference fallback. |
| Gemini bounded non-generating connectivity probe | INTEGRATE | Validate the pinned SDK's candidate model-list operation and authentication semantics before use; public discovery success alone does not prove credential acceptance. |
| OpenAI-compatible explicit manual generation check | INTEGRATE | Use a registered provider/model and bounded existing non-stream generation path, separate admin probe accounting and controlled wire attempts. |
| Anthropic explicit manual generation check | INTEGRATE | Apply the same explicit-action, audit, output/time/call limits and usage isolation to the existing adapter. |
| Gemini explicit manual generation check | INTEGRATE | Apply the same explicit-action, audit, output/time/call limits and usage isolation to the existing adapter. |
| Connectivity/authentication/configuration/generation outcome classification | INTEGRATE | Report only what the operation demonstrates; preserve unknown/unsupported and bounded error codes, without clearing real-traffic failures. |
| Per-call SDK/transport limits, cancellation and redirect handling | INTEGRATE | Reuse actually verified Phase 30 controls; finite wire count, response bytes and duration, no redirects carrying credentials to a new target. |
| Probe target schema validation and registered-target restrictions | INTEGRATE | No arbitrary URL supplied to the test endpoint; preserve legitimate configured self-hosted providers under explicit target validation. |
| Public model/account capability directory synchronization | OPT-OUT | User chose Console prerequisites; model listings here are bounded probe operations, not a complete synchronized capability catalog. |
| Background generating inference or paid fallback from a light probe | OPT-OUT | User explicitly selected background non-generating probes and manual generation only. |
| Streaming generation as an administrator probe mode | OPT-OUT | Minimal generation validation uses the existing bounded non-stream path; client stream timing is validated separately through gateway traffic fixtures. |
| Vendor non-chat products and account administration APIs | OPT-OUT | Embeddings, images, audio, files, batches, fine-tuning and account administration are unrelated to the requested diagnostic operations and existing chat forwarding contract. |
| New provider families, OAuth flows or account credential management | OPT-OUT | Existing adapters and configured secrets are reused; provider/account expansion is deferred. |
| External trace/log-search service query integration | OPT-OUT | First delivery is an optional bounded node-local metadata store; existing optional trace export remains a separate observation path. |

For every integrated non-generating operation, execution must record the pinned SDK source/official reference and actual local HTTP fixture observations, including unsupported, public/discovery-only, rejected credentials, redirect, oversized response, timeout and cancellation. Use one bounded response/page; success does not certify model generation availability. Local fixtures establish gateway behavior, not availability of every remote proxy. No dependency upgrade or live credentials are part of this plan.
