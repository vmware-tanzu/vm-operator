// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateDelete,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vmSnapshot    *vmopv1.VirtualMachineSnapshot
	oldVMSnapshot *vmopv1.VirtualMachineSnapshot
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmSnapshot := builder.DummyVirtualMachineSnapshot("dummy-ns", "dummy-vm-snapshot", "dummy-vm")
	obj, err := builder.ToUnstructured(vmSnapshot)
	Expect(err).ToNot(HaveOccurred())

	var oldVMSnapshot *vmopv1.VirtualMachineSnapshot
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMSnapshot = vmSnapshot.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMSnapshot)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmSnapshot:                          vmSnapshot,
		oldVMSnapshot:                       oldVMSnapshot,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
		err error
	)

	type createArgs struct {
		emptyVMName           bool
		mismatchedVMNameLabel bool
		createVKSNode         bool
		hardware              *vmopv1.VirtualMachineHardwareSpec
		volumes               []vmopv1.VirtualMachineVolume
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.emptyVMName {
			ctx.vmSnapshot.Spec.VMName = ""
		}

		if args.mismatchedVMNameLabel {
			metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, "some-other-vm")
		}

		// Create a VM with CAPI labels to simulate a VKS/TKG node
		if args.hardware != nil || args.createVKSNode || args.volumes != nil {
			vm := builder.DummyBasicVirtualMachine(ctx.vmSnapshot.Spec.VMName, ctx.vmSnapshot.Namespace)
			if args.createVKSNode {
				vm.Labels = map[string]string{
					kubeutil.CAPWClusterRoleLabelKey: "worker",
				}
			}
			vm.Spec.Hardware = args.hardware
			vm.Spec.Volumes = args.volumes
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmSnapshot)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	vmNameField := field.NewPath("spec", "vmName")

	DescribeTable("Create VMSnapshot", validateCreate,
		Entry("should allow valid",
			createArgs{},
			true,
			nil,
			nil,
		),
		Entry("should deny empty vmName",
			createArgs{emptyVMName: true},
			false,
			field.Required(vmNameField, "vmName must be provided").Error(),
			nil,
		),
		Entry("should deny VMNameForSnapshotLabel that does not match spec.vmName",
			createArgs{mismatchedVMNameLabel: true},
			false,
			field.Invalid(
				field.NewPath("metadata", "labels").Key(vmopv1.VMNameForSnapshotLabel),
				"some-other-vm",
				`must match spec.vmName "dummy-vm"`).Error(),
			nil,
		),
		Entry("should deny snapshot for VKS/TKG node",
			createArgs{createVKSNode: true},
			false,
			field.Forbidden(vmNameField, "snapshots are not allowed for VKS/TKG nodes").Error(),
			nil,
		),

		Entry("should deny snapshot if controller has physical sharing mode",
			createArgs{
				hardware: &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					},
				},
			},
			false,
			field.NotSupported(vmNameField, "controller type SCSIControllers bus 0 is using unsupported sharingMode for snapshot: Physical", []string{string(vmopv1.VirtualControllerSharingModeNone)}).Error(),
			nil,
		),
		Entry("should deny snapshot if controller has virtual sharing mode",
			createArgs{
				hardware: &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModeVirtual,
						},
					},
				},
			},
			false,
			field.NotSupported(vmNameField, "controller type SCSIControllers bus 0 is using unsupported sharingMode for snapshot: Virtual", []string{string(vmopv1.VirtualControllerSharingModeNone)}).Error(),
			nil,
		),
		Entry("should allow snapshot if controller has none sharing mode",
			createArgs{
				hardware: &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				},
			},
			true,
			nil,
			nil,
		),
		Entry("should deny snapshot if a volume has MultiWriter sharing mode",
			createArgs{
				volumes: []vmopv1.VirtualMachineVolume{
					{
						Name:        "test-volume",
						SharingMode: vmopv1.VolumeSharingModeMultiWriter,
					},
				},
			},
			false,
			field.NotSupported(vmNameField, "volume test-volume is using unsupported sharingMode for snapshot: MultiWriter", []string{string(vmopv1.VolumeSharingModeNone)}).Error(),
			nil,
		),
		Entry("should allow snapshot if a volume has none sharing mode",
			createArgs{
				volumes: []vmopv1.VirtualMachineVolume{
					{
						Name:        "test-volume",
						SharingMode: vmopv1.VolumeSharingModeNone,
					},
				},
			},
			true,
			nil,
			nil,
		),
		Entry("should allow snapshot if an independent-mode volume (e.g. OracleRAC) has MultiWriter sharing mode",
			createArgs{
				volumes: []vmopv1.VirtualMachineVolume{
					{
						Name:            "test-volume",
						ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
						SharingMode:     vmopv1.VolumeSharingModeMultiWriter,
						DiskMode:        vmopv1.VolumeDiskModeIndependentPersistent,
					},
				},
			},
			true,
			nil,
			nil,
		),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx     *unitValidatingWebhookContext
		dummyVM = "dummy-vm"
	)

	type updateArgs struct {
		updateMemory      bool
		updateQuiesce     bool
		updateVMRef       bool
		updateVMNameLabel bool
		oldLabelMissing   bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		ctx.vmSnapshot.Spec.Description = "a new description"

		if args.updateMemory {
			ctx.vmSnapshot.Spec.Memory = !ctx.vmSnapshot.Spec.Memory
		}

		if args.updateQuiesce {
			ctx.vmSnapshot.Spec.Quiesce = &vmopv1.QuiesceSpec{Timeout: &metav1.Duration{Duration: 120 * time.Second}}
		}

		if args.updateVMRef {
			ctx.vmSnapshot.Spec.VMName = "another-vm"
		}

		if args.oldLabelMissing {
			delete(ctx.oldVMSnapshot.Labels, vmopv1.VMNameForSnapshotLabel)
		}

		if args.updateVMNameLabel {
			// Try to change the VM name label to a different value
			metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, "different-vm-name")
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmSnapshot)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMSnapshot)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)

		// Set up both snapshots with the VM name label set by mutation webhook
		metav1.SetMetaDataLabel(&ctx.oldVMSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, dummyVM)

		// New snapshot should also have the label by default (simulating no change)
		metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, dummyVM)
	})

	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("update table", validateUpdate,
		Entry("should allow updating description", updateArgs{}, true, "", nil),
		Entry("should not allow updating memory",
			updateArgs{updateMemory: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating quiesce",
			updateArgs{updateQuiesce: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating vmRef",
			updateArgs{updateVMRef: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating VM name label to a value that disagrees with spec.vmName",
			updateArgs{updateVMNameLabel: true},
			false,
			`must match spec.vmName "dummy-vm"`,
			nil,
		),
		Entry("should allow backfilling the VM name label when the old object was missing it",
			updateArgs{oldLabelMissing: true},
			true,
			"",
			nil,
		),
		Entry("should not allow backfilling the VM name label to a value that disagrees with spec.vmName",
			updateArgs{oldLabelMissing: true, updateVMNameLabel: true},
			false,
			`must match spec.vmName "dummy-vm"`,
			nil,
		),
	)
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("the delete is performed", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
