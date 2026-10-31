# Implementation Plan: Webhook Request Context Isolation

- **Spec**: [spec.md](spec.md)
- **Epic**: TBD
- **Date**: 2026-10-30

## Summary

Copy each handler's `WebhookContext` by value before constructing `WebhookRequestContext`. Join the synchronized manager configuration into the HTTP context unconditionally and store that context on the request-local copy. The existing VM-specific isolation helper continues to create a separate configuration only when an override is necessary.

## Technical context and structure

The root Go module (Go 1.26.10, controller-runtime admission handlers) changes only `pkg/builder/validating_webhook.go` and `pkg/builder/mutating_webhook.go`. No new dependencies or API modules are involved.

## Constitution check and API strategy

- Controller/provider boundaries and API/CRD schemas are unchanged; no generation is required.
- Existing feature-gate semantics are preserved. This repairs request ownership and cancellation; no new feature flag is introduced.
- Existing test suites remain unchanged.
- Ticket references remain pending before merge.
- New handler tests and E2E coverage are out of scope at the user's direction.

## Webhook impact

Both mutating and validating handlers preserve HTTP cancellation, deadlines, and context values. Certificate verification uses the joined context as before; joining now also occurs when verification is disabled. Logger and recorder fields remain shared, and privilege checks continue using manager configuration. The VM webhook paths require configuration and request logging; no manager channel, watcher, or OVF-cache context is required by these paths. Existing request-specific logger fields remain intact.

## Test strategy

Run the existing builder and VM-helper suites with the race detector, including existing certificate and privilege checks. Run targeted lint on the modified package. These existing suites do not directly exercise concurrent request isolation or HTTP cancellation; those behaviors are assessed by source review for this change.

## Rollout

No migration or configuration changes. The fix applies to all generic admission handlers when the updated operator is deployed. Document the isolation and cancellation corrections in the release note.
