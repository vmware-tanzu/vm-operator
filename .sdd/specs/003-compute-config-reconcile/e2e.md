# E2E Test Plan: VM Compute Configuration Reconciliation

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Tasks**: [`tasks.md`](./tasks.md) (T016, T019, T021)

This document specifies the end-to-end test suite for this feature: the scenarios the implementation must satisfy against a real Supervisor, organized by the user story in `spec.md` each one validates.

## Suite

- `VMComputeConfigSpec`, in `test/e2e/vmservice/vmservice/virtualmachine/vm_compute_config.go`, registered from `test/e2e/vmservice/vmservice_test.go` under a `Context("VM-COMPUTE-CONFIG", ...)` block.
- Every scenario carries either `"core-functional"` or `"extended-functional"` — the latter for scenarios that need direct `govmomi` access (drift injection, out-of-band power operations, host-property lookups) and skip when that access isn't available. A feature-specific label like `"compute-config"` is unnecessary: `TEST_FOCUS="VM-COMPUTE-CONFIG"` already matches the `Context` name for the same filtering. Every scenario also carries `"experimental"` per `e2e-testing.md`, to be dropped once verified on a real Supervisor.
- The suite requires two new wait-interval keys in `test/e2e/vmservice/config/wcp.yaml`: `default/wait-vm-compute-config-synced` (5m/10s) and `default/wait-vm-compute-config-powerstate` (3m/10s).
- The suite requires a capability constant in `test/e2e/vmservice/consts/consts.go`: `TelcoVMServiceAPICapabilityName = "supports_telco_vm_service_api"`.

## Gating

The suite skips entirely when the `supports_telco_vm_service_api` Supervisor capability is disabled. This is a coarser check than US4's acceptance criteria in `spec.md` (which also expects the webhook to actively reject the fields when disabled) — no scenario in this suite asserts the rejection behavior itself; that belongs at the webhook unit-test layer (`virtualmachine_validator_compute_test.go`). See "Scope boundaries" below.

## Scenarios

### US1 — direct control over VM compute configuration

| Scenario | Verifies |
|----------|----------|
| creates VM with hot-pluggable allocation and verifies condition True after live update | Two-phase: initial `requests`/`limits` apply on create; a hot-pluggable `requests` bump while powered on applies immediately without a power cycle |
| sets PowerOffRequired condition for multiple power-off-required fields while VM is powered on | Patching `cpuAdvanced.hotAddEnabled` + `memoryAdvanced.hotAddEnabled` while powered on yields `PowerOffRequired` naming both fields; a concurrent hot-pluggable `requests.cpu` bump still applies via the watcher-driven reconcile |
| applies power-off-required fields after VM is powered off | `hotAddEnabled` (both) + `coresPerSocket` deferred as `PowerOffRequired`, then applied and verified once the VM is powered off |
| applies iommuEnabled after power-off | `cpuAdvanced.iommuEnabled` power-off-required path |
| clears hot-pluggable allocation immediately and defers hotAddEnabled clear via merge patch | A merge-patch nulling `spec.resources` + `spec.cpuAdvanced` clears the hot-pluggable reservation/limit immediately, but defers the `hotAddEnabled` nil→false transition as `PowerOffRequired` |
| sets PrerequisiteNotMet condition when VM hardware version is below field minimum | `cpuAdvanced.hotAddEnabled` on a VM below vmx-11 sets `PrerequisiteNotMet`, naming the field and `hwVer`; skips if the environment's default image is already at vmx-11 or newer |
| applies spec CPU and memory size overrides while powered off and verifies after power-on | `spec.resources.size.{cpu,memory}` applies while powered off, confirmed via `status.class.cpu.total` / `status.class.memory.total` after power-on |

### US2 — diagnosing sync status from a single condition

Condition precedence and reason/message content are exercised throughout nearly every scenario above and below — each one waits on a specific `VirtualMachineComputeConfigSynced` reason rather than relying on one dedicated scenario. `PrerequisiteNotMet` and `PowerOffRequired` message content is asserted explicitly (see table above); `ComputeConfigMismatch` is exercised implicitly as a transitional state during `Eventually` polling but is not asserted on directly, since it's too transient to reliably catch against a real Supervisor.

### US3 — class-based resize applies the class's compute intent atomically

| Scenario | Verifies |
|----------|----------|
| resizes via VMClass and applies spec compute overlay while powered off | Full resize path: class change (2→4 vCPU) plus a spec-level overlay (`hotAddEnabled`, `limits.cpu`) applies atomically while powered off |
| resizes via VMClass while powered on by cycling power after PowerOffRequired | Same resize, initiated while powered on, completed by a manual power cycle |
| resizes from guaranteed-xsmall to best-effort-xsmall and clears reservation | Class-derived CPU reservation goes from non-zero to `0` on resize to a best-effort class; skips if either platform class isn't accessible in the namespace |
| resizes from best-effort-xsmall to guaranteed-xsmall and restores reservation | Inverse of the above — reservation restores to non-zero |

### Edge cases from `spec.md`

| Scenario | Edge case covered |
|----------|--------------------|
| re-applies all compute config fields after VM is deleted and recreated | Fields survive a full VM delete/recreate cycle |
| resolves PowerOffRequired after out-of-band vSphere power-off while change is pending | A `PowerOffRequired` field applies once vCenter reports the VM powered off, even when the power-off happens outside VM Operator (direct `govmomi`) |
| re-applies compute config fields after vSphere VM is destroyed out-of-band | Destroying the vSphere VM directly (Kubernetes object survives) and toggling `spec.powerState` triggers a full recreate that re-applies every compute field from spec |
| reverts live compute config drift injected directly in vSphere | A `ReconfigVM_Task` injecting CPU reservation/limit + memory reservation drift is detected and reverted to the spec-desired values on the next reconcile |
| reverts power-off-required field drift injected while VM is powered off | Same drift-revert guarantee for a power-off-required field (`nestedHardwareVirtualizationEnabled`) — limited to the one field the E2E service account has `Settings` privilege to drift-inject directly (no `CPUCount`/`Memory` privilege) |
| sets latency sensitivity High with full CPU and memory reservation | `latencySensitivity=High` + full CPU reservation (derived from the live host's `CpuMhz`) + `memoryAdvanced.reservationLockedToMax` together satisfy the full-reservation requirement from `model.md` |
| applies vnumaNodeCount topology via power-off reconfigure on vmx-20+ VM | `topology.{coresPerSocket,vnumaNodeCount}` applies while powered off, confirmed via live `NumaInfo.CoresPerNumaNode` after power-on; skips below vmx-20 |

## Scope boundaries (not E2E-tested by design)

- US4's webhook-rejection behavior when `supports_telco_vm_service_api` is disabled is webhook-unit-tested only (the capability gate rejecting `spec.resources`/`cpuAdvanced`/`memoryAdvanced`). It is a deterministic admission-time check with no reconcile-loop or real-vSphere interaction, so unit coverage is sufficient — no E2E scenario is needed. This suite's whole-suite skip when the capability is disabled (see "Gating" above) is the only E2E-level signal, and that's by design.
- The `requests.{cpu,memory}` negative-value rejection rule is webhook-unit-tested only; it has no cluster-observable side effect beyond the rejection itself, so no E2E scenario is needed.
- The `-1` unlimited-sentinel behavior on `limits.{cpu,memory}` is webhook-unit-tested only, for the same reason.
