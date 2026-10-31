# Tasks: Webhook Request Context Isolation

- **Spec**: [spec.md](spec.md)
- **Plan**: [plan.md](plan.md)
- **Epic**: TBD

## Implementation and validation

- [x] T001 Make each webhook context request-local and preserve HTTP context propagation (`pkg/builder/validating_webhook.go`, `pkg/builder/mutating_webhook.go`). Shipping-code ticket: TBD; required before merge.
- [x] T003 Run the existing builder and VM-helper suites with the race detector.
- [x] T004 Run targeted lint and record the final verification in the PR.
- [ ] T005 Supply epic/story references before merge (`spec.md`, `plan.md`, `tasks.md`). Owner: maintainer. Blocked on ticket references.
