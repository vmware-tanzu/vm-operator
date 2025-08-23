// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func vmSnapshotTests() {
	var (
		initObjects   []ctrlclient.Object
		ctx           *builder.TestContextForVCSim
		vmProvider    providers.VirtualMachineProviderInterface
		nsInfo        builder.WorkloadNamespaceInfo
		vmSnapshot    *vmopv1.VirtualMachineSnapshot
		vcVM          *object.VirtualMachine
		vm            *vmopv1.VirtualMachine
		vmCtx         pkgctx.VirtualMachineContext
		deleted       bool
		err           error
		dummySnapshot = "dummy-snapshot"
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{}, initObjects...)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		By("Creating VM")
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
		Expect(vcVM).ToNot(BeNil())
		vm = builder.DummyBasicVirtualMachine(dummySnapshot, nsInfo.Namespace)
		vm.Status.UniqueID = vcVM.Reference().Value
		vmSnapshot = builder.DummyVirtualMachineSnapshot(nsInfo.Namespace, dummySnapshot, vcVM.Name())
		Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

		// TODO (lubron): Add FCD to the VM and test the snapshot size once
		// vcsim has support to show attached disk as device

		By("Creating snapshot")
		logger := testutil.GinkgoLogr(5)
		vmCtx = pkgctx.VirtualMachineContext{
			Context: logr.NewContext(ctx, logger),
			Logger:  logger.WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}
		args := virtualmachine.SnapshotArgs{
			VMCtx:      vmCtx,
			VMSnapshot: *vmSnapshot,
			VcVM:       vcVM,
		}
		snapMo, err := virtualmachine.CreateSnapshot(args)
		Expect(err).To(BeNil())
		Expect(snapMo).ToNot(BeNil())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		vmSnapshot = nil
		vmCtx = pkgctx.VirtualMachineContext{}
		vm = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
		deleted = false
	})

	Context("GetSnapshotSize", func() {
		It("should return the size of the snapshot", func() {
			size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
			Expect(err).ToNot(HaveOccurred())
			// since we only have one snapshot, the size should be same as the vm
			var moVM mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot", "layoutEx", "config.hardware.device"}, &moVM)).To(Succeed())

			var sum int64
			for _, file := range moVM.LayoutEx.File {
				if strings.HasSuffix(file.Name, ".vmdk") || strings.HasSuffix(file.Name, ".vmsn") || strings.HasSuffix(file.Name, ".vmem") {
					sum += file.Size
				}
			}
			Expect(size).To(Equal(sum))
		})

		When("there is issue finding vm", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = ""
			})
			It("should return error", func() {
				size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
				Expect(err).To(HaveOccurred())
				Expect(size).To(BeZero())
			})
		})

		When("there is issue finding snapshot", func() {
			BeforeEach(func() {
				vmSnapshot.Name = ""
			})
			It("should return error", func() {
				size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
				Expect(err).To(HaveOccurred())
				Expect(size).To(BeZero())
			})
		})
	})

	Context("DeleteSnapshot", func() {
		JustBeforeEach(func() {
			deleted, err = vmProvider.DeleteSnapshot(ctx, vmSnapshot, vm, true, nil)
		})

		It("should return false and no error", func() {
			Expect(deleted).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
			snapMoRef, err := vcVM.FindSnapshot(ctx, dummySnapshot)
			Expect(err).To(HaveOccurred())
			Expect(snapMoRef).To(BeNil())
		})

		Context("VM is not found", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = ""
			})
			It("should return true and no error", func() {
				Expect(deleted).To(BeTrue())
				Expect(err).NotTo(HaveOccurred())
				snapMoRef, err := vcVM.FindSnapshot(ctx, dummySnapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapMoRef).NotTo(BeNil())
			})
		})

		Context("snapshot not found", func() {
			BeforeEach(func() {
				By("Deleting snapshot in advance")
				Expect(virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: *vmSnapshot,
					VcVM:       vcVM,
				})).To(Succeed())
			})
			It("should return false and no error", func() {
				Expect(deleted).To(BeFalse())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("SyncVMSnapshotTreeStatus", func() {
		It("should sync the VM's current and root snapshots status", func() {
			Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).To(Succeed())
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))
			Expect(vm.Status.RootSnapshots).To(HaveLen(1))
			Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))
		})

		When("VM is not found", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = ""
			})
			It("should return error", func() {
				Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).NotTo(Succeed())
			})
		})

		When("there is no snapshot", func() {
			BeforeEach(func() {
				Expect(virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: *vmSnapshot,
					VcVM:       vcVM,
				})).To(Succeed())
			})
			It("should show expected current snapshot and root snapshots", func() {
				Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).To(Succeed())
				Expect(vm.Status.CurrentSnapshot).To(BeNil())
				Expect(vm.Status.RootSnapshots).To(BeNil())
			})
		})
	})
}
