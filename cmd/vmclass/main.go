// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vmware/govmomi/vim25/json"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// configSpec is the VirtualMachineConfigSpec that is emitted to output. Update
// this object to represent the desired version of the ConfigSpec.
var configSpec = vimtypes.VirtualMachineConfigSpec{
	NumCPUs:  2,
	MemoryMB: 4000,
	DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device: &vimtypes.VirtualPCIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: -100,
					Backing: &vimtypes.VirtualPCIPassthroughDvxBackingInfo{
						DeviceClass: "com.intel.gaudi3",
					},
				},
			},
		},
	},
}

func main() {
	flag.Parse()

	switch out {
	case "json":
		handleOutputFormatJSON()
	case "dcli":
		if name == "" {
			flag.Usage()
			os.Exit(1)
		}
		handleOutputFormatDCLI()
	default:
		flag.Usage()
		os.Exit(1)
	}
}

var (
	out  string
	name string
)

func init() {
	flag.StringVar(
		&out,
		"out",
		"",
		"The output format of the command. Valid values include: json, dcli.",
	)

	flag.StringVar(
		&name,
		"name",
		"",
		"The name of the VM Class to create. Required when -out=dcli.",
	)
}

func handleOutputFormatJSON() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetDiscriminator(
		"_typeName",
		"_value",
		json.DiscriminatorEncodeTypeNameRootValue|
			json.DiscriminatorEncodeTypeNameAllObjects,
	)
	enc.SetTypeToDiscriminatorFunc(vimtypes.VmomiTypeName)

	if err := enc.Encode(configSpec); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func handleOutputFormatDCLI() {
	var w bytes.Buffer
	enc := vimtypes.NewJSONEncoder(&w)
	if err := enc.Encode(configSpec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	vmClassJSON := strings.TrimSuffix(w.String(), "\n")

	_, _ = fmt.Fprint(os.Stdout, "dcli com vmware vcenter namespacemanagement ")
	_, _ = fmt.Fprintf(os.Stdout, "virtualmachineclasses create --id %q ", name)
	_, _ = fmt.Fprintf(os.Stdout, "--cpu-count %d ", configSpec.NumCPUs)
	_, _ = fmt.Fprintf(os.Stdout, "--memory-mb %d ", configSpec.MemoryMB)
	_, _ = fmt.Fprintf(os.Stdout, "--config-spec %q", vmClassJSON)
	_, _ = fmt.Fprintln(os.Stdout)
}
