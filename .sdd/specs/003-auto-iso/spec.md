# Feature Specification: Automated Deployment from ISO image

- **Feature branch**: [`feature/auto-iso`](https://github.com/akutz/vm-operator/tree/feature/auto-iso/)
  - **Fork**: `akutz/vm-operator`
  - **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-06-29
- **Status**: In Progress (Architecture phase; code not yet started)
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



---

## User stories

### US1 — API changes (Priority: P0)

### US2 - Support Ubuntu Server 26.04 LTS (Priority: P0)

### US3 - Support Windows Server 2025 (Priority: P0)

### US4 - Support Ubuntu, Photon, Rocky, RHEL, other Linux distributions (Priority: P1)

---

## Edge cases


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