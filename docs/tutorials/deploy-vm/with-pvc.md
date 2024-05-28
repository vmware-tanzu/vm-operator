# With PVC

This tutorial describes how to deploy a VM with an additional `1Gi` disk provided by a [Persistent Volume Claim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) (PVC).

## Prerequisites

This tutorial assumes the following:

* There is a namespace in which the user is able to deploy `VirtualMachine` resources. In this tutorial, the namespace is `my-namespace`.
* The namespace has access to a `VirtualMachineClass`. In this tutorial, the class is `my-vm-class`.
* The namespace has access to a `VirtualMachineImage` (VMI) with Ubuntu 24.04 (Noble Numbat). In this tutorial, the VMI is `vmi-cc61318e786a2a31f`.
* The namespace has access to a `StorageClass`. In this tutorial, the class is `my-storage-class`.

    !!! note "Storage Classes"

        The `StorageClass` for the `VirtualMachine` resource applies only to the disks from the `VirtualMachineImage` from which the VM is deployed. PVCs may specify their own `StorageClass`, completely different from what the VM uses.

        However, for the purpose of simplicity, this tutorial uses the same `StorageClass` for both the `PersistentVolumeClaim` and `VirtualMachine` resources.


## Create PVC

The first step is to create a PVC named `my-disk` that will be used by the VM as additional storage:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-disk
  namespace: my-namespace
spec:
  accessModes:
  - ReadWriteOnce
  volumeMode: Block
  resources:
    requests:
      storage: 1Gi
  storageClassName: my-storage-class
```

## Deploy VM w PVC

The next step is to deploy a VM with the PVC attached as a disk:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm-1
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: vmi-cc61318e786a2a31f # Ubuntu 24.04 Noble Numbat
  storageClass: my-storage-class
  volumes:
  # Attach the PVC named "my-disk" to the VM.
  - name: my-disk
    persistentVolumeClaim:
      claimName: my-disk # Since the image used to deploy the VM has
                         # exactly one disk, this PVC will be attached
                         # as /dev/sdb. Please note, this name is not
                         # guaranteed or deterministic. Any scripts used
                         # to partition/mount disks in production systems
                         # should be more resilient than relying on the
                         # order of volumes in this list or the number of
                         # disks that might already be attached to the VM.
  bootstrap:
    cloudInit:
      cloudConfig:
        users:
        - name: jdoe
          shell: /bin/bash
          sudo: ALL=(ALL) NOPASSWD:ALL
          ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
        runcmd:
        # Tell Ubuntu to refresh its list of packages.
        - apt-get update  --assume-no

        # Install the parted command used to partition the disk.
        - apt-get install --assume-yes parted

        # Partition the disk, using as much of its space as possible.
        - parted -s /dev/sdb -- mkpart primary ext4 0% 100%

        # Create a filesystem on the disk.
        - mkfs -t ext4 /dev/sdb

        # Create the directory where the disk will be mounted.
        - mkdir -p /mnt/my-data-dir-1

        # Ensure the disk is mounted at boot.
        - echo '/dev/sdb /mnt/my-data-dir-1 ext4 defaults 0 2' >>/etc/fstab

        # Go ahead and mount the disk manually this time.
        - mount -a
```

## Verify PVC on VM

1. Wait for the VM to be deployed and get an IP address:

    ```shell
    kubectl -n my-namespace get vm my-vm-1 -owide -w
    ```

2. SSH into the VM and verify the disk was attached, formatted, and mounted as expected:

    ```shell
    ssh jdoe@<VM_IP>
    ```

3. List the system's block devices:

    ```shell
    jdoe@my-vm-1:~$ lsblk
    NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
    sda       8:0    0   10G  0 disk 
    ├─sda1    8:1    0    9G  0 part /
    ├─sda14   8:14   0    4M  0 part 
    ├─sda15   8:15   0  106M  0 part /boot/efi
    └─sda16 259:0    0  913M  0 part /boot
    sdb       8:16   0    1G  0 disk /mnt/my-data-dir-1
    ```

    And there it is! The device `/dev/sdb` is mounted to `/mnt/my-data-dir-1`!

4. Finally, create a file at the root of the mounted disk:

    ```shell
    echo world >/mnt/my-data-dir-1/hello
    ```

## Delete VM

Log out of the VM and go ahead and delete it, but <strike>take</strike> leave the <strike>cannoli</strike> PVC:

```
kubectl -n my-namespace delete vm my-vm-1
```

## Deploy new VM w same PVC

The file written to `/mnt/my-data-dir-1/hello` in the first VM should still be available on the disk managed by the PVC `my-disk`. To verify that, deploy a second VM and attach the PVC to it:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-vm-2
  namespace: my-namespace
spec:
  className: my-vm-class
  imageName: vmi-cc61318e786a2a31f # Ubuntu 24.04 Noble Numbat
  storageClass: my-storage-class
  volumes:
  # Attach the PVC named "my-disk" to the VM.
  - name: my-disk
    persistentVolumeClaim:
      claimName: my-disk
  bootstrap:
    cloudInit:
      cloudConfig:
        users:
        - name: jdoe
          shell: /bin/bash
          sudo: ALL=(ALL) NOPASSWD:ALL
          ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSL7uWGj...
        runcmd:
        # Create the directory where the disk will be mounted, using a different
        # path than the one from the first VM.
        - mkdir -p /mnt/my-data-dir-2

        #
        # Please note it is not necessary to install the tools
        # to partition the disk this time since the disk was
        # already partitioned and has a file system from the
        # previous VM.
        #

        # Ensure the disk is mounted at boot.
        - echo '/dev/sdb /mnt/my-data-dir-2 ext4 defaults 0 2' >>/etc/fstab
        
        # Go ahead and mount the disk manually this time.
        - mount -a
```

## Verify PVC on new VM

This section is similar to verifying the PVC on the first VM, but in this case the disk provided by the PVC was not partitioned or formatted prior to being attached, so the only way the disk was mounted successfully is if it is re-used from the previous VM, including the file `hello` that was written to the root of the disk:

1. Wait for the VM to be deployed and get an IP address:

    ```shell
    kubectl -n my-namespace get vm my-vm-2 -owide -w
    ```

2. SSH into the VM and verify the disk was attached, formatted, and mounted as expected:

    ```shell
    ssh jdoe@<VM_IP>
    ```

3. List the system's block devices:

    ```shell
    jdoe@my-vm-2:~$ lsblk
    NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
    sda       8:0    0   10G  0 disk 
    ├─sda1    8:1    0    9G  0 part /
    ├─sda14   8:14   0    4M  0 part 
    ├─sda15   8:15   0  106M  0 part /boot/efi
    └─sda16 259:0    0  913M  0 part /boot
    sdb       8:16   0    1G  0 disk /mnt/my-data-dir-2
    ```

    And there it is! The device `/dev/sdb` is mounted to `/mnt/my-data-dir-2`!

4. Finally, verify the file written to the root of the filesystem on `/dev/sdb` from the first VM is present in the path where the disk is mounted in the new VM:

    ```shell
    jdoe@my-vm-2:~$ cat /mnt/my-data-dir-2/hello
    world
    ```

    Even though the file was originally written to `/mnt/my-data-dir-1/hello` on the first VM, the same disk is mounted to `/mnt/my-data-dir-2` in this new VM, and thus the file, with the expected content, is present at `/mnt/my-data-dir-2/hello`!
