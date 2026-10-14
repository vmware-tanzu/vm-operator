// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"errors"
	"testing"

	"github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// BenchmarkCreateOrUpdateVirtualMachineUpdatePath reports allocations for the
// steady-state VM update reconciliation path. Run it with:
//
//	go test ./pkg/providers/vsphere -run '^$' -bench '^BenchmarkCreateOrUpdateVirtualMachineUpdatePath$' -benchmem
func BenchmarkCreateOrUpdateVirtualMachineUpdatePath(b *testing.B) {
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		b.Helper()
		b.Fatal(message)
	})

	parentCtx := newVMTestParentContext()
	vmClass, vm := newVMTestObjects("allocation-benchmark-vm")
	ctx, vmProvider, _ := setupVMTest(
		parentCtx,
		newVMTestConfig(),
		vmClass,
		vm)
	b.Cleanup(ctx.AfterEach)

	// Settle create and initial status work before measuring steady-state
	// updates. This follows the retry behavior used by the vmprovider_vm tests.
	benchmarkReconcileUntilSettled(b, ctx, vmProvider, vm)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		opCtx := ctxop.WithContext(ctx)
		if err := vmProvider.CreateOrUpdateVirtualMachine(opCtx, vm); err != nil {
			b.Fatalf("failed to reconcile VM update: %v", err)
		}
		if !ctxop.IsUpdate(opCtx) {
			b.Fatal("benchmark reconciliation did not take the VM update path")
		}
		ctxop.MarkUpdate(ctx)
	}
}

func benchmarkReconcileUntilSettled(
	b *testing.B,
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) {
	b.Helper()

	var consecutiveNonErrorCallCount int
	for range createOrUpdateVMMaxAllowedCallCount {
		opCtx := ctxop.WithContext(ctx)
		err := provider.CreateOrUpdateVirtualMachine(opCtx, vm)
		if ctxop.IsUpdate(opCtx) {
			ctxop.MarkUpdate(ctx)
		}

		if err == nil {
			consecutiveNonErrorCallCount++
			if consecutiveNonErrorCallCount == 2 {
				return
			}
			continue
		}

		consecutiveNonErrorCallCount = 0
		if !benchmarkReconcileErrorCanRetry(err) {
			b.Fatalf("failed to prepare VM update benchmark: %v", err)
		}
	}

	b.Fatalf("VM did not settle after %d calls", createOrUpdateVMMaxAllowedCallCount)
}

func benchmarkReconcileErrorCanRetry(err error) bool {
	return errors.Is(err, vsphere.ErrCreate) ||
		errors.Is(err, vsphere.ErrBackup) ||
		errors.Is(err, vsphere.ErrBootstrapCustomize) ||
		errors.Is(err, vsphere.ErrBootstrapReconfigure) ||
		errors.Is(err, vsphere.ErrReconfigure) ||
		errors.Is(err, vsphere.ErrRestart) ||
		errors.Is(err, vsphere.ErrSetPowerState) ||
		errors.Is(err, vsphere.ErrUpgradeHardwareVersion) ||
		errors.Is(err, vsphere.ErrPromoteDisks) ||
		errors.Is(err, vsphere.ErrSnapshotRevert) ||
		errors.Is(err, vsphere.ErrPolicyNotReady) ||
		errors.Is(err, vsphere.ErrUpgradeSchema) ||
		errors.Is(err, vsphere.ErrUpgradeObject)
}
