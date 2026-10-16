# Investigation and verification

## Baseline and original branch

Baseline main: `5a4c18798`. The quota controller resolves the webhook CA on every normal reconciliation, then creates/patches VM usages and enabled snapshot usages for matching storage classes. Its snapshot migration workaround may delete and recreate usages with an old immutable extension name.

The original infrastructure controller watched the named validating webhook configuration through a private cache, while reading it through the manager client. This created duplicate typed webhook informers during normal operation. Its reconciliation listed usages and patched only stale CA bundles, including usages outside normal quota management.

`StoragePolicyQuotaMR` commit `6305b077e` removes that controller and maps stale usages to quota owners. It predates the controller directory move, v1alpha6 API, current snapshot extensions, and removal of the UnifiedStorageQuota feature gate. Its mapper performs CA and usage reads before enqueueing; failures return no requests. Parent reconciliation cannot independently repair usages outside its matching storage classes.

## Final mapper design

Per the user's direction, the consolidated webhook watch now fans out directly to every watched StoragePolicyQuota. All quotas depend on the same CA source, and normal reconciliation already resolves that CA. There is no need to fetch the CA, list usages, test stale bundles, inspect owner references, or gate snapshot handling in the mapper. Quotas with missing usages are included as well.

The mapper uses the manager's established quota cache. Its list disables deep copies and reads only quota names/namespaces, avoiding allocation of copies of quota specs/status. It does not mutate or retain cached objects. Requests are normal namespaced quota keys; workqueue deduplication and existing quota concurrency apply. Controller-runtime may invoke the mapper for both the old and new objects on updates, but duplicate quota keys are coalesced by its mapped event handler.

The prior committed special dispatch for a cluster-scoped webhook key and the CA-only propagation helper have been removed. Normal quota reconciliation remains the only writer. CA resolution, storage class listing, and usage write failures retry per quota. An unexpected cached quota-list failure in the mapper is logged and returns no requests; it needs a subsequent relevant event or quota reconciliation to recover.

Keep the shared full-object webhook watch/cache. Normal quota reconciliation still reads the full configuration, so a metadata-only watch alone would create a second representation. API-reader reads would instead add a live GET per quota reconcile. This revision does not introduce either. Webhook cache memory is unchanged from the prior consolidated commit; mapping now allocates quota keys instead of a full usage list, while fan-out can increase queue entries and total reconcile work. No heap or RSS reduction is claimed without measurement.

## Behavior scope

Managed VM and enabled snapshot usages converge through the existing quota path. Status and unrelated metadata remain intact; controller-owned spec fields and owner references are reconciled normally. Unchanged desired state causes no writes. Usages with no live quota/matching storage class no longer get an independent CA-only update. Snapshot extension migration can run as part of webhook-triggered quota reconciliation. These are explicit tradeoffs of the requested mapper design.

## Verification

- Baseline controller/util tests passed before consolidation.
- Utility tests retain CA source precedence and Secret fallback coverage. Mapper tests cover all namespaces, identical quota names across namespaces, quotas with no usages/CA source, unrelated configuration filtering, empty quota lists, and list failures. Interceptors reject CA reads or usage lists in the mapper.
- Controller unit tests retain normal quota creation, snapshot gating, ownership, upgrade migration, and error behavior. Added fault injection verifies a mapped quota request retries CA lookup, storage class lookup, and usage patch failures. The tests also verify manager-context snapshot settings and unchanged-write suppression.
- Envtests exercise an actual API server/etcd and consolidated controller. They verify VM creation, CA rotation with preserved quota status/ownership, VM and snapshot fan-out across namespaces, disabled snapshot/unmanaged usage exclusion, webhook recreation, and recovery after a missing Secret appears without another webhook event. The Secret test observes an increased reconcile error count before creating the Secret.
- Fixtures wait for the storage class cache before creating quotas and remove quotas explicitly during cleanup to prevent deleted test namespaces from leaving retrying resources.
- Final focused suite passed: 19 controller specs and 39 utility specs (58 total), including envtest integration cases. Focused lint passed with zero issues for the controller, SPQ utilities, and infrastructure registration package. `git diff --check` passed.
- RBAC permission markers are unchanged from the previously verified consolidation; the new mapper uses existing quota-list permission.

Real Supervisor E2E execution was not performed. Local integration tests validate the managed-resource API/watch/retry path but do not validate a live quota framework calling the extension service over TLS. No new Supervisor feature, admission behavior, API, CRD, or feature flag is introduced.
