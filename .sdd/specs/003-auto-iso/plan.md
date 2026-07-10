# Implementation Plan: Automated Deployment from ISO Image

- **Spec**: `spec.md` in this directory
- **Research**: `research.md` in this directory
- **Created**: 2026-07-10
- **Status**: Draft
- **Related epic**: vmop-TBD

---

## Summary

This plan translates the auto-ISO spec into a constitutionally-compliant technical
approach. The feature adds `spec.bootstrap.iso` to the VirtualMachine API, enabling
automated OS installation from an ISO image via a shared framework — a virtual USB
keyboard that sends scan codes to the VM's boot loader, and an ephemeral HTTP server
Pod on the Supervisor that serves user-supplied bootstrap assets (kickstart files,
autoinstall configs, Windows answer files, provisioning scripts) — plus per-OS-family
wiring documented in `research.md`.

The mechanism is directly analogous to the vSphere Packer ISO builder
(`packer-plugin-vsphere/builder/vsphere/iso`), which solves the same problem. The key
insight from studying that code: the vSphere `PutUsbScanCodes` API
(`govmomi.VirtualMachine.PutUsbScanCodes`) replaces VNC and is all that is needed to
type into a VM console without direct network access to the guest.

This plan is organized in two parts:

1. **The framework** — the OS-agnostic pieces every automation technology depends on
   (API, feature flag, keyboard driver, HTTP server, template variables, bootstrap
   integration, webhook validation, conditions, E2E harness).
2. **End-to-end walkthroughs** for the three P0/P1 user stories named in `spec.md` —
   Ubuntu (Autoinstall), RHEL (Kickstart), and Windows (Unattend) — showing how the
   framework's pieces compose for each, using the syntax researched in `research.md`.

Debian/Preseed is covered in `research.md` (Finding 2) but is not a named user story
in `spec.md`; it is expected to fall out of the same framework as Ubuntu/RHEL once
the netcfg-style command renderer described below exists, and is not walked
end-to-end in this plan.

---

## Technical context

- **Go version**: matches root `go.mod` (see `go.mod` in repository root).
- **API version(s) touched**: `v1alpha6` (new field), `v1alpha5` (stub for
  conversion).
- **Modules touched**: root module only (`github.com/vmware-tanzu/vm-operator`).
- **New dependencies**: none expected; `govmomi`'s `PutUsbScanCodes` and
  `UsbScanCodeSpec` types are already vendored at a sufficient version (see
  "Current State").

---

## Current State

The following is already implemented or available:

- **CD-ROM device support** — `api/v1alpha6/virtualmachine_hardware_types.go` defines
  `VirtualMachineCdromSpec` and `VirtualMachineHardwareSpec.Cdrom`; the reconciler
  lives in `pkg/vmconfig/cdrom/`. Every OS family in this plan attaches exactly one
  CD-ROM entry (the OS installation ISO) — none of the end-to-end flows below need
  `spec.hardware.cdrom`'s existing support for multiple entries.
- **ISO content library item type** — `ContentLibraryItemTypeIso = "ISO"` exists in
  both `external/image-registry-operator/api/v1alpha1/` and `v1alpha2/`.
  `VirtualMachineImageTypeLabel` is defined in `api/v1alpha6/virtualmachineimage_types.go`.
- **`PutUsbScanCodes` API** — `govmomi v0.55.0-alpha` exposes
  `object.VirtualMachine.PutUsbScanCodes(ctx, types.UsbScanCodeSpec)`.
  Types `UsbScanCodeSpec`, `UsbScanCodeSpecKeyEvent`, and `UsbScanCodeSpecModifierType`
  are all available in `vim25/types`.
- **Template rendering infrastructure** — `GetTemplateRenderFunc` in
  `pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go` already provides a
  full `text/template` function map with network helpers (firstIP, subnetMask, etc.)
  for all API versions v1alpha1–v1alpha6.
- **Bootstrap integration point** — `DoBootstrap` in
  `pkg/providers/vsphere/vmlifecycle/bootstrap.go` is the single dispatch point for
  all bootstrap providers; adding an ISO case fits naturally here.
- **Feature flag infrastructure** — `pkg/config/` and `pkg/config/env/` already
  support adding a new FSS (feature state switch) toggle.

---

## Constitution check

| Rule | Status | Notes |
|------|--------|-------|
| Controllers are thin; business logic in `pkg/` | OK | All ISO logic in `pkg/providers/vsphere/vmlifecycle/`, `pkg/util/vsphere/keyboard/`, `pkg/util/kube/isohttp/` |
| No direct vSphere API calls from controllers | OK | `PutUsbScanCodes` called from the provider layer only |
| `status.observedGeneration` and `Ready` condition | OK | New `VirtualMachineBootstrapISOSynced` condition added |
| `+optional` / `+required` markers on all fields | Must verify when typing new API structs |
| Finalizer pattern for cleanup | HTTP server Pod/Service owned by VM via owner reference — GC handles cleanup |
| `make generate-go` run after API changes | Required; blocks code phase |
| E2E coverage ships with behavior | OK — see "Test strategy" |
| Markdown: no hard line wrapping | OK — this document uses soft wrap |

---

## The framework

These phases are OS-agnostic. Every end-to-end walkthrough below is built entirely
out of these pieces — no per-OS controller or webhook code is introduced.

### Phase 1 — API (US1, P0)

**Files touched:**
- `api/v1alpha6/virtualmachine_bootstrap_types.go`
- `api/v1alpha6/virtualmachine_types.go` (new condition constants)
- `api/v1alpha6/zz_generated.deepcopy.go` (regenerated)
- `api/v1alpha5/virtualmachine_bootstrap_types.go` (stub for conversion)
- Corresponding `zz_generated.conversion.go` files

**New types** in `api/v1alpha6/virtualmachine_bootstrap_types.go`:

```go
// VirtualMachineBootstrapISOSpec describes the ISO-based automated
// installation configuration used to bootstrap the VM.
type VirtualMachineBootstrapISOSpec struct {
    // +optional
    // +listType=atomic

    // Commands is an ordered list of boot commands sent to the VM via the
    // virtual USB keyboard immediately after power-on.
    //
    // Each element may contain:
    //   - Literal printable ASCII characters (typed one at a time).
    //   - Special key tokens: <esc>, <enter>, <tab>, <bs>, <del>,
    //     <spacebar>, <f1>-<f12>, <up>, <down>, <left>, <right>.
    //   - Wait tokens: <wait> (1s), <wait5> (5s), <wait10> (10s), or an
    //     arbitrary Go duration, e.g. <wait3m30s>.
    //   - Modifier hold/release: <leftShiftOn>x<leftShiftOff>.
    //   - Go text/template expressions evaluated against the VM's intended
    //     network config -- see the template variable reference in spec.md.
    //
    // Used by every guest OS family this bootstrap provider supports,
    // including Windows -- see plan.md's per-OS end-to-end sections for
    // worked examples of the token sequence each one needs.
    Commands []string `json:"commands,omitempty"`

    // +optional
    // +listType=map
    // +listMapKey=name

    // Assets is a list of references to keys in Secret resources in the same
    // namespace. Each referenced key's value is served as a file by an
    // ephemeral HTTP server that VM Operator creates and tears down for this
    // VM, reachable from the VM during installation.
    //
    // The served URL path is /<secretName>/<key>. The address of the HTTP
    // server is available in Commands via the {{V1Alpha6_BootstrapService}}
    // template function.
    Assets []VirtualMachineBootstrapISOAsset `json:"assets,omitempty"`
}

// VirtualMachineBootstrapISOAsset references a single key in a Secret
// resource that is served by the ephemeral bootstrap HTTP server.
type VirtualMachineBootstrapISOAsset struct {
    // +required
    // +kubebuilder:validation:MinLength=1

    // Name is the name of the Secret resource in the same namespace as the VM.
    Name string `json:"name"`

    // +required
    // +kubebuilder:validation:MinLength=1

    // Key is the key within the Secret whose value is the file content to serve.
    Key string `json:"key"`
}
```

**Add to `VirtualMachineBootstrapSpec`:**

```go
// +optional

// ISO may be used to automate the installation of a VM from an ISO image.
//
// Boot commands are sent via the virtual USB keyboard to the VM console
// immediately after power-on. User-supplied assets (kickstart files,
// autoinstall configs, Windows answer files, etc.) are served from an
// ephemeral HTTP server Pod on the Supervisor, reachable from the VM's
// workload network.
//
// Please note this bootstrap provider may not be used in conjunction with
// CloudInit, LinuxPrep, Sysprep, or VAppConfig.
ISO *VirtualMachineBootstrapISOSpec `json:"iso,omitempty"`
```

**New condition constants** in `api/v1alpha6/virtualmachine_types.go`:

```go
// VirtualMachineBootstrapISOSynced indicates that the ISO bootstrap
// process (boot commands and/or ephemeral HTTP server) has been applied.
VirtualMachineBootstrapISOSynced = "VirtualMachineBootstrapISOSynced"
```

**After types are written:** run `make generate-go` to regenerate deepcopy and
conversion.

---

### Phase 2 — Feature Flag

**Files touched:**
- `pkg/config/config.go` (add `AutoISO bool` to `FeatureStates`)
- `pkg/config/env/env.go` (add `FSSAutoISO`)
- `pkg/config/env.go` (wire env var -> feature flag)

The feature flag is `WCP_Auto_ISO`. All controller and bootstrap logic that touches
the ISO path must gate on `pkgcfg.FromContext(ctx).Features.AutoISO`.

---

### Phase 3 — USB Keyboard Driver

**New package:** `pkg/util/vsphere/keyboard/`

This is a direct port and adaptation of Packer's
`builder/vsphere/common/bootcommand/usb_driver.go` and
`builder/vsphere/driver/vm_keyboard.go`. The abstraction boundary:

- `pkg/util/vsphere/keyboard/hid.go` — USB HID scan code table mapping printable
  ASCII characters, special keys (esc, enter, tab, backspace, delete, function
  keys, arrow keys), and modifier keys to `int32` USB HID codes. Encoding:
  `hidCode << 16 | 7` (same as Packer).

- `pkg/util/vsphere/keyboard/command.go` — boot command string parser:

  ```go
  // Token types: Literal, Wait, SpecialKey, ModifierOn, ModifierOff
  type Token struct { ... }

  // ParseCommands parses a slice of boot command strings into an ordered
  // slice of Tokens, resolving template expressions first.
  func ParseCommands(commands []string, render func(string) string) ([]Token, error)
  ```

- `pkg/util/vsphere/keyboard/send.go` — sends tokens to a govmomi VM:

  ```go
  // SendCommands sends the parsed boot command tokens to the VM via the
  // virtual USB keyboard. Waits are honored via ctx (cancellable).
  // Returns an error if any PutUsbScanCodes call fails or ctx is cancelled.
  func SendCommands(ctx context.Context, vm *object.VirtualMachine, tokens []Token) error
  ```

  The function batches consecutive non-wait keystrokes into a single
  `PutUsbScanCodes` call (Packer sends one call per key, which is fine, but
  batching reduces API call count). Wait tokens use `time.Sleep` gated by
  `ctx.Done()`.

**Key implementation details from Packer analysis:**
- HID code encoding: `scancode << 16 | 7` (the `7` is the USB usage page for
  keyboard).
- Modifiers are set per-keystroke via `UsbScanCodeSpecModifierType`.
- `<leftShiftOn>/<leftShiftOff>` hold the shift modifier across subsequent
  keystrokes; needed for symbols like `!`, `@`, `#` etc. on US keyboards.
- The default inter-key delay (Packer: 100ms via `PACKER_KEY_INTERVAL`) is not
  needed if we batch; the vSphere API processes all events in the spec
  atomically.

**Network-parameter renderer split (from `research.md` Finding 1/2/3/4):** the
template helpers that render network parameters into `Commands` must support two
distinct grammars, not one:

- **dracut-style** (`ip=...`, `ifname=...`) — used by Ubuntu (Finding 3) and RHEL
  (Finding 4).
- **netcfg-style** (`netcfg/get_ipaddress=...`, ...) — used by Debian (Finding 2),
  not walked end-to-end in this plan but kept in mind so the renderer interface
  does not silently assume dracut syntax is universal.

Windows does not use this renderer at all — see the Windows end-to-end section.

---

### Phase 4 — Ephemeral HTTP Server

**New package:** `pkg/util/kube/isohttp/`

The ephemeral HTTP server serves bootstrap assets to the VM during OS installation.
VM Operator owns the entire lifecycle of the Pod and the Service that fronts it —
there is no user-facing field naming or otherwise configuring the Service; it is an
implementation detail of how `Assets` get to the VM, not part of the API contract.

#### 4a. Pod / Service lifecycle

The package manages:

1. **Secret validation** — verify that every `spec.bootstrap.iso.assets[*].{name,key}`
   exists and is readable.
2. **Pod creation** — creates a Pod in the same namespace as the VM:
   - Mounts each referenced Secret as a volume.
   - Runs a minimal HTTP server container (see §4b).
   - Name derived deterministically from the VM (e.g. `<vm-name>-iso-bootstrap`),
     so `EnsureReady` can `CreateOrPatch` idempotently without persisting a
     separate generated name anywhere.
   - Labels: `vmoperator.vmware.com/iso-bootstrap-vm: <vm-name>`.
   - Owner reference: the VirtualMachine resource (so GC fires on VM deletion).
3. **Service creation** — creates a Service under the same deterministic name
   (`<vm-name>-iso-bootstrap`), of type `LoadBalancer` (preferred for NSX-T
   Supervisor) or `NodePort` (VDS fallback), targeting the Pod:
   - Annotation `vmoperator.vmware.com/iso-bootstrap-vm: <vm-name>`.
   - Owner reference: the VirtualMachine resource.
   - `EnsureReady` is a `CreateOrPatch` keyed on the deterministic name — safe
     to call every reconcile without an annotation to remember what was
     created (the boot-command idempotency annotation described in Phase 6 is
     still required, since sending commands twice is not safe — that guard is
     about `Commands`, not about the Service).
4. **Address discovery** — wait for `service.status.loadBalancer.ingress[0].ip`
   (LoadBalancer) or use `nodeIP:nodePort` (NodePort) to obtain the address/port
   substituted into `{{V1Alpha6_BootstrapService}}` (see Phase 5).
5. **Teardown** — delete the Pod and Service VM Operator created for this VM
   when the VM condition `VirtualMachineBootstrapISOSynced` is set to True or
   after a configurable timeout (default: 60 minutes).

URL path convention: `http://<service-address>:<service-port>/<secretName>/<key>`.

```go
// Manager creates and tears down the ephemeral HTTP server for ISO bootstrap.
// The Pod and Service names are derived from the VM's name; callers never
// supply or see the name -- it is purely an implementation detail.
type Manager struct { ... }

func NewManager(k8sClient ctrlclient.Client, vm *vmopv1.VirtualMachine) *Manager
func (m *Manager) EnsureReady(ctx context.Context) (ip string, port int, err error)
func (m *Manager) Teardown(ctx context.Context) error
```

#### 4b. HTTP server container image

The Pod runs a dedicated minimal image (e.g., `vmop-iso-httpserver:VERSION`) that:
- Accepts a root directory as an argument.
- Serves files with content-type `application/octet-stream`.
- Logs requests.
- Is under 10 MB.

The image name is configurable via an operator config environment variable
(`VSPHERE_ISO_HTTP_SERVER_IMAGE`), defaulting to the image shipped with the
operator release.

The HTTP server source lives at `cmd/iso-httpserver/` in this repo, similar to
other vmop cmd binaries.

#### 4c. Networking constraint

The VM must be able to reach the Pod's Service IP. In the Supervisor:

- **NSX-T deployments**: Use `type: LoadBalancer`. NSX-T allocates an IP on the
  workload segment. This is the preferred path.
- **VDS deployments**: Use `type: NodePort`. The NodePort is reachable via the ESXi
  host's management VMkernel NIC, which is usually on the same routable network as
  the VM gateway. The VM's boot command would use the node IP.

The `isohttp.Manager.EnsureReady` selects the service type based on the cluster
capability (detected via the existing Supervisor capabilities API at
`external/capabilities/`).

#### 4d. Per-technology serving quirks

`research.md` calls out that not every automation technology wants a single
file at a single URL:

- **Ubuntu Autoinstall** (`ds=nocloud-net;s=<url>/`) needs `user-data` **and**
  `meta-data` to resolve under the same URL prefix. `Manager` must special-case
  this: when the VM's asset list contains both a `user-data` and a `meta-data`
  key under one Secret, serve them at `/<secretName>/user-data` and
  `/<secretName>/meta-data` (a real subpath, not the flat `/<secretName>/<key>`
  layout) so the trailing-slash `ds=nocloud-net;s=` convention resolves both
  files without extra installer-side configuration.
- **Kickstart** and **Windows** (the answer file, fetched via `curl.exe` in the
  boot-command sequence) both use the flat `/<secretName>/<key>` convention
  as-is; no special-casing required.

---

### Phase 5 — Template Variables for ISO Boot Commands

**File:** `pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go`

`Commands` (and the Windows answer-file content, per its end-to-end section) are
rendered through the exact same `GetTemplateRenderFunc`/`renderTemplate` machinery
already used to template vAppConfig properties today — there is no separate,
ISO-specific function map. Network/hostname values come entirely from the
existing `v1a6TemplateFunctions` function map and the `templateData.V1alpha6`
struct field (`VM`/`Net`) that `GetTemplateRenderFunc` already builds; ISO
bootstrap adds exactly **one** new function, `V1Alpha6_BootstrapService`.

Functions already available today (`pkg/providers/vsphere/constants/constants.go`,
`v1a6TemplateFunctions`) and reused as-is by ISO `Commands`:

| Template call | Returns |
|---|---|
| `{{V1alpha6_FirstIP}}` | First IP address (CIDR notation) of the first NIC |
| `{{V1alpha6_FirstNicMacAddr}}` | MAC address of the first NIC |
| `{{V1alpha6_FirstIPFromNIC <index>}}` | First IP address (CIDR notation) of the `<index>`th NIC |
| `{{V1alpha6_IPsFromNIC <index>}}` | All IP addresses of the `<index>`th NIC |
| `{{V1alpha6_FormatIP <ip> <mask>}}` | Reformats `<ip>` with `<mask>` (a dotted mask or `/N` prefix); `<mask>=""` strips to a bare address |
| `{{V1alpha6_IP <ip>}}` | Formats a bare `<ip>` with its address family's default netmask CIDR |
| `{{V1alpha6_SubnetMask <cidr>}}` | Dotted-decimal subnet mask derived from a CIDR string |
| `{{V1alpha6_FormatNameservers <count> <delimiter>}}` | Joins up to `<count>` nameservers with `<delimiter>` |

Hostname and gateway are not functions at all — they are plain dot-notation field
access against the down-converted VM object already exposed as `.V1alpha6.VM`,
exactly as `spec.md`'s own worked example uses them:

| Template expression | Returns |
|---|---|
| `{{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}` | IPv4 gateway of the first NIC |
| `{{.V1alpha6.VM.Status.Network.Config.DNS.HostName}}` | VM hostname |

**New for this feature:**

| Template call | Returns |
|---|---|
| `{{V1Alpha6_BootstrapService}}` | `<address>:<port>` of the ephemeral bootstrap HTTP Service VM Operator created for this VM (Phase 4) |

`V1Alpha6_BootstrapService` follows the same per-API-version naming convention as
the existing `V1alpha6_*` functions, but unlike them it cannot be computed purely
from `BootstrapArgs`/network status at `GetTemplateRenderFunc` construction time —
its value only exists once `isohttp.Manager.EnsureReady` (Phase 4) has created the
Pod/Service and resolved an address. `BootstrapISO` (Phase 6) therefore calls
`EnsureReady` *before* building the render func, and passes the resolved
address/port into `v1a6TemplateFunctions` (or a small ISO-only wrapper around it)
as an additional closure parameter, the same way `networkStatusV1A6`/
`networkDevicesStatusV1A6` are threaded in today. The function's name constant
(`constants.V1alpha6BootstrapService = "V1Alpha6_BootstrapService"`) is added
alongside the existing `V1alpha6First*`/`V1alpha6Format*` constants in
`pkg/providers/vsphere/constants/constants.go`.

There is no separate `http_ip`/`http_port` pair — `spec.md`'s Ubuntu example uses
`{{V1Alpha6_BootstrapService}}` as a single combined value everywhere, including
inside the Windows answer-file XML (Phase 4's HTTP server exposes one address and
one port; splitting them into two template calls would be a distinction without a
consumer).

---

### Phase 6 — Bootstrap Integration

**File:** `pkg/providers/vsphere/vmlifecycle/bootstrap_iso.go` (new)

```go
// BootstrapISO performs the ISO automated installation bootstrap for a VM.
// It creates the ephemeral HTTP server, renders boot commands with network
// template variables, and sends the resulting keystrokes to the VM via the
// virtual USB keyboard.
func BootstrapISO(
    vmCtx pkgctx.VirtualMachineContext,
    vcVM *object.VirtualMachine,
    k8sClient ctrlclient.Client,
    bootstrapArgs BootstrapArgs,
) error
```

Integration into `DoBootstrap` in `bootstrap.go`:

```go
case bootstrap.ISO != nil:
    return BootstrapISO(vmCtx, vcVM, k8sClient, bootstrapArgs)
```

`BootstrapISO` is shared by every OS family, Windows included; it does not branch
on guest OS at all. Every family renders and sends `spec.bootstrap.iso.commands`
through the identical Phase 3 keyboard driver — only the *content* of `Commands`
differs per OS (dracut/netcfg parameters and an installer-file pointer for
Debian/Ubuntu/RHEL; `Shift+F10`/`netsh`/`curl.exe`/`setup.exe` for Windows). See
each OS's end-to-end section for the concrete token sequences.

**State tracking** — to ensure boot commands are sent exactly once (level-triggered
reconciliation requires idempotency guards):

- Hash of `spec.bootstrap.iso` stored in `vmoperator.vmware.com/bootstrap-iso-hash`
  annotation (same pattern as `BootstrapHashConfigSpecAnnotationKey` used by other
  bootstrap providers).
- No separate annotation is needed to track the Pod/Service created for this VM —
  their names are derived deterministically from the VM's own name (Phase 4a), so
  `EnsureReady` can always be called without first looking up what was created.
- If annotation hash matches current spec hash, skip sending boot commands again
  (idempotent). `EnsureReady`/`Teardown` for the Pod/Service are themselves
  idempotent via `CreateOrPatch` and do not need the hash guard.

**Return value:** `ErrBootstrapISO = pkgerr.NoRequeueNoErr("bootstrapped vm from iso")`
after successfully sending commands (same pattern as `ErrBootstrapReconfigure`).

---

### Phase 7 — Webhook Validation

**File:** `webhooks/virtualmachine/v1alpha6/` (validation webhook)

Rules to enforce:

1. `spec.bootstrap.iso` is mutually exclusive with CloudInit, LinuxPrep, Sysprep,
   and VAppConfig.
2. When `spec.bootstrap.iso` is non-nil, at least one entry in
   `spec.hardware.cdrom` must reference an ISO-type image.
3. `spec.bootstrap.iso.assets[*].name` must be a valid DNS subdomain name
   (standard Kubernetes Secret name validation).
4. `spec.bootstrap.iso.commands` may not be changed while the VM is powered on
   and the bootstrap annotation is set (immutable after first boot).

Because the Pod and Service that host `Assets` are created and named entirely
by VM Operator (Phase 4), there is no user-facing field to validate for them —
no name collision is possible across VMs, since each VM's Service name is
derived from that VM's own name.

---

### Phase 8 — Conditions & Status

When ISO bootstrap is in progress or complete, reflect status via conditions:

| Condition | True | False/Reason |
|---|---|---|
| `VirtualMachineBootstrapISOSynced` | Commands sent, HTTP server created | `HTTPServerNotReady`, `KeyboardSendFailed`, `AssetNotFound` |

The HTTP server Pod readiness is surfaced via the `VirtualMachineBootstrapReady`
condition (already exists) with a reason like `ISOHTTPServerPending`.

---

### Phase 9 — E2E Tests

Per `e2e-sync-with-changes.md`, E2E coverage ships with the behavior. Target tests in
`test/e2e/vmservice/virtualmachine/`:

- `iso_bootstrap_test.go` — deploy a VM from a known ISO (e.g., a minimal custom
  ISO that immediately writes a file and powers off), verify the VM completes
  installation.

E2E tests require a pre-uploaded ISO image in the Content Library, which must be
documented in `test/e2e/README.md`.

---

## End-to-end: Ubuntu Server (Autoinstall) — US2, P0

Walks the framework phases above end to end for the Ubuntu case, matching
`research.md` Finding 3.

1. **User applies the VM manifest.** `spec.bootstrap.iso.assets` references a
   Secret with two keys, `user-data` (the `autoinstall:` document) and
   `meta-data` (may be empty). `spec.bootstrap.iso.commands` contains the
   boot-command sequence below. `spec.hardware.cdrom` references the Ubuntu
   Server ISO's `VirtualMachineImage`. This is the exact shape of `spec.md`'s
   worked example — there is no Service to name; VM Operator creates and
   tears down its own.
2. **Pre-power-on** (Phase 6/7): the CD-ROM reconciler (`pkg/vmconfig/cdrom/`)
   attaches and connects the ISO. The webhook (Phase 7) confirms the CD-ROM is
   ISO-typed and no conflicting bootstrap provider is set.
3. **Pre-power-on** (Phase 4): `isohttp.Manager.EnsureReady` creates the Pod +
   Service (named deterministically from the VM), mounts the Secret, and —
   per Phase 4d — serves `user-data` and `meta-data` under one URL prefix
   `/<secretName>/`.
4. **Power on.** The VM boots the Ubuntu Server ISO's GRUB2 menu.
5. **`BootstrapISO` (Phase 6) renders and sends `Commands`**, matching
   `spec.md`'s worked example (enter GRUB's command-line mode with `c`, then
   type the `linux`/`initrd`/`boot` sequence directly, rather than editing the
   default menu entry in place):

   ```text
   <esc><wait>
   ifname=bootnet:{{V1alpha6_FirstNicMacAddr}};ip={{(V1alpha6_FormatIP V1alpha6_FirstIP "")}}:{{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}:{{(V1alpha6_SubnetMask V1alpha6_FirstIP)}}:{{.V1alpha6.VM.Status.Network.Config.DNS.HostName}}:bootnet
   <wait3s>c<wait3s>
   linux /casper/vmlinuz --- autoinstall ds="nocloud-net;seedfrom=http://{{V1Alpha6_BootstrapService}}/"
   <enter><wait>
   initrd /casper/initrd
   <enter><wait>
   boot
   <enter>
   ```

   - The `ifname=`/`ip=` tokens (dracut-style, Phase 3) bring up static
     networking on the bootloader's own kernel before `c` is even pressed —
     GRUB2 itself runs with a Linux kernel already loaded into memory at this
     point on Ubuntu's ISO, so the networking parameters are typed as part of
     the *default* menu entry's implicit boot line before dropping to the
     GRUB command line to override `linux`/`initrd`/`boot` explicitly.
   - `<wait3s>c<wait3s>` drops into GRUB's command-line mode.
   - `{{V1Alpha6_BootstrapService}}` resolves to the address/port of the
     ephemeral Service Phase 4 created and resolved in step 3 above.
6. **Subiquity fetches `user-data`/`meta-data`, installs unattended.** The
   autoinstall document's `late-commands` may `curl` further provisioning
   scripts from the same HTTP endpoint, addressed the same way via
   `{{V1Alpha6_BootstrapService}}` if the script itself is templated before
   being placed in the Secret (Finding 3's chaining pattern).
7. **Post-install** (Phase 8): once `status.network.primaryIP` is populated,
   `VirtualMachineBootstrapISOSynced` flips True and `isohttp.Manager.Teardown`
   removes the Pod/Service (subject to Open Issue OI-5 below).
8. **E2E** (Phase 9): `iso_bootstrap_test.go` gains an Ubuntu-specific scenario
   asserting the VM reaches `Ready` and its guest hostname matches
   `.V1alpha6.VM.Status.Network.Config.DNS.HostName`.

---

## End-to-end: RHEL / Rocky / AlmaLinux (Kickstart) — US4, P1

Walks the framework phases above end to end for the RHEL-family case, matching
`research.md` Finding 4. Steps 1-4 and 7-8 are structurally identical to the
Ubuntu flow above; only step 5's command syntax and step 3's serving convention
differ.

1. **User applies the VM manifest.** `spec.bootstrap.iso.assets` references a
   Secret with a single key holding the kickstart file. `spec.hardware.cdrom`
   references the RHEL/Rocky/AlmaLinux ISO's `VirtualMachineImage`.
2. **Pre-power-on**: identical to Ubuntu step 2.
3. **Pre-power-on** (Phase 4): `isohttp.Manager.EnsureReady` serves the
   kickstart file at the flat `/<secretName>/<key>` path — no special-casing
   needed (Phase 4d).
4. **Power on.** The VM boots the RHEL-family ISO's GRUB2 menu (same boot
   loader as Ubuntu — Anaconda ships on the same GRUB2 base).
5. **`BootstrapISO` renders and sends `Commands`** via the same dracut-style
   renderer used for Ubuntu (Phase 3), since Anaconda's initramfs is dracut
   itself:

   ```text
   <wait10>e
   <end> ifname=bootnet:{{V1alpha6_FirstNicMacAddr}} ip={{(V1alpha6_FormatIP V1alpha6_FirstIP "")}}::{{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}:{{(V1alpha6_SubnetMask V1alpha6_FirstIP)}}:{{.V1alpha6.VM.Status.Network.Config.DNS.HostName}}:bootnet:none inst.ks=http://{{V1Alpha6_BootstrapService}}/<secretName>/<key>
   <f10>
   ```

   - `inst.ks=` replaces Ubuntu's `autoinstall ds=nocloud-net;seedfrom=` —
     Anaconda fetches the kickstart file directly (no separate
     `user-data`/`meta-data` split), and the flat asset-serving convention
     already works unmodified.
   - `{{V1Alpha6_BootstrapService}}` resolves the same way it does for
     Ubuntu — the address/port of the ephemeral Service VM Operator created
     for this VM.
6. **Anaconda fetches the kickstart file, installs unattended.** The kickstart
   file's `%post` section may `curl` further provisioning scripts from the
   same HTTP endpoint (Finding 4's chaining pattern).
7. **Post-install**: identical to Ubuntu step 7.
8. **E2E**: `iso_bootstrap_test.go` gains a RHEL-family scenario. Because
   Rocky/Alma/RHEL/CentOS Stream all share Anaconda unmodified, one test
   parametrized over the ISO image is sufficient rather than one test per
   distribution (`research.md` Finding 4, "Note on downstream RHEL
   derivatives").

---

## End-to-end: Windows Server (Unattend) — US3, P0

Windows does not have a GRUB-equivalent boot-command line, so it cannot use the
exact `ip=`/`inst.ks=`/`ds=nocloud-net;seedfrom=` idiom the three Linux flows
share. This plan nonetheless keeps Windows on the **same architecture** as
Linux — the Phase 3 keyboard driver types a boot-time command sequence that
fetches the answer file from the Phase 4 HTTP server over the network — rather
than pre-baking the answer file onto a second, synthesized CD-ROM
(`research.md` Finding 5, Option A vs. Option B). No new "synthesize an ISO
from templated text" capability is introduced anywhere in this plan; Windows
reuses Phase 3 and Phase 4 exactly as built for Debian/Ubuntu/RHEL.

The trade-off, made explicitly: Windows inherits the same boot-command timing
risk (OI-4, "Possible Issues" item 2) that Debian/Ubuntu/RHEL already accept,
in exchange for one shared mechanism across every OS family instead of two.

### Steps

1. **User applies the VM manifest.** `spec.bootstrap.iso.assets` references a
   Secret whose key is named `autounattend.xml` (the Windows answer file), plus
   optionally further keys for `RunSynchronousCommand` payloads (e.g. a
   PowerShell provisioning script). `spec.bootstrap.iso.commands` contains the
   boot-command sequence below. `spec.hardware.cdrom` references the Windows
   Server installation ISO — a single entry, exactly like the Linux flows;
   Windows does not need a second `cdrom` entry.
2. **Pre-power-on** (Phase 4): `isohttp.Manager.EnsureReady` creates the Pod +
   Service and serves `autounattend.xml` (and any further provisioning script)
   at `/<secretName>/<key>`, same as every other OS family.
3. **Power on.** Windows Setup boots to its first screen ("language and other
   preferences").
4. **`BootstrapISO` (Phase 6) renders and sends `Commands`** via a
   Windows-specific token sequence (not the dracut-style renderer — Windows
   has no kernel command line to append to):

   ```text
   <waitNNs>
   <leftShiftOn>f10<leftShiftOff>
   wpeutil initializenetwork
   <enter><wait>
   netsh interface ip set address name="Ethernet" static {{(V1alpha6_FormatIP V1alpha6_FirstIP "")}} {{(V1alpha6_SubnetMask V1alpha6_FirstIP)}} {{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}
   <enter><wait>
   curl.exe -o X:\autounattend.xml http://{{V1Alpha6_BootstrapService}}/<secretName>/autounattend.xml
   <enter><wait>
   X:\sources\setup.exe /unattend:X:\autounattend.xml
   <enter>
   ```

   - `<waitNNs>` is the calibrated, ISO-version-dependent delay for Setup's
     first screen to render — the same class of risk `spec.md`'s own Windows
     sketch and OI-4/"Possible Issues" item 2 already call out for every OS
     family.
   - `<leftShiftOn>f10<leftShiftOff>` (Shift+F10) opens a command prompt
     inside the running WinPE environment.
   - `wpeutil initializenetwork` brings up the NIC driver stack before
     `netsh` can configure an address on it.
   - `netsh interface ip set address ...` brings up static networking —
     the Windows analogue of the Linux `ip=`/`netcfg` kernel parameters,
     using the same `V1alpha6_FormatIP`/`V1alpha6_SubnetMask`/gateway
     template calls as the Linux flows (Phase 5).
   - `curl.exe` is the literal stand-in for the URL-fetch step every Linux
     installer does natively — `setup.exe /unattend:<path>` only accepts a
     local filesystem path, so the answer file must be downloaded to WinPE's
     `X:` RAM disk first. `{{V1Alpha6_BootstrapService}}` resolves the same
     way it does for the Linux flows (Phase 5).
   - `X:\sources\setup.exe /unattend:X:\autounattend.xml` launches Setup
     fully unattended from that point on.
5. **Setup applies networking and runs `RunSynchronousCommand` entries** from
   the answer file itself — the `windowsPE` pass, `Microsoft-Windows-TCPIP`
   component can additionally set a static address for Setup's *own* later
   phases if `netsh` from step 4 is not sufficient, and
   `Microsoft-Windows-Setup` component `RunSynchronousCommand` entries pull
   any further provisioning payload from the same HTTP endpoint (e.g.
   `cmd /c curl.exe -o C:\provision.ps1 http://{{V1Alpha6_BootstrapService}}/<secretName>/provision.ps1`)
   — the direct analogue of Debian's `preseed/run` and Kickstart's `%post`.
   `oobeSystem` pass `FirstLogonCommands` handles anything that must run
   after first boot rather than during Setup.
6. **Post-install** (Phase 8): once `status.network.primaryIP` is populated
   (or, alternatively, once the answer file's last `RunSynchronousCommand`
   signals completion — see OI-5), `VirtualMachineBootstrapISOSynced` flips
   True and `isohttp.Manager.Teardown` removes the Pod/Service.
7. **E2E** (Phase 9): `iso_bootstrap_test.go` gains a Windows scenario. Because
   this path shares the Phase 3 keyboard driver and Phase 4 HTTP server with
   every other OS family, no Windows-specific test infrastructure is needed
   beyond a Windows Server ISO and the boot-command sequence above; flakiness
   from `<waitNNs>` calibration is tracked the same way it already is for
   Ubuntu/RHEL (OI-1, "Possible Issues" item 2), not as a Windows-only risk.

### Alternative (attached-media autodetection, not pursued for v1)

`research.md` Finding 5, Option B describes attaching a second, synthesized
CD-ROM containing `autounattend.xml`, which Windows Setup auto-discovers with
zero keystrokes. It would remove the boot-command timing risk entirely for
Windows, but at the cost of a second bootstrap mechanism (ISO synthesis)
alongside the HTTP-serving one every other OS family uses, and of `spec.hardware.cdrom`
needing a second entry only for this one OS family. Not built in this plan;
revisit only if Option A's boot-command timing proves unworkable in practice.

---

## Sequence Diagram (Linux path — Ubuntu/RHEL)

```
User kubectl apply VM (spec.bootstrap.iso set, Commands present)
  |
VM Controller reconcile
  |
[Pre-power-on] cdrom reconciler ensures CD-ROM attached and connected
  |
[Pre-power-on] isohttp.Manager.EnsureReady
  -> create Pod + Service (named deterministically from the VM) in namespace
  -> wait for Service external/cluster IP
  |
[Power on] VM powered on with CD-ROM as boot device
  |
DoBootstrap -> BootstrapISO:
  1. Build render func: existing V1alpha6_* network functions (vAppConfig's
     v1a6TemplateFunctions) plus the new V1Alpha6_BootstrapService, backed
     by the address/port isohttp.Manager.EnsureReady just resolved
  2. Parse boot command tokens (dracut-style renderer)
  3. Call vcVM.PutUsbScanCodes (govmomi) for each keystroke batch
     -- <waitN> tokens call time.Sleep(N) with ctx cancellation
  4. Store hash annotation
  5. Return ErrBootstrapISO (no-requeue, not an error)
  |
VM installs OS, pulls assets from http://<service-address>:<service-port>/<secret>/<key>
  |
VM reports IP via VMware Tools / guest info
  |
[Post-install] status.network.primaryIP populated
  -> VirtualMachineBootstrapISOSynced condition set to True
  -> isohttp.Manager.Teardown (delete Pod + Service)
  |
CD-ROM disconnected (user sets spec.hardware.cdrom[*].connected = false)
VM reboots from disk
```

## Sequence Diagram (Windows path)

```
User kubectl apply VM (spec.bootstrap.iso set, Commands present,
                       assets include autounattend.xml)
  |
VM Controller reconcile
  |
[Pre-power-on] cdrom reconciler ensures the Windows install ISO is attached
  and connected (a single cdrom entry, same as every other OS family)
  |
[Pre-power-on] isohttp.Manager.EnsureReady
  -> create Pod + Service (named deterministically from the VM) in namespace
  -> wait for Service external/cluster IP
  |
[Power on] VM powered on with CD-ROM as boot device
  |
DoBootstrap -> BootstrapISO (no Windows-specific branch -- same call as Linux):
  1. Build render func (V1alpha6_* network functions + V1Alpha6_BootstrapService)
  2. Parse boot command tokens (Windows-specific token sequence, not the
     dracut-style renderer)
  3. Call vcVM.PutUsbScanCodes (govmomi) for each keystroke batch:
     Shift+F10 -> wpeutil initializenetwork -> netsh (static IP) ->
     curl.exe (fetch autounattend.xml to X:\) -> setup.exe /unattend:X:\...
  4. Store hash annotation
  5. Return ErrBootstrapISO (no-requeue, not an error)
  |
Setup runs windowsPE-pass RunSynchronousCommand entries, pulls further
  provisioning assets from http://<service-address>:<service-port>/<secret>/<key>
  |
VM reports IP via VMware Tools / guest info
  |
[Post-install] status.network.primaryIP populated
  -> VirtualMachineBootstrapISOSynced condition set to True
  -> isohttp.Manager.Teardown (delete Pod + Service)
  |
CD-ROM disconnected (user sets spec.hardware.cdrom[*].connected = false)
VM reboots from disk
```

---

## Open Issues

### OI-1: Boot command execution blocking the reconcile loop

Sending boot commands blocks the reconcile goroutine for the total duration of all
`<waitN>` tokens in the command list. For a typical Ubuntu autoinstall sequence this
is 10-30 seconds. Risks:

- Controller-manager liveness probe timeout if many VMs bootstrap simultaneously.
- Goroutine starvation under load.

This applies to all four OS families, Windows included — every one of them sends
boot commands through the same Phase 3 keyboard driver.

**Proposed mitigation (v1):** Run `BootstrapISO` in a dedicated long-running
goroutine managed by the controller, signalling completion back via a channel
source (using the existing `pkg/util/kube/cource` pattern). Annotate the VM with
`vmoperator.vmware.com/bootstrap-iso-phase: sending-commands` before starting and
`complete` when done.

**Simpler alternative (v1 fallback):** Cap total wait time at 120 seconds. If the
sum of all `<waitN>` tokens exceeds this, return an error. Document the limitation.
This is acceptable for the initial MVP.

### OI-2: HTTP server Pod networking on VDS

On VDS (non-NSX-T) Supervisors, `LoadBalancer` Services are not available. The
`NodePort` fallback requires the VM to route traffic to an ESXi node IP. Depending
on the workload network topology, the ESXi management VMkernel NIC may not be
reachable from the workload network segment.

**Proposed mitigation:** Require NSX-T for the initial implementation. Document that
VDS Supervisor deployments are not supported for automated ISO bootstrap in the
first release. VDS support is a P2 follow-up.

### OI-3: HTTP server container image in air-gapped environments

The `vmop-iso-httpserver` image must be available in the local registry used by the
Supervisor cluster. In air-gapped environments, the image must be pre-seeded.

**Proposed mitigation:** Document the image as a required artifact alongside the
operator image. Use the existing vmop image distribution pipeline to ship it. Make
the image name configurable via operator env var with a sensible default.

### OI-4: Windows automated bootstrap reliability

Automating the Windows setup screen via boot-command keystrokes is not
deterministic without VNC — there is no reliable way to know when the first
screen appears, and `Shift-F10` timing is undocumented. This plan deliberately
accepts that risk for Windows (`research.md` Finding 5, Option A) rather than
engineering around it with a synthesized second CD-ROM (Option B), so that
Windows shares one mechanism with Debian/Ubuntu/RHEL instead of introducing a
second one. Concretely, this is the same class of risk already recorded in
"Possible Issues" item 2 for the other three OS families — Windows does not get
a worse reliability story than they already have, but it does not get a better
one either.

**Status: open, accepted trade-off.** No mitigation beyond calibrated
`<waitNNs>` tokens is planned for v1. If Windows boot-command timing proves
materially less reliable in practice than the Linux flows (e.g. because Setup's
screen-render time varies more across hardware/ISO versions than GRUB2's does),
revisit `research.md` Finding 5 Option B (synthesized second CD-ROM) as a
follow-up rather than a v1 requirement.

### OI-5: Detecting installation completion

The current plan tears down the HTTP server when `status.network.primaryIP` is
populated (VMware Tools reports an IP). This heuristic is imperfect — the OS could
report an IP before the installer has finished, for either the Linux or Windows
path.

**Alternative approaches:**
- Tear down after a configurable timeout (simpler, explicit).
- Expose a user-controlled field `spec.bootstrap.iso.autoCleanup: false` to let the
  user tear down manually.
- Tear down on next reconcile after the VM is powered off and back on (post-install
  reboot cycle).
- Windows-specific: have the last `RunSynchronousCommand`/`FirstLogonCommands`
  entry in the answer file signal completion explicitly (e.g., write a marker file
  the controller can poll for via guest info), since Setup's own lifecycle events
  are more observable than a Linux installer's.

**Recommended for v1:** Configurable timeout (default 60 minutes) with manual
override via annotation.

### OI-6: `V1Alpha6_BootstrapService` is visible outside the ISO path

Phase 5 adds `V1Alpha6_BootstrapService` to the same shared `v1a6TemplateFunctions`
map used to render vAppConfig properties, since that is the existing, correct home
for every other `V1alpha6_*` function. That also means the function is callable
from a vAppConfig template on a VM that is **not** using `spec.bootstrap.iso` at
all — `GetTemplateRenderFunc` does not know ahead of time which bootstrap provider
is active. Calling it outside an ISO-bootstrap reconcile has no address to
resolve, since `isohttp.Manager.EnsureReady` never ran.

**Proposed mitigation:** only register `V1Alpha6_BootstrapService` in the `FuncMap`
when `BootstrapISO` (Phase 6) builds its own render func — i.e., `DoBootstrap`'s
vAppConfig path continues to call `GetTemplateRenderFunc` exactly as it does
today (function absent), while `BootstrapISO` calls a small wrapper that takes
`GetTemplateRenderFunc`'s existing `v1a6TemplateFunctions` output and adds the one
new entry on top, scoped to that single render call. This avoids a version bump
or a parallel function-map builder while keeping the function absent (and
therefore a clear "undefined function" template error, not a confusing empty
string) anywhere it cannot mean anything.

---

## Possible Issues

1. **`PutUsbScanCodes` requires the VM to be powered on and have a USB controller.**
   The VM must have a USB HID keyboard device. By default, vSphere VMs created via
   `CreateVM_Task` may not have a virtual keyboard attached if not explicitly
   specified in the ConfigSpec. The cdrom reconciler or the ISO bootstrap path must
   verify a virtual keyboard exists and add one if needed. Applies to all four OS
   families, Windows included.

2. **Bootloader/setup-screen race condition.** Boot commands are sent after
   `boot_wait` (a configurable initial delay). If the ISO's BIOS/UEFI POST (or,
   for Windows, Setup's own load time) takes longer than expected, the first
   keystrokes may be dropped. The end-to-end examples above use explicit
   `<waitN>` tokens to handle this, but the user must calibrate these for their
   specific ISO. Document that the initial `<wait>` duration may need tuning —
   see OI-4 for why this risk is accepted rather than engineered around for
   Windows specifically.

3. **Non-US keyboard layout.** The HID scan code table is based on a US QWERTY
   layout. Users running VMs with non-US keyboard layouts configured in the VM's
   BIOS may receive wrong characters. Document this limitation. Applies to all
   four OS families, Windows included.

4. **Secret permissions.** The VM controller must have `get` and `watch` RBAC on
   Secrets in the VM's namespace to read bootstrap assets. The existing RBAC markers
   may need updating.

5. **Pod security context.** The ephemeral HTTP server Pod must comply with the
   namespace's Pod Security Admission policy. Use a non-root user, read-only
   filesystem, and drop all capabilities.

6. **Content library ISO VirtualMachineImage creation.** The image-registry-operator
   only surfaces OVF-type content library items as VirtualMachineImages today. This
   must be extended to include ISO-type items. This work is likely in the
   image-registry-operator, not vm-operator. If it is not done, ISO images will not
   appear as VirtualMachineImage resources and `spec.hardware.cdrom[*].image` cannot
   reference them. This is a **hard dependency** that may block the feature.

7. **Content library storage URI for CD-ROM backing.** The vSphere CD-ROM device
   requires a datastore path (`[datastore] path/to/file.iso`) as its backing. The
   existing `cdrom_reconciler.go` must resolve the VirtualMachineImage's content
   library item to a datastore path via the `content/library/item/storage` vSphere
   REST API. Verify this path is already wired up in the CD-ROM reconciler; every
   OS family in this plan, Windows included, attaches only its own installation
   ISO this way, so there is no second CD-ROM-backing case to handle.

---

## Module Impact

Only the root module (`github.com/vmware-tanzu/vm-operator`) is impacted. No
sub-modules require changes. CRD manifests must be regenerated via
`make generate-manifests`.

---

## Test strategy

- **Unit** (`testlabels.Controller`): `pkg/util/vsphere/keyboard/` token parsing and
  HID encoding, including the Windows-specific token sequence (`Shift+F10`,
  literal command strings, `<waitNNs>`) alongside the dracut/netcfg-style
  renderers; `pkg/util/kube/isohttp/` Pod/Service spec construction, including
  that the Pod/Service name is derived deterministically from the VM name and
  `EnsureReady` is idempotent across repeated calls (fake client).
- **Integration** (`testlabels.EnvTest` / `testlabels.VCSim`): `BootstrapISO`
  dispatch from `DoBootstrap` against vcsim, including the annotation-hash
  idempotency guard — one code path exercised identically for every OS family,
  Windows included, since there is no Windows-specific branch to test separately.
- **E2E** (mandatory, `test/e2e/vmservice/virtualmachine/iso_bootstrap_test.go`):
  one scenario per end-to-end walkthrough above — Ubuntu Autoinstall, RHEL-family
  Kickstart (parametrized across Rocky/Alma/RHEL where feasible), and Windows
  Unattend. Per `e2e-sync-with-changes.md`, this ships in the same PR as each user
  story's code.

---

## Rollout / migration

- **Feature flag**: `pkgcfg.Features.AutoISO`, wired to `WCP_Auto_ISO`, default
  `false`.
- **Schema upgrade**: none — `spec.bootstrap.iso` is a net-new, additive field on
  `v1alpha6`; `v1alpha5` carries a stub for round-trip conversion.
- **Partner comms**: Content Library ISO-type-item support (Possible Issue 6) is a
  cross-repo dependency (image-registry-operator) that must ship before or
  alongside this feature; call this out explicitly to partner teams before GA.
- **Sequencing**: ship US1 (API) + framework Phases 1-2 first, then US2 (Ubuntu)
  end to end, then US4 (RHEL family) end to end, then US3 (Windows) last. Windows
  needs no new framework phases — it reuses Phase 3/4 exactly as built for
  Linux — but is sequenced last so its boot-command timing (OI-4) can be
  calibrated against a working reference (the already-shipped Linux flows)
  rather than in isolation.

---

## Complexity tracking

| Violation | Why needed | Simpler alternative rejected because |
|-----------|------------|--------------------------------------|
| Two network-parameter renderers (dracut-style, netcfg-style) instead of one | Debian's `netcfg` component does not accept dracut's `ip=` grammar (`research.md` Finding 1 vs. Finding 2) | A single renderer would either break Debian or require Debian to be excluded from the shared framework entirely |
