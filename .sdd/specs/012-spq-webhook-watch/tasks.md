# Tasks: storage quota webhook watch

- **Spec**: [spec.md](spec.md)
- **Plan**: [plan.md](plan.md)
- **Epic**: vmop-4310
- **Shipping task ticket**: TBD; assign under vmop-4310 before merge.

- [x] T001 Compare `StoragePolicyQuotaMR` with current controllers and document the requested mapper/fan-out approach in `spec.md` and `plan.md`.
- [x] T002 Add the quota fan-out mapper in `pkg/util/kube/spq/spq.go`, wire it into `controllers/storage/storagepolicyquota/storagepolicyquota_controller.go`, and remove the special webhook reconcile path and CA-only helper.
- [x] T003 Replace obsolete CA-only tests with mapper coverage in `pkg/util/kube/spq/spq_test.go` and managed-usage fan-out/retry coverage in `controllers/storage/storagepolicyquota/storagepolicyquota_controller_unit_test.go` and `controllers/storage/storagepolicyquota/storagepolicyquota_controller_intg_test.go`.
- [x] T004 Run focused suites/lint and record final behavior and verification in `research.md`.
- [x] T005 Verify all webhook configuration consumers, move the shared name into `pkg/constants/constants.go`, and restrict the manager cache by name in `pkg/manager/manager.go`; update callers in `pkg/util/kube/spq/` and `controllers/storage/storagepolicyquota/`.
- [x] T006 Test server-side filtering and cache lifecycle in `pkg/manager/test/cache_test.go`; rerun quota/util suites and focused lint, then record results in `research.md` and the scoped follow-up in `.sdd/specs/000-sdd/report.md`.
