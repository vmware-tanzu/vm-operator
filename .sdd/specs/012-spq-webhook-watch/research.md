# Investigation and verification

## Baseline and original branch

Baseline main: `5a4c18798`. The quota controller resolves the webhook CA on every normal reconciliation, then creates/patches VM usages and enabled snapshot usages for matching storage classes. Its snapshot migration workaround may delete and recreate usages with an old immutable extension name.

The original infrastructure controller watched the named validating webhook configuration through a private cache, while reading it through the manager client. This created duplicate typed webhook informers during normal operation. Its reconciliation listed usages and patched only stale CA bundles, including usages outside normal quota management.

`StoragePolicyQuotaMR` commit `6305b077e` removes that controller and maps stale usages to quota owners. It predates the controller directory move, v1alpha6 API, current snapshot extensions, and removal of the UnifiedStorageQuota feature gate. Its mapper performs CA and usage reads before enqueueing; failures return no requests. Parent reconciliation cannot independently repair usages outside its matching storage classes.

## Final mapper design

Per the user's direction, the consolidated webhook watch now fans out directly to every watched StoragePolicyQuota. All quotas depend on the same CA source, and normal reconciliation already resolves that CA. There is no need to fetch the CA, list usages, test stale bundles, inspect owner references, or gate snapshot handling in the mapper. Quotas with missing usages are included as well.

The mapper uses the manager's established quota cache. Its list disables deep copies and reads only quota names/namespaces, avoiding allocation of copies of quota specs/status. It does not mutate or retain cached objects. Requests are normal namespaced quota keys; workqueue deduplication and existing quota concurrency apply. Controller-runtime may invoke the mapper for both the old and new objects on updates, but duplicate quota keys are coalesced by its mapped event handler.

The prior committed special dispatch for a cluster-scoped webhook key and the CA-only propagation helper have been removed. Normal quota reconciliation remains the only writer. CA resolution, storage class listing, and usage write failures retry per quota. An unexpected cached quota-list failure in the mapper is logged and returns no requests; it needs a subsequent relevant event or quota reconciliation to recover.

Keep the shared full-object webhook watch/cache. Normal quota reconciliation still reads the full configuration, so a metadata-only watch alone would create a second representation. API-reader reads would instead add a live GET per quota reconcile. This revision does not introduce either. Mapping allocates quota keys instead of a full usage list, while fan-out can increase queue entries and total reconcile work. The subsequent name selector described below reduces webhook objects retained by this cache; no measured heap or RSS reduction is claimed.

## Restrict webhook cache retention

A repository-wide search of production Go code for `ValidatingWebhookConfiguration`, its plural resource name, and the literal configuration name found only the quota controller watch and SPQ CA lookup/mapper. There are no other production readers, writers, unstructured accesses, or private caches for this type. Deployment YAML also defines the quota admission configuration, but serving an admission endpoint does not require caching its configuration. Test webhook installation uses the direct integration client. The only configuration needed by the manager cache is `vmware-system-vmop-validating-webhook-configuration`.

Move that name to `pkg/constants.ValidatingWebhookConfigurationName`. Both the manager's per-object field selector and quota consumers use this constant, avoiding a manager dependency on SPQ business utilities and preventing their selectors and lookup keys from diverging.

The manager now uses `cache.ByObject.Field` with `metadata.name=<name>`. In controller-runtime v0.25.2, `pkg/cache/internal/informers.go` applies the selector to both list and watch options through `Selector.ApplyToList`; unrelated objects are excluded by the API server before reaching the informer. The full configuration remains available to ordinary cached CA reads, including annotations used for Secret fallback. Deletion and recreation are observed by the same name-filtered watch. Other resource types and the existing managed-field stripping transform are unaffected.

This removes retained Go objects, index entries, and processing of unrelated configuration events. The user reports about 256 KB of YAML for the cluster's complete configuration list, but that is not a heap measurement: YAML/base64 size, Go object overhead, shared values, and the cache's existing managed-field stripping differ. Savings depend on the excluded configurations. This does not reduce the selected configuration itself or quota/usage cache retention, and it does not add a live GET per quota reconciliation.

References: [controller-runtime ByObject](https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.2/pkg/cache#ByObject), [selector application](https://github.com/kubernetes-sigs/controller-runtime/blob/v0.25.2/pkg/cache/internal/informers.go), [Kubernetes field selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/field-selectors/). ValidatingWebhookConfiguration is cluster-scoped, so `kubectl -A` does not expand this resource's scope.

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
- Name-filtered cache verification passed: 4 manager specs plus the 19 controller and 39 utility specs (62 total). The new manager envtest proves an unrelated configuration exists through the API reader but is absent from cached Get/List, verifies the selected configuration's update/delete/recreate lifecycle, and records actual cache HTTP requests to verify their name selector. The test wraps the supplied cache HTTP client's transport; wrapping only the REST config would miss requests because the manager already supplies that client. Focused lint passed with zero issues for `pkg/manager/...`, `pkg/constants`, the quota controller, and SPQ utilities; whitespace checks passed. Logs: `/tmp/spq-webhook-cache-tests.log`, `/tmp/spq-webhook-cache-lint.log`.

Real Supervisor E2E execution was not performed. Local integration tests validate the managed-resource API/watch/retry path but do not validate a live quota framework calling the extension service over TLS. No new Supervisor feature, admission behavior, API, CRD, or feature flag is introduced.
