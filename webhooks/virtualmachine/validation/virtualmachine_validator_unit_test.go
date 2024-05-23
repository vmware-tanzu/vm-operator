// Copyright (c) 2019-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	updateSuffix                   = "-updated"
	dummyInstanceIDVal             = "dummy-instance-id"
	dummyFirstBootDoneVal          = "dummy-first-boot-done"
	dummyCreatedAtBuildVersionVal  = "dummy-created-at-build-version"
	dummyCreatedAtSchemaVersionVal = "dummy-created-at-schema-version"
	dummyPausedVMLabelVal          = "dummy-devops"
)

type testParams struct {
	setup         func(ctx *unitValidatingWebhookContext)
	validate      func(response admission.Response)
	expectAllowed bool
}

func doValidateWithMsg(msgs ...string) func(admission.Response) {
	return func(response admission.Response) {
		reasons := strings.Split(string(response.Result.Reason), ", ")
		for _, m := range msgs {
			ExpectWithOffset(1, reasons).To(ContainElement(m))
		}
		// This may be overly strict in some cases but catches missed assertions.
		ExpectWithOffset(1, reasons).To(HaveLen(len(msgs)))
	}
}

func unitTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.V1Alpha3,
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
	vm.Namespace = "dummy-vm-namespace-for-webhook-validation"
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	var oldVM *vmopv1.VirtualMachine
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVM = vm.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
	}

	zone := builder.DummyAvailabilityZone()
	initObjects := []client.Object{zone}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, initObjects...),
		vm:                                  vm,
		oldVM:                               oldVM,
	}
}

func unitTestsValidateCreate() {

	const (
		vmiKind          = "VirtualMachineImage"
		cvmiKind         = "Cluster" + vmiKind
		invalidImageKind = "supported: " + vmiKind + "; " + cvmiKind
	)

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
		isNoAvailabilityZones      bool
		isInvalidAvailabilityZone  bool
		isEmptyAvailabilityZone    bool
		powerState                 vmopv1.VirtualMachinePowerState
		nextRestartTime            string
		instanceUUID               string
		biosUUID                   string
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

		if args.isNoAvailabilityZones {
			Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
		}
		//nolint:gocritic // Ignore linter complaint about converting to switch case since the following is more readable.
		if args.isEmptyAvailabilityZone {
			delete(ctx.vm.Labels, topology.KubernetesTopologyZoneLabelKey)
		} else if args.isInvalidAvailabilityZone {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = "invalid"
		} else {
			zoneName := builder.DummyAvailabilityZoneName
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}

		ctx.vm.Spec.PowerState = args.powerState
		ctx.vm.Spec.NextRestartTime = args.nextRestartTime
		ctx.vm.Spec.InstanceUUID = args.instanceUUID
		ctx.vm.Spec.BiosUUID = args.biosUUID

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
		Entry("should deny invalid volume source spec", createArgs{invalidVolumeSource: true}, false,
			field.Required(volPath.Index(0).Child("persistentVolumeClaim"), "").Error(), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false,
			field.Required(volPath.Index(0).Child("persistentVolumeClaim", "claimName"), "").Error(), nil),
		Entry("should deny invalid PVC read only", createArgs{invalidPVCReadOnly: true}, false,
			field.NotSupported(volPath.Index(0).Child("persistentVolumeClaim", "readOnly"), true, []string{"false"}).Error(), nil),
		Entry("should deny when there are instance storage volumes and user is SSO user", createArgs{withInstanceStorageVolumes: true}, false,
			field.Forbidden(volPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow when there are instance storage volumes and user is service user", createArgs{isServiceUser: true, withInstanceStorageVolumes: true}, true, nil, nil),

		Entry("should allow when VM specifies no availability zone, there are availability zones", createArgs{isEmptyAvailabilityZone: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones", createArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true}, true, nil, nil),
		Entry("should allow when VM specifies valid availability zone, there are availability zones", createArgs{}, true, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are availability zones", createArgs{isInvalidAvailabilityZone: true}, false, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are no availability zones", createArgs{isInvalidAvailabilityZone: true, isNoAvailabilityZones: true}, false, nil, nil),
		Entry("should deny when there are no availability zones and WCP FaultDomains FSS is enabled", createArgs{isNoAvailabilityZones: true}, false, nil, nil),

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
		Entry("should allow creating VM with biosUUID set by admin user", createArgs{biosUUID: "uuid", isServiceUser: true}, true, nil, nil),
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
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKind).Error(),
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
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKind).Error(),
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
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKind).Error(),
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
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKind).Error(),
				),
			},
		),

		//
		// FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled
		//
		Entry("allow empty spec.image for privileged user when FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET is enabled",
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
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKind).Error(),
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
					field.Required(field.NewPath("spec", "image").Child("kind"), invalidImageKind).Error(),
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
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKind).Error(),
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
					field.Invalid(field.NewPath("spec", "image").Child("kind"), "invalid", invalidImageKind).Error(),
				),
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
					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error()),
				},
			),
			Entry("should allow creating VM with admin-only annotations set by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true

						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
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
					},
					expectAllowed: true,
				},
			),
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
							storageClass := builder.DummyStorageClass()
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								storageClass.Name+"-storagepolicyquota", ctx.vm.Namespace, storageClass.Name)
							Expect(ctx.Client.Create(ctx, storagePolicyQuota)).To(Succeed())
						},
						expectAllowed: true,
					},
				),
				Entry("storage class not associated with namespace",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							storageClass := builder.DummyStorageClass()
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								storageClass.Name+"-storagepolicyquota", ctx.vm.Namespace, storageClass.Name+"foo")
							Expect(ctx.Client.Create(ctx, storagePolicyQuota)).To(Succeed())
						},
						validate: doValidateWithMsg(
							`spec.storageClass: Invalid value: "dummy-storage-class": Storage policy is not associated with the namespace dummy-vm-namespace-for-webhook-validation`),
					},
				),
				Entry("WFFC storage class associated with namespace",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							storageClass := builder.DummyStorageClass()
							baseSCName := storageClass.Name
							storageClass.Name += "wffc"
							Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
							ctx.vm.Spec.StorageClass = storageClass.Name

							storagePolicyQuota := builder.DummyStoragePolicyQuota(
								baseSCName+"-storagepolicyquota", ctx.vm.Namespace, storageClass.Name)
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
						`spec.bootstrap.sysprep.sysprep: Invalid value: "identification": joinWorkgroup and domainAdmin/domainAdminPassword are mutually exclusive`,
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
						`spec.bootstrap.vAppConfig.properties.value: Invalid value: "value": from and value is mutually exclusive`,
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

			Entry("disallow global nameservers and search domains with CloudInit",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
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
						`spec.network.nameservers: Invalid value: "not-an-ip,8.8.8.8,2001:4860:4860::8888": nameservers is available only with the following bootstrap providers: LinuxPrep and Sysprep`,
						`spec.network.searchDomains: Invalid value: "dev.local": searchDomains is available only with the following bootstrap providers: LinuxPrep and Sysprep`,
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
						`spec.network.interfaces[1].name: Invalid value: "dummy-vm-dummy-nw-dummy_If": is the resulting network interface name: a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters`,
						`'-' or '.'`,
						`and must start and end with an alphanumeric character (e.g. 'example.com'`,
						"regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')"),
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
						ctx.vm.Spec.MinHardwareVersion = 22
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						`spec.minHardwareVersion: Invalid value: 22: should be less than or equal to 21`,
					),
					expectAllowed: false,
				},
			),
		)
	})
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

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
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		// Init immutable fields that aren't set in the dummy VM.
		if ctx.oldVM.Spec.Reserved == nil {
			ctx.oldVM.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
		}
		ctx.oldVM.Spec.Reserved.ResourcePolicyName = "policy"
		ctx.oldVM.Spec.InstanceUUID = args.oldInstanceUUID
		ctx.oldVM.Spec.BiosUUID = args.oldBiosUUID

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
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
		}
		if args.changeZoneName {
			ctx.oldVM.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName + updateSuffix
		}

		if args.withInstanceStorageVolumes {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}
		if args.changeInstanceStorageVolume {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.oldVM.Spec.Volumes = append(ctx.oldVM.Spec.Volumes, instanceStorageVolumes...)
			instanceStorageVolumes[0].Name += updateSuffix
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}

		if args.isSysprepTransportUsed {
			ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			if ctx.vm.Spec.Bootstrap == nil {
				ctx.vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
			}
			ctx.vm.Spec.Bootstrap.Sysprep = &vmopv1.VirtualMachineBootstrapSysprepSpec{
				RawSysprep: &common.SecretKeySelector{},
			}
		}

		if args.oldPowerState != "" {
			ctx.oldVM.Spec.PowerState = args.oldPowerState
		}
		if args.newPowerState != "" || args.newPowerStateEmptyAllowed {
			ctx.vm.Spec.PowerState = args.newPowerState
		}

		ctx.oldVM.Spec.NextRestartTime = args.lastRestartTime
		ctx.vm.Spec.NextRestartTime = args.nextRestartTime

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
	)

	doTest := func(args testParams) {
		args.setup(ctx)

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

	Context("Annotations", func() {
		annotationPath := field.NewPath("metadata", "annotations")

		DescribeTable("update", doTest,
			Entry("should disallow updating admin-only annotations by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(constants.CreatedAtBuildVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(constants.CreatedAtSchemaVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error()),
				},
			),
			Entry("should disallow removing admin-only annotations by SSO user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
					},
					validate: doValidateWithMsg(
						field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(constants.CreatedAtBuildVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error(),
						field.Forbidden(annotationPath.Child(constants.CreatedAtSchemaVersionAnnotationKey), "modifying this annotation is not allowed for non-admin users").Error()),
				},
			),
			Entry("should allow updating admin-only annotations by service user",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.IsPrivilegedAccount = true

						ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
						ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
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
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
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
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
						ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
						ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal + updateSuffix
						ctx.vm.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal + updateSuffix
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
						ctx.oldVM.Annotations[constants.CreatedAtBuildVersionAnnotationKey] = dummyCreatedAtBuildVersionVal
						ctx.oldVM.Annotations[constants.CreatedAtSchemaVersionAnnotationKey] = dummyCreatedAtSchemaVersionVal
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

	Context("ClassName", func() {

		DescribeTable("class name", doTest,

			Entry("disallow changing class name when FSS_WCP_VMSERVICE_RESIZE is disabled",
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
						ctx.vm.Spec.MinHardwareVersion = 22
						ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					},
					validate: doValidateWithMsg(
						`spec.minHardwareVersion: Invalid value: 22: should be less than or equal to 21`,
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
