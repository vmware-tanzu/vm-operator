// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vm    *vmopv1.VirtualMachine
	oldVM *vmopv1.VirtualMachine
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachine()
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	var oldVM *vmopv1.VirtualMachine
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVM = vm.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vm:                                  vm,
		oldVM:                               oldVM,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidImageName           bool
		invalidClassName           bool
		invalidNetworkName         bool
		invalidNetworkType         bool
		invalidVolumeName          bool
		dupVolumeName              bool
		invalidVolumeSource        bool
		multipleVolumeSource       bool
		invalidPVCName             bool
		invalidMetadataTransport   bool
		invalidMetadataConfigMap   bool
		invalidVsphereVolumeSource bool
		invalidVmVolumeProvOpts    bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidClassName {
			ctx.vm.Spec.ClassName = ""
		}
		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
		}
		if args.invalidNetworkName {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = ""
		}
		if args.invalidNetworkType {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = "bogusNetworkType"
		}
		if args.invalidVolumeName {
			ctx.vm.Spec.Volumes[0].Name = ""
		}
		if args.dupVolumeName {
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, ctx.vm.Spec.Volumes[0])
		}
		if args.invalidVolumeSource {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = nil
			ctx.vm.Spec.Volumes[0].VsphereVolume = nil
		}
		if args.multipleVolumeSource {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{}
			ctx.vm.Spec.Volumes[0].VsphereVolume = &vmopv1.VsphereVolumeSource{}
		}
		if args.invalidPVCName {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ClaimName = ""
		}
		if args.invalidMetadataTransport {
			ctx.vm.Spec.VmMetadata.Transport = "blah"
		}
		if args.invalidMetadataConfigMap {
			ctx.vm.Spec.VmMetadata.ConfigMapName = ""
		}
		if args.invalidVsphereVolumeSource {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = nil
			deviceKey := 2000
			ctx.vm.Spec.Volumes[0].VsphereVolume = &vmopv1.VsphereVolumeSource{
				DeviceKey: &deviceKey,
				Capacity: map[corev1.ResourceName]resource.Quantity{
					"ephemeral-storage": resource.MustParse("1Ki"),
				},
			}
		}
		if args.invalidVmVolumeProvOpts {
			setProvOpts := true
			ctx.vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
				DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
					EagerZeroed:     &setProvOpts,
					ThinProvisioned: &setProvOpts,
				},
			}
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
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

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should deny invalid class name", createArgs{invalidClassName: true}, false, messages.ClassNotSpecified, nil),
		Entry("should deny invalid image name", createArgs{invalidImageName: true}, false, messages.ImageNotSpecified, nil),
		Entry("should deny invalid network name", createArgs{invalidNetworkName: true}, false, fmt.Sprintf(messages.NetworkNameNotSpecifiedFmt, 0), nil),
		Entry("should deny invalid network type", createArgs{invalidNetworkType: true}, false, fmt.Sprintf(messages.NetworkTypeNotSupportedFmt, 0), nil),
		Entry("should deny invalid volume name", createArgs{invalidVolumeName: true}, false, fmt.Sprintf(messages.VolumeNameNotSpecifiedFmt, 0), nil),
		Entry("should deny duplicated volume names", createArgs{dupVolumeName: true}, false, fmt.Sprintf(messages.VolumeNameDuplicateFmt, 1), nil),
		Entry("should deny invalid volume source spec", createArgs{invalidVolumeSource: true}, false, fmt.Sprintf(messages.VolumeNotSpecifiedFmt, 0, 0), nil),
		Entry("should deny multiple volume source spec", createArgs{multipleVolumeSource: true}, false, fmt.Sprintf(messages.MultipleVolumeSpecifiedFmt, 0, 0), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false, fmt.Sprintf(messages.PersistentVolumeClaimNameNotSpecifiedFmt, 0), nil),
		Entry("should deny invalid vsphere volume source spec", createArgs{invalidVsphereVolumeSource: true}, false, fmt.Sprintf(messages.VsphereVolumeSizeNotMBMultipleFmt, 0), nil),
		Entry("should deny invalid vm volume provisioning opts", createArgs{invalidVmVolumeProvOpts: true}, false, fmt.Sprintf(messages.EagerZeroedAndThinProvisionedNotSupported), nil),
		Entry("should deny invalid vmmetadata transport", createArgs{invalidMetadataTransport: true}, false, messages.MetadataTransportNotSupported, nil),
		Entry("should deny invalid vmmetadata configmap", createArgs{invalidMetadataConfigMap: true}, false, messages.MetadataTransportConfigMapNotSpecified, nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		changeClassName bool
		changeImageName bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.changeClassName {
			ctx.vm.Spec.ClassName += "-updated"
		}
		if args.changeImageName {
			ctx.vm.Spec.ImageName += "-updated"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
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

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny class name change", updateArgs{changeClassName: true}, false, "updates to immutable fields are not allowed", nil),
		Entry("should deny image name change", updateArgs{changeImageName: true}, false, "updates to immutable fields are not allowed", nil),
	)

	When("the update is performed while object deletion", func() {
		JustBeforeEach(func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
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
			// BMV: Is this set at this point for Delete?
			//t := metav1.Now()
			//ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
