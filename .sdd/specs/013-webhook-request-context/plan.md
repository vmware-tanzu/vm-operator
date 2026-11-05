# Implementation Plan: Webhook Request Context Isolation

- **Spec**: [spec.md](spec.md)
- **Epic**: TBD
- **Date**: 2026-10-30

## Summary

`WebhookRequestContext` embeds its own request-scoped `context.Context` and holds the shared `WebhookContext` as a named, non-embedded field. A handler that replaces `ctx.Context` therefore only affects its own request. Every handler builds the request context through `builder.NewWebhookRequestContext`, which derives the context from the webhook's long-lived manager context (keeping its values) and cancels it, with the same cause, when the HTTP request is done and applies the HTTP deadline. The manager context is rooted at `context.Background()` and is never canceled, so request cancellation comes only from the HTTP request, as on `main`. The existing VM-specific isolation helper continues to create a separate configuration only when an override is necessary.

## Technical context and structure

The root Go module (Go 1.26.10, controller-runtime admission handlers) changes `pkg/context/webhook_request_context.go`, `pkg/context/fake/fake_webhook_request_context.go`, the new `pkg/builder/request_context.go`, the generic handlers in `pkg/builder/validating_webhook.go` and `pkg/builder/mutating_webhook.go`, and the unified storage quota handlers in `webhooks/unifiedstoragequota/validation/`. Reads of webhook-scoped fields formerly promoted through the embed (`Namespace`, `ServiceAccountName`) now go through `ctx.WebhookContext` explicitly. No new dependencies or API modules are involved.

## Constitution check and API strategy

- Controller/provider boundaries and API/CRD schemas are unchanged; no generation is required.
- Existing feature-gate semantics are preserved. This repairs request ownership and cancellation; no new feature flag is introduced.
- Existing test suites are updated only for the explicit `ctx.WebhookContext` field references.
- Ticket references remain pending before merge.
- New handler tests and E2E coverage are out of scope at the user's direction; `builder.NewWebhookRequestContext` has its own unit tests.

## Webhook impact

The mutating, validating, and unified storage quota handlers propagate HTTP cancellation and deadlines to downstream operations. Context values come from the manager context, so `pkglog.FromContextOrDefault` continues to return the manager logger and any manager-scoped value (configuration, channel sources, watcher, OVF cache) remains available. Certificate verification joins configuration into the HTTP context only when verification is enabled, as before. Logger and recorder fields remain shared, and privilege checks continue using manager configuration. The quota handlers now also populate `IsPrivilegedAccount`, `IsVMOperatorAccount`, and `RawObj`; they do not read these fields.

## Test strategy

Run the existing builder and VM-helper suites with the race detector, including existing certificate and privilege checks. Run targeted lint on the modified package. `pkg/builder/request_context_test.go` covers the request context constructor: manager-context values are retained, replacing the request context leaves the webhook context unchanged, HTTP cancellation (with cause) and deadline propagate, cancellation of the parent context propagates should it ever become cancelable, and the returned cancel function does not cancel either parent. The handlers themselves are not exercised by new tests.

## Rollout

No migration or configuration changes. The fix applies to all generic admission handlers and the unified storage quota handlers when the updated operator is deployed. Document the isolation and cancellation corrections in the release note.
