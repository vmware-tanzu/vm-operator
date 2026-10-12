// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"strings"
	"testing"

	"github.com/vmware/govmomi/vim25/types"
)

func TestGetVMClassInfo(t *testing.T) {
	dec := types.NewJSONDecoder(strings.NewReader(getVMClassJSON))

	var obj VMClassInfo

	err := dec.Decode(&obj)
	if err != nil {
		t.Error(err)
	}

	if obj.ConfigSpec == nil {
		t.Errorf("VM Class %s has an empty ConfigSpec", obj.ID)
	}

	if e, a := 1, len(obj.ConfigSpec.ExtraConfig); e != a {
		t.Errorf("should be %d extra config options: %d", e, a)
	}
}

func TestListVMClasses(t *testing.T) {
	dec := types.NewJSONDecoder(strings.NewReader(listVMClassesJSON))

	var obj []VMClassInfo

	err := dec.Decode(&obj)
	if err != nil {
		t.Error(err)
	}

	if e, a := 20, len(obj); e != a {
		t.Errorf("should be %d vm classes: %d", e, a)
	}

	for _, obj := range obj {
		if obj.ConfigSpec == nil {
			t.Errorf("VM Class %s has an empty ConfigSpec", obj.ID)
		}

		if obj.ID == "e2etest-vmclass-config-gpuddpio" {
			if e, a := 1, len(obj.ConfigSpec.ExtraConfig); e != a {
				t.Errorf("should be %d extra config options: %d", e, a)
			}
		}
	}
}

const getVMClassJSON = `{
    "devices": null,
    "instance_storage": null,
    "description": "",
    "config_status": "READY",
    "cpu_count": 2,
    "cpu_reservation": 100,
    "config_spec": {
        "extraConfig": [
            {
                "_typeName": "OptionValue",
                "value": {
                    "_typeName": "string",
                    "_value": "hello-test-value"
                },
                "key": "hello-test-key"
            }
        ],
        "_typeName": "VirtualMachineConfigSpec",
        "deviceChange": [
            {
                "_typeName": "VirtualDeviceConfigSpec",
                "device": {
                    "backing": {
                        "vgpu": "mockup-vmiop",
                        "_typeName": "VirtualPCIPassthroughVmiopBackingInfo"
                    },
                    "_typeName": "VirtualPCIPassthrough",
                    "key": -20000
                },
                "operation": "add"
            },
            {
                "_typeName": "VirtualDeviceConfigSpec",
                "device": {
                    "backing": {
                        "_typeName": "VirtualPCIPassthroughDynamicBackingInfo",
                        "customLabel": "SampleLabel2",
                        "deviceName": "",
                        "allowedDevice": [
                            {
                                "_typeName": "VirtualPCIPassthroughAllowedDevice",
                                "vendorId": 52,
                                "deviceId": 53
                            }
                        ]
                    },
                    "_typeName": "VirtualPCIPassthrough",
                    "key": -30000
                },
                "operation": "add"
            }
        ]
    },
    "memory_MB": 4096,
    "messages": [],
    "id": "e2etest-vmclass-config-gpuddpio",
    "memory_reservation": 100,
    "vms": [],
    "namespaces": []
}`

const listVMClassesJSON = `[
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 8,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 65536,
            "numCPUs": 8,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 65536,
        "messages": [],
        "id": "best-effort-2xlarge",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 32,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 131072,
            "numCPUs": 32,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 131072,
        "messages": [],
        "id": "guaranteed-8xlarge",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 64,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 64,
        "messages": [],
        "id": "test-vm-class-1674599315",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 8192,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 8192,
        "messages": [],
        "id": "best-effort-medium",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 4,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 16384,
            "numCPUs": 4,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 16384,
        "messages": [],
        "id": "best-effort-large",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 4,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 16384,
            "numCPUs": 4,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 16384,
        "messages": [],
        "id": "guaranteed-large",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 4,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 32768,
            "numCPUs": 4,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 32768,
        "messages": [],
        "id": "best-effort-xlarge",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 64,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 64,
        "messages": [],
        "id": "test-vm-class-1674599917",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 16,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 131072,
            "numCPUs": 16,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 131072,
        "messages": [],
        "id": "guaranteed-4xlarge",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 32,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 131072,
            "numCPUs": 32,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 131072,
        "messages": [],
        "id": "best-effort-8xlarge",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "_typeName": "VirtualMachineConfigSpec",
            "deviceChange": [
                {
                    "_typeName": "VirtualDeviceConfigSpec",
                    "device": {
                        "_typeName": "VirtualE1000",
                        "key": -10000
                    },
                    "operation": "add"
                }
            ]
        },
        "memory_MB": 4096,
        "messages": [],
        "id": "e2etest-vmclass-config-e1000",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 2048,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 2048,
        "messages": [],
        "id": "guaranteed-xsmall",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 8192,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 8192,
        "messages": [],
        "id": "guaranteed-medium",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 4,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 32768,
            "numCPUs": 4,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 32768,
        "messages": [],
        "id": "guaranteed-xlarge",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 8,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 65536,
            "numCPUs": 8,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 65536,
        "messages": [],
        "id": "guaranteed-2xlarge",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": 100,
        "config_spec": {
            "memoryMB": 4096,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 4096,
        "messages": [],
        "id": "guaranteed-small",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 16,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 131072,
            "numCPUs": 16,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 131072,
        "messages": [],
        "id": "best-effort-4xlarge",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": 100,
        "config_spec": {
            "extraConfig": [
                {
                    "_typeName": "OptionValue",
                    "value": {
                        "_typeName": "string",
                        "_value": "hello-test-value"
                    },
                    "key": "hello-test-key"
                }
            ],
            "_typeName": "VirtualMachineConfigSpec",
            "deviceChange": [
                {
                    "_typeName": "VirtualDeviceConfigSpec",
                    "device": {
                        "backing": {
                            "vgpu": "mockup-vmiop",
                            "_typeName": "VirtualPCIPassthroughVmiopBackingInfo"
                        },
                        "_typeName": "VirtualPCIPassthrough",
                        "key": -20000
                    },
                    "operation": "add"
                },
                {
                    "_typeName": "VirtualDeviceConfigSpec",
                    "device": {
                        "backing": {
                            "_typeName": "VirtualPCIPassthroughDynamicBackingInfo",
                            "customLabel": "SampleLabel2",
                            "deviceName": "",
                            "allowedDevice": [
                                {
                                    "_typeName": "VirtualPCIPassthroughAllowedDevice",
                                    "vendorId": 52,
                                    "deviceId": 53
                                }
                            ]
                        },
                        "_typeName": "VirtualPCIPassthrough",
                        "key": -30000
                    },
                    "operation": "add"
                }
            ]
        },
        "memory_MB": 4096,
        "messages": [],
        "id": "e2etest-vmclass-config-gpuddpio",
        "memory_reservation": 100,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 2048,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 2048,
        "messages": [],
        "id": "best-effort-xsmall",
        "memory_reservation": null,
        "vms": [],
        "namespaces": []
    },
    {
        "devices": null,
        "instance_storage": null,
        "description": "",
        "config_status": "READY",
        "cpu_count": 2,
        "cpu_reservation": null,
        "config_spec": {
            "memoryMB": 4096,
            "numCPUs": 2,
            "_typeName": "VirtualMachineConfigSpec"
        },
        "memory_MB": 4096,
        "messages": [],
        "id": "best-effort-small",
        "memory_reservation": null,
        "vms": [
            "vm-9w0uy3\/vm-nbpp",
            "vm-w999eu\/vm-ep0y",
            "vm-qqizem\/vm-6arf",
            "vm-ppdgsb\/vm-5oz9",
            "vmsvc-e2e-vmb7sp\/vmsvc-e2e-vmsvc-e2e-vmb7sp-p6t-255r6-2vjdw",
            "vmsvc-e2e-vmb7sp\/vmsvc-e2e-vmsvc-e2e-vmb7sp-p6t-worker-tx5gf-79f5f7f65c-dfjx7"
        ],
        "namespaces": [
            "vm-ppdgsb",
            "vm-qqizem",
            "packer-fjdbg5",
            "vmsvc-e2e-vmb7sp",
            "vm-9w0uy3",
            "vm-w999eu"
        ]
    }
]`
