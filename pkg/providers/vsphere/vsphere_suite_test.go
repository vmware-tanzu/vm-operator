// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("CPUFreq", cpuFreqTests)
	Describe("SyncVirtualMachineImage", syncVirtualMachineImageTests)
	Describe("ResourcePolicyTests", resourcePolicyTests)
	Describe("VirtualMachine", vmTests)
	Describe("VirtualMachineE2E", vmE2ETests)
	Describe("VirtualMachineResize", vmResizeTests)
	Describe("VirtualMachineUtilsTest", vmUtilTests)
}

func TestVSphereProvider(t *testing.T) {
	suite.Register(t, "VMProvider Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

func createOrUpdateVM(
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		if !pkgcfg.FromContext(ctx).AsyncSignalDisabled {
			if !pkgcfg.FromContext(ctx).AsyncCreateDisabled {
				By("non-blocking createOrUpdateVM")
				return createOrUpdateVMAsync(ctx, provider, vm)
			}
		}
	}

	By("blocking createOrUpdateVM")
	return provider.CreateOrUpdateVirtualMachine(ctx, vm)
}

func createOrUpdateAndGetVcVM(
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) (*object.VirtualMachine, error) {

	if err := createOrUpdateVM(ctx, provider, vm); err != nil {
		return nil, err
	}

	ExpectWithOffset(1, vm.Status.UniqueID).ToNot(BeEmpty())
	vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
	ExpectWithOffset(1, vcVM).ToNot(BeNil())
	return vcVM, nil
}

func createOrUpdateVMAsync(
	testCtx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	// This ensures there is no current operation set on the context.
	ctx := ctxop.WithContext(testCtx)

	chanErr, err := provider.CreateOrUpdateVirtualMachineAsync(ctx, vm)
	if err != nil {
		return err
	}

	// Unlike the VM controller, this test helper blocks until the async
	// parts of CreateOrUpdateVM are complete. This is to avoid a large
	// refactor for now.
	for err2 := range chanErr {
		if err2 != nil {
			if err == nil {
				err = err2
			} else {
				err = fmt.Errorf("%w,%w", err, err2)
			}
		}
	}
	if err != nil {
		return err
	}

	if ctxop.IsCreate(ctx) {
		// The async create operation does not fall-through to the
		// update logic, so we need to call CreateOrUpdateVirtualMachine
		// a second time to cause the update.
		ExpectWithOffset(1, testCtx.Client.Get(
			ctx,
			client.ObjectKeyFromObject(vm),
			vm)).To(Succeed())

		if _, err := provider.CreateOrUpdateVirtualMachineAsync(
			ctx,
			vm); err != nil {

			return err
		}
	}

	return nil
}
