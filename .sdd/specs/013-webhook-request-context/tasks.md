# Tasks: Webhook Request Context Isolation

- **Spec**: [spec.md](spec.md)
- **Plan**: [plan.md](plan.md)
- **Epic**: TBD

## Implementation and validation

- [x] T001 Make `WebhookRequestContext` own its context and build every request context through `builder.NewWebhookRequestContext`, which keeps manager-context values and adds HTTP cancellation and deadline (`pkg/context/webhook_request_context.go`, `pkg/context/fake/fake_webhook_request_context.go`, `pkg/builder/request_context.go`, `pkg/builder/validating_webhook.go`, `pkg/builder/mutating_webhook.go`, `webhooks/unifiedstoragequota/validation/unifiedstoragequota_validator.go`, VM webhook `WebhookContext` field references). Shipping-code ticket: TBD; required before merge.
- [x] T002 Add unit tests for `builder.NewWebhookRequestContext` (`pkg/builder/request_context_test.go`). Shipping-code ticket: TBD; required before merge.
- [x] T003 Run the existing builder and VM-helper suites with the race detector.
- [x] T004 Run targeted lint and record the final verification in the PR.
- [ ] T005 Supply epic/story references before merge (`spec.md`, `plan.md`, `tasks.md`). Owner: maintainer. Blocked on ticket references.
