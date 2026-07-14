# Feature Specification: Automated Deployment from ISO image

- **Feature branch**: [`feature/auto-iso`](https://github.com/akutz/vm-operator/tree/feature/auto-iso/)
  - **Fork**: `akutz/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-06-29
- **Status**: In Progress (framework and US1/US2 (Ubuntu) implemented; US3 (Windows) and US4 (RHEL family) not yet started)
- **Epic**: vmop-TBD
- **Wiki parent**: VM Service: Auto ISO
- **Spike (done)**: vmop-TBD

---

## Summary

A previous effort resulted in the ability to boot and manually configure a VM Service VM from an ISO image. This new effort aims to automate this process. The two, primary impediments to achieve this are:

1. The client must be able to configure the VM to use a temporary network so it can access assets over the network that inform the guest how to perform the unattended installation. Typically a Packer workflow would use VNC to send commands into the guest since it would be running locally. While it *is* possible to run VNC for ESXi-hosted VMs, it is more challenging due to firewalls and being disabled by default.
2. The endpoint hosting the assets mentioned in step one must be accessible by the VM. Typically a Packer workflow would start an HTTP server local to where the `packer` command is running, but this would not be accessible by the VM Service VM.

In other words:

1. How does VM Operator configure a VM's guest such that it can access the network before the OS is even installed?
2. How does VM Operator make assets available via the network to the VM's guest OS's installation process?

It turns out there is already a [Packer builder for deploying a vSphere VM from ISO](https://developer.hashicorp.com/packer/integrations/vmware/vsphere/latest/components/builder/vsphere-iso) ([GitHub](https://github.com/vmware/packer-plugin-vsphere/tree/main/builder/vsphere/iso)). This builder uses the a virtual USB keyboard to send keyscan codes to communicate to the VM's guest in place of VNC. For Linux, tt should therefore be
possible to:

1. Update the VM API so that `spec.bootstrap.iso` enables automated installation from an ISO image, ex.:

    ```yaml
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: my-vm-1-boot-disk
      namespace: my-namespace-1
    spec:
      accessModes:
      - ReadWriteOnce
      volumeMode: Block
      resources:
        requests:
          storage: 20Gi
      storageClassName: my-storage-class-1

    ---

    apiVersion: vmoperator.vmware.com/v1alpha6
    kind: VirtualMachine
    metadata:
      name: my-vm-1
      namespace: my-namespace-1
    spec:
      className: best-effort-small
      imageName: ubuntu-24.04-iso
      storageClassName: my-storage-class-1
      guestID: ubuntu64Guest
      bootstrap:
        iso:
          commands:
          # Configure the Linux kernel with networking. There should be specific
          # parameters, ex. {{V1alpha6_FirstNicMacAddr}}, that are pre-defined
          # and map to values from the VM's intended network config,
          # ex. `status.network.config`.
          - "<esc><wait> "
          - "ifname=bootnet:{{V1alpha6_FirstNicMacAddr}};ip={{(V1alpha6_FormatIP V1alpha6_FirstIP \"\")}}:{{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}:{{(V1alpha6_SubnetMask V1alpha6_FirstIP)}}:{{.V1alpha6.VM.Status.Network.Config.DNS.HostName}}:bootnet "
          
          # This waits for 3 seconds, sends the "c" key, and then waits for
          # another 3 seconds. In the GRUB boot loader, this is used to enter
          # command line mode.
          - "<wait3s>c<wait3s>"
          
          # This types a command to load the Linux kernel from the specified
          # path with the 'autoinstall' option and the value of the
          # 'data_source_command' local variable.
          # The 'autoinstall' option is used to automate the installation
          # process.
          # The '{{V1Alpha6_BootstrapService}}' function resolves to the
          # IP_ADDR:PORT of the ephemeral bootstrap HTTP service that VM
          # Operator automatically creates and tears down for this VM.
          - "linux /casper/vmlinuz --- autoinstall ds=\"nocloud-net;seedfrom=http://{{V1Alpha6_BootstrapService}}/\""
          
          # This sends the "enter" key and then waits. This is typically used to
          # execute the command and give the system time to process it.
          - "<enter><wait>"
          
          # This types a command to load the initial RAM disk from the specified
          # path.
          - "initrd /casper/initrd"
          
          # This sends the "enter" key and then waits. This is typically used to
          # execute the command and give the system time to process it.
          - "<enter><wait>",
          
          # This types the "boot" command. This starts the boot process using
          # the loaded kernel and initial RAM disk.
          - "boot"
          
          # This sends the "enter" key. This is typically used to execute the
          # command.
          - "<enter>"
    
          assets:
          # The client has already uploaded the bootstrap assets to a Secret
          # resource named my-vm-1-bootstrap-data where each asset is a file
          # accessible at key-1 and key-2. This Secret will be mounted to a Pod
          # on Supervisor and the keys in the Secret will be accessible via the
          # network when the Pod starts a web server pointed at the path where
          # the Secret is mounted.
          - name: my-vm-1-bootstrap-data
            key: key-1
          - name: my-vm-1-bootstrap-data
            key: key-2
      volumes:
      - name: boot-disk
        persistentVolumeClaim:
          claimName: my-vm-1-boot-disk
    ```

2. Use a virtual USB keyboard to send scan codes that configure the [Linux kernel parameters](https://linuxlink.timesys.com/docs/static_ip) so any Linux installer can access the network.
3. Place any bootstrap assets into a `Secret` resource that is then made available on the same network as the VM via a PodVM with the `Secret` mounted as a volume and surfaced via a web server, ex. `python3 -m http.server`.
4. Use the virtual USB keyboard again to send scan codes from the `spec.bootstrap.iso.commands` property that indicates how to automate the installation.

For Windows systems it would be similar, the boot commands may differ, ex.:

1. Wait `X` amount of time to assume the system is at the first screen of the setup process
2. Use the virtual keyboard to send the `Shift-F10` key to access the command line.
3. Use `netsh` to then configure the network, ex.: `netsh interface ipv4 set address name="Ethernet" static {{(V1alpha6_FormatIP V1alpha6_FirstIP \"\")}} {{(V1alpha6_SubnetMask V1alpha6_FirstIP)}} {{(index .V1alpha6.VM.Status.Network.Config.Interfaces 0).Gateway4}}`.


---

## Reconciliation pipeline (big picture)

1. A DevOps user applies a `VirtualMachine` with `spec.bootstrap.iso` set, a
   `spec.hardware.cdrom` entry referencing an ISO-type `VirtualMachineImage`,
   and a Secret in the same namespace holding the referenced
   `spec.bootstrap.iso.assets`.
2. Before power-on, the CD-ROM reconciler attaches and connects the ISO. The
   validation webhook confirms the CD-ROM is ISO-typed, no conflicting
   bootstrap provider is set, and every asset's Secret name is well-formed.
3. VM Operator creates an ephemeral HTTP server (a Pod + `LoadBalancer`
   Service, named deterministically from the VM) that serves each asset at
   `/<secretName>/<key>`, reachable from the VM's workload network.
4. Once the VM is powered on and the ephemeral Service has an address, VM
   Operator sends `spec.bootstrap.iso.commands` to the VM's virtual USB
   keyboard, resolving Go template expressions (network config, the
   ephemeral server's address via `{{V1Alpha6_BootstrapService}}`) first.
   This happens exactly once per generation of `spec.bootstrap.iso` (tracked
   via an annotation hash).
5. The guest OS installer fetches its assets from the ephemeral HTTP server
   and completes an unattended install.
6. Once the VM reports a primary IP (or a configurable timeout elapses), VM
   Operator tears down the ephemeral Pod/Service and marks the
   `VirtualMachineBootstrapISOSynced` condition `True`.


---

## User stories

### US1 — API changes (Priority: P0)

- **Given** a DevOps user is authoring a `VirtualMachine` manifest, **When**
  they set `spec.bootstrap.iso.commands` and `spec.bootstrap.iso.assets`,
  **Then** the API accepts the manifest so long as no other bootstrap
  provider (CloudInit, LinuxPrep, Sysprep, VAppConfig) is also set, and at
  least one `spec.hardware.cdrom` entry references an ISO-type image.
- **Given** a `VirtualMachine` already has `spec.bootstrap.iso` set and boot
  commands have been sent, **When** the DevOps user attempts to change
  `spec.bootstrap.iso.commands` while the VM is powered on, **Then** the API
  rejects the update.

### US2 - Support Ubuntu Server 26.04 LTS (Priority: P0)

- **Given** a DevOps user applies a `VirtualMachine` referencing an Ubuntu
  Server ISO-type image, with `spec.bootstrap.iso.assets` pointing at a
  Secret containing `user-data` (an `autoinstall:` document) and `meta-data`,
  **When** the VM powers on, **Then** VM Operator sends boot commands that
  configure static networking on the GRUB2-loaded kernel and boot the
  installer with `autoinstall ds="nocloud-net;seedfrom=http://<ephemeral
  service>/"`, and the guest completes an unattended install without any
  VNC/console access.
- **Given** the Ubuntu install has completed and the VM reports a primary
  IP, **When** VM Operator next reconciles the VM, **Then** the ephemeral
  HTTP server Pod/Service are deleted and `VirtualMachineBootstrapISOSynced`
  is `True`.

### US3 - Support Windows Server 2025 (Priority: P0)

_Not yet implemented — see `plan.md`'s Windows end-to-end section for the
planned approach (reuses the same USB-keyboard/HTTP-server framework as
US2/US4, with a Windows-specific boot-command token sequence)._

### US4 - Support Ubuntu, Photon, Rocky, RHEL, other Linux distributions (Priority: P1)

_Not yet implemented beyond Ubuntu (US2) — see `plan.md`'s RHEL/Rocky/
AlmaLinux end-to-end section for the planned Kickstart-based approach, which
reuses the same framework as US2._

---

## Edge cases

- **Boot commands with excessive wait time**: the sum of every `<waitN>`
  token in `spec.bootstrap.iso.commands` is capped at 120 seconds; exceeding
  it fails the reconcile with a clear error before any keystrokes are sent
  (see `plan.md` OI-1).
- **Ephemeral HTTP server never gets an address**: VM Operator does not block
  waiting for the `LoadBalancer` Service's address; it requeues and retries,
  surfacing `HTTPServerNotReady` on `VirtualMachineBootstrapISOSynced` in the
  meantime.
- **Referenced asset Secret or key does not exist**: the reconcile fails with
  `AssetNotFound` on `VirtualMachineBootstrapISOSynced`; the same condition
  applies if the Secret is deleted after boot commands were already sent.
  (Discovering a missing asset before power-on, at admission time, is not
  currently validated — the webhook checks asset name well-formedness, not
  Secret/key existence.)
- **Guest reports an IP before the installer finishes**: the heuristic used
  to tear down the ephemeral HTTP server (primary IP populated, or a 60
  minute timeout) can be wrong in either direction; see `plan.md` OI-5.
- **Non-US keyboard layouts**: the virtual USB keyboard driver's scan-code
  table assumes a US QWERTY layout; VMs with a non-US layout configured in
  BIOS/UEFI may receive incorrect characters.

---

## Out of scope

- Support for non-Windows and non-Linux guest operating systems.

---

## Review & acceptance checklist

- [ ] All user stories have at least two Given/When/Then scenarios.
- [ ] Each scenario is independently testable.
- [ ] ExtraConfig Allow/Deny precedence is stated.
- [ ] Out-of-scope items are listed.

---

## References

* WIKI page 1872226468
* [William's manual testing](https://github.com/warroyo/packer-supervisor-examples/tree/main/manual-testing)
* [Ubuntu boot commands](https://manpages.ubuntu.com/manpages/xenial/en/man7/dracut.cmdline.7.html) (including networking)
* [RHEL boot commands](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/networking_guide/sec-configuring_ip_networking_from_the_kernel_command_line) (including networking)
* [Packer template](https://github.com/haproxytech/vmware-haproxy/blob/main/packer.json#L58-L63) to build a HAProxy VM using the existing VMware ISO builder
* Configure [Linux networking via kernel parameters](https://linuxlink.timesys.com/docs/static_ip)
* Use virtual USB keyboard to send keystrokes to remote VM:
    * [Example 1](https://github.com/hashicorp/packer-plugin-vsphere/blob/main/builder/vsphere/common/step_boot_command.go)
    * [Example 2](https://github.com/hashicorp/packer-plugin-vsphere/blob/main/builder/vsphere/driver/vm_keyboard.go)
* Packer examples for vSphere:
    * [Base](https://github.com/vmware/packer-examples-for-vsphere/tree/develop/builds)
    * [Linux](https://github.com/vmware/packer-examples-for-vsphere/tree/develop/builds/linux)
    * [Windows](https://github.com/vmware/packer-examples-for-vsphere/tree/develop/builds/windows)