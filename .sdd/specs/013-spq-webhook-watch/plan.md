# Implementation plan: storage quota webhook watch

- **Spec**: [spec.md](spec.md)
- **Epic**: vmop-4310
- **Date**: 2026-10-16

## Summary

Adapt `StoragePolicyQuotaMR` commit `6305b077e` to current main using the user's requested mapper and fan-out design. Retain the consolidated controller and shared webhook watch. Map a relevant webhook event to all watched StoragePolicyQuotas, then use the existing normal quota reconcile path for CA resolution and usage writes.

List quotas directly rather than locating stale usages and their owners. Every quota depends on the same webhook configuration, so there is no selective relationship to index. The mapper uses a read-only list with deep copying disabled because it accesses only names and namespaces. This avoids copying quota specs/status, avoids CA/Secret reads and usage lists in the mapper, includes quotas whose usages are missing, and naturally covers snapshot usages through the existing feature gate. The primary quota informer is already running and synchronized before event processing, so mapping normally reads the established manager cache. Unexpected list failures are logged; a MapFunc cannot return an error for requeue.

Remove cluster-scoped webhook-key dispatch and the standalone CA-only utility. Keep the manager's typed webhook cache because normal quota reconciliation already reads the full configuration. A metadata watch would add a second representation unless all typed reads changed; uncached reads would add API requests per quota reconcile.

Restrict the manager's `cache.Options.ByObject` entry for `admissionregistration/v1.ValidatingWebhookConfiguration` with `fields.OneTermEqualSelector("metadata.name", pkgconst.ValidatingWebhookConfigurationName)`. This selector applies to API list/watch requests, excluding unrelated configurations before decoding and storage. Keep the existing managed-field stripping transform. Move the name from SPQ utilities into `pkg/constants`, shared by manager setup, quota predicates, CA lookup, mapper, and tests. Production code has no other reader or watcher of this type; deployment manifests and webhook test installation use direct API clients. Other object caches remain unchanged.

## Technical context

- Root Go module, Go 1.26.9; existing controller-runtime dependency.
- Existing admissionregistration/v1 and cns.vmware.com/v1alpha2 types; no dependency changes.
- Modify `controllers/storage/storagepolicyquota/` and `pkg/util/kube/spq/`.
- Modify `pkg/constants/`, `pkg/manager/`, and `pkg/manager/test/` for the shared name and cache restriction.
- The dedicated infrastructure controller remains removed.

## Constitution check

| Rule | Result |
|------|--------|
| API and CRD compatibility | No type, schema, or RBAC permission changes. |
| Thin controllers / provider boundary | Watch orchestration in the controller; reusable mapper in SPQ utilities. No vSphere calls. |
| Context and feature gates | Normal reconcile joins manager config; preserve VMSnapshots gating. UnifiedStorageQuota is always enabled on current main. |
| Reconcile order, conditions, finalizers | No VM lifecycle ordering or new resource lifecycle. |
| Tests and E2E | External test packages and envtest labels. Replace tests of the removed CA-only path with mapper, managed-usage fan-out, snapshot, retry, status preservation, and no-op coverage. Existing managed-resource behavior is exercised with an actual API server and controller; no new Supervisor feature or admission behavior is introduced. |
| Copyright, errors, references | Current Go headers, contextual logging, upstream-safe references. |
| SDD tracking | Draft artifacts tracked under vmop-4310; shipping task ticket assignment remains required before merge. |

## Rollout and tradeoffs

No new flag or migration. Webhook events enqueue one key per watched quota, with normal queue deduplication and quota concurrency controls. Each quota resolves the CA and reconciles all matching storage classes; no global usage scan occurs in the mapper. CA and write failures retry per quota. Unexpected mapper-list errors require a subsequent relevant event or quota reconciliation. Orphan usages and usages without matching storage classes receive no independent repair. Normal quota reconciliation may correct controller-owned fields and invoke the existing snapshot extension migration.

## Verification

Test mapper filtering, all namespaces, quotas without usages, no CA/usage reads during mapping, empty lists, and list failures. Run controller envtests for CA rotation across multiple quotas/namespaces, enabled/disabled snapshots, Secret fallback recovery, webhook recreation, status preservation, and no-op suppression. Keep existing quota unit tests and CA source utility tests. Run focused lint and whitespace checks. Real Supervisor execution requires a configured cluster.

Manager envtests verify that an unrelated configuration exists through the API reader but is absent from cached Get/List, while the named configuration remains readable, tracks updates, disappears on deletion, and returns on recreation. Verify the actual list/watch requests contain the name selector. This changes internal cache retention only; existing quota envtests guard external behavior.
