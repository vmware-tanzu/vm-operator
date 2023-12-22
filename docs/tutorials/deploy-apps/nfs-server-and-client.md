# Deploy an NFS server and client

This tutorial describes how to deploy two VMs in different namespaces, where the first VM acts as an NFS server and the other as the client, able to mount the exported share via a `VirtualMachineService`.

## Prerequisites

This tutorial assumes the following:

* There are two namespaces in which the user is able to deploy `VirtualMachine` resources. In this tutorial, the namespaces shall be referred to as `ns-1` and `ns-2`.
* Both namespaces have access to a `VirtualMachineClass`. In this tutorial, the class used is `best-effort-small`.
* Both namespaces have access to a `VirtualMachineImage` (VMI) with Ubuntu 22.04+. In this tutorial, the VMI used is `vmi-a5bad5914ea06d469`.
* Both namespaces have access to a `StorageClass`. In this tutorial, the class used is `global-storage`.

## Deploy the server

The first step is to deploy a VM that will act as the NFS server, which is achieved by applying the following YAML:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachine
metadata:
  name: nfs-server
  namespace: ns-1
  labels:
    app: nfs-server
spec:
  className: best-effort-small
  imageName: vmi-a5bad5914ea06d469 # Ubuntu 22.04 (jammy)
  storageClass: global-storage
  vmMetadata:
    transport:  CloudInit
    secretName: nfs-server-bootstrap-data

---

apiVersion: v1
kind: Secret
metadata:
  name: nfs-server-bootstrap-data
  namespace: ns-1
stringData:
  user-data: |
    #cloud-config
    ssh_pwauth: true
    users:
    - name: user
      primary_group: user
      sudo: ALL=(ALL) NOPASSWD:ALL
      groups: users
      lock_passwd: false
      # Password is secret
      passwd: '$1$SaltSalt$YhgRYajLPrYevs14poKBQ0'
      shell: /bin/bash
    runcmd:
    - apt-get update --assume-no
    - apt-get install --assume-yes nfs-kernel-server sudo
    - mkdir -p /mnt/share
    - echo 'world.' >/mnt/share/hello
    - echo '/mnt/share *(rw,sync,no_subtree_check,insecure)' >>/etc/exports
    - exportfs -a
```

## Create service to access NFS exports

Great, an NFS server is up and running in namespace `ns-1`! To ensure workloads outside of that namespace are able to mount the NFS server's exports, a `VirtualMachineService` is used to provide a load balanced IP address that can be accessed across namespaces.

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachineService
metadata:
  name: nfs-server
  namespace: ns-1
spec:
  selector:
    app: nfs-server
  type: LoadBalancer
  ports:
  - name: tcp-ssh
    port: 22
    protocol: TCP
    targetPort: 22
  - name: tcp-rpcbind
    port: 111
    protocol: TCP
    targetPort: 111
  - name: tcp-nfs
    port: 2049
    protocol: TCP
    targetPort: 2049
```

Use `kubectl` to make note of the service's external IP address, ex.:

```shell
$ kubectl -n ns-1 get service nfs-server
NAME         TYPE           CLUSTER-IP      EXTERNAL-IP   PORT(S)                   AGE
nfs-server   LoadBalancer   172.24.116.43   192.168.0.4   22/TCP,111/TCP,2049/TCP   2m3s
```

## Deploy the client

With the NFS server up and running, it is time to deploy the VM that will act as the client by applying the following YAML:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachine
metadata:
  name: nfs-client
  namespace: ns-2
  labels:
    app: nfs-client
spec:
  className: best-effort-small
  imageName: vmi-a5bad5914ea06d469 # Ubuntu 22.04 (jammy)
  storageClass: global-storage
  vmMetadata:
    transport:  CloudInit
    secretName: nfs-client-bootstrap-data

---

apiVersion: v1
kind: Secret
metadata:
  name: nfs-client-bootstrap-data
  namespace: ns-2
stringData:
  user-data: |
    #cloud-config
    ssh_pwauth: true
    users:
    - name: user
      primary_group: user
      sudo: ALL=(ALL) NOPASSWD:ALL
      groups: users
      lock_passwd: false
      # Password is secret
      passwd: '$1$SaltSalt$YhgRYajLPrYevs14poKBQ0'
      shell: /bin/bash
    runcmd:
    - apt-get update --assume-no
    - apt-get install --assume-yes nfs-common sudo
    - mkdir /var/share
    - chmod 0777 /var/share
```

## Create service to access NFS client via SSH

Just like a service is required to access the NFS exports across namespaces, the following YAML creates a service that allows external actors to access the NFS client via SSH:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha1
kind: VirtualMachineService
metadata:
  name: nfs-client
  namespace: ns-2
spec:
  selector:
    app: nfs-client
  type: LoadBalancer
  ports:
  - name: tcp-ssh
    port: 22
    protocol: TCP
    targetPort: 22
```

Use `kubectl` to make note of the service's external IP address, ex.:

```shell
$ kubectl -n ns-2 get service nfs-client
NAME         TYPE           CLUSTER-IP     EXTERNAL-IP   PORT(S)   AGE
nfs-client   LoadBalancer   172.24.204.9   192.168.0.5   22/TCP    51s
```

## Mount the exported share

Alright, it is time to see if everything works as intended!

1. With the external IP address from the `nfs-client` service, SSH into the VM acting as the NFS client:

    ```shell
    ssh user@<NFS_CLIENT_SERVICE_EXTERNAL_IP>
    ```

2. Print the contents of the file `/var/share/hello` to the screen:

    ```shell
    cat /var/share/hello
    ```

    Oops! An error occurred, because that file does not exist!

    ```shell
    cat: /var/share/hello: No such file or directory
    ```

3. Mount the export from the NFS server:

    ```shell
    sudo mount -t nfs <NFS_SERVER_SERVICE_EXTERNAL_IP>:/mnt/share /var/share
    ```

4. Print the contents of the file `/var/share/hello` one more time:

    ```shell
    cat /var/share/hello
    ```

    If the text `world.` appeared on the screen, then congratulations, the NFS export was successfully mounted to the client!

