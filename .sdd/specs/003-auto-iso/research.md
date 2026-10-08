# Research: Unattended OS Installation from ISO on vSphere

- **Feature**: `specs/003-auto-iso/`
- **Related**: `spec.md`, `plan.md` in this directory
- **Epic**: vmop-TBD

---

## Problem statement

`spec.md` and `plan.md` establish the mechanism for reaching a guest that has no
network yet: a virtual USB keyboard (`PutUsbScanCodes`) sends keystrokes into
the boot loader, and an ephemeral HTTP server on the Supervisor serves
user-supplied assets (kickstart files, preseed files, autoinstall
configs, Windows answer files, provisioning scripts). This document is the
per-OS-family investigation of **what those keystrokes actually need to say**
for each unattended-install technology VM Operator must support. Every
family reduces to the same three steps named in the spec:

1. **Boot the ISO** far enough to reach an editable boot-command line (or, for
   Windows, the point where a command prompt is reachable).
2. **Pause the boot process** and inject enough configuration that the
   installer's *early* environment — which normally has no network at all —
   can reach the HTTP server.
3. **Point the installer at the external script/assets** so the rest of the
   installation proceeds unattended, driven entirely from the network.

The four technologies differ in *where* the pause happens and *what syntax*
step 2 and step 3 use, but not in the shape of the problem.

---

## Finding 1: the three Linux families share one enabling mechanism — kernel command-line networking

Preseed, Autoinstall, and Kickstart are three different installers
(debian-installer, Subiquity, Anaconda) with three different automation
file formats, but all three boot a Linux kernel via GRUB2 (or isolinux on
legacy BIOS media) and all three accept **extra kernel command-line
parameters** typed in at the boot menu. That command line is the only
"early" configuration surface available before the installer's own
environment starts — which is exactly the surface the virtual USB keyboard
can reach.

### Pausing the boot loader

For all three Linux families, the mechanic is the same:

1. Wait for the GRUB2 (or isolinux) menu to render (`<wait>`/`<waitNNs>`
   tokens calibrated to the ISO's POST + menu-render time — see
   `plan.md` "Possible Issues" item 2).
2. Highlight the default/first entry (already highlighted on most ISOs) and
   press `e` (GRUB2) or `Tab` (isolinux/syslinux) to edit the boot command
   line in place.
3. Navigate to the `linux`/`linuxefi` line and append the extra parameters
   (a trailing space plus the new tokens).
4. Press `Ctrl+X` (GRUB2) or `Enter` (isolinux) to boot with the edited line.

### Early networking on the kernel command line

Debian's own installer (`netcfg`) and the dracut-based initramfs used by
both Ubuntu's casper/live-boot image and RHEL's Anaconda accept network
configuration directly as kernel parameters, so the installer's *first*
network-dependent action (fetching the preseed/autoinstall/kickstart file
over HTTP) already has a working interface. The dracut `ip=` grammar
(cited in `spec.md` References, "Configure Linux networking via kernel
parameters") is:

```text
ip=<client-IP>:<peer>:<gateway-IP>:<netmask>:<hostname>:<iface>:<autoconf>
```

- `<client-IP>` — the VM's intended static IPv4 address.
- `<peer>` — NFS-root peer address; empty for our use case.
- `<gateway-IP>` — the network's IPv4 gateway.
- `<netmask>` — dotted-decimal subnet mask (not CIDR).
- `<hostname>` — the VM's intended hostname.
- `<iface>` — the kernel network interface name (`eth0`/`ens192` for
  legacy naming, or resolved via `ifname=<name>:<mac>` when the guest uses
  predictable interface names and the MAC is known ahead of boot).
- `<autoconf>` — `none`/`off` for static addressing.

This is exactly the shape already sketched in `spec.md`'s example
`commands` list (`ifname=bootnet:{{...MacAddr}};ip={{...}}:...`). Because
Debian's `netcfg` component does **not** consume dracut's `ip=` syntax
(see Finding 2), this single grammar covers Ubuntu and RHEL but not Debian.

---

## Finding 2: Debian / Preseed

- **Installer**: debian-installer (`d-i`), a debconf-driven installer — not
  dracut-based.
- **Upstream reference**: <https://wiki.debian.org/DebianInstaller/Preseed>

### Step 2 — early network configuration

`d-i`'s `netcfg` component owns network configuration and is driven by
debconf keys, not dracut's `ip=` syntax. The keys can be preseeded directly
on the kernel command line as `path=value` pairs, which is what makes
"chicken-and-egg" bootstrapping possible — the values needed to bring up
the network are themselves supplied before the network exists:

```text
netcfg/disable_autoconfig=true
netcfg/choose_interface=<iface>
netcfg/get_ipaddress=<client-IP>
netcfg/get_netmask=<netmask>
netcfg/get_gateway=<gateway-IP>
netcfg/get_nameservers=<dns-IP>
netcfg/confirm_static=true
```

`netcfg/disable_autoconfig=true` is required — otherwise `netcfg` attempts
DHCP first and only falls back to the static values on failure/timeout,
adding non-deterministic delay.

### Step 3 — pointing at the external preseed file and assets

Debian supports fetching the *rest* of the preseed answers from the network
once step 2 has brought up an interface, via the `url=` / `auto=true`
boot parameters:

```text
auto=true priority=critical url=http://<http-ip>:<http-port>/<secret>/<key>
```

- `auto=true` selects the "auto" preseed profile (aggressive defaults,
  minimal prompting for anything not covered by the preseed file).
- `priority=critical` suppresses any question whose priority is below
  critical, which is what makes the install silent even for questions the
  preseed file does not answer.
- `url=` is expanded by `d-i` internally to
  `http://<url>/preseed.cfg` unless the value already ends in a
  filename with an extension; supplying the full path (as VM Operator's
  asset-serving convention `/<secretName>/<key>` does) avoids ambiguity.

The preseed file itself (served from the Secret-backed HTTP endpoint) can
in turn use the `preseed/include` or `d-i preseed/run` directives to chain
to further post-install shell scripts hosted at the same endpoint — this is
the standard Debian pattern for "run an arbitrary provisioning script after
the base install completes."

### Combined command line

```text
auto=true priority=critical netcfg/disable_autoconfig=true \
  netcfg/get_ipaddress=<client-IP> netcfg/get_netmask=<netmask> \
  netcfg/get_gateway=<gateway-IP> netcfg/get_nameservers=<dns-IP> \
  netcfg/confirm_static=true \
  url=http://<http-ip>:<http-port>/<secret>/<key>
```

---

## Finding 3: Ubuntu / Autoinstall

- **Installer**: Subiquity, driven by an `autoinstall` config consumed via
  `cloud-init`'s `NoCloud` datasource.
- **Upstream reference**:
  <https://canonical-subiquity.readthedocs-hosted.com/en/latest/intro-to-autoinstall.html>

### Step 2 — early network configuration

Ubuntu Server's live-installer initramfs (casper) is dracut-compatible for
early networking, so the `ip=` grammar from Finding 1 applies directly.
This is the mechanism the `spec.md` example already models with
`ifname=...;ip=...`.

### Step 3 — pointing at the external autoinstall config and assets

`cloud-init`'s `NoCloud` datasource is selected and pointed at an HTTP
endpoint via the `ds=nocloud-net` kernel parameter:

```text
autoinstall ds=nocloud-net;s=http://<http-ip>:<http-port>/<secret>/
```

- `autoinstall` tells Subiquity to run non-interactively once it locates a
  valid autoinstall config (as opposed to merely finding cloud-init
  user-data, which cloud-init consumes on every boot regardless).
- `ds=nocloud-net;s=<url>` — the trailing slash is significant; cloud-init
  appends `user-data` and `meta-data` to `<url>` and fetches both over
  HTTP. The served `user-data` file's top-level key is `autoinstall:`.
  `meta-data` may be an empty file — cloud-init requires it to exist but
  does not require content for this use case.
- Because VM Operator's asset convention serves individual Secret keys at
  `/<secretName>/<key>` rather than a directory, the HTTP server package
  (`pkg/util/kube/isohttp/`, `plan.md` Phase 4) needs a small
  accommodation for Ubuntu specifically: either (a) serve a synthetic
  directory listing so `.../<secretName>/user-data` and
  `.../<secretName>/meta-data` both resolve, or (b) require the user to
  supply both keys (`user-data`, `meta-data`) under one Secret and have
  the HTTP handler special-case the `nocloud-net` layout. Track as an
  open item feeding into `plan.md` Phase 4.
- The autoinstall config's own `late-commands` (or `user-data`'s
  `runcmd`) can `curl`/`wget` additional provisioning scripts from the
  same HTTP endpoint, mirroring the Debian `preseed/run` chaining pattern.

### Combined command line

```text
autoinstall ds=nocloud-net;s=http://<http-ip>:<http-port>/<secret>/ \
  ifname=bootnet:<mac> \
  ip=<client-IP>::<gateway-IP>:<netmask>:<hostname>:bootnet:none
```

---

## Finding 4: RHEL / Kickstart

- **Installer**: Anaconda, dracut-based initramfs.
- **Upstream reference**:
  <https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/7/html/installation_guide/sect-kickstart-howto>

### Step 2 — early network configuration

Anaconda's initramfs is dracut itself, so the `ip=` grammar from Finding 1
applies unmodified — this is the same reference already cited in
`spec.md` ("RHEL boot commands ... including networking").

### Step 3 — pointing at the external kickstart file and assets

```text
inst.ks=http://<http-ip>:<http-port>/<secret>/<key>
```

- `inst.ks=` (the modern prefix; bare `ks=` remains a supported alias on
  RHEL 7) fetches the kickstart file over HTTP once the interface from
  step 2 is up.
- `inst.ks.device=<iface>` can be set explicitly when Anaconda's automatic
  "which NIC has a route to the kickstart URL" heuristic is ambiguous
  (more than one NIC configured) — not needed for VM Operator's
  single-NIC bootstrap network case, but worth documenting for the
  multi-NIC edge case.
- The kickstart file's own `%post --nochroot` (or plain `%post`) section
  is the standard place to `curl` further provisioning assets from the
  same HTTP endpoint — the same chaining pattern as Debian/Ubuntu.

### Combined command line

```text
inst.ks=http://<http-ip>:<http-port>/<secret>/<key> \
  ip=<client-IP>::<gateway-IP>:<netmask>:<hostname>:<iface>:none
```

### Note on downstream RHEL derivatives

Rocky Linux, AlmaLinux, and CentOS Stream all ship Anaconda unmodified from
RHEL, so this section applies to all of them without changes beyond the
`inst.ks=` parameter name (stable across RHEL 8/9). `spec.md` User Story
US4 ("Rocky, RHEL, other Linux distributions") is fully covered by this
finding — no per-distribution branching is required in
`pkg/util/vsphere/keyboard/` beyond the network-parameter templating
already shared with Ubuntu.

---

## Finding 5: Windows / Unattend — a structurally different mechanism

- **Upstream reference**:
  <https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/>

Windows Setup has no GRUB-equivalent boot-command line, and its automation
file (`autounattend.xml`, an "unattend" answer file) is **not** referenced
via a kernel/boot parameter. This means the "pause the boot process, type
a URL" pattern used by the three Linux families does not transfer
directly, and the choice of mechanism materially changes the reliability
story flagged in `plan.md` OI-4.

### Option A — boot-command keystrokes into a WinPE command prompt, fetching the answer file over HTTP (recommended)

This is the direct Windows analogue of the pattern the three Linux families
already use: type a boot-time command that fetches the automation file
from the network, rather than pre-baking it onto attached media. It keeps
Windows on the same architecture as Linux — the Phase 3 keyboard driver is
the only mechanism used across all four OS families, and `Assets` are
always reached over HTTP, never via a synthesized ISO.

1. Wait for the Windows Setup "language and other preferences" screen —
   this wait has no reliable signal to key off of (`plan.md` OI-4);
   it must be a calibrated `<waitNNs>`.
2. Send `<leftShiftOn>f10<leftShiftOff>` (Shift+F10) to open a command
   prompt inside the WinPE environment Setup is running in.
3. Bring up static networking from the command prompt:

   ```text
   wpeutil initializenetwork
   netsh interface ip set address name="Ethernet" static <client-IP> <netmask> <gateway-IP>
   ```

   (`spec.md`'s example already sketches this `netsh` invocation.)
4. Fetch the answer file from the HTTP endpoint and launch Setup with it,
   e.g.:

   ```text
   curl.exe -o X:\autounattend.xml http://<http-ip>:<http-port>/<secret>/<key>
   X:\sources\setup.exe /unattend:X:\autounattend.xml
   ```

   (`X:` is WinPE's conventional RAM-disk drive letter for the running
   Setup environment.) `setup.exe /unattend:<path>` requires a local
   filesystem path, not a URL — the `curl.exe` step is what stands in for
   the URL fetch a Linux installer does natively via `url=`/
   `ds=nocloud-net;seedfrom=`/`inst.ks=`.
5. Further provisioning assets/scripts referenced from the answer file
   (`RunSynchronousCommand` during `windowsPE`, or `FirstLogonCommands` in
   `oobeSystem` for post-first-boot steps) are pulled from the same HTTP
   endpoint the same way — the direct analogue of Debian's `preseed/run`
   and Kickstart's `%post`.

This path inherits the same timing risk `plan.md` OI-4 already calls out
for the boot-command mechanism in general — there is no deterministic
signal for "the setup screen is now showing," so `<waitNNs>` values are
ISO-version- and hardware-speed-dependent and must be recalibrated
whenever the Windows ISO changes. That risk is judged acceptable here
because it is the same risk already accepted for Ubuntu/RHEL/Debian
(`plan.md` "Possible Issues" item 2) — Windows does not get a materially
different reliability story than the rest of the framework, and the whole
feature stays on one mechanism instead of two.

### Option B — attached-media autodetection (not pursued for v1)

Windows Setup automatically scans every attached removable volume (floppy,
USB, or a second CD/DVD drive) for a file literally named
`autounattend.xml` at its root, **before any user interaction**, and
applies it with zero keystrokes if found. This is the mechanism the
existing Packer vSphere-ISO builder relies on for Windows: the Windows
installation ISO is attached as one CD-ROM device, and a small second ISO
(or floppy image) built from the answer file is attached as a second
CD-ROM device.

This sidesteps the boot-command timing problem entirely — there is nothing
to time, because Setup checks for the file on its own before rendering the
first screen — but it requires the ISO-bootstrap path to gain a wholly new
capability (synthesizing a small ISO/floppy image from templated text) that
the HTTP-serving design in `plan.md` Phase 4 does not otherwise need, and it
departs from the "fetch assets over HTTP" shape shared by every other OS
family. Recorded here as a known alternative — e.g., if Option A's boot-
command timing proves unworkable in practice for a given Windows ISO — but
not the direction this plan takes for v1.

### Recommendation

Prefer Option A: it keeps Windows on the same boot-command + HTTP-serving
architecture as Debian/Ubuntu/RHEL, using the Phase 3 keyboard driver and
Phase 4 HTTP server exactly as built for Linux, with no new "synthesize an
ISO" capability. The cost is that Windows inherits the same boot-command
timing risk (OI-4) the other three families already accept.

---

## Comparison summary

| | Debian (Preseed) | Ubuntu (Autoinstall) | RHEL (Kickstart) | Windows (Unattend) |
|---|---|---|---|---|
| Installer | debian-installer | Subiquity + cloud-init | Anaconda | Windows Setup |
| Boot loader | GRUB2 / isolinux | GRUB2 | GRUB2 / isolinux | Windows Boot Manager |
| Pause mechanic | Edit kernel cmdline (`e`/`Tab`) | Edit kernel cmdline (`e`) | Edit kernel cmdline (`e`/`Tab`) | Shift+F10 |
| Early network syntax | `netcfg/get_*=` debconf keys | dracut `ip=` | dracut `ip=` | `netsh` from a WinPE command prompt |
| Automation-file pointer | `auto=true priority=critical url=` | `autoinstall ds=nocloud-net;s=` | `inst.ks=` | `curl.exe` fetch + `setup.exe /unattend:<local path>` |
| Post-install script chaining | `preseed/include`, `preseed/run` | `late-commands` / `runcmd` | `%post` | `RunSynchronousCommand` / `FirstLogonCommands` |

---

## Open items feeding back into `plan.md`

1. **Ubuntu `nocloud-net` needs two files at one URL** (`user-data` +
   `meta-data`), which does not map cleanly onto the
   one-Secret-key-per-asset serving convention in `plan.md` Phase 4. Needs
   a decision before Phase 4 is implemented for US2.
2. **Windows uses the same boot-command path as Linux, on purpose.** Finding
   5's Option A keeps Windows on the shared Phase 3 keyboard driver and
   Phase 4 HTTP server rather than introducing a second, ISO-synthesis-based
   mechanism (Option B). This means OI-4's boot-command timing risk applies
   to Windows exactly as it does to Debian/Ubuntu/RHEL — it is accepted, not
   engineered away, in exchange for one shared mechanism across all four OS
   families.
3. **Debian's `netcfg` keys vs. dracut's `ip=`** means
   `pkg/util/vsphere/keyboard`'s command templating cannot share one
   "network parameters" template across all Linux families — it needs at
   least two renderers (netcfg-style vs. dracut-style), selected by the
   image's installer family. `spec.md`/`plan.md` do not yet name this
   distinction explicitly.

---

## References

- Debian Preseed: <https://wiki.debian.org/DebianInstaller/Preseed>
- Ubuntu Autoinstall: <https://canonical-subiquity.readthedocs-hosted.com/en/latest/intro-to-autoinstall.html>
- RHEL Kickstart: <https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/7/html/installation_guide/sect-kickstart-howto>
- Windows unattended setup / answer files: <https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/>
- dracut kernel command line (`ip=` grammar): referenced in `spec.md` as
  "RHEL boot commands" / "Ubuntu boot commands"
- Configure Linux networking via kernel parameters: referenced in
  `spec.md` ("Configure Linux networking via kernel parameters")
- `spec.md` (this feature) — boot-command and asset-serving API shape
- `plan.md` (this feature) — USB keyboard driver (Phase 3), ephemeral HTTP
  server (Phase 4), Windows reliability risk (OI-4)
