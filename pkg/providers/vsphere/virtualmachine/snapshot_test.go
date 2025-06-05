// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func snapShotTests() {
	var (
		ctx        *builder.TestContextForVCSim
		vcVM       *object.VirtualMachine
		vmCtx      pkgctx.VirtualMachineContext
		vmSnapshot vmopv1.VirtualMachineSnapshot
		testConfig builder.VCSimTestConfig
		err        error
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
	})

	JustBeforeEach(func() {
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		vm := builder.DummyVirtualMachine()
		timeout, err := time.ParseDuration("1h35m")
		Expect(err).To(BeNil())
		vmSnapshot = vmopv1.VirtualMachineSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snap-1",
				Namespace: vm.Namespace,
			},
			Spec: vmopv1.VirtualMachineSnapshotSpec{
				VMRef: &vmopv1common.LocalObjectRef{
					APIVersion: vm.APIVersion,
					Kind:       vm.Kind,
					Name:       vm.Name,
				},
				Quiesce: &vmopv1.QuiesceSpec{
					Timeout: &metav1.Duration{Duration: timeout},
				},
				Memory:      true,
				Description: "This is a dummy-snap",
			},
		}

		vm.Spec.CurrentSnapshot = &vmopv1common.LocalObjectRef{
			APIVersion: vmSnapshot.APIVersion,
			Kind:       vmSnapshot.Kind,
			Name:       vmSnapshot.Name,
		}

		logger := testutil.GinkgoLogr(5)
		vmCtx = pkgctx.VirtualMachineContext{
			Context: logr.NewContext(ctx, logger),
			Logger:  logger.WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}

		err = vcVM.Properties(vmCtx, vcVM.Reference(), vsphere.VMUpdatePropertiesSelector, &vmCtx.MoVM)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vcVM = nil
	})

	Context("SnapshotVirtualMachine", func() {
		It("succeeds", func() {
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}

			snapMo, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo).ToNot(BeNil())
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       vmSnapshot.Name,
			}))

			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))

			// retry the same snapshot again, no-op (ie) no child snapshot created.
			snapMoDup, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo).ToNot(BeNil())
			Expect(snapMo).To(Equal(snapMoDup))
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       vmSnapshot.Name,
			}))

			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			/// should point to the same one.
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMoDup.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))
			// zero child snapshots
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList).To(HaveLen(0))

			// Create a new snapshot with a different name, child snapshot created.
			args.VMSnapshot.Name = "snap-2"
			snapMo2, err := virtualmachine.SnapshotVirtualMachine(args)
			Expect(err).ToNot(HaveOccurred())
			Expect(snapMo2).ToNot(BeNil())
			Expect(vmCtx.VM.Status.CurrentSnapshot).To(Equal(&vmopv1common.LocalObjectRef{
				APIVersion: vmSnapshot.APIVersion,
				Kind:       vmSnapshot.Kind,
				Name:       args.VMSnapshot.Name,
			}))

			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo2.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal("snap-1"))
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].ChildSnapshotList[0].Name).To(Equal(args.VMSnapshot.Name))
		})
	})

	Context("CreateSnapshot", func() {
		It("succeeds", func() {
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: vmSnapshot,
				VcVM:       vcVM,
			}

			snapMo, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo).ToNot(BeNil())
			moVM := mo.VirtualMachine{}
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
			Expect(moVM.Snapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())
			Expect(moVM.Snapshot.CurrentSnapshot.Value).To(Equal(snapMo.Value))
			Expect(moVM.Snapshot.RootSnapshotList).To(HaveLen(1))
			Expect(moVM.Snapshot.RootSnapshotList[0].Name).To(Equal("snap-1"))
		})
	})
}
