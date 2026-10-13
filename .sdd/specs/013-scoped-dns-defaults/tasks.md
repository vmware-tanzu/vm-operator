# Tasks: Scoped Guest DNS Defaults

- **Spec**: [`spec.md`](./spec.md)
- **Plan**: [`plan.md`](./plan.md)
- **Epic**: TBD

> **Ticket tags**: Tasks tagged `[vmop-TBD]` produce shipping code. Each MUST be re-tagged with a real story or sub-task before merge.

## Phase 1 — Setup

- [x] T001 Allocate the spec directory and index entry (`.sdd/specs/013-scoped-dns-defaults/`, `.sdd/INDEX.md`)

## Phase 2 — Foundational

- [x] T002 [vmop-TBD] Add the `ScopedDNSDefaults` feature and the `supports_vm_service_scoped_dns_defaults` capability key (`pkg/config/config.go`, `pkg/config/capabilities/capabilities.go`, `pkg/config/capabilities/capabilities_test.go`)
- [x] T003 [vmop-TBD] Add the `DNSDefaultsAnnotationKey` / `DNSDefaultsLegacy` / `DNSDefaultsScoped` constants (`pkg/constants/constants.go`)
- [x] T004 [P] [vmop-TBD] Allow VM users to add, change, and remove the DNS defaults annotation; verify admission without privileged-account status (`webhooks/virtualmachine/validation/virtualmachine_validator.go`, `..._unit_test.go`)
- [x] T005 [P] [vmop-TBD] Add `Bootstrap.IsStatic`, `Bootstrap.AddressFamilies`, `Bootstrap.GatewayFamilies`, `PrimaryInterface` and `FilterNameserversByFamily`. Stop copying VM-level DNS to interfaces in `InterfaceBootstrap` (`pkg/providers/vsphere/network/bootstrap.go`)

## Phase 3 — Scoped defaults (G1–G13)

- [x] T006 [vmop-TBD] Factor out `getEffectiveBootstrapSpec`. Move the current defaulting, and the VM-level DNS copy from `InterfaceBootstrap`, unchanged into `applyLegacyDNSDefaults`. Add `useScopedDNSDefaults` and `applyScopedDNSDefaults`. Add `TemplateDNSServers` (`pkg/providers/vsphere/vmlifecycle/bootstrap.go`)
- [x] T007 [vmop-TBD] Point templates at `TemplateDNSServers` (`bootstrap_templatedata.go` and its tests)
- [x] T008 [vmop-TBD] Unit tests for `GetBootstrapArgs` (`pkg/providers/vsphere/vmlifecycle/bootstrap_test.go`)
- [x] T009 [vmop-TBD] E2E: scoped Cloud-Init and LinuxPrep, plus non-admin mode changes/removal and automatic reselection (`test/e2e/vmservice/vmservice/virtualmachine/vm_guestcustomization.go`, `test/e2e/vmservice/consts/consts.go`)
- [x] T009a [vmop-TBD] Plumb provider nameservers only: `Bootstrap.ProviderNameservers`, treated like interface-level nameservers in Scoped mode and merged into the global nameserver list for LinuxPrep; providers do not supply search domains (`pkg/providers/vsphere/network/bootstrap.go`, `pkg/providers/vsphere/vmlifecycle/bootstrap.go`, and their tests)
- [ ] T009b [vmop-TBD] Set provider nameservers from `SubnetPort.status.dnsResolverIPs` in `bootstrapFromVPC` once its API is available. Before landing: decide how existing scoped VMs are protected from the resulting bootstrap hash change (re-applied Cloud-Init guestinfo, GOSC re-customization without the latch), for example by shipping it with the capability; update the scoped E2E expectations, which assume only the ConfigMap supplies DNS; reconcile the upstream priority proposal with bootstrap-engine-specific precedence; require automated per-node VKS prerequisite checks for required resolution through the selected VPC resolver, including forwarding to Supervisor workload DNS where needed (WIKI page 2485688854, comment 3126531699); and update the API field docs for provider precedence

## Phase Final — Polish

- [x] T010 Document the behavior (`docs/concepts/services-networking/guest-net-config.md`)
- [x] T010a Document the first-interface rule, the static-only VM-level DNS, and the per-adapter Sysprep VM-level nameservers in the API field docs, and regenerate the CRDs (`api/v1alpha6/virtualmachine_bootstrap_types.go`, `api/v1alpha6/virtualmachine_network_types.go`, `config/crd/bases/`)
- [ ] T011 Resolve the spec's `[NEEDS CLARIFICATION]` items: epic, VM-level DNS on DHCP-only VKS nodes, NoIPAM with addresses
- [x] T012 [vmop-TBD] Extend E2E with scoped DNS cases in `plan.md`: legacy/scoped coexistence with stable reconciles, VM-level and interface-level Cloud-Init DNS, LinuxPrep global DNS, and Sysprep per-adapter DNS run on every network provider using the default network (VPC creates its own Subnets); Cloud-Init VM-level DNS with a DHCP interface and DHCP-first LinuxPrep run on VPC only. The cases assert on Cloud-Init metadata and VM status and need no fixture; retain the non-admin mode-edit/removal test (`test/e2e/vmservice/vmservice/virtualmachine/vm_scopeddns.go`, `vm_vpcnetworking.go`, `vm_guestcustomization.go`)

- [x] T013 [vmop-TBD] Preserve legacy DNS for automatically selected, already-powered-on VMs without existing-guest markers; retain explicit overrides and capability-off behavior (`pkg/providers/vsphere/vmlifecycle/bootstrap.go`, `bootstrap_test.go`)

- [ ] T014 Validate the scoped DNS E2E cases on VPC, VDS, and NSX-T Supervisors and remove their `experimental` label. Local compilation does not establish Supervisor behavior. DHCP on VDS/NSX-T and guest resolver checks have no E2E coverage, and the Windows case does not wait for guest customization (see `plan.md`).
