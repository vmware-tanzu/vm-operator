// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func TestVMContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM Context Suite")
}

var _ = Describe("VM Context Helper Functions", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("HasVMRunningSnapshotTask", func() {
		It("should return false when no tasks are present", func() {
			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeFalse())
		})

		It("should return false when no snapshot tasks are present", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.powerOn",
					State:         vimtypes.TaskInfoStateRunning,
				},
				{
					DescriptionId: "VirtualMachine.reconfigure",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeFalse())
		})

		It("should return true when running createSnapshot task is present", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.createSnapshot",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeTrue())
		})

		It("should return true when running removeSnapshot task is present", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.removeSnapshot",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeTrue())
		})

		It("should return true when running revertToSnapshot task is present", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.revertToSnapshot",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeTrue())
		})

		It("should return false when snapshot tasks are completed", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.createSnapshot",
					State:         vimtypes.TaskInfoStateSuccess,
				},
				{
					DescriptionId: "VirtualMachine.revertToSnapshot",
					State:         vimtypes.TaskInfoStateError,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeFalse())
		})

		It("should return true when at least one snapshot task is running", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.createSnapshot",
					State:         vimtypes.TaskInfoStateSuccess,
				},
				{
					DescriptionId: "VirtualMachine.removeSnapshot",
					State:         vimtypes.TaskInfoStateRunning,
				},
				{
					DescriptionId: "VirtualMachine.powerOn",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningSnapshotTask(ctx)
			Expect(result).To(BeTrue())
		})
	})

	Describe("HasVMRunningTask", func() {
		It("should return false when no tasks are present", func() {
			result := pkgctx.HasVMRunningTask(ctx, false)
			Expect(result).To(BeFalse())
		})

		It("should return true when running task is present", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.powerOn",
					State:         vimtypes.TaskInfoStateRunning,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningTask(ctx, false)
			Expect(result).To(BeTrue())
		})

		It("should return false when all tasks are completed", func() {
			tasks := []vimtypes.TaskInfo{
				{
					DescriptionId: "VirtualMachine.powerOn",
					State:         vimtypes.TaskInfoStateSuccess,
				},
				{
					DescriptionId: "VirtualMachine.reconfigure",
					State:         vimtypes.TaskInfoStateError,
				},
			}
			ctx = pkgctx.WithVMRecentTasks(ctx, tasks)

			result := pkgctx.HasVMRunningTask(ctx, false)
			Expect(result).To(BeFalse())
		})
	})
})
