# Controller workflows

This document describes how zone discovery fans out into cluster-scoped config metadata and guest option resources.

**Hardware and guest descriptors:** `Zone → ConfigTarget → VirtualMachineConfigOptions → VirtualMachineGuestOptions`.

## End-to-end flow

```mermaid
flowchart TB
  subgraph zone["Zone controller"]
    ZW["Watch Zone (namespace-scoped)"]
    ZP["Read spec.namespace.poolMoIDs"]
    ZC["Derive unique cluster MoIDs from pools"]
    ZCT["Create/patch ConfigTarget per cluster<br/>metadata.name = cluster MoID"]
    ZW --> ZP --> ZC --> ZCT
  end

  subgraph ct["ConfigTarget controller"]
    CTW["Watch ConfigTarget (cluster-scoped)"]
    QCT["QueryConfigTarget → status from ConfigTarget"]
    QCOD["QueryConfigOptionDescriptor → hardware version keys"]
    CTO["Create/patch VirtualMachineConfigOptions per version<br/>metadata.name = e.g. vmx-22"]
    CTW --> QCT
    CTW --> QCOD --> CTO
  end

  subgraph vmco["VirtualMachineConfigOptions controller"]
    VW["Watch VirtualMachineConfigOptions (cluster-scoped)"]
    QCOE["QueryConfigOptionEx for that hardware version"]
    VST["Fill status; collect GuestOsDescriptor IDs → status.guestIDs"]
    VGO["Create/patch VirtualMachineGuestOptions per guest ID<br/>metadata.name = DNS-safe guest ID"]
    VW --> QCOE --> VST --> VGO
  end

  ZCT --> CTW
  CTO --> VW
```

Each `VirtualMachineGuestOptions` reconciliation updates `status.hardwareVersions` with one entry for the hardware version of the `VirtualMachineConfigOptions` being reconciled; reconciles of other hardware versions append their own entries (see the `VirtualMachineConfigOptions` controller section above).

## vSphere API and Kubernetes writes

```mermaid
sequenceDiagram
  participant K as Kubernetes API
  participant ZC as Zone controller
  participant CTC as ConfigTarget controller
  participant EB as vSphere EnvironmentBrowser
  participant VMCO as VirtualMachineConfigOptions controller

  ZC->>K: Create/patch ConfigTarget (per cluster MoID)

  CTC->>EB: QueryConfigTarget
  EB-->>CTC: vim.vm.ConfigTarget
  CTC->>K: Patch ConfigTarget status

  CTC->>EB: QueryConfigOptionDescriptor
  EB-->>CTC: ConfigOptionDescriptor list (hw keys)
  loop Each hardware version
    CTC->>K: Create/patch VirtualMachineConfigOptions
  end

  VMCO->>EB: QueryConfigOptionEx(hw version)
  EB-->>VMCO: vim.vm.ConfigOption
  VMCO->>K: Patch VirtualMachineConfigOptions status (guest IDs, etc.)
  loop Each guest OS ID from descriptors
    VMCO->>K: Create/patch VirtualMachineGuestOptions + patch status.hardwareVersions
  end
```

## Controllers

### `Zone` controller

1. `Zone` controller watches `Zone` resources in a namespace.
2. When `Zone` object is reconciled, its `spec.namespace.poolMoIDs` list is inspected to get a list of the resource pool IDs belonging to the zone.
3. A unique set of vSphere cluster managed object IDs is derived from the list of the zone's pool IDs.
4. For each of the unique cluster managed object IDs, the `Zone` controller creates or patches a cluster-scoped `ConfigTarget` resource with the `metadata.name` of the resource being the managed object ID of the cluster.

### `ConfigTarget` controller

1. The new `ConfigTarget` controller watches for the `ConfigTarget` resources. Upon seeing a `ConfigTarget` resource, the controller calls [`QueryConfigTarget`](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.EnvironmentBrowser.html#queryConfigTarget) on the cluster's environment browser. The controller uses the [result](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.ConfigTarget.html) to populate the `ConfigTarget`'s status information.
2. The new `ConfigTarget` controller also calls the [`QueryConfigOptionDescriptor`](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.EnvironmentBrowser.html#queryConfigOptionDescriptor) API, getting a list of the hardware versions supported by the cluster (the [`key`](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.ConfigOptionDescriptor.html#key) field).
3. For each hardware version, the `ConfigTarget` controller creates or patches a cluster-scoped `VirtualMachineConfigOptions` resource, with its `metadata.name` field being the hardware version, ex. vmx-22.

### `VirtualMachineConfigOptions` controller

1. The new `VirtualMachineConfigOptions` controller watches the new, cluster-scoped `VirtualMachineConfigOptions` API. Upon seeing a `VirtualMachineConfigOptions` resource, the controller calls [`QueryConfigOptionEx`](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.EnvironmentBrowser.html#queryConfigOptionEx) using the hardware version the `VirtualMachineConfigOptions` object represents.
2.  Using the [result](https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.ConfigOption.html) of this call, the `VirtualMachineConfigOptions` controller fills in the status of the `VirtualMachineConfigOptions` object. However, for each of the `ConfigOption` result's `GuestOsDescriptors`, only the ID is used (after being converted to the CRD format of the ID). All the guest IDs are placed in the `VirtualMachineConfigOptions` object's `status.guestIDs` field.
3. For each guest ID, the `VirtualMachineConfigOptions` controller creates/patches a `VirtualMachineGuestOptions` object, with the `metadata.name` value being the DNS-safe version of the guest ID. The object's spec includes the guest ID. The controller fills out the object's status and updates its `status.hardwareVersions` listMap with the information for the hardware version represented by the `VirtualMachineConfigOptions` object being reconciled. Please note, this will mean filling out only a single element in the `status.hardwareVersions` list map, since other entries are populated when the `VirtualMachineConfigOptions` controller reconciles `VirtualMachineConfigOptions` objects representing other hardware versions.

## Governance

`VirtualMachineConfigPolicy` is removed (vmop-4318). VM Operator deletes its experimental CRD and objects on upgrade. The environment-browser pipeline keeps its existing capability gate. VM Class governance, resource ranges, ExtraConfig rules and admission enforcement belong to the separate Flex VM Classes specification (vmop-4034, PR #2067).
