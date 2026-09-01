# Functional Test Specification: `VirtualMachineReplicaSet`

**Status**: Draft
**Scope**: `VirtualMachineReplicaSet` only (`api/v1alpha6/virtualmachinereplicaset_types.go`). `VirtualMachineDeployment` and `VirtualMachineStatefulSet` are out of scope for this document — they build on top of the behavior specified here and should get their own TDS.
**Source material**:

- One Pager: *VM Service: Workload Management of VMs using Kubernetes Patterns* (Confluence page 1007649487)
- Kubernetes `apps/v1` `ReplicaSet` (the pattern this API is explicitly modeled after)
- KubeVirt `VirtualMachineInstanceReplicaSet` (`https://kubevirt.io/user-guide/user_workloads/replicaset/`) — the closest prior art for VM-shaped replicas rather than Pod-shaped replicas

**Purpose of this document**: define the observable, user-facing contract for `VirtualMachineReplicaSet` in enough detail that a suite of tests written against these acceptance criteria — independent of any particular controller implementation — gives confidence the feature is safe to ship/switch on. This is written **before** looking at controller code, so it reflects intended behavior, not implemented behavior. Any test that fails against the real implementation is either a bug to fix or a deliberate deviation that must be called out and reconciled with this document.

---

## 1. API contract recap

From `virtualmachinereplicaset_types.go`:

| Field | Notes |
|---|---|
| `spec.replicas` (`*int32`, default `1`) | Desired replica count. Pointer so explicit `0` is distinguishable from unset. |
| `spec.deletePolicy` (`string`, enum `Random`) | Only `Random` is a legal non-empty value today. |
| `spec.selector` (`*metav1.LabelSelector`, required) | Must match `spec.template.metadata.labels`. |
| `spec.template` (`VirtualMachineTemplateSpec`) | `metadata` (labels/annotations) + `spec` (`VirtualMachineSpec`) blueprint for replicas. |
| `status.replicas` | Observed current replica count. |
| `status.fullyLabeledReplicas` | Replicas whose labels match the template's labels. |
| `status.readyReplicas` | Replicas whose owned `VirtualMachine`'s `Ready` condition is `True`. |
| `status.observedGeneration` | Standard reconciler-freshness field. |
| `status.conditions` | `VirtualMachinesCreated`, `VirtualMachinesReady`, `Resized` (reasons `ScalingUp`/`ScalingDown`), plus `ReplicaFailure` reason. |
| Label `vmoperator.vmware.com/replicaset-name` | Applied by the controller to every owned `VirtualMachine`. |
| Scale subresource | `specpath=.spec.replicas`, `statuspath=.status.replicas` — i.e. `kubectl scale` / HPA-style clients must work. |

This is a **level-triggered controller over a fixed template**, not a rollout controller: there is no revision history, no rolling-update strategy, and no auto-scaling here — those are explicitly Non-Goals in the one-pager and belong to `VirtualMachineDeployment` (rollout) or are out of scope entirely (auto-scaling, affinity/anti-affinity, backup/restore).

---

## 2. Test categories and scenarios

Each scenario is written as **Given / When / Then**, is implementation-agnostic, and states the expected outcome only in terms of the CR's spec/status and the set of `VirtualMachine` objects in the namespace.

### 2.1 Basic reconciliation — bring actual state to desired state

1. **Create with N replicas from scratch**
   Given a new `VirtualMachineReplicaSet` with `spec.replicas=3`, a selector, and a template whose labels satisfy the selector.
   When the object is created.
   Then exactly 3 `VirtualMachine` objects exist in the namespace, each:
   - has an owner reference to the `VirtualMachineReplicaSet`,
   - carries all labels/annotations from `spec.template.metadata`,
   - carries the `vmoperator.vmware.com/replicaset-name` label set to the ReplicaSet's name,
   - has a `Spec` equal to `spec.template.spec`,
   - has a unique, generated name (not a fixed/predictable suffix — no ordinal identity, unlike StatefulSet).
   And `status.replicas == 3`, `status.fullyLabeledReplicas == 3`.

2. **Default replicas**
   Given a `VirtualMachineReplicaSet` created with `spec.replicas` unset.
   Then the persisted object's `spec.replicas == 1` (CRD default) and exactly one `VirtualMachine` is created.

3. **Explicit zero replicas**
   Given `spec.replicas=0`.
   Then no `VirtualMachine` objects are created (or, if some existed already from a previous higher replica count, all are deleted), and `status.replicas == 0`. The distinction between "explicit 0" and "unset" must not regress to defaulting.

4. **Idempotent reconciliation / no-op steady state**
   Given a `VirtualMachineReplicaSet` already at `status.replicas == spec.replicas` with all VMs healthy.
   When the controller reconciles again with no external changes.
   Then no `VirtualMachine` is created, deleted, or mutated, and `status.observedGeneration` remains aligned with `metadata.generation` (no spurious writes/conflicts).

5. **Controller restart / resync produces the same result**
   Given the same starting state as above, but the reconciliation is triggered by a full resync rather than a watch event (i.e., no assumption about create/update/delete event type — this is a level-triggered reconciler).
   Then the resulting state is identical to scenario 4 — reconciliation must be safe to run from a cold cache.

### 2.2 Scaling

6. **Scale up**
   Given a healthy `VirtualMachineReplicaSet` with `status.replicas == 2`.
   When `spec.replicas` is updated to `5`.
   Then 3 additional `VirtualMachine` objects are created (the original 2 are left untouched — not recreated), and eventually `status.replicas == 5`.
   And a `Resized` condition is set with reason `ScalingUp` during the transition.

7. **Scale down**
   Given `status.replicas == 5`.
   When `spec.replicas` is updated to `2`.
   Then exactly 3 owned `VirtualMachine` objects are deleted and 2 remain, and eventually `status.replicas == 2`.
   And a `Resized` condition is set with reason `ScalingDown` during the transition.

8. **Scale-down selection honors `deletePolicy: Random`**
   Given `deletePolicy: Random` and 5 replicas, some healthy and some unhealthy (e.g., one `VirtualMachine` is not yet `Ready`, or has a recent restart).
   When scaling down by 1.
   Then the implementation is free to pick any replica (that's what "Random" means) — the test should NOT assert a specific replica is deleted. Instead it should assert the **invariants** that must hold regardless of which one is chosen:
   - exactly one fewer `VirtualMachine` exists afterward,
   - the deleted VM is one that was actually owned by this ReplicaSet (never a foreign/unrelated VM),
   - the remaining VMs are untouched (not recreated, not mutated).

9. **Scale down to zero and back up**
   Given a ReplicaSet scaled to 0 (all VMs deleted), then scaled back to 3.
   Then 3 *new* `VirtualMachine` objects are created (no attempt to "resurrect" deleted VMs by name/UID), and `status.replicas == 3`.

10. **Rapid successive scale changes ("flapping")**
    Given `spec.replicas` is changed multiple times in quick succession (e.g., 1 → 5 → 2) before the controller fully converges on any intermediate value.
    Then the controller eventually converges to match the **final** value of `spec.replicas` (2), without leaking extra VMs and without deleting more than necessary at any point (no thrashing that deletes and recreates VMs unnecessarily for the same desired count).

11. **`kubectl scale` / scale subresource**
    Given a `VirtualMachineReplicaSet`.
    When a client updates only the `/scale` subresource (as `kubectl scale vmreplicaset foo --replicas=4` would), not the full object.
    Then this has the identical effect as editing `spec.replicas` directly on the main resource, and `status.replicas` continues to reflect the scale subresource's status path.

12. **Negative or absurd replica counts are rejected**
    Given an attempt to set `spec.replicas = -1`.
    Then the API server/webhook rejects the request (CRD/webhook validation), and no reconciliation is attempted against a negative count.

### 2.3 Selector / template consistency

13. **Selector must match template labels (webhook/validation)**
    Given a `VirtualMachineReplicaSet` where `spec.selector` does not match `spec.template.metadata.labels`.
    Then creation is rejected by validation with a clear error message — this must be caught at admission time, not discovered later as a silently-broken ReplicaSet. (KubeVirt's ReplicaSet notably does *not* enforce this and instead just logs errors while doing nothing useful; that is called out here explicitly as the behavior we do **not** want to replicate.)

14. **Selector is immutable-in-effect / changing it doesn't retroactively relabel VMs**
    Given a running ReplicaSet with 3 replicas matching selector A.
    When `spec.selector` is changed to selector B (assuming the API allows the edit at all).
    Then existing VMs are not relabeled to match B. If B no longer matches the existing VMs' labels, those VMs become unmanaged (orphaned relative to this ReplicaSet) and the controller creates *new* replicas matching B up to `spec.replicas`, rather than mutating live VM specs to satisfy the new selector. (Mutating a running `VirtualMachine`'s identity out from under a user would be a change with real blast radius — VMs are not disposable/ephemeral the way Pods are.)

15. **Template spec changes do not retroactively update existing replicas**
    Given a running ReplicaSet with 3 replicas created from template T1.
    When `spec.template.spec` is edited to T2 (e.g., different VM class, different image).
    Then the 3 existing `VirtualMachine` objects are **not** mutated/recreated as a side effect of the template edit alone (there is no rolling-update strategy on `ReplicaSet` itself — that's `VirtualMachineDeployment`'s job).
    And any *subsequent* scale-up creates new replicas using T2, producing a (temporarily) heterogeneous set — this divergence is expected and must not be "corrected" by the ReplicaSet controller.

16. **Template metadata (labels/annotations) changes: in-place propagation or not?**
    `[NEEDS CLARIFICATION]` — Kubernetes Pod `ReplicaSet` never mutates existing Pods' labels/annotations after creation, only stamps them at creation time. Confirm expected behavior:
    Given a running ReplicaSet with 3 replicas.
    When `spec.template.metadata.labels` (a non-selector-breaking label) is edited.
    Then either (a) existing VMs keep their original labels (Pod ReplicaSet parity — recommended default), or (b) labels are propagated in place. Whichever is chosen must be codified as a test and documented; silently doing (b) while other systems assume (a) resembles the risk called out for `VirtualMachineDeployment`'s metadata-only propagation, so mixing behavior between the two CRDs is confusing and should be an explicit, tested decision.

### 2.4 Ownership, orphans, and adoption

17. **Deleting the ReplicaSet cascades to its VMs**
    Given a ReplicaSet with 3 owned VMs.
    When the `VirtualMachineReplicaSet` is deleted (foreground or background deletion).
    Then all 3 owned `VirtualMachine` objects are eventually deleted too (owner-reference-driven garbage collection), and no orphaned VMs remain with a dangling owner reference.

18. **Deleting one managed VM triggers replacement**
    Given a healthy ReplicaSet with 3 replicas.
    When one owned `VirtualMachine` is deleted directly by a user/operator (bypassing the ReplicaSet).
    Then the controller creates exactly one new `VirtualMachine` to restore `status.replicas == spec.replicas`, and `status.replicas` transiently reflects 2 before returning to 3.

19. **A pre-existing VM that happens to match the selector *is* adopted, visibly**
    Given a standalone `VirtualMachine` (no owner reference, created independently of any ReplicaSet) whose labels match a ReplicaSet's `spec.selector`.
    When the `VirtualMachineReplicaSet` is created/reconciled.
    Then the ReplicaSet **adopts** that VM (sets itself as controller owner reference) rather than leaving it permanently unmanaged. This supersedes the one-pager's "adoption is future work" framing: adoption here never relocates or reconfigures the VM — it is a pure ownership claim — so it does not run into the zone/placement-mobility limits that motivate deferring true affinity/relocation features. Adopting also keeps the controller self-healing (a ReplicaSet created after a matching standalone VM already exists should not get permanently stuck below its desired replica count). Concretely:
    - the adopted VM counts toward `spec.replicas` (the controller does not create a redundant new VM if the adopted one alone satisfies the desired count),
    - a **`Condition`** is set on the `VirtualMachineReplicaSet` recording that this replica was adopted rather than created (in addition to the existing `SuccessfulAdopt` Event, which is transient and not sufficient on its own — it doesn't survive being scrolled off and isn't visible via `kubectl describe`),
    - the adopted VM's spec is *not* forced to match the template (same non-goal as scenario 15 — adoption doesn't imply conformance).
    Tracked as vmop-4017 (epic vmop-1701) for the missing Condition — the test for this scenario is expected to fail on the Condition assertion until that ticket is implemented; that is intentional and should not be "fixed" by weakening the test.

20. **Two ReplicaSets with overlapping selectors ("fighting over" VMs)**
    Given two `VirtualMachineReplicaSet` objects whose selectors both match an overlapping set of labels.
    Then this is a user-error scenario (same as upstream Kubernetes and KubeVirt): the test should assert the system does not corrupt state or crash-loop, and each ReplicaSet only ever deletes/creates VMs it actually owns (checked via owner reference), never a VM owned by the *other* ReplicaSet. A `Condition` surfacing this misconfiguration is desirable but the hard requirement is "no cross-ownership deletion."

21. **Namespace scoping**
    Given a `VirtualMachineReplicaSet` in namespace `ns-a` and a `VirtualMachine` with matching labels in namespace `ns-b`.
    Then the ReplicaSet in `ns-a` never considers, counts, adopts, or deletes the VM in `ns-b` — selector matching is namespace-scoped, consistent with `VirtualMachine` and `ReplicaSet` both being namespaced resources.

### 2.5 Status and conditions

22. **`status.replicas` always reflects actual owned-VM count**
    At every point across create/scale-up/scale-down/delete-and-replace scenarios above, `status.replicas` must equal the number of `VirtualMachine` objects currently owned by the ReplicaSet — never a stale or aspirational number.

23. **`status.fullyLabeledReplicas` detects label drift on owned VMs**
    Given a ReplicaSet with 3 owned VMs, then a user manually removes/changes a label on one owned VM so it no longer matches the template's labels (but the VM is still owned/not orphaned since ownership is by owner-reference, not by label).
    Then `status.fullyLabeledReplicas` decreases to 2 while `status.replicas` remains 3 — these two fields must be able to diverge, and both must be independently observable.

24. **`status.readyReplicas` tracks the owned VMs' `Ready` condition**
    Given 3 owned VMs where 2 have `Ready=True` and 1 has `Ready=False` (e.g., still powering on or provisioning failed).
    Then `status.readyReplicas == 2`. As the third VM's `Ready` condition flips to `True`, `status.readyReplicas` becomes 3 without any other spec change.

25. **`VirtualMachinesCreated` condition reflects create-path failures**
    Given a template that will fail to produce a valid `VirtualMachine` (e.g., references a nonexistent `VirtualMachineClass`, or otherwise fails admission when the controller tries to create it).
    Then `VirtualMachinesCreated` is `False` with reason `VirtualMachineCreationFailed` and a message containing the underlying error, `status.replicas` reflects only the VMs that *did* get created (not the failed attempts), and the controller keeps retrying (does not give up permanently) — an amended template that fixes the error should let the ReplicaSet self-heal without requiring the object to be recreated.

26. **`ReplicaFailure` condition surfaces ongoing failure to maintain desired count**
    Given a scenario where a required replica keeps failing to become healthy (e.g., a class/image that always fails to power on, or a finalizer that blocks deletion of a replica the controller is trying to replace).
    Then a condition/reason around `ReplicaFailure` is set, distinguishing "we are still trying and failing" from "everything converged."

27. **`VirtualMachinesReady` condition aggregates readiness**
    Given `status.readyReplicas < spec.replicas`.
    Then `VirtualMachinesReady` is `False`. Once `status.readyReplicas == spec.replicas` (and `>= 1` if replicas is nonzero), it becomes `True`. For `spec.replicas == 0`, define and test the expected condition state explicitly (`[NEEDS CLARIFICATION]`: likely `True`/vacuously-ready, but must be codified rather than left ambiguous, since some Ready aggregations for empty sets choose `Unknown` instead).

28. **`observedGeneration` freshness**
    Given a spec edit (e.g., scale from 2 to 4).
    Then `status.observedGeneration` only advances to the new `metadata.generation` once the controller has actually processed that generation's spec — a consumer polling status should never see `observedGeneration` matching the new generation while `status.replicas` still reflects the old desired count in a way inconsistent with "reconciliation has started."

29. **`Resized` condition clears once converged**
    Given a scale-up/down that has fully converged (actual == desired, all healthy).
    Then the `Resized` condition either clears or transitions to a "not resizing"/steady state — it must not get stuck reporting `ScalingUp`/`ScalingDown` forever after convergence, since that would mislead anyone using it as a rollout-progress signal.

### 2.6 Label / annotation propagation details

30. **`vmoperator.vmware.com/replicaset-name` label is present and correct on every owned VM**
    For every `VirtualMachine` created by a `VirtualMachineReplicaSet`, the label `vmoperator.vmware.com/replicaset-name` equals the ReplicaSet's `metadata.name`, and this label is also usable as a reliable way to `kubectl get vm -l vmoperator.vmware.com/replicaset-name=<name>` and get exactly the owned set (modulo the orphan/adoption caveats in §2.4).

31. **User edits to that label on an owned VM don't break ownership tracking, but do break label-based discovery**
    Given a user manually removes the `replicaset-name` label from an owned VM.
    Then owner-reference-based operations (cascade delete, scale-down accounting via `status.replicas`) still work correctly since they don't depend on this label, but label-based tooling built on top of it will (correctly, expectedly) no longer find that VM — this is a documented limitation, not a bug, and the test differentiates "ownership" (owner ref, authoritative) from "discoverability" (label, best-effort).

### 2.7 Interaction with `VirtualMachine` lifecycle

32. **A replica's own finalizers don't stall the whole ReplicaSet**
    Given a scale-down that needs to delete VM `x`, but `x` has a slow-draining finalizer (e.g., attached volume detachment in progress).
    Then the ReplicaSet controller issues the delete and waits — it does not spin up a *replacement* replica while `x` is terminating in a way that would temporarily overshoot `spec.replicas` (unless `x`'s deletion is specifically what's being counted toward the decrease). Assert final convergence: once `x` fully terminates, `status.replicas == spec.replicas` and no more, no less.

33. **A replica that is deleted by something other than the ReplicaSet (e.g., a competing controller, storage policy quota enforcement, or manual `kubectl delete`) is treated identically to any other missing replica**
    This is really a restatement of scenario 18, but explicitly generalized: the controller has no special-cased logic keyed on *why* a VM disappeared — it is purely level-triggered on "count of currently-owned, live VirtualMachines vs. `spec.replicas`."

34. **Power state / VM.Spec fields inside the template are passed through verbatim, not defaulted/mutated by the ReplicaSet controller**
    Given `spec.template.spec.powerState = PoweredOn`.
    Then every created replica's `Spec.PowerState` is exactly `PoweredOn` — the ReplicaSet controller must not impose its own opinion on fields that belong entirely to `VirtualMachineSpec`'s own webhook/defaulting; it is a pure stamping operation from template to instance.

### 2.8 Explicit non-goals to assert as *absence* of behavior

These deserve tests too — asserting a feature does **not** exist is how you catch scope creep or accidental behavior changes later:

35. No rolling update / no revision history: confirm there is no `ControllerRevision`-equivalent, no rollout status, and no partial/staged replacement strategy anywhere in `VirtualMachineReplicaSet` — that belongs to `VirtualMachineDeployment`.

36. No auto-scaling: `status.replicas` never changes in response to VM resource utilization; only external edits to `spec.replicas` change desired count. (An HPA *pointed at* the scale subresource is fine and expected to work per scenario 11 — that's an external actor, not built-in behavior.)

37. No anti-affinity / spreading: replicas created by a `VirtualMachineReplicaSet` carry no implicit affinity or anti-affinity rules on placement — any host spreading is out of scope for this CRD per the one-pager's Non-Goals, so a test should confirm replicas can legitimately land on the same host/cluster module without the controller treating that as an error.

38. No shared/stable storage or stable network identity: unlike `VirtualMachineStatefulSet`, replicas from a `VirtualMachineReplicaSet` get no per-ordinal PVC and no stable name/network identity — every replica's name is opaque/generated and interchangeable. A test recreating a replica should show the new replica has a *different* generated name, never reusing the deleted replica's identity.

---

## 3. Cross-cutting invariants (assert in every scenario where applicable)

- **Owner reference is authoritative for ownership**, never labels alone (see §2.6).
- **No VM outside the owned set is ever mutated or deleted** by a `VirtualMachineReplicaSet` reconcile, full stop — this is the single highest-value invariant to fuzz/property-test given how much of this feature is about counting and culling VMs.
- **Idempotent / restart-safe**: every scenario should be repeatable by forcing a full re-reconcile (not just relying on incremental watch events) and observing no drift.
- **`status.replicas` is a *measurement*, not a mirror of `spec.replicas`**: during any transition, they may legitimately differ; only at steady state must they converge.

## 4. Suggested test-suite shape

Per `.sdd/memory/testing-standards.md`, implement these as Ginkgo specs in `controllers/virtualmachinereplicaset/virtualmachinereplicaset_controller_test.go`, split by label. Note the three test tiers in this repo have distinct backing infrastructure and are not interchangeable:

- **Unit** (`testlabels.Controller`) — fake client, no external infra: scenarios 1–16, 22–34 — fast, exhaustive edge cases on selector/template/status logic.
- **Integration** (`testlabels.Controller`, `testlabels.EnvTest`) — real API server via `envtest`, fake `VMProvider`: scenarios 17–21 (ownership/GC needs real garbage collection semantics from envtest), 24 (readiness — driven via `providerfake.SetCreateOrUpdateFunction` setting the owned VM's `Ready` condition, not a real vSphere backend), 32 (finalizer draining). `testlabels.VCSim` (real vcsim-backed vSphere behavior via `test/builder`'s `TestContextForVCSim`) is **not** needed here: the `VirtualMachineReplicaSet` controller only creates/updates/deletes `VirtualMachine` CRs and never calls `pkg/providers/vsphere` directly — any vSphere-isms of an owned replica (power-on, disks, network) are already covered by the existing VM-level vcsim tests under `pkg/providers/vsphere/` (e.g. `vmprovider_vm_power_test.go`, `session_vm_update_test.go`). A new vcsim-tier test specifically for ReplicaSet would be redundant.
- **E2E** (`test/e2e/vmservice/`) — runs against a **real vCenter/WCP cluster only** (`test/e2e/README.md`; no vcsim option exists in this tier): a thin smoke pass, labeled per the existing `smoke`/`core-functional` conventions, that creates/scales/deletes a `VirtualMachineReplicaSet` end-to-end and confirms VMs actually power on and the scale subresource works, per `.sdd/memory/e2e-sync-with-changes.md`.

Once every scenario above has a passing, independent test (not derived from reading the controller's current code path, but from this contract), that is the bar for confidence that switching this feature on is safe.
