# Tasks: Scoped Guest DNS Defaults

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: TBD

> **Ticket tags**: Tasks tagged `[vmop-TBD]` produce shipping code. Each MUST be re-tagged with a real story or sub-task before merge.

## Phase 1 — Setup

- [x] T001 Allocate the spec directory and index entry (`.sdd/specs/012-scoped-dns-defaults/`, `.sdd/INDEX.md`)

## Phase 2 — Foundational

- [x] T002 [vmop-TBD] Add the `ScopedDNSDefaults` feature and the `supports_vm_service_scoped_dns_defaults` capability key (`pkg/config/config.go`, `pkg/config/capabilities/capabilities.go`, `pkg/config/capabilities/capabilities_test.go`)
- [x] T003 [vmop-TBD] Add the `DNSDefaultsAnnotationKey` / `DNSDefaultsLegacy` / `DNSDefaultsScoped` constants (`pkg/constants/constants.go`)
- [x] T004 [P] [vmop-TBD] Restrict the annotation to privileged users (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `..._unit_test.go`)
- [x] T005 [P] [vmop-TBD] Add `Bootstrap.IsStatic`, `Bootstrap.AddressFamilies`, `Bootstrap.GatewayFamilies`, `PrimaryInterface` and `FilterNameserversByFamily`. Stop copying VM-level DNS to interfaces in `InterfaceBootstrap` (`pkg/providers/vsphere/network/bootstrap.go`)

## Phase 3 — Scoped defaults (G1–G13)

- [x] T006 [vmop-TBD] Factor out `getEffectiveBootstrapSpec`. Move the current defaulting, and the VM-level DNS copy from `InterfaceBootstrap`, unchanged into `applyLegacyDNSDefaults`. Add `useScopedDNSDefaults` and `applyScopedDNSDefaults`. Add `TemplateDNSServers` (`pkg/providers/vsphere/vmlifecycle/bootstrap.go`)
- [x] T007 [vmop-TBD] Point templates at `TemplateDNSServers` (`bootstrap_templatedata.go` and its tests)
- [x] T008 [vmop-TBD] Unit tests for `GetBootstrapArgs` (`pkg/providers/vsphere/vmlifecycle/bootstrap_test.go`)
- [x] T009 [vmop-TBD] E2E: scoped Cloud-Init and LinuxPrep (`test/e2e/vmservice/vmservice/virtualmachine/vm_guestcustomization.go`, `test/e2e/vmservice/consts/consts.go`)

## Phase Final — Polish

- [x] T010 Document the behavior (`docs/concepts/services-networking/guest-net-config.md`)
- [x] T010a Document the first-interface rule, the static-only VM-level DNS, and the per-adapter Sysprep VM-level nameservers in the API field docs, and regenerate the CRDs (`api/v1alpha6/virtualmachine_bootstrap_types.go`, `api/v1alpha6/virtualmachine_network_types.go`, `config/crd/bases/`)
- [ ] T011 Resolve the spec's `[NEEDS CLARIFICATION]` items: capability key name, epic, VM-level DNS on DHCP-only VKS nodes, NoIPAM with addresses, pinning of powered-on VMs without the G3 annotations
- [ ] T012 [vmop-TBD] E2E: multi-NIC Cloud-Init (DHCP + static) and Sysprep, including that Windows uses the per-adapter VM-level nameservers, once a testbed with a second workload network and Windows images is available
