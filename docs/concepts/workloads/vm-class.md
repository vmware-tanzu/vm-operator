# VirtualMachineClass

// TODO ([github.com/vmware-tanzu/vm-operator#95](https://github.com/vmware-tanzu/vm-operator/issues/95))

_VirtualMachineClasses_ (VM Class) represent a collection of virtual hardware that is used to realize a new VirtualMachine (VM).

## ControllerName

The field `spec.controllerName` in a VM Class indicates the name of the controller responsible for reconciling `VirtualMachine` resources created from the VM Class. The value `vmoperator.vmware.com/vsphere` indicates the default `VirtualMachine` controller is used. VM Classes that do not have this field set or set to an empty value behave as if the field is set to the value returned from the environment variable `DEFAULT_VM_CLASS_CONTROLLER_NAME`. If this environment variable is empty, it defaults to `vmoperator.vmware.com/vsphere`.

## `spec.configSpec`

The field `spec.configSpec` contains a vSphere [`VirtualMachineConfigSpec`](https://vdc-download.vmware.com/vmwb-repository/dcr-public/184bb3ba-6fa8-4574-a767-d0c96e2a38f4/ba9422ef-405c-47dd-8553-e11b619185b2/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.ConfigSpec.html), a sparse data type capable of representing all of the hardware and options used to deploy a VM, ex:

```yaml
apiVersion: vmoperator.vmware.com/v1alpha2
kind: VirtualMachineClass
metadata:
  name: my-vm-class
  namespace: my-namespace
spec:
  configSpec:
    _typeName: VirtualMachineConfigSpec
    memoryMB: 2048
    numCPUs: 2
```

### `VirtualMachineConfigSpec`

The reason the `VirtualMachineConfigSpec` type was selected is because it is sparse, and any value can be omitted. The type [`VirtualMachineConfigInfo`](https://vdc-download.vmware.com/vmwb-repository/dcr-public/184bb3ba-6fa8-4574-a767-d0c96e2a38f4/ba9422ef-405c-47dd-8553-e11b619185b2/SDK/vsphere-ws/docs/ReferenceGuide/vim.vm.ConfigInfo.html) makes more sense for desired state, but there are multiple properties that are required in that type, and it would not have worked as a sparse data type.

#### Device Changes

The only valid operation for a `VirtualMachineConfigSpec` type in a `VirtualMachineClass` resource is `add`. In the context of a `VirtualMachineClass`, the `VirtualMachineConfigSpec` can be thought of as a sparse `VirtualMachineConfigInfo` data type.

### Encoding

JSON was conceived as an alternative to XML, and as such, was designed to be very light-weight. Originally this also meant there was no schema support in JSON, meaning it was not the _best_ candidate for encoding polymorphic data structures. Given the vSphere API is *quite* object oriented and the fact that Kubernetes uses JSON (even if it is typically presented as YAML), how is the `VirtualMachineConfigSpec` data type encoded into a `VirtualMachineClass` resource's `spec.configSpec` field? The answer is JSON discriminators, a technique for embedding the encoded type name (and possibly value) into the data itself. When it comes to encoding the `VirtualMachineConfigSpec`, there are two special discriminator property names:

| Discriminator Property | Description |
|---|---|
| `_typeName` | The name of the vSphere type to which this JSON object maps |
| `_value` | If a Go `struct` has a field that is a Go interface, the actual value may be wrapped in a JSON object that has two properties, `_typeName` and `_value`, the wrapped value |

The file [`./pkg/docs/concepts/vm-class_test.go`](${{ config.repo_url }}/main/pkg/docs/concepts/vm-class_test.go) has several examples of a `VirtualMachineConfigSpec` encoded to both JSON and YAML. 

#### ExtraConfig

##### JSON

```json
{
  "_typeName": "VirtualMachineConfigSpec",
  "numCPUs": 2,
  "memoryMB": 2048,
  "extraConfig": [
    {
      "_typeName": "OptionValue",
      "key": "my-key-1",
      "value": {
        "_typeName": "string",
        "_value": "my-value-1"
      }
    },
    {
      "_typeName": "OptionValue",
      "key": "my-key-2",
      "value": {
        "_typeName": "string",
        "_value": "my-value-2"
      }
    }
  ]
}
```

##### YAML

```yaml
_typeName: VirtualMachineConfigSpec
extraConfig:
- _typeName: OptionValue
  key: my-key-1
  value:
    _typeName: string
    _value: my-value-1
- _typeName: OptionValue
  key: my-key-2
  value:
    _typeName: string
    _value: my-value-2
memoryMB: 2048
numCPUs: 2
```

#### vGPU

##### JSON

```json
{
  "_typeName": "VirtualMachineConfigSpec",
  "deviceChange": [
    {
      "_typeName": "VirtualDeviceConfigSpec",
      "operation": "add",
      "device": {
        "_typeName": "VirtualPCIPassthrough",
        "key": 0,
        "backing": {
          "_typeName": "VirtualPCIPassthroughVmiopBackingInfo",
          "vgpu": "my-vgpu-profile"
        }
      }
    }
  ]
}
```

##### YAML

```yaml
_typeName: VirtualMachineConfigSpec
deviceChange:
- _typeName: VirtualDeviceConfigSpec
  device:
    _typeName: VirtualPCIPassthrough
    backing:
      _typeName: VirtualPCIPassthroughVmiopBackingInfo
      vgpu: my-vgpu-profile
    key: 0
  operation: add
```

#### Dynamic Direct Path I/O

##### JSON

```json
{
  "_typeName": "VirtualMachineConfigSpec",
  "deviceChange": [
    {
      "_typeName": "VirtualDeviceConfigSpec",
      "operation": "add",
      "device": {
        "_typeName": "VirtualPCIPassthrough",
        "key": 0,
        "backing": {
          "_typeName": "VirtualPCIPassthroughDynamicBackingInfo",
          "deviceName": "",
          "allowedDevice": [
            {
              "_typeName": "VirtualPCIPassthroughAllowedDevice",
              "vendorId": -999,
              "deviceId": -999
            }
          ]
        }
      }
    }
  ]
}
```

##### YAML

```yaml
_typeName: VirtualMachineConfigSpec
deviceChange:
- _typeName: VirtualDeviceConfigSpec
  device:
    _typeName: VirtualPCIPassthrough
    backing:
      _typeName: VirtualPCIPassthroughDynamicBackingInfo
      allowedDevice:
      - _typeName: VirtualPCIPassthroughAllowedDevice
        deviceId: -999
        vendorId: -999
      deviceName: ""
    key: 0
  operation: add
```

