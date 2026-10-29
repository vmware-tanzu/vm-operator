# Tasks: CloudInit DHCP Overrides

- **Spec**: [spec.md](spec.md)
- **Plan**: [plan.md](plan.md)
- **Epic**: TBD
- Shipping tasks require real story/sub-task tickets before merge; ticket assignment is pending.

## Phase 1 — Specification

- [x] T001 Record behavior, plan, and upstream constraints (`.sdd/specs/012-dhcp-overrides/`).

## Phase 2 — Implementation

- [x] T002 Derive override intent and render/validate customization (`pkg/providers/vsphere/network/bootstrap.go`, `pkg/providers/vsphere/network/netplan.go`).
- [x] T003 Validate gateway suppression and dual-stack consistency (`webhooks/virtualmachine/validation/virtualmachine_validator.go`).

## Phase 3 — Validation and documentation

- [x] T004 Extend existing CloudInit bootstrap, Netplan and webhook tests in their existing test files.
- [x] T005 Add experimental E2E coverage (`test/e2e/vmservice/vmservice/virtualmachine/vm_vpcnetworking.go`).
- [x] T006 Document configuration and run focused checks (`docs/tutorials/deploy-vm/cloudinit.md`, `api/v1alpha6/virtualmachine_network_types.go`, `config/crd/bases/vmoperator.vmware.com_virtualmachines.yaml`, `config/crd/bases/vmoperator.vmware.com_virtualmachinereplicasets.yaml`).
- [ ] T007 Assign epic/story tickets before merge.
- [ ] T008 Run experimental E2E scenarios against a Supervisor and promote the spec after validation.
- [x] T009 Audit Netplan DHCP override semantics and verify serialized CloudInit metadata (`pkg/providers/vsphere/vmlifecycle/bootstrap_cloudinit_test.go`, `.sdd/specs/012-dhcp-overrides/research.md`).

- [x] T010 Limit DHCP DNS suppression to explicit interface nameservers and leave VM-level DNS policy to PR #2005 (`pkg/providers/vsphere/network/bootstrap.go`, provider tests, API descriptions, generated CRDs/reference, tutorial, and this feature spec).

## Local validation

- Passed after narrowing to CloudInit: affected network suites (`InterfaceBootstrap|Netplan|GOSC|BuildBootstraps`) and Netplan utilities with Go 1.26.9 and normal suite initialization. GOSC runs as regression coverage of unchanged behavior.
- Passed: network webhook unit tests (130 scenarios).
- CloudInit bootstrap checks exclude existing exact-gzip fixture failures. LinuxPrep and Sysprep additions have been removed from this feature.
- Passed: lint on the affected provider, Netplan, and validation packages with Go 1.26.9; manifest generation; E2E package compilation; `git diff --check`.
- Existing limitations: four exact-gzip CloudInit fixtures also fail with the original network production files restored via a temporary Go overlay. The default Go 1.27 toolchain is newer than the pinned linter; use Go 1.26.9. Excluding envtest initialization causes existing tests using `suite.GetLogger()` to panic; the final provider checks retain normal initialization.
- Hardware validation is pending. The new E2E test is labeled `experimental` and verifies the actual CloudInit network metadata delivered through vCenter on DHCP VPC Subnets and SubnetSets, with explicit interface nameservers and VM DHCP fields omitted.
- Passed on 2026-10-18 with Go 1.26.9: focused network tests (`InterfaceBootstrap|Netplan|GOSC|BuildBootstraps`), Netplan serialization tests, network webhook tests, and 11 CloudInit metadata scenarios exercising both GuestInfo and CloudInitPrep. The VM lifecycle package passed lint with the pinned golangci-lint v2.13.2 (zero issues), and `git diff --check` passed. The audit required no production-code changes.

- Passed on 2026-10-29 with Go 1.27.3 after narrowing DNS suppression: focused network tests (`InterfaceBootstrap|Netplan`) and CloudInit DHCP override metadata tests through both transports; manifest generation and `git diff --check`. Regenerated the affected API reference rows while excluding unrelated generated documentation updates. Existing E2E coverage already uses explicit interface nameservers; hardware validation remains pending.
