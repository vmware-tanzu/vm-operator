// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var suite = builder.NewTestSuite()

func vcSimTests() {
	Describe("CPUFreq", cpuFreqTests)
	Describe("ResourcePolicyTests", resourcePolicyTests)
	Describe("VirtualMachine", vmTests)
	Describe("VirtualMachineE2E", vmE2ETests)
	Describe("VirtualMachineResize", vmResizeTests)
	Describe("VirtualMachineUtilsTest", vmUtilTests)
	Describe("VirtualMachineGroup", vmGroupTests)
	Describe("VirtualMachineSnapshot", vmSnapshotTests)
	Describe("VirtualMachineUnmanagedVolumes", unmanagedVolumesTests)
}

func TestVSphereProvider(t *testing.T) {
	suite.Register(t, "VMProvider Tests", nil, vcSimTests)
}

var _ = BeforeSuite(suite.BeforeSuite)

var _ = AfterSuite(suite.AfterSuite)

const (
	createOrUpdateVMMaxAllowedCallCount = 100
)

func createOrUpdateVM(
	testCtx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	var fn func(ctx context.Context) error

	if pkgcfg.FromContext(testCtx).AsyncSignalEnabled &&
		pkgcfg.FromContext(testCtx).AsyncCreateEnabled {

		By("non-blocking createOrUpdateVM")
		fn = func(ctx context.Context) error {
			return createOrUpdateVMAsync(testCtx, provider, vm)
		}
	} else {
		By("blocking createOrUpdateVM")
		fn = func(ctx context.Context) error {
			return provider.CreateOrUpdateVirtualMachine(ctx, vm)
		}
	}

	var (
		totalCallCount    = 0
		nonErrorCallCount = 0
	)

	for {
		var (
			err    error
			repeat bool
			opctx  = ctxop.WithContext(testCtx)
		)

		err = fn(opctx)

		if ctxop.IsUpdate(opctx) {
			ctxop.MarkUpdate(testCtx)
		}

		if err != nil {
			switch {
			case errors.Is(err, vsphere.ErrCreate),
				errors.Is(err, vsphere.ErrBackup),
				errors.Is(err, vsphere.ErrBootstrapCustomize),
				errors.Is(err, vsphere.ErrBootstrapReconfigure),
				errors.Is(err, vsphere.ErrReconfigure),
				errors.Is(err, vsphere.ErrRestart),
				errors.Is(err, vsphere.ErrSetPowerState),
				errors.Is(err, vsphere.ErrUpgradeHardwareVersion),
				errors.Is(err, vsphere.ErrPromoteDisks),
				errors.Is(err, vsphere.ErrSnapshotRevert),
				errors.Is(err, vsphere.ErrPolicyNotReady),
				errors.Is(err, vsphere.ErrUpgradeSchema):

				repeat = true
			default:
				GinkgoLogr.Error(err, "createOrUpdateVM fail")
				return err
			}
		}

		if totalCallCount > 100 {
			ExpectWithOffset(1, totalCallCount).To(
				BeNumerically("<", createOrUpdateVMMaxAllowedCallCount),
				"cannot exceed createOrUpdateVMMaxAllowedCallCount for tests")
		}

		totalCallCount++

		if !repeat {
			nonErrorCallCount++
		}

		if nonErrorCallCount == 2 {
			GinkgoLogr.Info(
				"createOrUpdateVM success",
				"totalCalls", totalCallCount)
			return nil
		}

		GinkgoLogr.Info(
			"createOrUpdateVM repeat",
			"totalCalls", totalCallCount,
			"err", err)
	}
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
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	GinkgoLogr.Info("entered createOrUpdateVMAsync")

	chanErr, err := provider.CreateOrUpdateVirtualMachineAsync(ctx, vm)
	if err != nil {
		if errors.Is(err, vsphere.ErrUpgradeSchema) {
			ExpectWithOffset(1, ctx.Client.Update(
				ctx,
				vm)).To(Succeed())
		}
		GinkgoLogr.Info("createOrUpdateVMAsync returned", "err", err)
		return err
	}

	if chanErr != nil {
		// Unlike the VM controller, this test helper blocks until the async
		// parts of CreateOrUpdateVM are complete. This is to avoid a large
		// refactor for now.
		for err2 := range chanErr {
			if err2 != nil {
				GinkgoLogr.Info("createOrUpdateVMAsync chanErr", "err", err2)
				if err == nil {
					err = err2
				} else {
					err = fmt.Errorf("%w,%w", err, err2)
				}
			}
		}
	}

	if errors.Is(err, vsphere.ErrCreate) {
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			client.ObjectKeyFromObject(vm),
			vm)).To(Succeed())
	}

	GinkgoLogr.Info("createOrUpdateVMAsync returned post channel", "err", err)
	return err
}
