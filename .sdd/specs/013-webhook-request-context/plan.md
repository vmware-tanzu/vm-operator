# Implementation Plan: Webhook Request Context Isolation

- **Spec**: [spec.md](spec.md)
- **Epic**: TBD
- **Date**: 2026-10-30

## Summary

Copy each handler's `WebhookContext` by value before constructing `WebhookRequestContext`. Join the synchronized manager configuration into the HTTP context unconditionally and store that context on the request-local copy. The existing VM-specific isolation helper continues to create a separate configuration only when an override is necessary.

## Technical context and structure

The root Go module (Go 1.26.10, controller-runtime admission handlers) changes only `pkg/builder/validating_webhook.go`, `pkg/builder/mutating_webhook.go`, and regression coverage in `pkg/builder/builder_test.go`. No new dependencies or API modules are involved.

## Constitution check and API strategy

- Controller/provider boundaries and API/CRD schemas are unchanged; no generation is required.
- Existing feature-gate semantics are preserved. This repairs request ownership and cancellation; no new feature flag is introduced.
- Tests use the external test package and the webhook label, with channels instead of sleeps.
- Ticket references remain pending before merge.
- E2E coverage is explicitly out of scope at the user's direction; handler regressions cover this fix.

## Webhook impact

Both mutating and validating handlers preserve HTTP cancellation, deadlines, and context values. Certificate verification uses the joined context as before; joining now also occurs when verification is disabled. Logger and recorder fields remain shared, and privilege checks continue using manager configuration. The VM webhook paths require configuration and request logging; no manager channel, watcher, or OVF-cache context is required by these paths. Existing request-specific logger fields remain intact.

## Test strategy

Run the builder suite with the race detector. Exercise create/update sequential isolation after a rejected request, visibility of later manager configuration updates, concurrent Kubernetes/non-Kubernetes requests, and `ServeHTTP` cancellation reaching a blocked client operation. Keep existing certificate and privilege tests passing. Confirm the sequential regressions fail on the original implementation. Run targeted lint on the modified package.

## Rollout

No migration or configuration changes. The fix applies to all generic admission handlers when the updated operator is deployed. Document the isolation and cancellation corrections in the release note.
