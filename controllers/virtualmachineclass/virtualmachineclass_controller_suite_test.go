// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineclass_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/manager"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuiteForControllerWithContext(
	pkgcfg.NewContextWithDefaultConfig(),
	virtualmachineclass.AddToManager,
	manager.InitializeProvidersNoopFn)

func TestVirtualMachineClass(t *testing.T) {
	suite.Register(t, "VirtualMachineClass controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

// setConfigSpec populates the spec.configSpec field as wcpsvc would when creating a VM Class.
func setConfigSpec(vmClass *vmopv1.VirtualMachineClass) {
	spec := types.VirtualMachineConfigSpec{
		NumCPUs:  int32(vmClass.Spec.Hardware.Cpus),
		MemoryMB: vmClass.Spec.Hardware.Memory.Value(),
	}

	var err error
	vmClass.Spec.ConfigSpec, err = pkgutil.MarshalConfigSpecToJSON(spec)
	if err != nil {
		panic(err)
	}
}
