# Deploy a VM with ISO

This tutorial describes the end-to-end workflow for importing an ISO image from a URL, deploying a VM from the ISO image, and installing the guest OS via VM's web console.

## Import ISO from URL

ISO images are usually imported by an infrastructure administrator into a Content Library associated with a namespace or cluster. To check if any ISO images are available in a namespace or cluster, use the following commands:

```shell
kubectl get -n <NAMESPACE> vmi -l image.vmoperator.vmware.com/type=ISO

kubectl get cvmi -l image.vmoperator.vmware.com/type=ISO
```

DevOps can also import OVF/ISO images from a URL using the image registry operator's [ContentLibraryItemImportRequest](https://github.com/vmware-tanzu/image-registry-operator-api/blob/main/api/v1alpha1/contentlibraryitemimportrequest_types.go) API. The following example demonstrates how to import an upstream Rocky Linux ISO image:

```yaml
apiVersion: imageregistry.vmware.com/v1alpha1
kind: ContentLibraryItemImportRequest
metadata:
  name: rocky-dvd-iso
  namespace: my-namespace
spec:
  source:
    url: https://download.rockylinux.org/pub/rocky/9/isos/x86_64/Rocky-9.5-x86_64-dvd.iso # (1)
    sslCertificate: | # (2)
      -----BEGIN CERTIFICATE-----
      MIIFLzCCBBegAwIBAgISBGoStW8/uta8jDAR8ldbQDihMA0GCSqGSIb3DQEBCwUA
      MDMxCzAJBgNVBAYTAlVTMRYwFAYDVQQKEw1MZXQncyBFbmNyeXB0MQwwCgYDVQQD
      EwNSMTEwHhcNMjQxMjA2MTE1MzAzWhcNMjUwMzA2MTE1MzAyWjAhMR8wHQYDVQQD
      ExZtaXJyb3JzLnJvY2t5bGludXgub3JnMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
      MIIBCgKCAQEA6EZ30U0n8CBlWl8jC3Zwasky+3SQDXmehkPLo0K0v7kHPE4g+NUa
      kvyanUHYuGtudvleMTbZSvZp7bv8iFRll41r0Z01j49vVEIsL5wixb4NyJX2DDne
      a0G3ta+JzrNQhCqcKuXKFo4EgAYTvaKSa1KEZ+REUb8XhtafUyImSehF0xYWE1Tc
      obyg57oCO+SXfP9ypHjCC9/rZjbrU/UA+x+Sofydt1HkQ4g/dXX3prNnD75k8ue6
      3MkyP2QcfFc3IfzsoGFUXxtm10F6BGjwGm289qxSuIqcX7ylmMnCVGs4q+UrPWPN
      bXWDU7ItnOlGjML000+l8p500iFSXcQO3QIDAQABo4ICTTCCAkkwDgYDVR0PAQH/
      BAQDAgWgMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAMBgNVHRMBAf8E
      AjAAMB0GA1UdDgQWBBSGel3Hh031yt91oQYM89+kOLE7DTAfBgNVHSMEGDAWgBTF
      z0ak6vTDwHpslcQtsF6SLybjuTBXBggrBgEFBQcBAQRLMEkwIgYIKwYBBQUHMAGG
      Fmh0dHA6Ly9yMTEuby5sZW5jci5vcmcwIwYIKwYBBQUHMAKGF2h0dHA6Ly9yMTEu
      aS5sZW5jci5vcmcvMFQGA1UdEQRNMEuCF2Rvd25sb2FkLnJvY2t5bGludXgub3Jn
      ghhkb3dubG9hZHMucm9ja3lsaW51eC5vcmeCFm1pcnJvcnMucm9ja3lsaW51eC5v
      cmcwEwYDVR0gBAwwCjAIBgZngQwBAgEwggEEBgorBgEEAdZ5AgQCBIH1BIHyAPAA
      dQDgkrP8DB3I52g2H95huZZNClJ4GYpy1nLEsE2lbW9UBAAAAZOcBojlAAAEAwBG
      MEQCIF2OTCyynecFhDAg7Im7bxs0s1W5ieZxCp0wM/0CJ7HgAiB4gPxrdXVprVmR
      49faW7Lqhw8mfzOCQV7vyKN9CIVJBgB3AM8RVu7VLnyv84db2Wkum+kacWdKsBfs
      rAHSW3fOzDsIAAABk5wGiLMAAAQDAEgwRgIhAORV5PFPjCPkuX3zG3S1cbKwGU8n
      MkKDgjJyMmUO9cSGAiEAzmvrDHTwJIV4AeGWFzMO3adyo0pWOCnEoIcVqLwWk8Yw
      DQYJKoZIhvcNAQELBQADggEBAKm1NbsCgzov+GYAZHkkOHcw6ypbTsg1126HIB6k
      n4RKFDTpvj62YCZTkTSAKY7gHSMt18jNN7LWv344dxLSB+goA5QSMUNHrsURoRW3
      Bh7n2j6rr6LSF2HhBvRBPUk8HI8OgvuBXP0looxVgfBwybCEwjCa6iTaqcwPcECz
      8P5r+lRn2nj3aDFrQ/zYZjA28A2bIgjcedg/5q1FNwPZWzsQ/tK6wkDeENzLgs5x
      y9AY7M6fnRUbYGYauLA50clZAmxG63oApiaOvNFn1oK3TSAHkk1mZ16ibupI0e2F
      Ypmf48/C4TvbDkL8qGM0o8hDqzax8eEY4w6sWbV9U/HnzcM=
      -----END CERTIFICATE-----
    checksum: # (3)
      value: ba60c3653640b5747610ddfb4d09520529bef2d1d83c1feb86b0c84dff31e04e
      algorithm: SHA256
  target:
    item:
      name: rocky-dvd-iso
      type: ISO
    library:
      apiVersion: imageregistry.vmware.com/v1alpha1
      kind: ContentLibrary
      name: cl-bb87952414e9859e8 # (4)
```

1.  :wave: The URL specifies an ISO file to be imported as a `VirtualMachineImage` resource.
2.  :wave: A PEM encoded SSL Certificate for the endpoint specified by the above URL.
3.  :wave: An optional field to verify the checksum algorithm and value for the file at the URL.
4.  :wave: The name of the Kubernetes content library object associated with the namespace for importing the ISO image.

## Deploy VM from ISO

Once the ISO image is available, you can deploy a VM with a CD-ROM configured to mount the ISO image and a PVC (PersistentVolumeClaim) attached as the VM's boot disk. The following example demonstrates how to create both the PVC and VM objects for an ISO-based deployment:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-pvc
  namespace: my-namespace
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi # (1)
  storageClassName: wcpglobal-storage-profile
---
apiVersion: vmoperator.vmware.com/v1alpha3
kind: VirtualMachine
metadata:
  name: my-iso-vm
  namespace: my-namespace
spec:
  className: best-effort-small
  storageClass: wcpglobal-storage-profile
  guestID: vmwarePhoton64Guest # (2)
  cdrom:
  - name: cdrom1 # (3)
    image:
      kind: VirtualMachineImage # (4)
      name: vmi-0a0044d7c690bcbea # (5)
    connected: true # (6)
    allowGuestControl: true # (7)
  volumes:
    - name: boot-disk
      persistentVolumeClaim:
        claimName: my-pvc # (8)
```

1.  :wave: The `storage` field specifies the size of the VM's boot disk.
2.  :wave: The `guestID` field is required to specify the [guest operating system identifier](https://dp-downloads.broadcom.com/api-content/apis/API_VWSA_001/8.0U3/html/ReferenceGuides/vim.vm.GuestOsDescriptor.GuestOsIdentifier.html) when deploying a VM from an ISO image.
3.  :wave: The `name` field must consist of at least two lowercase letters or digits and be unique among all CD-ROM devices attached to the VM.
4.  :wave: The `image.kind` field specifies the type of the ISO image (either `VirtualMachineImage` or `ClusterVirtualMachineImage`).
5.  :wave: The `image.name` field specifies the Kubernetes object name of the ISO type `VirtualMachineImage` or `ClusterVirtualMachineImage` resource.
6.  :wave: The `connected` field indicates the desired connection state of the CD-ROM device (defaults to `true`).
7.  :wave: The `allowGuestControl` field specifies whether a web console connection can manage the CD-ROM device's connection state (defaults to `true`).
8.  :wave: The `claimName` field specifies the name of the PVC to be used as the VM's boot disk.

## Install Guest OS

Once the VM is deployed and powered on, the guest OS can be installed via the VM's web console. To access the web console, the following command may be used:

```shell
kubectl vsphere vm web-console -n <NAMESPACE> <VM_NAME>
```

Click on the URL generated by the command to access the web console. The installation steps will vary depending on the guest OS type.

To configure networking, you can retrieve the VM's network configuration details using:

```shell
kubectl get vm -n <NAMESPACE> <VM_NAME> -o jsonpath='{.status.network.config}'
```

The output will be similar to the following:

```json
{
  "dns": {
    "hostName": "<VM_NAME>",
    "nameservers": [
      "<NAMESERVER_IP_1>",
      "<NAMESERVER_IP_2>"
    ]
  },
  "interfaces": [
    {
      "ip": {
        "addresses": [
          "<IP_ADDRESS>/<NETMASK>"
        ],
        "gateway4": "<GATEWAY_IP>"
      },
      "name": "<INTERFACE_NAME>"
    }
  ]
}
```

After completing the OS installation via the web console, reboot the VM. The VM should power on with the configured IP address. If no IP address is reported, ensure that VMware Tools (or an open-source equivalent like open-vm-tools) is properly installed and running on the guest OS.
