// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/anno2extraconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	fake                           = "fake"
	updateSuffix                   = "-updated"
	dummyInstanceIDVal             = "dummy-instance-id"
	dummyFirstBootDoneVal          = "dummy-first-boot-done"
	dummyCreatedAtBuildVersionVal  = "dummy-created-at-build-version"
	dummyCreatedAtSchemaVersionVal = "dummy-created-at-schema-version"
	dummyRegisteredAnnVal          = "dummy-registered-annotation"
	dummyImportedAnnVal            = "dummy-imported-annotation"
	dummyFailedOverAnnVal          = "dummy-failedover-annotation"
	dummyPausedVMLabelVal          = "dummy-devops"
	dummyVmiName                   = "vmi-dummy"
	dummyNamespaceName             = "dummy-vm-namespace-for-webhook-validation"
	dummyClusterModuleAnnVal       = "dummy-cluster-module"
	dummyGroupName                 = "dummy-group"
	dummyPVCName                   = "dummy-pvc"
	vmiKind                        = "VirtualMachineImage"
	cvmiKind                       = "Cluster" + vmiKind
	invalidKind                    = "InvalidKind"
	newVMClass                     = "new-class"
	oldVMClass                     = "old-class"
	invalidImageKindMsg            = "supported: " + vmiKind + "; " + cvmiKind

	invalidClassInstanceReference              = "must specify a valid reference to a VirtualMachineClassInstance object"
	invalidClassInstanceReferenceNotActive     = "must specify a reference to a VirtualMachineClassInstance object that is active"
	invalidClassInstanceReferenceOwnerMismatch = "VirtualMachineClassInstance must be an instance of the VM Class specified by spec.class"
)

type testParams struct {
	setup                   func(ctx *unitValidatingWebhookContext)
	validate                func(response admission.Response)
	expectAllowed           bool
	skipBypassUpgradeCheck  bool
	skipSetControllerForPVC bool
}

type protectedAnnotationTestCase struct {
	annotationKey string
	oldValue      string
	newValue      string
}

// When defining any new protected annotations, be sure to add them to this
// list so they are covered via test cases. A protected condition is one whose
// key matches the regex `^.+\.protected(/.+)?$`.
//
// Examples that match:
//   - fu.bar.protected
//   - hello.world.protected/sub-key
//   - vmoperator.vmware.com.protected/reconcile-priority
//
// Examples that do NOT match:
//   - protected.fu.bar
//   - hello.world.protected.against/sub-key
var protectedAnnotationTestCases = []protectedAnnotationTestCase{
	{
		annotationKey: pkgconst.ReconcilePriorityAnnotationKey,
		oldValue:      "100",
		newValue:      "200",
	},
	{
		annotationKey: pkgconst.SkipDeletePlatformResourceKey,
		oldValue:      "true",
		newValue:      "false",
	},
	{
		annotationKey: pkgconst.ApplyPowerStateTimeAnnotation,
		oldValue:      time.Now().Format(time.RFC3339Nano),
		newValue:      time.Now().Add(time.Hour).Format(time.RFC3339Nano),
	},
	{
		annotationKey: "hello.world.protected/condition-status",
		oldValue:      "red",
		newValue:      "green",
	},
	{
		annotationKey: "condition.vmware.vmoperator.com.protected/hello-world",
		oldValue:      "True",
		newValue:      "False",
	},
}

func bypassUpgradeCheck(ctx *context.Context, objects ...metav1.Object) {
	pkgcfg.SetContext(*ctx, func(config *pkgcfg.Config) {
		config.BuildVersion = fake
	})

	for _, obj := range objects {
		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(map[string]string{})
		}
		a := obj.GetAnnotations()
		a[pkgconst.UpgradedToBuildVersionAnnotationKey] = fake
		a[pkgconst.UpgradedToSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
		obj.SetAnnotations(a)
	}
}

func doValidateWithMsg(msgs ...string) func(admission.Response) {
	return func(response admission.Response) {
		reasons := string(response.Result.Reason)
		for _, m := range msgs {
			ExpectWithOffset(1, reasons).To(ContainSubstring(m))
		}
	}
}

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
	controllerTests()
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
	vm, oldVM *vmopv1.VirtualMachine
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm"
	vm.Namespace = dummyNamespaceName

	var (
		oldVM  *vmopv1.VirtualMachine
		oldObj *unstructured.Unstructured
		err    error
	)

	if isUpdate {
		setControllerForPVC(vm)
		oldVM = vm.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
	}

	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	az := builder.DummyAvailabilityZone()
	zone := builder.DummyZone(dummyNamespaceName)
	initObjects := []client.Object{az, zone}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, initObjects...),
		vm:                                  vm,
		oldVM:                               oldVM,
	}
}

// setControllerForPVC sets controllerBusNumber and controllerType
// on all PVC volumes to simulate the mutation webhook having run. This is
// needed because this is required by the validation.
func setControllerForPVC(vm *vmopv1.VirtualMachine) {
	setControllerForPVCWithBusNumber(vm, 0)
}

func setControllerForPVCWithBusNumber(vm *vmopv1.VirtualMachine, defaultBusNum int32) {
	hasPVCVolumes := false
	busNumbersUsed := make(map[int32]bool)

	for i := range vm.Spec.Volumes {
		if vm.Spec.Volumes[i].PersistentVolumeClaim != nil {
			hasPVCVolumes = true
			if vm.Spec.Volumes[i].ControllerType == "" {
				vm.Spec.Volumes[i].ControllerType = vmopv1.VirtualControllerTypeSCSI
			}
			if vm.Spec.Volumes[i].ControllerBusNumber == nil {
				// Use the specified default bus number to avoid creating multiple controllers.
				vm.Spec.Volumes[i].ControllerBusNumber = ptr.To(defaultBusNum)
			}
			if vm.Spec.Volumes[i].UnitNumber == nil {
				vm.Spec.Volumes[i].UnitNumber = ptr.To(int32(i))
			}

			busNumbersUsed[*vm.Spec.Volumes[i].ControllerBusNumber] = true
		}
	}

	// If we have PVC volumes and no SCSI controllers, add controllers for all bus numbers used
	// to simulate what the mutation webhook would do.
	if hasPVCVolumes {
		if vm.Spec.Hardware == nil {
			vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
		}
		if len(vm.Spec.Hardware.SCSIControllers) == 0 {
			// Create a controller for each bus number that's referenced by volumes.
			for busNum := range busNumbersUsed {
				vm.Spec.Hardware.SCSIControllers = append(
					vm.Spec.Hardware.SCSIControllers,
					vmopv1.SCSIControllerSpec{
						BusNumber:   busNum,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				)
			}
		}

		// Set controllerBusNumber on volumes that don't have it set.
		// Use the first available SCSI controller's bus number.
		var defaultBusNumber *int32
		if len(vm.Spec.Hardware.SCSIControllers) > 0 {
			defaultBusNumber = ptr.To(vm.Spec.Hardware.SCSIControllers[0].BusNumber)
		}

		for i := range vm.Spec.Volumes {
			if vm.Spec.Volumes[i].PersistentVolumeClaim != nil {
				if vm.Spec.Volumes[i].ControllerBusNumber == nil && defaultBusNumber != nil {
					vm.Spec.Volumes[i].ControllerBusNumber = defaultBusNumber
				}
			}
		}
	}
}

func unitTestsValidateCreate() {

	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		isServiceUser              bool
		invalidVolumeName          bool
		dupVolumeName              bool
		invalidVolumeSource        bool
		invalidPVCName             bool
		invalidPVCReadOnly         bool
		withInstanceStorageVolumes bool
		powerState                 vmopv1.VirtualMachinePowerState
		nextRestartTime            string
		instanceUUID               string
		applyPowerStateChangeTime  string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}

		if args.invalidVolumeName {
			ctx.vm.Spec.Volumes[0].Name = "underscore_not_valid"
		}
		if args.dupVolumeName {
			ctx.vm.Spec.Volumes[0].Name = "duplicate-name"
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, ctx.vm.Spec.Volumes[0])
		}
		if args.invalidVolumeSource {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = nil
		}
		if args.invalidPVCName {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = ""
		}
		if args.invalidPVCReadOnly {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
		}

		if args.withInstanceStorageVolumes {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}

		if args.applyPowerStateChangeTime != "" {
			ctx.vm.Annotations[pkgconst.ApplyPowerStateTimeAnnotation] = args.applyPowerStateChangeTime
		}

		ctx.vm.Spec.PowerState = args.powerState
		ctx.vm.Spec.NextRestartTime = args.nextRestartTime
		ctx.vm.Spec.InstanceUUID = args.instanceUUID

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
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
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.WorkloadDomainIsolation = true
			config.Features.VMSharedDisks = true
		})
	})

	AfterEach(func() {
		ctx = nil
	})

	specPath := field.NewPath("spec")
	volPath := specPath.Child("volumes")
	nextRestartTimePath := specPath.Child("nextRestartTime")
	now := time.Now().UTC()

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),

		Entry("should deny invalid volume name", createArgs{invalidVolumeName: true}, false,
			field.Invalid(volPath.Index(0).Child("name"), "underscore_not_valid", validation.IsDNS1123Subdomain("underscore_not_valid")[0]).Error(), nil),
		Entry("should deny duplicated volume names", createArgs{dupVolumeName: true}, false,
			field.Duplicate(volPath.Index(1).Child("name"), "duplicate-name").Error(), nil),
		// TODO(akutz) Why did we ever consider a spec.volumes entry sans a PVC
		//             to be invalid?
		// Entry("should deny invalid volume source spec", createArgs{invalidVolumeSource: true}, false,
		// 	field.Required(volPath.Index(0).Child("persistentVolumeClaim"), "").Error(), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false,
			field.Required(volPath.Index(0).Child("persistentVolumeClaim", "claimName"), "").Error(), nil),
		Entry("should deny invalid PVC read only", createArgs{invalidPVCReadOnly: true}, false,
			field.NotSupported(volPath.Index(0).Child("persistentVolumeClaim", "readOnly"), true, []string{"false"}).Error(), nil),
		Entry("should deny when there are instance storage volumes and user is SSO user", createArgs{withInstanceStorageVolumes: true}, false,
			field.Forbidden(volPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow when there are instance storage volumes and user is service user", createArgs{isServiceUser: true, withInstanceStorageVolumes: true}, true, nil, nil),

		Entry("should disallow creating VM with suspended power state", createArgs{powerState: vmopv1.VirtualMachinePowerStateSuspended}, false,
			field.Invalid(specPath.Child("powerState"), vmopv1.VirtualMachinePowerStateSuspended, "cannot set a new VM's power state to Suspended").Error(), nil),

		Entry("should allow creating VM with empty nextRestartTime value", createArgs{}, true, nil, nil),
		Entry("should disallow creating VM with non-empty, valid nextRestartTime value", createArgs{
			nextRestartTime: now.Format(time.RFC3339Nano)}, false,
			field.Invalid(nextRestartTimePath, now.Format(time.RFC3339Nano), "cannot restart VM on create").Error(), nil),
		Entry("should disallow creating VM with non-empty, valid nextRestartTime value if mutation webhooks were running",
			createArgs{nextRestartTime: "now"}, false,
			field.Invalid(nextRestartTimePath, "now", "cannot restart VM on create").Error(), nil),
		Entry("should disallow creating VM with non-empty, invalid nextRestartTime value",
			createArgs{nextRestartTime: "hello"}, false,
			field.Invalid(nextRestartTimePath, "hello", "cannot restart VM on create").Error(), nil),
		Entry("should allow creating VM with instanceUUID set by admin user", createArgs{instanceUUID: "uuid", isServiceUser: true}, true, nil, nil),
		Entry("should allow creating VM with valid apply power state change time annotation by admin user",
			createArgs{applyPowerStateChangeTime: time.Now().Format(time.RFC3339Nano), isServiceUser: true}, true, nil, nil),
		Entry("should disallow creating VM with non-empty, invalid apply power state change time annotation",
			createArgs{applyPowerStateChangeTime: "hello", isServiceUser: true}, false,
			field.Invalid(field.NewPath("metadata").Child("annotations").Key(pkgconst.ApplyPowerStateTimeAnnotation), "hello", "must be formatted as RFC3339Nano").Error(), nil),
	)

	doTest := func(args testParams) {
		args.setup(ctx)

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		ExpectWithOffset(1, response.Allowed).To(Equal(args.expectAllowed))

		if args.validate != nil {
			args.validate(response)
		}
	}

	Context("PVC Volume Controller Fields", func() {
		DescribeTable("create", doTest,
			Entry("should deny PVC volume with only controllerType set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = nil
						ctx.vm.Spec.Volumes[0].UnitNumber = nil
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(0).Child("controllerBusNumber"), "").Error(),
					),
				},
			),
			Entry("should deny PVC volume with only controllerBusNumber set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Volumes[0].ControllerType = ""
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To(int32(0))
						ctx.vm.Spec.Volumes[0].UnitNumber = nil
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(0).Child("controllerType"), "").Error(),
					),
				},
			),
			Entry("should deny PVC volume with only unitNumber set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Volumes[0].ControllerType = ""
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = nil
						ctx.vm.Spec.Volumes[0].UnitNumber = ptr.To(int32(0))
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(0).Child("controllerType"), "").Error(),
						field.Required(volPath.Index(0).Child("controllerBusNumber"), "").Error(),
					),
				},
			),
			Entry("should deny PVC volume with controllerType and unitNumber but no controllerBusNumber",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = nil
						ctx.vm.Spec.Volumes[0].UnitNumber = ptr.To(int32(0))
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(0).Child("controllerBusNumber"), "").Error(),
					),
				},
			),
			Entry("should deny PVC volume with controllerBusNumber and unitNumber but no controllerType",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Volumes[0].ControllerType = ""
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To(int32(0))
						ctx.vm.Spec.Volumes[0].UnitNumber = ptr.To(int32(0))
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(0).Child("controllerType"), "").Error(),
					),
				},
			),
		)
	})

	Context("availability zone and zone", func() {
		DescribeTable("create", doTest,
			Entry("should allow when VM specifies no availability zone, there are availability zones and zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						delete(ctx.vm.Labels, corev1.LabelTopologyZone)
					},
					expectAllowed: true,
				},
			),
			Entry("should allow when VM specifies no availability zone, there are no availability zones or zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						delete(ctx.vm.Labels, corev1.LabelTopologyZone)
						Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: true,
				},
			),
			Entry("should allow when VM specifies valid availability zone, there are availability zones and zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
					},
					expectAllowed: true,
				},
			),
			Entry("when WorkloadDomainIsolation capability disabled and VKS node, should allow when VM specifies valid availability zone, there are availability zones but no zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.WorkloadDomainIsolation = false
						})
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[vmopv1util.KubernetesNodeLabelKey] = ""
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: true,
				},
			),
			Entry("when WorkloadDomainIsolation capability enabled and VKS node, should deny when VM specifies valid availability zone, there are availability zones but no zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[vmopv1util.KubernetesNodeLabelKey] = ""
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: false,
				},
			),
			Entry("when WorkloadDomainIsolation capability enabled, should deny when VM specifies valid availability zone, there are availability zones but no zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: false,
				},
			),
			Entry("when WorkloadDomainIsolation capability enabled, should deny when VM created by SSO user that specifies a zone being deleted",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.WorkloadDomainIsolation = true
						})
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						zone := &topologyv1.Zone{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: zoneName, Namespace: dummyNamespaceName}, zone)).To(Succeed())
						zone.Finalizers = []string{"test"}
						Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
						Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					},
					expectAllowed: false,
				},
			),
			Entry("when WorkloadDomainIsolation capability enabled, should allow when VM created by admin that specifies a zone being deleted",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.WorkloadDomainIsolation = true
							ctx.IsPrivilegedAccount = true
						})
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						zone := &topologyv1.Zone{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: zoneName, Namespace: dummyNamespaceName}, zone)).To(Succeed())
						zone.Finalizers = []string{"test"}
						Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
						Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					},
					expectAllowed: true,
				},
			),
			Entry("when WorkloadDomainIsolation capability enabled, should allow when VM created by CAPV that specifies a zone being deleted",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.WorkloadDomainIsolation = true
							ctx.UserInfo.Username = "system:serviceaccount:svc-tkg-domain-c52:default"
						})
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						zone := &topologyv1.Zone{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: zoneName, Namespace: dummyNamespaceName}, zone)).To(Succeed())
						zone.Finalizers = []string{"test"}
						Expect(ctx.Client.Update(ctx, zone)).To(Succeed())
						Expect(ctx.Client.Delete(ctx, zone)).To(Succeed())
					},
					expectAllowed: true,
				},
			),
			Entry("should deny when VM specifies valid availability zone, there are no availability zones or zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						zoneName := builder.DummyZoneName
						ctx.vm.Labels[corev1.LabelTopologyZone] = zoneName
						Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: false,
				},
			),
			Entry("should deny when VM specifies invalid availability zone, there are availability zones and zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels[corev1.LabelTopologyZone] = "invalid"
					},
					expectAllowed: false,
				},
			),
			Entry("should deny when VM specifies invalid availability zone, there are no availability zones or zones",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels[corev1.LabelTopologyZone] = "invalid"
						Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
						Expect(ctx.Client.Delete(ctx, builder.DummyZone(dummyNamespaceName))).To(Succeed())
					},
					expectAllowed: false,
				},
			),
		)
	})

	DescribeTable(
		"spec.className",
		doTest,

		//
		// FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled
		//
		Entry("require spec.className for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),
		Entry("require spec.className for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),

		//
		// FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled
		//
		Entry("allow empty spec.className for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("forbid empty spec.className for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Forbidden(field.NewPath("spec", "className"), "restricted to privileged users").Error(),
				),
			},
		),
	)

	DescribeTable(
		"spec.class and spec.className",
		doTest,

		Entry("should return error if class instance does not exist",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReference).Error(),
				),
			},
		),
		Entry("should error out if instance is not active",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance with the correct OnwerRef that points to the correct VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReferenceNotActive).Error(),
				),
			},
		),
		Entry("should return error if instance points to a different class than spec.className",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance without an OnwerRef that points to some other VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: "random-vm-class",
								},
							},
							// Set the label to mark the instance as active
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReferenceOwnerMismatch).Error(),
				),
			},
		),
		Entry("should succeed if instance is valid, and is active",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance with the correct OnwerRef that points to the correct VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: "vmoperator.vmware.com/v1alpha5",
									Name:       newVMClass,
									Kind:       "VirtualMachineClass",
								},
							},
							// Set the label to mark the instance as active
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				expectAllowed: true,
			},
		),
	)

	DescribeTable(
		"spec.image",
		doTest,

		//
		// FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled
		//
		Entry("require spec.image for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image"), "").Error(),
				),
			},
		),
		Entry("require spec.image for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image"), "").Error(),
				),
			},
		),
		Entry("require spec.image.kind for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require spec.image.kind for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),

		// FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled

		Entry("allow empty spec.image for privileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled and VM contains restored annotation",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					ctx.vm.Annotations = map[string]string{
						vmopv1.RestoredVMAnnotation: "",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("allow empty spec.image for privileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("require spec.image.kind for privileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("forbid empty spec.image for unprivileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Forbidden(field.NewPath("spec", "image"), "restricted to privileged users").Error(),
				),
			},
		),
		Entry("forbid empty spec.image for unprivileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled and annotation is present",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					ctx.vm.Annotations = map[string]string{
						vmopv1.RestoredVMAnnotation: "",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Forbidden(field.NewPath("spec", "image"), "restricted to privileged users").Error(),
				),
			},
		),
		Entry("require spec.image.kind for unprivileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for privileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for unprivileged user when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMIncrementalRestore = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),

		//
		// FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled
		//
		Entry("allow empty spec.image for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled and annotation is present",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					ctx.vm.Annotations = map[string]string{
						vmopv1.ImportedVMAnnotation: "",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("(To be removed) allow empty spec.image for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled, but the annotation is not present",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("require spec.image.kind for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("forbid empty spec.image for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Forbidden(field.NewPath("spec", "image"), "restricted to privileged users").Error(),
				),
			},
		),
		Entry("forbid empty spec.image for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled and annotation is present",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = nil
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					ctx.vm.Annotations = map[string]string{
						vmopv1.ImportedVMAnnotation: "",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Forbidden(field.NewPath("spec", "image"), "restricted to privileged users").Error(),
				),
			},
		),
		Entry("require spec.image.kind for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),
		Entry("require valid spec.image.kind for unprivileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
						Kind: "invalid",
					}
					ctx.vm.Spec.ImageName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKindMsg).Error(),
				),
			},
		),

		//
		// FSS_WCP_VMSERVICE_BYOK is disabled
		//
		Entry("disallow spec.crypto when FSS_WCP_VMSERVICE_BYOK is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = false
					})
				},
				validate: func(response admission.Response) {
					Expect(string(response.Result.Reason)).To(Equal(field.Invalid(
						field.NewPath("spec", "crypto"),
						&vmopv1.VirtualMachineCryptoSpec{},
						"the Bring Your Own Key (Provider) feature is not enabled").Error()))
				},
			},
		),

		//
		// FSS_WCP_VMSERVICE_BYOK is enabled
		//
		Entry("allow spec.crypto.encryptionClassName when FSS_WCP_VMSERVICE_BYOK is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					storageClass1 := builder.DummyStorageClass()
					Expect(ctx.Client.Create(ctx, storageClass1)).To(Succeed())

					rlName := storageClass1.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
					resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
					Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())

					ctx.vm.Spec.StorageClass = storageClass1.Name
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
						EncryptionClassName: fake,
					}
					ctx.vm.Spec.Volumes = nil

					var storageClass storagev1.StorageClass
					Expect(ctx.Client.Get(
						ctx,
						client.ObjectKey{Name: ctx.vm.Spec.StorageClass},
						&storageClass)).To(Succeed())
					Expect(kubeutil.MarkEncryptedStorageClass(
						ctx,
						ctx.Client,
						storageClass,
						true)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("disallow spec.crypto.encryptionClassName for non-encryption storage class when FSS_WCP_VMSERVICE_BYOK is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					storageClass1 := builder.DummyStorageClass()
					Expect(ctx.Client.Create(ctx, storageClass1)).To(Succeed())

					rlName := storageClass1.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
					resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
					Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())

					ctx.vm.Spec.StorageClass = storageClass1.Name
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
						EncryptionClassName: fake,
					}
					ctx.vm.Spec.Volumes = nil

					var storageClass storagev1.StorageClass
					Expect(ctx.Client.Get(
						ctx,
						client.ObjectKey{Name: ctx.vm.Spec.StorageClass},
						&storageClass)).To(Succeed())
					Expect(kubeutil.MarkEncryptedStorageClass(
						ctx,
						ctx.Client,
						storageClass,
						false)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = true
					})
				},
				validate: doValidateWithMsg(
					`spec.crypto.encryptionClassName: Invalid value: "fake": requires spec.storageClass specify an encryption storage class`),
			},
		),
		Entry("allow volume when spec.crypto.encryptionClassName is non-empty when FSS_WCP_VMSERVICE_BYOK is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					storageClass1 := builder.DummyStorageClass()
					Expect(ctx.Client.Create(ctx, storageClass1)).To(Succeed())

					storageClass2 := builder.DummyStorageClass()
					storageClass2.Name += "2"
					Expect(ctx.Client.Create(ctx, storageClass2)).To(Succeed())

					resourceQuota := builder.DummyResourceQuota(
						ctx.vm.Namespace,
						storageClass1.Name+".storageclass.storage.k8s.io/persistentvolumeclaims",
						storageClass2.Name+".storageclass.storage.k8s.io/persistentvolumeclaims")
					Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())

					pvc := builder.DummyPersistentVolumeClaim()
					pvc.Name = builder.DummyPVCName
					pvc.Namespace = ctx.vm.Namespace
					pvc.Spec.StorageClassName = ptr.To(storageClass2.Name)
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					ctx.vm.Spec.StorageClass = storageClass1.Name
					ctx.vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{
						EncryptionClassName: fake,
					}

					var storageClass storagev1.StorageClass
					Expect(ctx.Client.Get(
						ctx,
						client.ObjectKey{Name: ctx.vm.Spec.StorageClass},
						&storageClass)).To(Succeed())
					Expect(kubeutil.MarkEncryptedStorageClass(
						ctx,
						ctx.Client,
						storageClass,
						true)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = true
					})
				},
				expectAllowed: true,
			},
		),
	)

	Context("Annotations", func() {
		annotationPath := field.NewPath("metadata", "annotations")

		DescribeTable("create", doTest,
			Entry("should disallow creating VM with admin-only annotations set by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Key(vmopv1.RestoredVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.ImportedVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FailedOverVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyAllowListAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyWatermarkAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
					),
				},
			),
			Entry("should allow creating VM with admin-only annotations set by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					expectAllowed: true,
				},
			),
			Entry("should allow creating VM with admin-only annotations set by WCP user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						fakeWCPUser := "sso:wcp-12345-fake-machineid-67890@vsphere.local"
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.PrivilegedUsers = fakeWCPUser
						})

						ctx.UserInfo.Username = fakeWCPUser
						ctx.IsPrivilegedAccount = pkgbuilder.IsPrivilegedAccount(ctx.WebhookContext, ctx.UserInfo)

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					expectAllowed: true,
				},
			),
			Entry("should allow creating VM with cluster module",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Annotations[pkgconst.ClusterModuleNameAnnotationKey] = dummyClusterModuleAnnVal
						ctx.vm.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{
							ResourcePolicyName: "resource-policy",
						}
					},
					expectAllowed: true,
				},
			),
			Entry("should disallow creating VM with cluster module without resource policy",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Annotations[pkgconst.ClusterModuleNameAnnotationKey] = dummyClusterModuleAnnVal
					},
					validate: doValidateWithMsg(
						`metadata.annotations[vsphere-cluster-module-group]: Forbidden: cluster module assignment requires spec.reserved.resourcePolicyName to specify a VirtualMachineSetResourcePolicy`),
				},
			),
		)

		getProtectedAnnotationTableAllowCreate := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.IsPrivilegedAccount = true
							ctx.vm.Annotations[tc.annotationKey] = tc.newValue
						},
						expectAllowed: true,
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should allow create with "+tc.annotationKey, tc))
			}

			return table
		}

		getProtectedAnnotationTableDisallowCreate := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.vm.Annotations[tc.annotationKey] = tc.newValue
						},
						validate: doValidateWithMsg(
							field.Forbidden(annotationPath.Key(tc.annotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						),
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should disallow create with "+tc.annotationKey, tc))
			}

			return table
		}

		DescribeTable("disallow create with protected annotations by non-privileged user",
			getProtectedAnnotationTableDisallowCreate()...,
		)

		DescribeTable("allow create with protected annotations by non-privileged user",
			getProtectedAnnotationTableAllowCreate()...,
		)
	})

	Context("Label", func() {
		labelPath := field.NewPath("metadata", "labels")

		DescribeTable("create", doTest,
			Entry("should disallow creating VM with admin-only labels set by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
					},
					validate: doValidateWithMsg(
						field.Forbidden(labelPath.Child(vmopv1.PausedVMLabelKey), "modifying this label is not allowed for non-admin users").Error()),
				},
			),
			Entry("should allow creating VM with admin-only label set by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Readiness Probe", func() {

		DescribeTable("create", doTest,
			Entry("should fail when Readiness probe has multiple actions #2",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						cm := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      config.ProviderConfigMapName,
								Namespace: ctx.Namespace,
							},
							Data: make(map[string]string),
						}
						Expect(ctx.Client.Create(ctx, cm)).To(Succeed())

						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket:      &vmopv1.TCPSocketAction{},
							GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
						}
					},
					validate: doValidateWithMsg(
						`spec.readinessProbe: Forbidden: only one action can be specified`),
				},
			),
			Entry("should fail when Readiness probe has multiple actions #2",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							GuestInfo: []vmopv1.GuestInfoAction{
								{
									Key: "my-key",
								},
							},
							GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
						}
					},
					validate: doValidateWithMsg(
						`spec.readinessProbe: Forbidden: only one action can be specified`),
				},
			),
			Entry("should deny when TCP readiness probe is specified under VPC networking",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket: &vmopv1.TCPSocketAction{},
						}
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
						})
					},
					validate: doValidateWithMsg(
						`spec.readinessProbe.tcpSocket: Forbidden: VPC networking doesn't allow TCP readiness probe to be specified`),
				},
			),
			Entry("should allow when non-TCP readiness probe is specified under VPC networking",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
						}
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.NetworkProviderType = pkgcfg.NetworkProviderTypeVPC
						})
					},
					expectAllowed: true,
				},
			),
			Entry("should deny when restricted network and TCP port in readiness probe is not 6443",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						cm := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      config.ProviderConfigMapName,
								Namespace: ctx.Namespace,
							},
							Data: make(map[string]string),
						}
						cm.Data["IsRestrictedNetwork"] = "true"

						Expect(ctx.Client.Create(ctx, cm)).To(Succeed())

						portValue := 443
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket: &vmopv1.TCPSocketAction{Port: intstr.FromInt(portValue)},
						}

					},
					validate: doValidateWithMsg(
						`spec.readinessProbe.tcpSocket.port: Unsupported value: 443: supported values: "6443"`),
				},
			),
			Entry("should allow when restricted network and TCP port in readiness probe is 6443",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						cm := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      config.ProviderConfigMapName,
								Namespace: ctx.Namespace,
							},
							Data: make(map[string]string),
						}
						cm.Data["IsRestrictedNetwork"] = "true"

						Expect(ctx.Client.Create(ctx, cm)).To(Succeed())

						portValue := 6443
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket: &vmopv1.TCPSocketAction{Port: intstr.FromInt(portValue)},
						}

					},
					expectAllowed: true,
				},
			),
			Entry("should allow when not restricted network and TCP port in readiness probe is not 6443",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						cm := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      config.ProviderConfigMapName,
								Namespace: ctx.Namespace,
							},
							Data: make(map[string]string),
						}

						Expect(ctx.Client.Create(ctx, cm)).To(Succeed())

						portValue := 443
						ctx.vm.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket: &vmopv1.TCPSocketAction{Port: intstr.FromInt(portValue)},
						}

					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("StorageClass", func() {

		DescribeTable("StorageClass create", doTest,
			Entry("storage class not found",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.StorageClass = builder.DummyStorageClassName
					},
					validate: doValidateWithMsg(
						`spec.storageClass: Invalid value: "dummy-storage-class": Storage policy dummy-storage-class does not exist`),
				},
			),
			Entry("storage class not associated with namespace",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						storageClass := builder.DummyStorageClass()
						Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
						ctx.vm.Spec.StorageClass = storageClass.Name

						rlName := "not-found" + ".storageclass.storage.k8s.io/persistentvolumeclaims"
						resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
						Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
					},
					validate: doValidateWithMsg(
						`spec.storageClass: Invalid value: "dummy-storage-class": Storage policy is not associated with the namespace dummy-vm-namespace-for-webhook-validation`),
				},
			),
			Entry("storage class associated with namespace",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						storageClass := builder.DummyStorageClass()
						Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
						ctx.vm.Spec.StorageClass = storageClass.Name

						rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
						resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
						Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
					},
					expectAllowed: true,
				},
			),
		)

		Context("PodVMOnStretchedSupervisor is enabled", func() {

			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.PodVMOnStretchedSupervisor = true
				})
			})

			DescribeTable("StorageClass create", doTest,
				Entry("storage class associated with namespace",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							storageClass := builder.DummyStorageClassWithID("my-policy-id")
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								storageClass.Name+"-storagepolicyquota", ctx.vm.Namespace, "my-policy-id")
							Expect(ctx.Client.Create(ctx, storagePolicyQuota)).To(Succeed())
						},
						expectAllowed: true,
					},
				),
				Entry("storage class not associated with namespace",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							storageClass := builder.DummyStorageClassWithID("my-policy-id")
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								storageClass.Name+"-storagepolicyquota", ctx.vm.Namespace, "some-other-id")
							Expect(ctx.Client.Create(ctx, storagePolicyQuota)).To(Succeed())
						},
						validate: doValidateWithMsg(
							`spec.storageClass: Invalid value: "dummy-storage-class": Storage policy is not associated with the namespace dummy-vm-namespace-for-webhook-validation`),
					},
				),
				Entry("WFFC storage class associated with namespace",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							storageClass := builder.DummyStorageClassWithID("my-policy-id")
							baseSCName := storageClass.Name
							storageClass.Name += "wffc"
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								baseSCName+"-storagepolicyquota", ctx.vm.Namespace, "my-policy-id")
							Expect(ctx.Client.Create(ctx, storagePolicyQuota)).To(Succeed())
						},
						expectAllowed: true,
					},
				),
			)
		})
	})

	Context("Bootstrap", func() {

		DescribeTable("bootstrap create", doTest,
			Entry("allow CloudInit bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("allow LinuxPrep bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("allow vAppConfig bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("allow Sysprep bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("disallow empty Sysprep bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep: Invalid value: "sysPrep": either sysprep or rawSysprep must be provided`,
					),
				},
			),
			Entry("disallow CloudInit and LinuxPrep specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.cloudInit: Forbidden: CloudInit may not be used with any other bootstrap provider`,
						`spec.bootstrap.linuxPrep: Forbidden: LinuxPrep may not be used with either CloudInit or Sysprep bootstrap providers`),
				},
			),
			Entry("disallow CloudInit and Sysprep specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.cloudInit: Forbidden: CloudInit may not be used with any other bootstrap provider`,
						`spec.bootstrap.sysprep: Forbidden: Sysprep may not be used with either CloudInit or LinuxPrep bootstrap providers`,
					),
				},
			),
			Entry("disallow CloudInit and vAppConfig specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit:  &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.cloudInit: Forbidden: CloudInit may not be used with any other bootstrap provider`,
						`spec.bootstrap.vAppConfig: Forbidden: vAppConfig may not be used in conjunction with CloudInit bootstrap provider`,
					),
				},
			),
			Entry("disallow LinuxPrep and Sysprep specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.linuxPrep: Forbidden: LinuxPrep may not be used with either CloudInit or Sysprep bootstrap providers`,
						`spec.bootstrap.sysprep: Forbidden: Sysprep may not be used with either CloudInit or LinuxPrep bootstrap providers`,
					),
				},
			),
			Entry("allow LinuxPrep and vAppConfig specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep:  &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("allow Sysprep and vAppConfig specified at the same time",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("disallow CloudInit mixing inline CloudConfig and RawCloudConfig",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								CloudConfig:    &cloudinit.CloudConfig{},
								RawCloudConfig: &common.SecretKeySelector{},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.cloudInit: Invalid value: "cloudInit": cloudConfig and rawCloudConfig are mutually exclusive`,
					),
				},
			),
			Entry("disallow Sysprep mixing inline Sysprep and RawSysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep:    &sysprep.Sysprep{},
								RawSysprep: &common.SecretKeySelector{},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep: Invalid value: "sysPrep": sysprep and rawSysprep are mutually exclusive`,
					),
				},
			),
			Entry("disallow LinuxPrep mixing ScriptText Value From Secret and direct String pointer",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.GuestCustomizationVCDParity = true
						})

						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
								ScriptText: &common.ValueOrSecretKeySelector{
									From:  &common.SecretKeySelector{},
									Value: ptr.To("foo"),
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.linuxPrep.scriptText.value: Invalid value: "value": from and value are mutually exclusive`,
					),
				},
			),
			Entry("disallow LinuxPrep VCD parity fields when capability is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.GuestCustomizationVCDParity = false
						})

						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
								ExpirePasswordAfterNextLogin: true,
								Password:                     &common.PasswordSecretKeySelector{},
								ScriptText:                   &common.ValueOrSecretKeySelector{},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.linuxPrep.expirePasswordAfterNextLogin: Forbidden: VC guest customization VCD parity capability is not enabled`,
						`spec.bootstrap.linuxPrep.password: Forbidden: VC guest customization VCD parity capability is not enabled`,
						`spec.bootstrap.linuxPrep.scriptText: Forbidden: VC guest customization VCD parity capability is not enabled`,
					),
				},
			),

			Entry("disallow Sysprep mixing inline Sysprep identification",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "foo-domain",
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{
									Identification: &sysprep.Identification{
										JoinWorkgroup: "foo-wg",
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep.sysprep: Invalid value: "identification": spec.network.domainName and joinWorkgroup are mutually exclusive`,
						`spec.bootstrap.sysprep.sysprep: Invalid value: "identification": spec.network.domainName requires domainAdmin and domainAdminPassword selector to be set`,
					),
				},
			),
			Entry("disallow Sysprep mixing inline Sysprep identification",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{
									Identification: &sysprep.Identification{
										JoinWorkgroup: "foo-wg",
										DomainAdmin:   "admin@os.local",
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep.sysprep: Invalid value: "identification": joinWorkgroup and domainAdmin/domainAdminPassword/domainOU are mutually exclusive`,
					),
				},
			),
			Entry("disallow vAppConfig mixing inline Properties and RawProperties",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
								Properties: []common.KeyValueOrSecretKeySelectorPair{
									{
										Key: "key",
									},
								},
								RawProperties: "some-vapp-prop",
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.vAppConfig: Invalid value: "vAppConfig": properties and rawProperties are mutually exclusive`,
					),
				},
			),

			Entry("disallow vAppConfig mixing Properties Value From Secret and direct String pointer",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
								Properties: []common.KeyValueOrSecretKeySelectorPair{
									{
										Key: "key",
										Value: common.ValueOrSecretKeySelector{
											From: &common.SecretKeySelector{
												Name: "secret-name",
												Key:  "key",
											},
											Value: ptr.To("value"),
										},
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.vAppConfig.properties.value: Invalid value: "value": from and value are mutually exclusive`,
					),
				},
			),

			Entry("disallow vAppConfig inline Properties missing Key",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
								Properties: []common.KeyValueOrSecretKeySelectorPair{
									{
										Value: common.ValueOrSecretKeySelector{
											Value: ptr.To("value"),
										},
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.vAppConfig.properties.key: Invalid value: "key": key is a required field in vAppConfig Properties`,
					),
				},
			),

			Entry("disallow inline sysPrep autoLogon with missing autoLogonCount and password",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{
									GUIUnattended: &sysprep.GUIUnattended{
										AutoLogon: true,
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep.sysprep: Invalid value: "guiUnattended": autoLogon requires autoLogonCount to be specified`,
						`spec.bootstrap.sysprep.sysprep: Invalid value: "guiUnattended": autoLogon requires password selector to be set`,
					),
				},
			),

			Entry("disallow inline sysPrep ScriptText Value From Secret and direct String pointer",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.GuestCustomizationVCDParity = true
						})
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{
									ScriptText: &common.ValueOrSecretKeySelector{
										From:  &common.SecretKeySelector{},
										Value: ptr.To("foo"),
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep.sysprep.scriptText.value: Invalid value: "value": from and value are mutually exclusive`,
					),
				},
			),

			Entry("disallow Sysprep VCD parity fields when capability is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.GuestCustomizationVCDParity = false
						})

						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{
									ExpirePasswordAfterNextLogin: true,
									ScriptText:                   &common.ValueOrSecretKeySelector{},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.bootstrap.sysprep.expirePasswordAfterNextLogin: Forbidden: VC guest customization VCD parity capability is not enabled`,
						`spec.bootstrap.sysprep.scriptText: Forbidden: VC guest customization VCD parity capability is not enabled`),
				},
			),
		)
	})

	Context("Network", func() {

		DescribeTable("network create", doTest,
			Entry("allow default",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					},
					expectAllowed: true,
				},
			),

			Entry("allow disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Disabled: true,
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow global nameservers and search domains with LinuxPrep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Nameservers: []string{
								"8.8.8.8",
								"2001:4860:4860::8888",
							},
							SearchDomains: []string{
								"foo.bar",
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow global nameservers and search domains with Sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Nameservers: []string{
								"8.8.8.8",
								"2001:4860:4860::8888",
							},
							SearchDomains: []string{
								"dev.local",
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow global nameservers and search domains with CloudInit when UseGlobals...AsDefaults are nil",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								UseGlobalNameserversAsDefault:   nil,
								UseGlobalSearchDomainsAsDefault: nil,
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Nameservers: []string{
								"8.8.8.8",
								"2001:4860:4860::8888",
							},
							SearchDomains: []string{
								"dev.local",
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow global nameservers and search domains with CloudInit when UseGlobals...AsDefaults are true",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								UseGlobalNameserversAsDefault:   ptr.To(true),
								UseGlobalSearchDomainsAsDefault: ptr.To(true),
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Nameservers: []string{
								"8.8.8.8",
								"2001:4860:4860::8888",
							},
							SearchDomains: []string{
								"dev.local",
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow global nameservers and search domains with CloudInit when UseGlobals...AsDefaults are false",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								UseGlobalNameserversAsDefault:   ptr.To(false),
								UseGlobalSearchDomainsAsDefault: ptr.To(false),
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Nameservers: []string{
								"not-an-ip",
								"8.8.8.8",
								"2001:4860:4860::8888",
							},
							SearchDomains: []string{
								"dev.local",
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.nameservers: Invalid value: "not-an-ip,8.8.8.8,2001:4860:4860::8888": nameservers is available only for CloudInit when UseGlobalNameserversAsDefault is true`,
						`spec.network.searchDomains: Invalid value: "dev.local": searchDomains is available only for CloudInit when UseGlobalSearchDomainsAsDefault is true`,
						`spec.network.nameservers[0]: Invalid value: "not-an-ip": must be an IPv4 or IPv6 address`,
					),
				},
			),

			Entry("allow static",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Addresses: []string{
										"192.168.1.100/24",
										"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
									},
									DHCP4:    false,
									DHCP6:    false,
									Gateway4: "192.168.1.1",
									Gateway6: "2605:a601:a0ba:720:2ce6::1",
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow static with disabled gateways",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Addresses: []string{
										"192.168.1.100/24",
										"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
									},
									DHCP4:    false,
									DHCP6:    false,
									Gateway4: "None",
									Gateway6: "None",
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow guestDeviceName, static address, mtu, nameservers, routes and searchDomains when bootstrap is CloudInit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:            "eth0",
									GuestDeviceName: "mydev42",
									Addresses: []string{
										"192.168.1.100/24",
										"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
									},
									DHCP4:    false,
									DHCP6:    false,
									Gateway4: "192.168.1.1",
									Gateway6: "2605:a601:a0ba:720:2ce6::1",
									MTU:      ptr.To[int64](9000),
									Nameservers: []string{
										"8.8.8.8",
										"2001:4860:4860::8888",
									},
									Routes: []vmopv1.VirtualMachineNetworkRouteSpec{
										{
											To:     "10.100.10.1/24",
											Via:    "10.10.1.1",
											Metric: 42,
										},
										{
											To:  "fbd6:93e7:bc11:18b2:514f:2b1d:637a:f695/48",
											Via: "ef71:6ce2:3b91:8349:b2b2:f76c:86ae:915b",
										},
									},
									SearchDomains: []string{"dev.local"},
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallows guestDeviceName without CloudInit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:            "eth0",
									GuestDeviceName: "mydev",
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].guestDeviceName: Invalid value: "mydev": guestDeviceName is available only with the following bootstrap providers: CloudInit`,
					),
				},
			),

			Entry("allow dhcp",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:  "eth0",
									DHCP4: true,
									DHCP6: true,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow mixing static and dhcp",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Addresses: []string{
										"192.168.1.100/24",
										"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
									},
									DHCP4:    true,
									DHCP6:    true,
									Gateway4: "192.168.1.1",
									Gateway6: "2605:a601:a0ba:720:2ce6::1",
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].dhcp4: Invalid value: "192.168.1.100/24": dhcp4 cannot be used with IPv4 addresses in addresses field`,
						`spec.network.interfaces[0].gateway4: Invalid value: "192.168.1.1": gateway4 is mutually exclusive with dhcp4`,
						`spec.network.interfaces[0].dhcp6: Invalid value: "2605:a601:a0ba:720:2ce6:776d:8be4:2496/48": dhcp6 cannot be used with IPv6 addresses in addresses field`,
						`spec.network.interfaces[0].gateway6: Invalid value: "2605:a601:a0ba:720:2ce6::1": gateway6 is mutually exclusive with dhcp6`,
					),
				},
			),

			Entry("validate addresses",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Interfaces[0].Addresses = []string{
							"1.1.",
							"1.1.1.1",
							"not-an-ip",
							"7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].addresses[0]: Invalid value: "1.1.": invalid CIDR address: 1.1.`,
						`spec.network.interfaces[0].addresses[1]: Invalid value: "1.1.1.1": invalid CIDR address: 1.1.1.1`,
						`spec.network.interfaces[0].addresses[2]: Invalid value: "not-an-ip": invalid CIDR address: not-an-ip`,
						`spec.network.interfaces[0].addresses[3]: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": invalid CIDR address: 7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072`,
					),
				},
			),

			Entry("validate gateway4",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Interfaces[0].Gateway4 = "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072"
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].gateway4: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": gateway4 must have an IPv4 address in the addresses field`,
						`spec.network.interfaces[0].gateway4: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": must be a valid IPv4 address`,
					),
				},
			),

			Entry("validate gateway6",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Interfaces[0].Gateway6 = "192.168.1.1"
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].gateway6: Invalid value: "192.168.1.1": gateway6 must have an IPv6 address in the addresses field`,
						`spec.network.interfaces[0].gateway6: Invalid value: "192.168.1.1": must be a valid IPv6 address`,
					),
				},
			),

			// Please note mtu is available only with the following bootstrap providers: CloudInit
			Entry("validate mtu when bootstrap doesn't support mtu",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									MTU:  ptr.To[int64](9000),
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].mtu: Invalid value: 9000: mtu is available only with the following bootstrap providers: CloudInit`,
					),
				},
			),

			Entry("validate mtu when bootstrap supports mtu",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									MTU:  ptr.To[int64](9000),
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			// Please note nameservers is available only with the following bootstrap
			// providers: CloudInit and Sysprep.
			Entry("validate nameservers when bootstrap doesn't support nameservers",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].Nameservers = []string{
							"not-an-ip",
							"192.168.1.1/24",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].nameservers[0]: Invalid value: "not-an-ip": must be an IPv4 or IPv6 address`,
						`spec.network.interfaces[0].nameservers[1]: Invalid value: "192.168.1.1/24": must be an IPv4 or IPv6 address`,
						`spec.network.interfaces[0].nameservers: Invalid value: "not-an-ip,192.168.1.1/24": nameservers is available only with the following bootstrap providers: CloudInit and Sysprep`,
					),
				},
			),

			Entry("disallows nameservers vAppConfig",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].Nameservers = []string{
							"192.168.1.1",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].nameservers: Invalid value: "192.168.1.1": nameservers is available only with the following bootstrap providers: CloudInit and Sysprep`,
					),
				},
			),

			Entry("allows nameservers with Sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
						ctx.vm.Spec.Network.Interfaces[0].Nameservers = []string{
							"8.8.8.8",
							"2001:4860:4860::8888",
						}
					},
					expectAllowed: true,
				},
			),

			// Please note routes is available only with the following bootstrap providers: CloudInit
			Entry("validate routes when bootstrap doesn't support routes",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								RawSysprep: &common.SecretKeySelector{},
							},
						}
						ctx.vm.Spec.Network.Interfaces[0].Routes = []vmopv1.VirtualMachineNetworkRouteSpec{
							{
								To:  "10.100.10.1",
								Via: "192.168.1",
							},
							{
								To:  "2605:a601:a0ba:720:2ce6::/48",
								Via: "2463:foobar",
							},
							{
								To:  "192.168.1.1/24",
								Via: "ef71:6ce2:3b91:8349:b2b2:f76c:86ae:915b",
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].routes[0].to: Invalid value: "10.100.10.1": invalid CIDR address: 10.100.10.1`,
						`spec.network.interfaces[0].routes[0].via: Invalid value: "192.168.1": must be an IPv4 or IPv6 address`,
						`spec.network.interfaces[0].routes[1].via: Invalid value: "2463:foobar": must be an IPv4 or IPv6 address`,
						`spec.network.interfaces[0].routes[2]: Invalid value: "": cannot mix IP address families`,
						`spec.network.interfaces[0].routes: Invalid value: "routes": routes is available only with the following bootstrap providers: CloudInit`,
					),
				},
			),

			Entry("validate routes when bootstrap supports routes",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].Routes = []vmopv1.VirtualMachineNetworkRouteSpec{
							{
								To:     "10.100.10.1/24",
								Via:    "10.10.1.1",
								Metric: 42,
							},
							{
								To:  "fbd6:93e7:bc11:18b2:514f:2b1d:637a:f695/48",
								Via: "ef71:6ce2:3b91:8349:b2b2:f76c:86ae:915b",
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow routes with default To when bootstrap supports routes",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].Routes = []vmopv1.VirtualMachineNetworkRouteSpec{
							{
								To:     "default",
								Via:    "10.10.1.1",
								Metric: 42,
							},
							{
								To:  "default",
								Via: "ef71:6ce2:3b91:8349:b2b2:f76c:86ae:915b",
							},
						}
					},
					expectAllowed: true,
				},
			),

			// Please note this feature is available only with the following bootstrap providers: CloudInit
			Entry("validate searchDomains when bootstrap doesn't support searchDomains",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].SearchDomains = []string{"dev.local"}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].searchDomains: Invalid value: "dev.local": searchDomains is available only with the following bootstrap providers: CloudInit`,
					),
				},
			),

			Entry("allows per-interface searchDomains with CloudInit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Network.Interfaces[0].SearchDomains = []string{"dev.local"}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow creating VM with network interfaces resulting in a non-DNS1123 combined network interface CR name/label (`vmName-networkName-interfaceName`)",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: fmt.Sprintf("%x", make([]byte, validation.DNS1123SubdomainMaxLength)),
									Network: &common.PartialObjectRef{
										Name: "dummy-nw",
									},
								},
								{
									Name: "dummy_If",
									Network: &common.PartialObjectRef{
										Name: "dummy-nw",
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.interfaces[0].name: Invalid value: "dummy-vm-dummy-nw-%x": is the resulting network interface name: must be no more than 253 characters`, make([]byte, validation.DNS1123SubdomainMaxLength)),
						`spec.network.interfaces[1].name: Invalid value: "dummy-vm-dummy-nw-dummy_If": is the resulting network interface name: a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', `+
							`and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')`,
					),
				},
			),

			Entry("allow creating a VM with network interface with a specified mac address for support network providers",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								/*
									// TBD: If we want to support this on VDS.
										{
											Name: "eth0",
											Network: &common.PartialObjectRef{
												TypeMeta: metav1.TypeMeta{
													APIVersion: "netoperator.vmware.com/v1alpha1",
												},
												Name: "vds-network",
											},
											MACAddr: "00:00:00:00:BB:AA",
										},
								*/
								{
									Name: "eth1",
									Network: &common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											APIVersion: "crd.nsx.vmware.com/v1alpha1",
										},
										Name: "vpc-network",
									},
									MACAddr: "00:00:00:00:CC:DD",
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow creating a VM with network interface with a specified mac address for not support network providers",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Network: &common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											APIVersion: "vmware.com/v1alpha1",
										},
										Name: "nsxt-network",
									},
									MACAddr: "00:00:00:00:BB:AA",
								},
								{
									Name: "eth1",
									Network: &common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											APIVersion: "foobar/v1alpha1",
										},
										Name: "dummy-network",
									},
									MACAddr: "00:00:00:00:CC:DD",
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].macAddr: Invalid value: "00:00:00:00:BB:AA": macAddr is available only with the following network providers: crd.nsx.vmware.com`,
						`spec.network.interfaces[1].macAddr: Invalid value: "00:00:00:00:CC:DD": macAddr is available only with the following network providers: crd.nsx.vmware.com`,
					),
				},
			),
		)
		DescribeTable("network create - host and domain names", doTest,

			Entry("allow simple host name",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "hello-world",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow host name with one character",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "a",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow host name with leading digit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "1a",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow host name with unicode",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "ch✓ck",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow host name with invalid character",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "hello_world",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "hello_world": %s`, vmopv1util.ErrInvalidHostName),
					),
				},
			),

			Entry("disallow host name with leading dash",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "-hello-world",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "-hello-world": %s`, vmopv1util.ErrInvalidHostName),
					),
				},
			),

			Entry("disallow host name longer than 63 characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: strings.Repeat("a", 64),
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "%s": %s`, strings.Repeat("a", 64), vmopv1util.ErrInvalidHostName),
					),
				},
			),

			Entry("disallow host name longer than 15 characters if sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: strings.Repeat("a", 16),
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "%s": %s`, strings.Repeat("a", 16), vmopv1util.ErrInvalidHostNameWindows),
					),
				},
			),

			Entry("disallow host name with valid FQDN",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "hello-world.com",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "hello-world.com": %s`, vmopv1util.ErrInvalidHostName),
					),
				},
			),

			Entry("allow IP4 as host name when domain name is empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "1.2.3.4",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow IP6 as host name when domain name is empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName: "2001:db8:3333:4444:5555:6666:7777:8888",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow IP4 as host name when domain name is non-empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName:   "1.2.3.4",
							DomainName: "com",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "1.2.3.4": %s`, vmopv1util.ErrInvalidHostNameIPWithDomainName),
					),
				},
			),

			Entry("disallow IP6 as host name when domain name is non-empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName:   "2001:db8:3333:4444:5555:6666:7777:8888",
							DomainName: "com",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "2001:db8:3333:4444:5555:6666:7777:8888": %s`, vmopv1util.ErrInvalidHostNameIPWithDomainName),
					),
				},
			),

			Entry("disallow top-level domain with fewer than two characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "c",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "c": %s`, vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("allow valid top-level domain name",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "com",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow domain name with multiple parts",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "example.com",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow domain name with unicode in sub-domain",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "ch✓ck.com",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow domain name with unicode in top-level domain",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "check.✓om",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "check.✓om": %s`, vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("disallow domain name with invalid character",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "hello_world",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "hello_world": %s`, vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("disallow domain name with leading dash",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "-hello-world",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "-hello-world": %s`, vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("disallow domain name with one or more segments longer than 63 characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: "abc." + strings.Repeat("a", 64) + ".com",
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "abc.%s.com": %s`, strings.Repeat("a", 64), vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("disallow domain name longer than 255 characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							DomainName: strings.Repeat("a", 256),
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.domainName: Invalid value: "%s": %s`, strings.Repeat("a", 256), vmopv1util.ErrInvalidDomainName),
					),
				},
			),

			Entry("disallow host and domain name if combined if combined they are longer than 255 characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							HostName:   strings.Repeat("a", 63),
							DomainName: fmt.Sprintf("%[1]s.%[1]s.%[1]s.com", strings.Repeat("a", 63)),
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network: Invalid value: "%[1]s.%[1]s.%[1]s.%[1]s.com": %s`, strings.Repeat("a", 63), vmopv1util.ErrInvalidHostAndDomainName),
					),
				},
			),
		)
	})

	Context("HardwareVersion", func() {

		DescribeTable("MinHardwareVersion", doTest,
			Entry("disallow greater than max valid hardware version",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion + 1)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.minHardwareVersion: Invalid value: %d: should be less than or equal to %d`,
							int32(vimtypes.MaxValidHardwareVersion+1), int32(vimtypes.MaxValidHardwareVersion)),
					),
					expectAllowed: false,
				},
			),
		)
	})

	Context("CD-ROM", func() {

		DescribeTable("CD-ROM create", doTest,

			Entry("allow creating a VM with empty CD-ROM",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						if ctx.vm.Spec.Hardware == nil {
							ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
						}
						ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow creating a VM with CD-ROM and empty guest ID",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GuestID = ""
					},
					validate: doValidateWithMsg(
						`spec.guestID: Required value: when deploying a VM with CD-ROMs`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with invalid CD-ROM image ref kind",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "cdromInvalidImgKind",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: "InvalidKind",
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].image.kind: Unsupported value: "InvalidKind": supported values: "VirtualMachineImage"`,
						`"ClusterVirtualMachineImage"`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with duplicate CD-ROM image ref from VMI kind",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: vmiKind,
									},
								},
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: vmiKind,
									},
								},
							},
						}

					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[1].image.name: Duplicate value: "vmi-dummy"`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with duplicate CD-ROM image ref from CVMI kind",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: cvmiKind,
									},
								},
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: cvmiKind,
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[1].image.name: Duplicate value: "vmi-dummy"`),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with duplicate CD-ROM image ref from VMI and CVMI kinds",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: vmiKind,
									},
								},
								{
									Name: "cdromDupVmi",
									Image: vmopv1.VirtualMachineImageRef{
										Name: dummyVmiName,
										Kind: vmiKind,
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[1].image.name: Duplicate value: "vmi-dummy"`),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with CD-ROM that has only controllerType set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeIDE
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = nil
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].controllerBusNumber: Required value: must be set when controllerType is specified`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow creating a VM with CD-ROM that has only controllerBusNumber set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = ""
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(0))
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].controllerType: Required value: must be set when controllerBusNumber is specified`,
					),
					expectAllowed: false,
				},
			),
		)
	})

	Context("BootOptions", func() {
		DescribeTable("BootOptions create", doTest,

			Entry("allow empty bootOptions",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = nil
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting bootRetryDelay when bootRetry is unset",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.bootRetry: Required value: bootRetry must be set when setting bootRetryDelay",
					),
				},
			),

			Entry("disallow setting bootRetryDelay when bootRetry is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryDisabled,
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.bootRetry: Required value: bootRetry must be set when setting bootRetryDelay",
					),
				},
			),

			Entry("allow setting bootRetryDelay when bootRetry is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting efiSecureBoot when firmware is unset",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.efiSecureBoot: Forbidden: cannot set efiSecureBoot when image firmware is not 'efi'",
					),
				},
			),

			Entry("disallow setting efiSecureBoot when firmware is BIOS",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							Firmware:      vmopv1.VirtualMachineBootOptionsFirmwareTypeBIOS,
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.efiSecureBoot: Forbidden: cannot set efiSecureBoot when image firmware is not 'efi'",
					),
				},
			),

			Entry("allow setting efiSecureBoot when firmware is EFI",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {

						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							Firmware:      vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting bootOrder on create",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
							},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.bootOrder: Forbidden: when creating a VM",
					),
				},
			),
		)
	})

	Context("check.vmoperator.vmware.com", func() {

		DescribeTable("poweron.check.vmoperator.vmware.com", doTest,

			Entry("allow adding annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow adding annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
					},
					expectAllowed: true,
				},
			),
		)

		DescribeTable("delete.check.vmoperator.vmware.com", doTest,

			Entry("allow adding annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow adding annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app1: Forbidden: adding this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),
		)
	})

	Context("Snapshots", func() {
		snapshotPath := field.NewPath("spec", "currentSnapshotName")

		DescribeTable("currentSnapshot", doTest,
			Entry("when a VM is created with a currentSnapshot",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						vmSnapshot := builder.DummyVirtualMachineSnapshot(
							ctx.vm.Namespace,
							"dummy-vm-snapshot",
							ctx.vm.Name,
						)

						ctx.vm.Spec.CurrentSnapshotName = vmSnapshot.Name
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Forbidden(snapshotPath, "creating VM with current snapshot is not allowed").Error(),
					),
				},
			),
		)
	})

	Context("GroupName", func() {
		DescribeTable("validateGroupName", doTest,
			Entry("disallow spec.groupName when VM Groups feature is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GroupName = dummyGroupName
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMGroups = false
						})
					},
					validate: func(response admission.Response) {
						Expect(string(response.Result.Reason)).To(Equal(field.Invalid(
							field.NewPath("spec", "groupName"),
							dummyGroupName,
							"the VM Groups feature is not enabled").Error()))
					},
					expectAllowed: false,
				},
			),
			Entry("allow spec.groupName when VM Groups feature is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GroupName = dummyGroupName
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMGroups = true
						})
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Affinity", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMGroups = true
				ctx.vm.Spec.GroupName = dummyGroupName
				ctx.vm.Spec.Affinity = &vmopv1.AffinitySpec{}
			})
		})

		DescribeTable("create", doTest,
			Entry("allow empty Affinity spec",
				testParams{
					setup:         func(ctx *unitValidatingWebhookContext) {},
					expectAllowed: true,
				},
			),

			Entry("disallow Affinity without GroupName",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GroupName = ""
					},
					validate: doValidateWithMsg(`spec.groupName: Required value: when setting affinity`),
				},
			),

			Entry("allow VM Affinity with RequiredDuringSchedulingPreferredDuringExecution and PreferredDuringSchedulingPreferredDuringExecution with supported fields",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels = map[string]string{"foo": "bar"}
						ctx.vm.Spec.Affinity.VMAffinity = &vmopv1.VMAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "foo",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "foo",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"bar"},
											},
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow VM Affinity RequiredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchLabels",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels = map[string]string{
							"foo":                          "bar",
							"vmoperator.vmware.com/paused": "true",
						}
						ctx.vm.Spec.Affinity.VMAffinity = &vmopv1.VMAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo":                          "bar",
											"vmoperator.vmware.com/paused": "true",
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAffinity.requiredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchLabels: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),

			Entry("disallow VM Affinity RequiredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchExpressions",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels = map[string]string{
							"foo":                          "bar",
							"vmoperator.vmware.com/paused": "true",
						}
						ctx.vm.Spec.Affinity.VMAffinity = &vmopv1.VMAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "vmoperator.vmware.com/paused",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"true"},
											},
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAffinity.requiredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchExpressions[0].key: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),

			Entry("disallow VM Affinity PreferredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchLabels",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels = map[string]string{
							"foo":                            "bar",
							"vmicache.vmoperator.vmware.com": "ready",
						}
						ctx.vm.Spec.Affinity.VMAffinity = &vmopv1.VMAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo":                            "bar",
											"vmicache.vmoperator.vmware.com": "ready",
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchLabels: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),

			Entry("disallow VM Affinity PreferredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchExpressions",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Labels = map[string]string{
							"foo":                            "bar",
							"vmicache.vmoperator.vmware.com": "ready",
						}
						ctx.vm.Spec.Affinity.VMAffinity = &vmopv1.VMAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "vmicache.vmoperator.vmware.com",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"ready"},
											},
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchExpressions[0].key: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),

			Entry("allow VM Anti Affinity with RequiredDuringSchedulingPreferredDuringExecution and Zone topology key for non-privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow VM Anti Affinity with RequiredDuringSchedulingPreferredDuringExecution and Zone topology key for privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelTopologyZone,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow VM Anti Affinity with RequiredDuringSchedulingPreferredDuringExecution and Host topology key for non-privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAntiAffinity.requiredDuringSchedulingPreferredDuringExecution[0].topologyKey: Unsupported value: "kubernetes.io/hostname": supported values: "topology.kubernetes.io/zone"`),
				},
			),

			Entry("allow VM Anti Affinity with RequiredDuringSchedulingPreferredDuringExecution and Host topology key for privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow VM Anti Affinity with PreferredDuringSchedulingPreferredDuringExecution and Host topology key for non-privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow VM Anti Affinity with PreferredDuringSchedulingPreferredDuringExecution and Host topology key for privileged users",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow VM Anti Affinity with PreferredDuringSchedulingPreferredDuringExecution with supported fields",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Operator: metav1.LabelSelectorOpIn,
											},
										},
									},
									TopologyKey: corev1.LabelTopologyZone,
								},
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Operator: metav1.LabelSelectorOpIn,
											},
										},
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow VM Anti Affinity with PreferredDuringSchedulingPreferredDuringExecution with unsupported fields",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Operator: metav1.LabelSelectorOpNotIn,
											},
										},
									},
									TopologyKey: "",
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAntiAffinity.preferredDuringSchedulingPreferredDuringExecution[0].topologyKey: Unsupported value: "": supported values: "topology.kubernetes.io/zone", "kubernetes.io/hostname"`,
						`spec.affinity.vmAntiAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchExpressions[0].operator: Unsupported value: "NotIn": supported values: "In"`),
				},
			),

			Entry("disallow VM Anti Affinity PreferredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchLabels",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo":                            "bar",
											"vmicache.vmoperator.vmware.com": "ready",
										},
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAntiAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchLabels: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),

			Entry("disallow VM Anti Affinity PreferredDuringSchedulingPreferredDuringExecution with VM operator labels in MatchExpressions",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Affinity.VMAntiAffinity = &vmopv1.VMAntiAffinitySpec{
							PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"foo": "bar",
										},
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "vmicache.vmoperator.vmware.com",
												Operator: metav1.LabelSelectorOpIn,
												Values:   []string{"ready"},
											},
										},
									},
									TopologyKey: corev1.LabelHostname,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.affinity.vmAntiAffinity.preferredDuringSchedulingPreferredDuringExecution[0].labelSelector.matchExpressions[0].key: Forbidden: label selector can not contain VM Operator managed labels (vmoperator.vmware.com)`),
				},
			),
		)
	})

	Context("PVC Access Mode and Sharing Mode Combinations", func() {
		DescribeTable("validate PVC access mode and sharing mode combinations",
			func(testName string, setup func(*unitValidatingWebhookContext), expectAllowed bool) {
				setup(ctx)

				var err error
				ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
				Expect(err).ToNot(HaveOccurred())

				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(Equal(expectAllowed))
			},

			Entry("should allow ReadWriteOnce volume with None sharing mode and None controller",
				"readwriteonce-none-sharing-none-controller",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteOnce volume and None sharing mode
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeNone
					ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
					ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To[int32](1)

					// Add SCSI controller with None sharing mode
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					}
				},
				true),

			Entry("should reject ReadWriteOnce volume with MultiWriter sharing mode "+
				"and PVC with ReadWriteOnce access mode",
				"readwriteonce-multiwriter-sharing-pvc-readwriteonce",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteOnce volume and MultiWriter sharing mode
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeMultiWriter
				},
				false,
			),

			Entry("should reject ReadWriteOnce volume with Physical controller sharing mode",
				"readwriteonce-physical-controller",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteOnce volume and Physical controller
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
					ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To[int32](0)

					// Add SCSI controller with Physical sharing mode
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					}
				},
				false),

			Entry("should allow ReadWriteMany volume with MultiWriter sharing mode",
				"readwritemany-multiwriter-sharing",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteMany PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteMany volume and MultiWriter sharing mode
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeMultiWriter
				},
				true,
			),

			Entry("should allow ReadWriteMany volume with Physical controller",
				"readwritemany-physical-controller",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteMany PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteMany volume and Physical controller
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeMultiWriter
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
					ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To[int32](1)

					// Add SCSI controller with Physical sharing mode
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					}
				},
				true,
			),

			Entry("should reject ReadWriteMany volume with neither MultiWriter sharing mode nor Physical controller",
				"readwritemany-neither-multiwriter-nor-physical",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteMany PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteMany volume but neither
					// MultiWriter nor Physical controller.
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeNone
					ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
					ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To[int32](0)

					// Add SCSI controller with None sharing mode.
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					}
				},
				false),

			Entry("should allow ReadWriteMany volume with both MultiWriter sharing mode and Physical controller",
				"readwritemany-both-multiwriter-and-physical",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteMany PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Configure VM with ReadWriteMany volume with both MultiWriter and Physical controller
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = dummyPVCName
					ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeMultiWriter
					ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
					ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To[int32](1)

					// Add SCSI controller with Physical sharing mode
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					}
				},
				true,
			),

			Entry("should handle missing PVC gracefully",
				"missing-pvc",
				func(ctx *unitValidatingWebhookContext) {
					// Configure VM with non-existent PVC
					ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = "non-existent-pvc"
				},
				true),
		)
	})

	Context("spec.biosUUID", func() {
		DescribeTable("create", doTest,
			Entry("should allow when VM specifies valid UUID",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BiosUUID = uuid.NewString()
					},
					expectAllowed: true,
				},
			),
			Entry("should not allow when VM specifies invalid UUID",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BiosUUID = "invalid UUID"
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						field.Invalid(field.NewPath("spec", "biosUUID"), "invalid UUID", "must provide a valid UUID").Error(),
					),
				},
			),
			Entry("should allow when VM specifies no UUID",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BiosUUID = ""
					},
					expectAllowed: true,
				},
			),
		)
	})
}

type updateArgs struct {
	isServiceUser               bool
	changeInstanceUUID          bool
	changeBiosUUID              bool
	changeImageRef              bool
	changeImageName             bool
	changeStorageClass          bool
	changeResourcePolicy        bool
	assignZoneName              bool
	changeZoneName              bool
	unsetZone                   bool
	isSysprepTransportUsed      bool
	withInstanceStorageVolumes  bool
	changeInstanceStorageVolume bool
	oldInstanceUUID             string
	oldBiosUUID                 string
	oldPowerState               vmopv1.VirtualMachinePowerState
	newPowerState               vmopv1.VirtualMachinePowerState
	newPowerStateEmptyAllowed   bool
	nextRestartTime             string
	lastRestartTime             string
	applyPowerStateChangeTime   string
}

func setupOldVMForUpdate(ctx *unitValidatingWebhookContext, args updateArgs) {
	// Init immutable fields that aren't set in the dummy VM.
	if ctx.oldVM.Spec.Reserved == nil {
		ctx.oldVM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
	}
	ctx.oldVM.Spec.Reserved.ResourcePolicyName = "policy"
	ctx.oldVM.Spec.InstanceUUID = args.oldInstanceUUID
	ctx.oldVM.Spec.BiosUUID = args.oldBiosUUID

	if args.oldPowerState != "" {
		ctx.oldVM.Spec.PowerState = args.oldPowerState
	}
	ctx.oldVM.Spec.NextRestartTime = args.lastRestartTime

	if args.changeZoneName {
		ctx.oldVM.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName
	}
	if args.changeInstanceStorageVolume {
		instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
		ctx.oldVM.Spec.Volumes = append(ctx.oldVM.Spec.Volumes, instanceStorageVolumes...)
	}

	setControllerForPVC(ctx.oldVM)
}

func setupNewVMForUpdate(ctx *unitValidatingWebhookContext, args updateArgs) {
	if args.isServiceUser {
		ctx.IsPrivilegedAccount = true
	}

	if args.changeImageRef {
		if ctx.vm.Spec.Image == nil {
			ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{}
		}
		ctx.vm.Spec.Image.Name += updateSuffix
	}
	if args.changeImageName {
		ctx.vm.Spec.ImageName += updateSuffix
	}
	if args.changeInstanceUUID {
		ctx.vm.Spec.InstanceUUID += updateSuffix
	}
	if args.changeBiosUUID {
		ctx.vm.Spec.BiosUUID += updateSuffix
	}
	if args.changeStorageClass {
		ctx.vm.Spec.StorageClass += updateSuffix
	}

	if ctx.vm.Spec.Reserved == nil {
		ctx.vm.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
	}
	ctx.vm.Spec.Reserved.ResourcePolicyName = "policy"
	if args.changeResourcePolicy {
		ctx.vm.Spec.Reserved.ResourcePolicyName = "policy" + updateSuffix
	}

	if args.assignZoneName {
		ctx.vm.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName
	}
	if args.changeZoneName {
		if args.unsetZone {
			delete(ctx.vm.Labels, corev1.LabelTopologyZone)
		} else {
			ctx.vm.Labels[corev1.LabelTopologyZone] = builder.DummyZoneName + updateSuffix
		}
	}

	if args.newPowerState != "" || args.newPowerStateEmptyAllowed {
		ctx.vm.Spec.PowerState = args.newPowerState
	}
	if args.applyPowerStateChangeTime != "" {
		ctx.vm.Annotations[pkgconst.ApplyPowerStateTimeAnnotation] = args.applyPowerStateChangeTime
	}
	ctx.vm.Spec.NextRestartTime = args.nextRestartTime

	if args.isSysprepTransportUsed {
		ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		if ctx.vm.Spec.Bootstrap == nil {
			ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
		}
		ctx.vm.Spec.Bootstrap.Sysprep = &vmopv1.VirtualMachineBootstrapSysprepSpec{
			RawSysprep: &common.SecretKeySelector{},
		}
	}

	if args.withInstanceStorageVolumes {
		instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
		ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
	}
	if args.changeInstanceStorageVolume {
		instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
		instanceStorageVolumes[0].Name += updateSuffix
		ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
	}

	// Set controllerBusNumber on volumes to simulate mutation webhook
	setControllerForPVC(ctx.vm)
}

func unitTestsValidateUpdate() { //nolint:gocyclo
	var (
		ctx *unitValidatingWebhookContext
	)

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)

		setupOldVMForUpdate(ctx, args)
		setupNewVMForUpdate(ctx, args)

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(HaveSuffix(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})

	AfterEach(func() {
		ctx = nil
	})

	msg := "field is immutable"
	volumesPath := field.NewPath("spec", "volumes")
	volPath := field.NewPath("spec", "volumes")
	powerStatePath := field.NewPath("spec", "powerState")
	nextRestartTimePath := field.NewPath("spec", "nextRestartTime")

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),

		Entry("should deny image ref change", updateArgs{changeImageRef: true}, false, msg, nil),
		Entry("should deny image name change", updateArgs{changeImageName: true}, false, msg, nil),
		Entry("should deny instance uuid change", updateArgs{changeInstanceUUID: true, oldInstanceUUID: "uuid"}, false, msg, nil),
		Entry("should deny bios uuid change", updateArgs{changeBiosUUID: true, oldBiosUUID: "uuid"}, false, msg, nil),
		Entry("should deny storageClass change", updateArgs{changeStorageClass: true}, false, msg, nil),
		Entry("should deny resourcePolicy change", updateArgs{changeResourcePolicy: true}, false, msg, nil),

		Entry("should allow empty instance uuid change", updateArgs{changeInstanceUUID: true}, true, nil, nil),
		Entry("should allow empty bios uuid change", updateArgs{changeBiosUUID: true}, true, nil, nil),
		Entry("should allow initial zone assignment", updateArgs{assignZoneName: true}, true, nil, nil),
		Entry("should deny zone change for non-privileged user", updateArgs{changeZoneName: true}, false,
			field.Invalid(field.NewPath("metadata", "labels").Key(corev1.LabelTopologyZone), builder.DummyZoneName+updateSuffix, "field is immutable").Error(), nil),
		Entry("should allow zone change for privileged user", updateArgs{changeZoneName: true, isServiceUser: true}, true, nil, nil),
		Entry("should allow zone unset for privileged user", updateArgs{changeZoneName: true, isServiceUser: true, unsetZone: true}, true, nil, nil),

		Entry("should deny instance storage volume name change, when user is SSO user", updateArgs{changeInstanceStorageVolume: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should deny adding new instance storage volume, when user is SSO user", updateArgs{withInstanceStorageVolumes: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow adding new instance storage volume, when user type is service user", updateArgs{withInstanceStorageVolumes: true, isServiceUser: true}, true, nil, nil),
		Entry("should allow instance storage volume name change, when user type is service user", updateArgs{changeInstanceStorageVolume: true, isServiceUser: true}, true, nil, nil),

		Entry("should allow sysprep", updateArgs{isSysprepTransportUsed: true}, true, nil, nil),

		Entry("should allow updating suspended VM to powered on", updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateSuspended, newPowerState: vmopv1.VirtualMachinePowerStateOn}, true,
			nil, nil),
		Entry("should allow updating suspended VM to powered off", updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateSuspended, newPowerState: vmopv1.VirtualMachinePowerStateOff}, true,
			nil, nil),
		Entry("should disallow updating powered off VM to suspended", updateArgs{oldPowerState: vmopv1.VirtualMachinePowerStateOff, newPowerState: vmopv1.VirtualMachinePowerStateSuspended}, false,
			field.Invalid(powerStatePath, vmopv1.VirtualMachinePowerStateSuspended, "cannot suspend a VM that is powered off").Error(), nil),

		Entry("should allow updating VM with non-empty, valid nextRestartTime value", updateArgs{
			nextRestartTime: time.Now().UTC().Format(time.RFC3339Nano)}, true, nil, nil),
		Entry("should allow updating VM with empty nextRestartTime value if existing value is also empty",
			updateArgs{nextRestartTime: ""}, true, nil, nil),
		Entry("should disallow updating VM with empty nextRestartTime value",
			updateArgs{lastRestartTime: time.Now().UTC().Format(time.RFC3339Nano), nextRestartTime: ""}, false,
			field.Invalid(nextRestartTimePath, "", "must be formatted as RFC3339Nano").Error(), nil),
		Entry("should disallow updating VM with non-empty, valid nextRestartTime value if mutation webhooks were running",
			updateArgs{nextRestartTime: "now"}, false,
			field.Invalid(nextRestartTimePath, "now", "mutation webhooks are required to restart VM").Error(), nil),
		Entry("should disallow updating VM with non-empty, invalid nextRestartTime value ",
			updateArgs{nextRestartTime: "hello"}, false,
			field.Invalid(nextRestartTimePath, "hello", "must be formatted as RFC3339Nano").Error(), nil),
		Entry("should allow updating VM with non-empty, valid apply power state change time annotation",
			updateArgs{applyPowerStateChangeTime: time.Now().UTC().Format(time.RFC3339Nano), isServiceUser: true}, true, nil, nil),
		Entry("should disallow updating VM with non-empty, invalid apply power state change time annotation",
			updateArgs{applyPowerStateChangeTime: "hello"}, false,
			field.Invalid(field.NewPath("metadata").Child("annotations").Key(pkgconst.ApplyPowerStateTimeAnnotation), "hello", "must be formatted as RFC3339Nano").Error(), nil),
	)

	doTest := func(args testParams) {

		args.setup(ctx)

		// Set controllerBusNumber on volumes after setup to simulate mutation webhook
		if !args.skipSetControllerForPVC {
			setControllerForPVC(ctx.vm)
			if ctx.oldVM != nil {
				setControllerForPVC(ctx.oldVM)
			}
		}

		if !args.skipBypassUpgradeCheck {
			bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.expectAllowed))

		if args.validate != nil {
			args.validate(response)
		}
	}

	DescribeTable(
		"spec.class and spec.className update",
		doTest,

		Entry("should deny className change when resize features are disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = false
						config.Features.VMResizeCPUMemory = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "className"), newVMClass, apivalidation.FieldImmutableErrorMsg).Error(),
				),
			},
		),
		Entry("should allow className change when VMResize feature is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = false
					})
				},
				expectAllowed: true,
			},
		),
		Entry("should allow className change when VMResizeCPUMemory feature is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = false
						config.Features.VMResizeCPUMemory = true
						config.Features.ImmutableClasses = false
					})
				},
				expectAllowed: true,
			},
		),
		Entry("should require className when changing to empty for unprivileged user with VMImportNewNet disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.VMImportNewNet = false
						config.Features.ImmutableClasses = false
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),
		Entry("should allow changing to empty className for privileged user with VMImportNewNet enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.VMImportNewNet = true
						config.Features.ImmutableClasses = false
					})
				},
				expectAllowed: true,
			},
		),
		Entry("should return error if class instance does not exist",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReference).Error(),
				),
			},
		),

		Entry("should error out if instance is not active",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance with the correct OnwerRef that points to the correct VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReferenceNotActive).Error(),
				),
			},
		),

		Entry("should return error if instance points to a different class than spec.className",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance without an OnwerRef that points to some other VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
							// Set the label to mark the instance as active
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									Name: "random-vm-class",
								},
							},
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "class").Child("name"), "new-class-instance", invalidClassInstanceReferenceOwnerMismatch).Error(),
				),
			},
		),
		Entry("should succeed if instance is valid, and is active",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = oldVMClass
					ctx.vm.Spec.ClassName = newVMClass
					ctx.vm.Spec.Class = &common.LocalObjectRef{
						Name: "new-class-instance",
					}

					// Create the class
					class := &vmopv1.VirtualMachineClass{
						ObjectMeta: metav1.ObjectMeta{
							Name:      newVMClass,
							Namespace: ctx.vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, class)).To(Succeed())
					// Fetch the class so we can set an ownerref.
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(class), class)).To(Succeed())

					// Create the instance with the correct OnwerRef that points to the correct VM class
					classInstance := &vmopv1.VirtualMachineClassInstance{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "new-class-instance",
							Namespace: ctx.vm.Namespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: "vmoperator.vmware.com/v1alpha5",
									Name:       newVMClass,
									Kind:       "VirtualMachineClass",
								},
							},
							// Set the label to mark the instance as active
							Labels: map[string]string{
								vmopv1.VMClassInstanceActiveLabelKey: "",
							},
						},
					}
					Expect(ctx.Client.Create(ctx, classInstance)).To(Succeed())

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMResize = true
						config.Features.ImmutableClasses = true
					})
				},
				expectAllowed: true,
			},
		),
	)

	Context("Annotations", func() {
		annotationPath := field.NewPath("metadata", "annotations")

		DescribeTable("update", doTest,
			Entry("should disallow updating admin-only annotations by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName + updateSuffix

					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Key(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(pkgconst.CreatedAtBuildVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(pkgconst.CreatedAtSchemaVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.RestoredVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.ImportedVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FailedOverVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyAllowListAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyWatermarkAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
					),
				},
			),
			Entry("should disallow removing admin-only annotations by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Key(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(pkgconst.CreatedAtBuildVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(pkgconst.CreatedAtSchemaVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.RestoredVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.ImportedVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(vmopv1.FailedOverVMAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyAllowListAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Key(anno2extraconfig.ManagementProxyWatermarkAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
					),
				},
			),
			Entry("should allow updating admin-only annotations by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName + updateSuffix
					},
					expectAllowed: true,
				},
			),
			Entry("should allow removing admin-only annotations by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[pkgconst.ClusterModuleNameAnnotationKey] = dummyClusterModuleAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					expectAllowed: true,
				},
			),
			Entry("should allow updating admin-only annotations by privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						privilegedUsersEnvList := "  , foo ,bar , test,  "
						privilegedUser := "bar"

						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.PrivilegedUsers = privilegedUsersEnvList
						})

						ctx.UserInfo.Username = privilegedUser
						ctx.IsPrivilegedAccount = pkgbuilder.IsPrivilegedAccount(ctx.WebhookContext, ctx.UserInfo)

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
						ctx.vm.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName + updateSuffix
						ctx.vm.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName + updateSuffix
					},
					expectAllowed: true,
				},
			),
			Entry("should allow removing admin-only annotations by privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						privilegedUsersEnvList := "  , foo ,bar , test,  "
						privilegedUser := "bar"

						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.PrivilegedUsers = privilegedUsersEnvList
						})

						ctx.UserInfo.Username = privilegedUser
						ctx.IsPrivilegedAccount = pkgbuilder.IsPrivilegedAccount(ctx.WebhookContext, ctx.UserInfo)

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[pkgconst.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.oldVM.Annotations[vmopv1.RestoredVMAnnotation] = dummyRegisteredAnnVal
						ctx.oldVM.Annotations[vmopv1.ImportedVMAnnotation] = dummyImportedAnnVal
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = dummyFailedOverAnnVal
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyAllowListAnnotation] = dummyVmiName
						ctx.oldVM.Annotations[anno2extraconfig.ManagementProxyWatermarkAnnotation] = dummyVmiName
					},
					expectAllowed: true,
				},
			),
			Entry("should disallow changing cluster module annotation by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Annotations[pkgconst.ClusterModuleNameAnnotationKey] = dummyClusterModuleAnnVal
						ctx.vm.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{ResourcePolicyName: "resource-policy"}
						ctx.oldVM.Annotations[pkgconst.ClusterModuleNameAnnotationKey] = dummyClusterModuleAnnVal + updateSuffix
						ctx.oldVM.Spec.Reserved = ctx.vm.Spec.Reserved
					},
					validate: doValidateWithMsg(
						`metadata.annotations[vsphere-cluster-module-group]: Forbidden: modifying this annotation is not allowed for non-admin users`),
				},
			),
		)

		getProtectedAnnotationTableAllowUpdate := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.IsPrivilegedAccount = true
							ctx.oldVM.Annotations[tc.annotationKey] = tc.oldValue
							if tc.newValue != "" {
								ctx.vm.Annotations[tc.annotationKey] = tc.newValue
							}
						},
						expectAllowed: true,
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should allow update of "+tc.annotationKey, tc))
			}

			return table
		}

		getProtectedAnnotationTableAllowRemoval := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.IsPrivilegedAccount = true
							ctx.oldVM.Annotations[tc.annotationKey] = tc.oldValue
							delete(ctx.vm.Annotations, tc.annotationKey)
						},
						expectAllowed: true,
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should allow removal of "+tc.annotationKey, tc))
			}

			return table
		}

		getProtectedAnnotationTableDisallowUpdate := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)
							ctx.oldVM.Annotations[tc.annotationKey] = tc.oldValue
							if tc.newValue != "" {
								ctx.vm.Annotations[tc.annotationKey] = tc.newValue
							}
						},
						validate: doValidateWithMsg(
							field.Forbidden(annotationPath.Key(tc.annotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						),
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should disallow update of "+tc.annotationKey, tc))
			}

			return table
		}

		getProtectedAnnotationTableDisallowRemoval := func() []any {
			table := []any{
				func(tc protectedAnnotationTestCase) {
					doTest(testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)
							ctx.oldVM.Annotations[tc.annotationKey] = tc.oldValue
							delete(ctx.vm.Annotations, tc.annotationKey)
						},
						validate: doValidateWithMsg(
							field.Forbidden(annotationPath.Key(tc.annotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						),
					})
				},
			}

			for i := range protectedAnnotationTestCases {
				tc := protectedAnnotationTestCases[i]
				table = append(table, Entry("should disallow removal of "+tc.annotationKey, tc))
			}

			return table
		}

		DescribeTable("disallow update of protected annotations by non-privileged user",
			getProtectedAnnotationTableDisallowUpdate()...,
		)

		DescribeTable("disallow removal of protected annotations by non-privileged user",
			getProtectedAnnotationTableDisallowRemoval()...,
		)

		DescribeTable("allow update of protected annotations by privileged user",
			getProtectedAnnotationTableAllowUpdate()...,
		)

		DescribeTable("allow removal of protected annotations by privileged user",
			getProtectedAnnotationTableAllowRemoval()...,
		)
	})

	Context("Bootstrap", func() {
		DescribeTable("update", doTest,

			Entry("allow no bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = nil
						ctx.vm.Spec.Bootstrap = nil
					},
					expectAllowed: true,
				},
			),
			Entry("allow setting bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = nil
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					},
					expectAllowed: true,
				},
			),
			Entry("allow setting specific bootstrap provider",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("disallow clearing all bootstrap",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
						ctx.vm.Spec.Bootstrap = nil
					},
					validate: doValidateWithMsg(`spec.bootstrap: Forbidden: bootstrap provider type cannot be changed`),
				},
			),
			Entry("disallow unsetting CloudInit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					},
					validate: doValidateWithMsg(`spec.bootstrap.cloudInit: Forbidden: bootstrap provider type cannot be changed`),
				},
			),
			Entry("disallow unsetting LinuxPrep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					},
					validate: doValidateWithMsg(`spec.bootstrap.linuxPrep: Forbidden: bootstrap provider type cannot be changed`),
				},
			),
			Entry("disallow unsetting Sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					},
					validate: doValidateWithMsg(`spec.bootstrap.sysprep: Forbidden: bootstrap provider type cannot be changed`),
				},
			),
			Entry("disallow changing bootstrap providers",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
						}
					},
					validate: doValidateWithMsg(`spec.bootstrap.linuxPrep: Forbidden: bootstrap provider type cannot be changed`),
				},
			),
			Entry("allow changing bootstrap providers if privileged",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
						}
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow bootstrap update if VM is desired powered on with halt annotation",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn: "",
						}
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn

						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Label", func() {
		labelPath := field.NewPath("metadata", "labels")

		DescribeTable("update", doTest,
			Entry("should disallow updating VM with admin-only labels set by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
						ctx.vm.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal + updateSuffix
					},
					validate: doValidateWithMsg(
						field.Forbidden(labelPath.Child(vmopv1.PausedVMLabelKey), "modifying this label is not allowed for non-admin users").Error()),
				},
			),
			Entry("should disallow removing admin-only labels set by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
					},
					validate: doValidateWithMsg(
						field.Forbidden(labelPath.Child(vmopv1.PausedVMLabelKey), "modifying this label is not allowed for non-admin users").Error()),
				},
			),
			Entry("should allow updating VM with admin-only label by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
						ctx.vm.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal + updateSuffix
					},
					expectAllowed: true,
				},
			),
			Entry("should allow removing admin-only label by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Labels[vmopv1.PausedVMLabelKey] = dummyPausedVMLabelVal
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Image and ImageName", func() {
		DescribeTable("imageName", doTest,
			Entry("forbid changing imageName to non empty value",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.ImageName = dummyVmiName

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ImageName = dummyVmiName + updateSuffix
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.imageName: Invalid value: "%v": field is immutable`, dummyVmiName+updateSuffix)),
				},
			),

			Entry("forbid unset of imageName if FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.ImageName = dummyVmiName

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ImageName = ""
						ctx.vm.Spec.Image = nil
					},
					validate: doValidateWithMsg(
						`spec.imageName: Invalid value: "": field is immutable`),
				},
			),

			// FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled
			Entry("forbid unset of imageName if FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled, but annotation is not present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})

						ctx.oldVM.Spec.ImageName = dummyVmiName
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ImageName = ""
						ctx.vm.Spec.Image = nil
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.imageName: Invalid value: "%v": field is immutable`, "")),
				},
			),

			Entry("forbid unset of imageName by unprivileged users if FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled and failover annotation is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.FailedOverVMAnnotation: "foo",
						}

						ctx.oldVM.Spec.ImageName = dummyVmiName
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ImageName = ""
						ctx.vm.Spec.Image = nil
					},
					validate: doValidateWithMsg(
						field.Forbidden(field.NewPath("spec", "imageName"), "restricted to privileged users").Error()),
				},
			),

			Entry("allow unset of imageName for privileged users when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled, and failover annotation is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})

						ctx.IsPrivilegedAccount = true

						ctx.oldVM.Spec.ImageName = dummyVmiName
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.FailedOverVMAnnotation: "foo",
						}
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ImageName = ""
						ctx.vm.Spec.Image = nil
					},
					expectAllowed: true,
				},
			),
		)

		DescribeTable("image", doTest,
			Entry("forbid changing image from nil to a non-nil value",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Image = nil

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName + updateSuffix,
						}
					},
					validate: doValidateWithMsg(
						field.Invalid(field.NewPath("spec", "image"), &vmopv1.VirtualMachineImageRef{Name: dummyVmiName + updateSuffix}, apivalidation.FieldImmutableErrorMsg).Error()),
				},
			),
			Entry("forbid changing image from non-nil to non-nil value",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName,
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName + updateSuffix,
						}
					},
					validate: doValidateWithMsg(
						field.Invalid(field.NewPath("spec", "image"), &vmopv1.VirtualMachineImageRef{Name: dummyVmiName + updateSuffix}, apivalidation.FieldImmutableErrorMsg).Error()),
				},
			),

			Entry("forbid unset of image when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName,
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = nil
						ctx.vm.Spec.ImageName = ""
					},
					validate: doValidateWithMsg(
						field.Invalid(field.NewPath("spec", "image"), nil, apivalidation.FieldImmutableErrorMsg).Error()),
				},
			),

			Entry("forbid unset of image for privileged users when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled, but failover annotation is not present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})

						ctx.IsPrivilegedAccount = true

						ctx.oldVM.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName,
						}
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = nil
						ctx.vm.Spec.ImageName = ""
					},
					validate: doValidateWithMsg(
						field.Invalid(field.NewPath("spec", "image"), nil, apivalidation.FieldImmutableErrorMsg).Error()),
				},
			),

			Entry("forbid unset of image by unprivileged users when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled and the failover annotation is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})
						ctx.IsPrivilegedAccount = false

						ctx.oldVM.Annotations = map[string]string{
							vmopv1.FailedOverVMAnnotation: "bar",
						}

						ctx.oldVM.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName,
						}
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = nil
						ctx.vm.Spec.ImageName = ""
					},
					validate: doValidateWithMsg(
						field.Forbidden(field.NewPath("spec", "image"), "restricted to privileged users").Error()),
				},
			),

			Entry("allow changing image for privileged users when FSS_WCP_VMSERVICE_INCREMENTAL_RESTORE is enabled, and failover annotation is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMIncrementalRestore = true
						})

						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.FailedOverVMAnnotation: "bar",
						}

						ctx.oldVM.Spec.Image = &vmopv1.VirtualMachineImageRef{
							Name: dummyVmiName,
						}
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Image = nil
						ctx.vm.Spec.ImageName = ""
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("ClassName", func() {

		DescribeTable("class name", doTest,

			Entry("disallow changing class name when FSS_WCP_VMSERVICE_RESIZE is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.ClassName = "class"

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = newVMClass
					},
					validate: doValidateWithMsg(
						`spec.className: Invalid value: "new-class": field is immutable`),
				},
			),

			Entry("allow changing class name when FSS_WCP_VMSERVICE_RESIZE is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMResize = true
						})

						ctx.oldVM.Spec.ClassName = "small-class"
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = "big-class"
					},
					expectAllowed: true,
				},
			),

			Entry("disallow changing class name to empty string when FSS_WCP_VMSERVICE_RESIZE is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMResize = true
						})

						ctx.oldVM.Spec.ClassName = "small-class"
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = ""
					},
					validate: doValidateWithMsg("spec.className: Required value"),
				},
			),

			Entry("disallow changing class name when FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.ClassName = "class"

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = "new-class"
					},
					validate: doValidateWithMsg(
						`spec.className: Invalid value: "new-class": field is immutable`),
				},
			),

			Entry("allow changing class name when FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMResizeCPUMemory = true
						})

						ctx.oldVM.Spec.ClassName = "xsmall-class"
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = "big-class"
					},
					expectAllowed: true,
				},
			),

			Entry("disallow changing class name to empty string when FSS_WCP_VMSERVICE_RESIZE_CPU_MEMORY is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMResizeCPUMemory = true
						})

						ctx.oldVM.Spec.ClassName = "xsmall-class"
						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.ClassName = ""
					},
					validate: doValidateWithMsg("spec.className: Required value"),
				},
			),
		)
	})

	Context("Network", func() {

		DescribeTable("update network", doTest,

			Entry("disallow changing network interface name",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Network.Interfaces[0].Name = "eth100"
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].name: Forbidden: field is immutable`),
				},
			),

			Entry("disallow changing network interface network",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:    "eth0",
									Network: &common.PartialObjectRef{Name: "my-network"},
								},
							},
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Network.Interfaces[0].Network.Name = "my-other-network"
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces[0].network: Forbidden: field is immutable`),
				},
			),

			Entry("disallow changing number of network interfaces",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Network.Interfaces = append(ctx.vm.Spec.Network.Interfaces,
							vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth1"})

					},
					validate: doValidateWithMsg(
						`spec.network.interfaces: Forbidden: network interfaces cannot be added or removed`),
				},
			),

			Entry("allow interface name change if VM has failover label",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						if ctx.oldVM.Labels == nil {
							ctx.oldVM.Labels = make(map[string]string)
						}
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = "foo"

						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Network.Interfaces[0].Name = "eth100"
					},
					expectAllowed: true,
				},
			),
			Entry("allow changing network interface network if VM has failover label",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						if ctx.oldVM.Labels == nil {
							ctx.oldVM.Labels = make(map[string]string)
						}
						ctx.oldVM.Annotations[vmopv1.FailedOverVMAnnotation] = "foo"

						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:    "eth0",
									Network: &common.PartialObjectRef{Name: "my-network"},
								},
							},
						}

						ctx.vm = ctx.oldVM.DeepCopy()
						ctx.vm.Spec.Network.Interfaces[0].Network.Name = "my-other-network"
					},
					expectAllowed: true,
				},
			),
		)

		DescribeTable("update network - host and domain names", doTest,

			Entry("disallow host name longer than 15 characters if sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Network.HostName = strings.Repeat("a", 16)
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "%s": %s`, strings.Repeat("a", 16), vmopv1util.ErrInvalidHostNameWindows),
					),
				},
			),

			Entry("disallow host name longer than 15 characters if sysprep and old VM has host name longer than 15 characters",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network.HostName = strings.Repeat("a", 16)
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Network.HostName = strings.Repeat("a", 16)
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.hostName: Invalid value: "%s": %s`, strings.Repeat("a", 16), vmopv1util.ErrInvalidHostNameWindows),
					),
				},
			),

			Entry("allow host name longer than 15 characters if sysprep and old VM has host name longer than 15 characters and is sysprep",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network.HostName = strings.Repeat("a", 16)
						ctx.oldVM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Network.HostName = strings.Repeat("a", 16)
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &sysprep.Sysprep{},
							},
						}
					},
					expectAllowed: true,
				},
			),
		)
	})

	When("the update is performed while object deletion", func() {
		It("should allow the request", func() {
			bypassUpgradeCheck(
				&ctx.WebhookRequestContext.Context,
				ctx.WebhookRequestContext.Obj,
				ctx.WebhookRequestContext.OldObj)
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})

	Context("HardwareVersion", func() {

		DescribeTable("MinHardwareVersion", doTest,
			Entry("disallow greater than max valid hardware version",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.MinHardwareVersion = int32(vimtypes.MaxValidHardwareVersion + 1)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.minHardwareVersion: Invalid value: %d: should be less than or equal to %d`,
							int32(vimtypes.MaxValidHardwareVersion+1), int32(vimtypes.MaxValidHardwareVersion)),
					),
					expectAllowed: false,
				},
			),

			Entry("allow same version",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 17
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("allow upgrade",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow downgrade",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 19
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 17
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						`spec.minHardwareVersion: Invalid value: 17: cannot downgrade hardware version`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow if powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow if powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.minHardwareVersion: Invalid value: 19: cannot upgrade hardware version unless powered off`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow if suspended",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					},
					validate: doValidateWithMsg(
						`spec.minHardwareVersion: Invalid value: 19: cannot upgrade hardware version unless powered off`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow if powered on but updating to powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("allow if powered off but updating to powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					expectAllowed: true,
				},
			),

			Entry("allow if suspended but updating to powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.MinHardwareVersion = 17
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
						ctx.vm.Spec.MinHardwareVersion = 19
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("GuestID", func() {
		const (
			guestID      = "vmwarePhoton64Guest"
			otherGuestID = "otherGuest64"
		)

		DescribeTable("GuestID update with different VM power states", doTest,

			Entry("allow if powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.GuestID = guestID
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.GuestID = otherGuestID
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow if powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.GuestID = guestID
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.GuestID = otherGuestID
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.guestID: Forbidden: updates to this field is not allowed when VM power is on`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow if powered on but updating to powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.GuestID = guestID
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.GuestID = otherGuestID
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("allow if powered off but updating to powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.GuestID = guestID
						ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.GuestID = otherGuestID
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("CD-ROM", func() {

		DescribeTable("CD-ROM update", doTest,

			Entry("allow adding CD-ROM when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom = append(
							ctx.vm.Spec.Hardware.Cdrom,
							vmopv1.VirtualMachineCdromSpec{
								Name: "new",
								Image: vmopv1.VirtualMachineImageRef{
									Name: "vmi-new",
									Kind: vmiKind,
								},
							},
						)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow adding CD-ROM when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom = append(
							ctx.vm.Spec.Hardware.Cdrom,
							vmopv1.VirtualMachineCdromSpec{
								Name: "new2",
								Image: vmopv1.VirtualMachineImageRef{
									Name: "vmi-new",
									Kind: vmiKind,
								},
							},
						)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom: Forbidden: updates to this field is not allowed when VM power is on`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow removing CD-ROM when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow removing CD-ROM when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{},
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom: Forbidden: updates to this field is not allowed when VM power is on`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow changing CD-ROM name when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].Name = "cdromNew"
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow adding CD-ROM when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].Name = "new3"
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].name: Forbidden: adding new CD-ROMs is not allowed when VM is powered on`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow removing CD-ROMs when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom: Forbidden: updates to this field is not allowed when VM power is on`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow changing CD-ROM image ref when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].Image = vmopv1.VirtualMachineImageRef{
							Name: "cvmi-new",
							Kind: cvmiKind,
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow changing CD-ROM image ref when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].Image = vmopv1.VirtualMachineImageRef{
							Name: "cvmi-new",
							Kind: cvmiKind,
						}
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].image: Forbidden: updates to this field is not allowed when VM power is on`,
					),
					expectAllowed: false,
				},
			),

			Entry("allow changing CD-ROM connection when VM is powered on",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						oldConnected := ptr.Deref(ctx.oldVM.Spec.Hardware.Cdrom[0].Connected)
						oldAllowGuestControl := ptr.Deref(ctx.oldVM.Spec.Hardware.Cdrom[0].AllowGuestControl)
						ctx.vm.Spec.Hardware.Cdrom[0].Connected = ptr.To(!oldConnected)
						ctx.vm.Spec.Hardware.Cdrom[0].AllowGuestControl = ptr.To(!oldAllowGuestControl)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					},
					expectAllowed: true,
				},
			),

			Entry("allow changing CD-ROM controller spec when status.PowerState is empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMSharedDisks = true
						})
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
						ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(1))
						ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(1))
					},
					expectAllowed: true,
				},
			),

			Entry("allow changing CD-ROM connection when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						oldConnected := ptr.Deref(ctx.oldVM.Spec.Hardware.Cdrom[0].Connected)
						oldAllowGuestControl := ptr.Deref(ctx.oldVM.Spec.Hardware.Cdrom[0].AllowGuestControl)
						ctx.vm.Spec.Hardware.Cdrom[0].Connected = ptr.To(!oldConnected)
						ctx.vm.Spec.Hardware.Cdrom[0].AllowGuestControl = ptr.To(!oldAllowGuestControl)
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					expectAllowed: true,
				},
			),

			Entry("disallow updating CD-ROM with invalid image ref kind when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind = "InvalidKind"
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[0].image.kind: Unsupported value: "InvalidKind": supported values: "VirtualMachineImage"`,
						`"ClusterVirtualMachineImage"`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow updating CD-ROM with duplicate image ref when VM is powered off",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Hardware.Cdrom[1].Image.Name = ctx.vm.Spec.Hardware.Cdrom[0].Image.Name
						ctx.vm.Spec.Hardware.Cdrom[1].Image.Kind = ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						`spec.hardware.cdrom[1].image.name: Duplicate value: "vmi-0123456789"`,
					),
					expectAllowed: false,
				},
			),
		)
	})

	Context("BootOptions", func() {
		DescribeTable("BootOptions update", doTest,

			Entry("allow empty bootOptions",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.BootOptions = nil
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting bootRetryDelay VM is PoweredOn",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions: Forbidden: updates to this field is not allowed when VM power is on",
					),
				},
			),

			Entry("disallow setting bootRetryDelay when bootRetry is unset",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.bootRetry: Required value: bootRetry must be set when setting bootRetryDelay",
					),
				},
			),

			Entry("disallow setting bootRetryDelay when bootRetry is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryDisabled,
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.bootRetry: Required value: bootRetry must be set when setting bootRetryDelay",
					),
				},
			),

			Entry("allow setting bootRetryDelay when bootRetry is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
							BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting efiSecureBoot when VM is PoweredOn",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
							Firmware:      vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions: Forbidden: updates to this field is not allowed when VM power is on",
					),
				},
			),

			Entry("disallow setting efiSecureBoot when firmware is unset",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.efiSecureBoot: Forbidden: cannot set efiSecureBoot when image firmware is not 'efi'",
					),
				},
			),

			Entry("disallow setting efiSecureBoot when firmware is BIOS",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
							Firmware:      vmopv1.VirtualMachineBootOptionsFirmwareTypeBIOS,
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions.efiSecureBoot: Forbidden: cannot set efiSecureBoot when image firmware is not 'efi'",
					),
				},
			),

			Entry("allow setting efiSecureBoot when firmware is EFI",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							Firmware:      vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
							EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting bootOrder when VM is PoweredOn",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								},
							},
						}
					},
					expectAllowed: false,
					validate: doValidateWithMsg(
						"spec.bootOptions: Forbidden: updates to this field is not allowed when VM power is on",
					),
				},
			),

			Entry("allow setting bootOrder when VM is PoweredOff",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow setting bootOrder when disk or network device is missing name",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								},
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow setting bootOrder when duplicate disk device is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow setting bootOrder when duplicate network device is present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow when specified network interface is not present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "new",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "vmi-new",
										Kind: vmiKind,
									},
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth1",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow when vm.spec.network is nil",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "new",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "vmi-new",
										Kind: vmiKind,
									},
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth1",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}

						ctx.vm.Spec.Network = nil
					},
					expectAllowed: false,
				},
			),

			Entry("disallow when specified disk is not present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "new",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "vmi-new",
										Kind: vmiKind,
									},
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-1",
								},
							},
						}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow when CD-ROM is specified, but not configured",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
						ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{}
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("disallow when type is unsupported",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "new",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "vmi-new",
										Kind: vmiKind,
									},
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: "usb",
									Name: "usb0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
						ctx.vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: false,
				},
			),

			Entry("allow when all specified devices are present",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "new",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "vmi-new",
										Kind: vmiKind,
									},
								},
							},
						}
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
								},
							},
						}
						ctx.vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
							},
						}
						ctx.vm.Status.PowerState = vmopv1.VirtualMachinePowerStateOff
						ctx.vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name: "disk-0",
							},
						}
					},
					expectAllowed: true,
				},
			),
		)
	})

	DescribeTable(
		"spec.className",
		doTest,

		//
		// RESIZE_CPU is disabled, FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled
		//

		Entry("always allow changes to non-class fields for classless VMs",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.ClassName = ""
					ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff

					ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					ctx.vm.Spec.ClassName = ""

					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
						config.Features.VMResizeCPUMemory = false
					})
				},
				expectAllowed: true,
			},
		),

		Entry("require spec.className for privileged user when FSS_RESIZE_CPU & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET are disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
						config.Features.VMResizeCPUMemory = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "className"), "", apivalidation.FieldImmutableErrorMsg).Error(),
				),
			},
		),
		Entry("require spec.className for unprivileged user when FSS_RESIZE_CPU & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET are disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
						config.Features.VMResizeCPUMemory = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "className"), "", apivalidation.FieldImmutableErrorMsg).Error(),
				),
			},
		),

		//
		// RESIZE_CPU is disabled, FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled
		//
		Entry("allow empty spec.className for privileged user when FSS_RESIZE_CPU is disabled & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
						config.Features.VMResizeCPUMemory = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "className"), "", apivalidation.FieldImmutableErrorMsg).Error(),
				),
			},
		),
		Entry("require spec.className for unprivileged user when FSS_RESIZE_CPU is disabled & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
						config.Features.VMResizeCPUMemory = false
					})
				},
				validate: doValidateWithMsg(
					field.Invalid(field.NewPath("spec", "className"), "", apivalidation.FieldImmutableErrorMsg).Error(),
				),
			},
		),

		//
		// RESIZE_CPU is enabled, FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled
		//
		Entry("require spec.className for privileged user when FSS_RESIZE_CPU is enabled & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
						config.Features.VMResizeCPUMemory = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),
		Entry("require spec.className for unprivileged user when FSS_RESIZE_CPU is enabled & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is disabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = false
						config.Features.VMResizeCPUMemory = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),

		//
		// RESIZE_CPU is enabled, FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled
		//
		Entry("allow empty spec.className for privileged user when FSS_RESIZE_CPU & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET are enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
						config.Features.VMResizeCPUMemory = true
					})
				},
				expectAllowed: true,
			},
		),
		Entry("require spec.className for unprivileged user when FSS_RESIZE_CPU & FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET are enabled",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.vm.Spec.ClassName = ""
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMImportNewNet = true
						config.Features.VMResizeCPUMemory = true
					})
				},
				validate: doValidateWithMsg(
					field.Required(field.NewPath("spec", "className"), "").Error(),
				),
			},
		),
	)

	Context("Network", func() {

		DescribeTable("network update", doTest,
			Entry("allow Network go from nil to not-nil",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network = nil
						ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
					},
					expectAllowed: true,
				},
			),
			Entry("allow Network go from not-nil to nil",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
						ctx.vm.Spec.Network = nil
					},
					expectAllowed: true,
				},
			),
		)

		Context("MutableNetworks is disabled", func() {

			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.MutableNetworks = false
				})
			})

			DescribeTable("disallow updates", doTest,
				Entry("disallow adding Network Interface",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
									},
									{
										Name: "eth1",
									},
								},
							}
						},
						validate: doValidateWithMsg(`spec.network.interfaces: Forbidden: network interfaces cannot be added or removed`),
					},
				),
				Entry("disallow Network Interface Name change",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth1",
									},
								},
							}
						},
						validate: doValidateWithMsg(`spec.network.interfaces[0].name: Forbidden: field is immutable`),
					},
				),
				Entry("disallow Network Interface Network ref change",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name:    "eth0",
										Network: &common.PartialObjectRef{Name: "net1"},
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name:    "eth0",
										Network: &common.PartialObjectRef{Name: "net99"},
									},
								},
							}
						},
						validate: doValidateWithMsg(`spec.network.interfaces[0].network: Forbidden: field is immutable`),
					},
				),
				Entry("disallow Network Interface MAC address change",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "foo",
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "bar",
									},
								},
							}
						},
						validate: doValidateWithMsg(`spec.network.interfaces[0].macAddr: Invalid value: "bar": field is immutable`),
					},
				),
			)
		})

		Context("MutableNetworks is enabled", func() {

			BeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.MutableNetworks = true
				})
			})

			DescribeTable("allow updates", doTest,

				// When Mutable_Networks Enabled
				Entry("allow adding Network Interface",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.MutableNetworks = true
							})

							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
									},
									{
										Name: "eth1",
									},
								},
							}
						},
						expectAllowed: true,
					},
				),
				Entry("allow Network Interface Network ref change",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.MutableNetworks = true
							})

							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name:    "eth0",
										Network: &common.PartialObjectRef{Name: "net1"},
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name:    "eth0",
										Network: &common.PartialObjectRef{Name: "net99"},
									},
								},
							}
						},
						expectAllowed: true,
					},
				),

				Entry("disallow Network Interface MAC address change",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.MutableNetworks = true
							})

							ctx.oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "foo",
									},
									{
										Name: "eth42",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "unchanged",
									},
									{
										Name: "eth99",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
									},
								},
							}
							ctx.vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
								Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
									{
										Name: "eth0",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "bar",
									},
									{
										Name: "eth42",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "unchanged",
									},
									{
										Name: "eth99",
										Network: &common.PartialObjectRef{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "crd.nsx.vmware.com/v1alpha1",
											},
											Name: "net1",
										},
										MACAddr: "my-new-mac",
									},
								},
							}
						},
						validate: doValidateWithMsg(
							`spec.network.interfaces[0].macAddr: Invalid value: "bar": field is immutable`,
							`spec.network.interfaces[2].macAddr: Invalid value: "my-new-mac": field is immutable`),
					},
				),
			)
		})
	})

	Context("check.vmoperator.vmware.com", func() {

		DescribeTable("poweron.check.vmoperator.vmware.com", doTest,

			Entry("allow adding annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow deleting annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{}
					},
					expectAllowed: true,
				},
			),

			Entry("allow modifying annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason1",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow adding annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app1: Forbidden: adding this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow deleting annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app1: Forbidden: removing this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow modifying annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason1",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app1: Forbidden: modifying this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow adding one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
							vmopv1.CheckAnnotationPowerOn + "/" + "app2": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app2: Forbidden: adding this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow deleting one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
							vmopv1.CheckAnnotationPowerOn + "/" + "app2": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app2: Forbidden: removing this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow modifying one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
							vmopv1.CheckAnnotationPowerOn + "/" + "app2": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationPowerOn + "/" + "app1": "reason",
							vmopv1.CheckAnnotationPowerOn + "/" + "app2": "reason1",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.poweron.check.vmoperator.vmware.com/app2: Forbidden: modifying this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),
		)

		DescribeTable("delete.check.vmoperator.vmware.com", doTest,

			Entry("allow adding annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow deleting annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{}
					},
					expectAllowed: true,
				},
			),

			Entry("allow modifying annotation for privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason1",
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow adding annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app1: Forbidden: adding this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow deleting annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app1: Forbidden: removing this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow modifying annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason1",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app1: Forbidden: modifying this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow adding one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
							vmopv1.CheckAnnotationDelete + "/" + "app2": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app2: Forbidden: adding this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow deleting one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
							vmopv1.CheckAnnotationDelete + "/" + "app2": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app2: Forbidden: removing this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),

			Entry("disallow modifying one annotation for non-privileged user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = false
						ctx.oldVM.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
							vmopv1.CheckAnnotationDelete + "/" + "app2": "reason",
						}
						ctx.vm.Annotations = map[string]string{
							vmopv1.CheckAnnotationDelete + "/" + "app1": "reason",
							vmopv1.CheckAnnotationDelete + "/" + "app2": "reason1",
						}
					},
					validate: doValidateWithMsg(
						`metadata.annotations.delete.check.vmoperator.vmware.com/app2: Forbidden: modifying this annotation is restricted to privileged users`,
					),
					expectAllowed: false,
				},
			),
		)
	})

	DescribeTable("Schema upgrade", doTest,

		Entry("disallow adding upgradedToBuildVersion annotation for non VM Op service account / system:masters",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.PrivilegedUsers = fake
					})
					ctx.UserInfo.Username = fake
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				validate: doValidateWithMsg(
					`metadata.annotations[vmoperator.vmware.com/upgraded-to-build-version]: Forbidden: modifying this annotation is restricted to privileged users`,
				),
			},
		),

		Entry("disallow adding upgradedToSchemaVersion annotation for non VM Op service account / system:masters",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = true
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.PrivilegedUsers = fake
					})
					ctx.UserInfo.Username = fake
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToSchemaVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				validate: doValidateWithMsg(
					`metadata.annotations[vmoperator.vmware.com/upgraded-to-schema-version]: Forbidden: modifying this annotation is restricted to privileged users`,
				),
			},
		),

		Entry("allow adding upgradedToBuildVersion annotation for VM Op service account",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					ctx.UserInfo.Username = strings.Join(
						[]string{
							"system",
							"serviceaccount",
							ctx.Namespace,
							ctx.ServiceAccountName,
						}, ":")
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow adding upgradedToSchemaVersion annotation for VM Op service account",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					ctx.UserInfo.Username = strings.Join(
						[]string{
							"system",
							"serviceaccount",
							ctx.Namespace,
							ctx.ServiceAccountName,
						}, ":")
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToSchemaVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow adding upgradedToBuildVersion annotation for system:masters",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					ctx.UserInfo.Groups = []string{"system:masters"}
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow adding upgradedToSchemaVersion annotation for system:masters",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					ctx.UserInfo.Groups = []string{"system:masters"}
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToSchemaVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("disallow BiosUUID change when upgradedToBuildVersion annotation is missing",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = fake
					})
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{}
					// Change a field that is validated during schema upgrade
					ctx.vm.Spec.BiosUUID = "new-bios-uuid"
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          false,
				validate: doValidateWithMsg(
					`spec.biosUUID: Forbidden: modifying this VM is not allowed until it is upgraded`,
				),
			},
		),

		Entry("disallow IDEControllers change when upgradedToSchemaVersion annotation is missing",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSharedDisks = true
					})
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{}
					// Change a field that is validated during schema upgrade
					if ctx.vm.Spec.Hardware == nil {
						ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
					}
					// we have two IDE controllers by default.
					ctx.vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          false,
				validate: doValidateWithMsg(
					`spec.hardware.ideControllers: Forbidden: modifying this VM is not allowed until it is upgraded`,
				),
			},
		),

		Entry("allow update when both VMs have nil Hardware",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSharedDisks = true
					})
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{}
					// Remove volumes to avoid volume-controller validation
					ctx.oldVM.Spec.Volumes = nil
					ctx.vm.Spec.Volumes = nil

					ctx.oldVM.Spec.Hardware = nil
					ctx.vm.Spec.Hardware = nil
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow change to unrestricted field when upgradedToBuildVersion annotation is missing",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = fake
					})
					ctx.oldVM.Annotations = map[string]string{}
					ctx.vm.Annotations = map[string]string{}
					// Change a field that is NOT validated during schema upgrade
					ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow change when both annotations are present",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = fake
					})
					ctx.oldVM.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey:  fake,
						pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
					}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey:  fake,
						pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow anyone to delete upgradedToBuildVersion",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = fake
					})
					ctx.oldVM.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey:  fake,
						pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
					}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),

		Entry("allow anyone to delete upgradedToSchemaVersion",
			testParams{
				setup: func(ctx *unitValidatingWebhookContext) {
					ctx.IsPrivilegedAccount = false
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.BuildVersion = fake
					})
					ctx.oldVM.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey:  fake,
						pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
					}
					ctx.vm.Annotations = map[string]string{
						pkgconst.UpgradedToBuildVersionAnnotationKey: fake,
					}
				},
				skipBypassUpgradeCheck: true,
				expectAllowed:          true,
			},
		),
	)

	Context("Snapshots", func() {
		snapshotPath := field.NewPath("spec", "currentSnapshotName")

		DescribeTable("currentSnapshot", doTest,
			Entry("when the VirtualSnapshot exists",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						vmSnapshot := builder.DummyVirtualMachineSnapshot(
							ctx.vm.Namespace,
							"dummy-vm-snapshot",
							ctx.vm.Name,
						)

						ctx.vm.Spec.CurrentSnapshotName = vmSnapshot.ObjectMeta.Name
					},
					expectAllowed: true,
				},
			),
			Entry("when the currentSnapshot Name is empty",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.CurrentSnapshotName = ""
					},
					validate:      nil, // Empty string is valid for CurrentSnapshot (means no revert)
					expectAllowed: true,
				},
			),
			Entry("when the VM is a VKS node attempting snapshot revert",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						vmSnapshot := builder.DummyVirtualMachineSnapshot(
							ctx.vm.Namespace,
							"dummy-vm-snapshot",
							ctx.vm.Name,
						)

						// Add CAPI labels to mark VM as VKS/TKG node
						if ctx.vm.Labels == nil {
							ctx.vm.Labels = make(map[string]string)
						}
						ctx.vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = "worker"

						ctx.vm.Spec.CurrentSnapshotName = vmSnapshot.Name
					},
					validate: doValidateWithMsg(
						field.Forbidden(snapshotPath, "snapshot revert is not allowed for VKS nodes").Error(),
					),
					expectAllowed: false,
				},
			),
			Entry("when a VM is being reverted to a snapshot, revert should be rejected",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.CurrentSnapshotName = "new-snap"
						ctx.oldVM.Spec.CurrentSnapshotName = "old-snap"
					},
					validate: doValidateWithMsg(
						field.Forbidden(snapshotPath, "a snapshot revert is already in progress").Error(),
					),
					expectAllowed: false,
				},
			),
		)
	})

	Context("GroupName", func() {
		DescribeTable("validateGroupName", doTest,
			Entry("disallow spec.groupName when VM Groups feature is disabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GroupName = dummyGroupName
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMGroups = false
						})
					},
					validate: func(response admission.Response) {
						Expect(string(response.Result.Reason)).To(Equal(field.Invalid(
							field.NewPath("spec", "groupName"),
							dummyGroupName,
							"the VM Groups feature is not enabled").Error()))
					},
					expectAllowed: false,
				},
			),
			Entry("allow spec.groupName when VM Groups feature is enabled",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.GroupName = dummyGroupName
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMGroups = true
						})
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Affinity", func() {
		DescribeTable("Updates", doTest,
			Entry("Immutable",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Affinity = &vmopv1.AffinitySpec{
							VMAffinity: &vmopv1.VMAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar1",
											},
										},
									},
								},
							},
						}

						ctx.oldVM.Spec.Affinity = &vmopv1.AffinitySpec{
							VMAffinity: &vmopv1.VMAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar2",
											},
										},
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(`spec.affinity: Forbidden: updating Affinity is not allowed`),
				},
			),
		)
	})

	Context("Volume PVC immutable fields", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
		})

		AfterEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
		})

		DescribeTable("Updates", doTest,
			Entry("should allow identical ApplicationType",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
						ctx.vm.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
					},
					expectAllowed: true,
				},
			),
			Entry("should deny ApplicationType change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
						ctx.vm.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeMicrosoftWSFC
					},
					validate: doValidateWithMsg(`spec.volumes[0].applicationType: Invalid value: "MicrosoftWSFC": field is immutable`),
				},
			),
			Entry("should deny ControllerBusNumber change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].ControllerBusNumber = ptr.To(int32(0))
						ctx.vm.Spec.Volumes[0].ControllerBusNumber = ptr.To(int32(1))
					},
					validate: doValidateWithMsg(`spec.volumes[0].controllerBusNumber: Invalid value: 1: field is immutable`),
				},
			),
			Entry("should deny ControllerType change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSCSI
						ctx.vm.Spec.Volumes[0].ControllerType = vmopv1.VirtualControllerTypeSATA
					},
					validate: doValidateWithMsg(`spec.volumes[0].controllerType: Invalid value: "SATA": field is immutable`),
				},
			),
			Entry("should deny DiskMode change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].DiskMode = vmopv1.VolumeDiskModePersistent
						ctx.vm.Spec.Volumes[0].DiskMode = vmopv1.VolumeDiskModeIndependentPersistent
					},
					validate: doValidateWithMsg(`spec.volumes[0].diskMode: Invalid value: "IndependentPersistent": field is immutable`),
				},
			),
			Entry("should deny SharingMode change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeNone
						ctx.vm.Spec.Volumes[0].SharingMode = vmopv1.VolumeSharingModeMultiWriter
					},
					validate: doValidateWithMsg(`spec.volumes[0].sharingMode: Invalid value: "MultiWriter": field is immutable`),
				},
			),
			Entry("should deny UnitNumber change",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].UnitNumber = ptr.To(int32(0))
						ctx.vm.Spec.Volumes[0].UnitNumber = ptr.To(int32(1))
					},
					validate: doValidateWithMsg(`spec.volumes[0].unitNumber: Invalid value: 1: field is immutable`),
				},
			),
			Entry("should allow when volume is replaced",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Spec.Volumes[0].Name = "old-pvc"
						ctx.vm.Spec.Volumes[0].Name = "new-pvc"
						ctx.oldVM.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeOracleRAC
						ctx.vm.Spec.Volumes[0].ApplicationType = vmopv1.VolumeApplicationTypeMicrosoftWSFC
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("PVC Volume Controller Fields", func() {
		const (
			newVolName = "new-vol"
			newPVCName = "new-pvc"
		)

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
		})

		AfterEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
		})

		DescribeTable("update", doTest,
			Entry("should deny UPDATE with missing unitNumber",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Add a new volume without unitNumber
						newVol := ctx.vm.Spec.Volumes[0].DeepCopy()
						newVol.Name = newVolName
						newVol.PersistentVolumeClaim.ClaimName = newPVCName
						newVol.UnitNumber = nil
						newVol.ControllerBusNumber = ptr.To(int32(1))
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, *newVol)
						// Add the controller to hardware spec for both oldVM and vm
						newController := vmopv1.SCSIControllerSpec{
							BusNumber: 1,
							Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
						}
						ctx.vm.Spec.Hardware.SCSIControllers = append(ctx.vm.Spec.Hardware.SCSIControllers, newController)
						ctx.oldVM.Spec.Hardware.SCSIControllers = append(ctx.oldVM.Spec.Hardware.SCSIControllers, newController)
					},
					expectAllowed:           false,
					skipSetControllerForPVC: true,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(1).Child("unitNumber"), "").Error(),
					),
				},
			),
			Entry("should deny UPDATE with missing controllerType",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Add a new volume without controllerType
						newVol := ctx.vm.Spec.Volumes[0].DeepCopy()
						newVol.Name = newVolName
						newVol.PersistentVolumeClaim.ClaimName = newPVCName
						newVol.ControllerType = ""
						newVol.UnitNumber = ptr.To(int32(1))
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, *newVol)
					},
					expectAllowed:           false,
					skipSetControllerForPVC: true,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(1).Child("controllerType"), "").Error(),
					),
				},
			),
			Entry("should deny UPDATE with missing controllerBusNumber",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Add a new volume without controllerBusNumber
						newVol := ctx.vm.Spec.Volumes[0].DeepCopy()
						newVol.Name = newVolName
						newVol.PersistentVolumeClaim.ClaimName = newPVCName
						newVol.ControllerBusNumber = nil
						newVol.UnitNumber = ptr.To(int32(1))
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, *newVol)
					},
					expectAllowed:           false,
					skipSetControllerForPVC: true,
					validate: doValidateWithMsg(
						field.Required(volPath.Index(1).Child("controllerBusNumber"), "").Error(),
					),
				},
			),
			Entry("should allow UPDATE with all controller fields set",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// All fields are already set by setControllerForPVC
					},
					expectAllowed: true,
				},
			),
		)
	})

	Context("Volume PVC UnitNumber conflicts with attached devices", func() {

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
		})

		AfterEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
		})

		DescribeTable("Updates", doTest,
			Entry("should allow new volume with unit number not in status",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Setup old VM with one volume
						ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "existing-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "existing-pvc",
										},
									},
								},
								UnitNumber:          ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
							},
						}

						// New VM adds a second volume with different unit number
						ctx.vm.Spec.Volumes = slices.Clone(ctx.oldVM.Spec.Volumes)
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
							Name: "new-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "new-pvc",
									},
								},
							},
							UnitNumber:          ptr.To(int32(1)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
						})

						// Setup status with existing device
						ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
							Controllers: []vmopv1.VirtualControllerStatus{
								{
									Type:      vmopv1.VirtualControllerTypeSCSI,
									BusNumber: 0,
									Devices: []vmopv1.VirtualDeviceStatus{
										{
											Type:       vmopv1.VirtualDeviceTypeDisk,
											UnitNumber: 0,
										},
									},
								},
							},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("should deny new volume with unit number already used by CD-ROM",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Setup old VM with one volume
						ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "existing-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "existing-pvc",
										},
									},
								},
								UnitNumber:          ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
							},
						}

						// New VM adds a second volume with conflicting unit number
						ctx.vm.Spec.Volumes = slices.Clone(ctx.oldVM.Spec.Volumes)
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
							Name: "new-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "new-pvc",
									},
								},
							},
							UnitNumber:          ptr.To(int32(5)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
						})

						// Add CD-ROM at unit five in spec.
						ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
							{
								BusNumber:   0,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						}
						ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
							{
								Name:                "cdrom1",
								Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso", Kind: "VirtualMachineImage"},
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(5)),
							},
						}
						ctx.oldVM.Spec.Hardware = ctx.vm.Spec.Hardware.DeepCopy()
					},
					validate: doValidateWithMsg(
						"controller unit number SCSI:0:5 is already in use",
					),
				},
			),
			Entry("should allow new volume on different controller even if same unit number exists",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Setup old VM with one volume
						ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "existing-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "existing-pvc",
										},
									},
								},
								UnitNumber:          ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
							},
						}
						ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
							{
								BusNumber:   0,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						}

						// New VM adds volume on SATA controller with same unit number
						ctx.vm.Spec.Volumes = slices.Clone(ctx.oldVM.Spec.Volumes)
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
							Name: "new-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "new-pvc",
									},
								},
							},
							UnitNumber:          ptr.To(int32(0)),
							ControllerType:      vmopv1.VirtualControllerTypeSATA,
							ControllerBusNumber: ptr.To(int32(0)),
						})

						ctx.vm.Spec.Hardware = ctx.oldVM.Spec.Hardware.DeepCopy()
						ctx.vm.Spec.Hardware.SATAControllers = append(
							ctx.vm.Spec.Hardware.SATAControllers,
							vmopv1.SATAControllerSpec{
								BusNumber: 0,
							},
						)

						// Setup status with SCSI device at unit 0
						ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
							Controllers: []vmopv1.VirtualControllerStatus{
								{
									Type:      vmopv1.VirtualControllerTypeSCSI,
									BusNumber: 0,
									Devices: []vmopv1.VirtualDeviceStatus{
										{
											Type:       vmopv1.VirtualDeviceTypeDisk,
											UnitNumber: 0,
										},
									},
								},
							},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("should allow new volume on different bus even if same unit number exists",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Setup old VM with one volume
						ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "existing-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "existing-pvc",
										},
									},
								},
								UnitNumber:          ptr.To(int32(3)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
							},
						}

						// Add SCSI controllers for both buses
						ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
							{
								BusNumber:   0,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
							{
								BusNumber:   1,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						}

						// New VM adds volume on different SCSI bus with same unit number
						ctx.vm.Spec.Volumes = slices.Clone(ctx.oldVM.Spec.Volumes)
						ctx.vm.Spec.Hardware = ctx.oldVM.Spec.Hardware.DeepCopy()
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
							Name: "new-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "new-pvc",
									},
								},
							},
							UnitNumber:          ptr.To(int32(3)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(1)),
						})

						// Setup status with device on bus 0
						ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
							Controllers: []vmopv1.VirtualControllerStatus{
								{
									Type:      vmopv1.VirtualControllerTypeSCSI,
									BusNumber: 0,
									Devices: []vmopv1.VirtualDeviceStatus{
										{
											Type:       vmopv1.VirtualDeviceTypeDisk,
											UnitNumber: 3,
										},
									},
								},
							},
						}
					},
					expectAllowed: true,
				},
			),
			Entry("should not check existing volumes against status",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						// Setup old VM with volume at unit 5
						ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "existing-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "existing-pvc",
										},
									},
								},
								UnitNumber:          ptr.To(int32(5)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								ControllerBusNumber: ptr.To(int32(0)),
							},
						}

						// New VM keeps the same volume (no changes)
						ctx.vm.Spec.Volumes = slices.Clone(ctx.oldVM.Spec.Volumes)

						// Setup status with device at unit 5 (the existing volume's device)
						// This should be allowed because we don't validate old volumes against status
						ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
							Controllers: []vmopv1.VirtualControllerStatus{
								{
									Type:      vmopv1.VirtualControllerTypeSCSI,
									BusNumber: 0,
									Devices: []vmopv1.VirtualDeviceStatus{
										{
											Type:       vmopv1.VirtualDeviceTypeDisk,
											UnitNumber: 5,
										},
									},
								},
							},
						}
					},
					expectAllowed: true,
				},
			),
		)
	})

	unitTestsValidateVolumeUnitNumber(doTest)

	Context("PVC Access Mode and Sharing Mode Combinations", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
		})

		AfterEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
		})

		DescribeTable("validate PVC access mode and sharing mode combinations",
			func(testName string,
				setup func(*unitValidatingWebhookContext),
				expectAllowed bool) {

				setup(ctx)

				// Set controllerBusNumber on volumes to simulate mutation webhook
				setControllerForPVC(ctx.vm)
				if ctx.oldVM != nil {
					setControllerForPVC(ctx.oldVM)
				}

				var err error
				ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
				Expect(err).ToNot(HaveOccurred())

				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(Equal(expectAllowed))
			},
			Entry("should reject when only volume is updated with invalid combination",
				"only-volume-updated-invalid",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Ensure oldVM and vm have the same hardware
					scsiController := vmopv1.SCSIControllerSpec{
						BusNumber:   0,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					}
					ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						scsiController,
					}
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						scsiController,
					}

					// Configure oldVM with a valid volume
					oldVol := vmopv1.VirtualMachineVolume{
						Name: "old-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dummyPVCName,
								},
							},
						},
						SharingMode: vmopv1.VolumeSharingModeNone,
					}
					ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{oldVol}

					// Configure vm with an invalid volume (ReadWriteOnce with MultiWriter)
					newVol := vmopv1.VirtualMachineVolume{
						Name: "new-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dummyPVCName,
								},
							},
						},
						SharingMode: vmopv1.VolumeSharingModeMultiWriter,
					}
					ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{newVol}
				},
				false,
			),

			Entry("should reject when only hardware is updated with invalid combination",
				"only-hardware-updated-invalid",
				func(ctx *unitValidatingWebhookContext) {
					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Ensure oldVM and vm have the same volume
					vol := vmopv1.VirtualMachineVolume{
						Name: "test-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dummyPVCName,
								},
							},
						},
						SharingMode:         vmopv1.VolumeSharingModeNone,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: ptr.To[int32](0),
					}
					ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{vol}
					ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{vol}

					// Configure oldVM with a valid controller (None sharing)
					oldScsiController := vmopv1.SCSIControllerSpec{
						BusNumber:   0,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					}
					ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						oldScsiController,
					}

					// Configure vm with an invalid controller (Physical sharing)
					newScsiController := vmopv1.SCSIControllerSpec{
						BusNumber:   0,
						SharingMode: vmopv1.VirtualControllerSharingModePhysical,
					}
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						newScsiController,
					}
				},
				false,
			),

			Entry("should allow when neither volume nor hardware has changed",
				"neither-volume-nor-hardware-changed",
				func(ctx *unitValidatingWebhookContext) {
					ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{
						{
							Name: "non-existent-volume",
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "non-existent-pvc",
									},
								},
							},
							SharingMode:         vmopv1.VolumeSharingModeNone,
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(1)),
						},
					}

					ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					}

					ctx.vm = ctx.oldVM.DeepCopy()
				},
				true,
			),

			Entry("should allow when privileged account updates with invalid combination",
				"privileged-account-invalid-combination",
				func(ctx *unitValidatingWebhookContext) {
					// Mark as privileged account
					ctx.IsPrivilegedAccount = true

					// Create a ReadWriteOnce PVC
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      dummyPVCName,
							Namespace: ctx.Namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
						},
					}
					Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())

					// Ensure oldVM and vm have the same hardware.
					ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					}
					ctx.vm.Spec.Hardware = ctx.oldVM.Spec.Hardware.DeepCopy()

					// Configure oldVM with a valid volume.
					oldVol := vmopv1.VirtualMachineVolume{
						Name: "old-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dummyPVCName,
								},
							},
						},
						SharingMode:         vmopv1.VolumeSharingModeNone,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: ptr.To(int32(1)),
						UnitNumber:          ptr.To(int32(0)),
					}
					ctx.oldVM.Spec.Volumes = []vmopv1.VirtualMachineVolume{oldVol}

					// Configure vm with an invalid volume (ReadWriteOnce with MultiWriter)
					// but should be allowed for privileged account.
					newVol := vmopv1.VirtualMachineVolume{
						Name: "new-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dummyPVCName,
								},
							},
						},
						SharingMode:         vmopv1.VolumeSharingModeMultiWriter,
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: ptr.To(int32(1)),
						UnitNumber:          ptr.To(int32(1)),
					}
					ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{newVol}
				},
				true,
			),
		)
	})
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

func unitTestsValidateVolumeUnitNumber(
	doTest func(testParams),
) {
	Context("Volume PVC UnitNumber", func() {
		type volumeConfig struct {
			unitNumber          *int32
			controllerType      vmopv1.VirtualControllerType
			controllerBusNumber *int32
		}

		setupFnForVolumeUnitNumberUpdates := func(
			volConfigs ...volumeConfig,
		) func(ctx *unitValidatingWebhookContext) {
			return func(ctx *unitValidatingWebhookContext) {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMSharedDisks = true
				})

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

				controllersToCreate := sets.New[pkgutil.ControllerID]()

				for i, cfg := range volConfigs {
					vol := vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("volume-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("pvc-%d", i),
								},
							},
						},
						UnitNumber:          cfg.unitNumber,
						ControllerType:      cfg.controllerType,
						ControllerBusNumber: cfg.controllerBusNumber,
					}
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vol)

					if cfg.controllerBusNumber != nil {
						controllersToCreate.Insert(pkgutil.ControllerID{
							ControllerType: cfg.controllerType,
							BusNumber:      *cfg.controllerBusNumber,
						})
					}
				}

				// Create controllers for all controllers referenced by volumes
				for controllerID := range controllersToCreate {
					switch controllerID.ControllerType {
					case vmopv1.VirtualControllerTypeSCSI:
						ctx.vm.Spec.Hardware.SCSIControllers = append(
							ctx.vm.Spec.Hardware.SCSIControllers,
							vmopv1.SCSIControllerSpec{
								BusNumber:   controllerID.BusNumber,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						)
					case vmopv1.VirtualControllerTypeSATA:
						ctx.vm.Spec.Hardware.SATAControllers = append(
							ctx.vm.Spec.Hardware.SATAControllers,
							vmopv1.SATAControllerSpec{
								BusNumber: controllerID.BusNumber,
							},
						)
					case vmopv1.VirtualControllerTypeNVME:
						ctx.vm.Spec.Hardware.NVMEControllers = append(
							ctx.vm.Spec.Hardware.NVMEControllers,
							vmopv1.NVMEControllerSpec{
								BusNumber: controllerID.BusNumber,
							},
						)
					case vmopv1.VirtualControllerTypeIDE:
						ctx.vm.Spec.Hardware.IDEControllers = append(
							ctx.vm.Spec.Hardware.IDEControllers,
							vmopv1.IDEControllerSpec{
								BusNumber: controllerID.BusNumber,
							},
						)
					}
				}
			}
		}

		DescribeTable("UnitNumber uniqueness per controller", doTest,
			Entry("should allow VM with no controller specified",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{},
						volumeConfig{},
					),
					expectAllowed: true,
				},
			),
			Entry("should allow VM with no unit numbers specified",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
					),
					expectAllowed: true,
				},
			),
			Entry("should allow VM with unique unit numbers on same controller",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(1)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
					),
					expectAllowed: true,
				},
			),
			Entry("should allow VM with same unit number on different controllers",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSATA,
							controllerBusNumber: ptr.To(int32(0)),
						},
					),
					expectAllowed: true,
				},
			),
			Entry("should allow VM with same unit number on different bus numbers",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(1)),
						},
					),
					expectAllowed: true,
				},
			),
			Entry("should deny VM with duplicate unit numbers on same controller",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							unitNumber:          ptr.To(int32(3)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(3)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
					),
					validate: doValidateWithMsg(
						"controller unit number SCSI:0:3 is already in use",
					),
				},
			),
			Entry("should deny VM with duplicate unit numbers across three volumes on same controller",
				testParams{
					setup: setupFnForVolumeUnitNumberUpdates(
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(1)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
						volumeConfig{
							unitNumber:          ptr.To(int32(0)),
							controllerType:      vmopv1.VirtualControllerTypeSCSI,
							controllerBusNumber: ptr.To(int32(0)),
						},
					),
					validate: doValidateWithMsg(
						"controller unit number SCSI:0:0 is already in use",
					),
				},
			),
		)
	})
}
