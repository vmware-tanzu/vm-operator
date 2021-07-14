// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation/messages"
)

const updateSuffix = "-updated"

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vm      *vmopv1.VirtualMachine
	oldVM   *vmopv1.VirtualMachine
	vmImage *vmopv1.VirtualMachineImage
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm-for-webhook-validation"
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	vmImage := builder.DummyVirtualMachineImage(vm.Spec.ImageName)
	vmImage1 := builder.DummyVirtualMachineImage(vm.Spec.ImageName + updateSuffix)
	zone := builder.DummyAvailabilityZone()

	var oldVM *vmopv1.VirtualMachine
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVM = vm.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, vmImage, vmImage1, zone),
		vm:                                  vm,
		oldVM:                               oldVM,
		vmImage:                             vmImage,
	}
}

func setConfigMap(isRestrictedEnv bool) *corev1.ConfigMap {
	configMapIn := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ProviderConfigMapName,
			Namespace: "namespace",
		},
		Data: make(map[string]string),
	}
	if isRestrictedEnv {
		configMapIn.Data["IsRestrictedNetwork"] = "true"
	}
	return configMapIn
}

func setReadinessProbe(validPortProbe bool) *vmopv1.Probe {
	portValue := 6443
	if !validPortProbe {
		portValue = 443
	}
	return &vmopv1.Probe{
		TCPSocket: &vmopv1.TCPSocketAction{Port: intstr.FromInt(portValue)},
	}
}

// nolint:gocyclo
func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidImageName                  bool
		invalidClassName                  bool
		invalidNetworkName                bool
		invalidNetworkType                bool
		invalidNetworkCardType            bool
		multipleNetIfToSameNetwork        bool
		emptyVolumeName                   bool
		invalidVolumeName                 bool
		dupVolumeName                     bool
		invalidVolumeSource               bool
		multipleVolumeSource              bool
		invalidPVCName                    bool
		invalidPVCReadOnly                bool
		invalidPVCHwVersion               bool
		invalidMetadataConfigMap          bool
		invalidVsphereVolumeSource        bool
		invalidVmVolumeProvOpts           bool
		invalidStorageClass               bool
		invalidResourceQuota              bool
		validStorageClass                 bool
		imageNonCompatible                bool
		invalidReadinessNoProbe           bool
		invalidReadinessProbe             bool
		isRestrictedNetworkEnv            bool
		isRestrictedNetworkValidProbePort bool
		isNonRestrictedNetworkEnv         bool
		isNoAvailabilityZones             bool
		isWCPFaultDomainsFSSEnabled       bool
		isInvalidAvailabilityZone         bool
		isEmptyAvailabilityZone           bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidClassName {
			ctx.vm.Spec.ClassName = ""
		}
		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
		}
		if args.imageNonCompatible {
			ctx.vmImage.Status.ImageSupported = &[]bool{false}[0]
			Expect(ctx.Client.Status().Update(ctx, ctx.vmImage)).ToNot(HaveOccurred())
		}
		if args.invalidNetworkName {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = ""
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = network.VdsNetworkType
		}
		if args.invalidNetworkType {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = "bogusNetworkType"
		}
		if args.invalidNetworkCardType {
			ctx.vm.Spec.NetworkInterfaces[0].EthernetCardType = "bogusCardType"
		}
		if args.multipleNetIfToSameNetwork {
			ctx.vm.Spec.NetworkInterfaces[1].NetworkName = ctx.vm.Spec.NetworkInterfaces[0].NetworkName
		}
		if args.emptyVolumeName {
			ctx.vm.Spec.Volumes[0].Name = ""
		}
		if args.invalidVolumeName {
			ctx.vm.Spec.Volumes[0].Name = "underscore_not_valid"
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
		if args.invalidPVCReadOnly {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ReadOnly = true
		}
		if args.invalidPVCHwVersion {
			ctx.vmImage.Spec.HardwareVersion = 12
			Expect(ctx.Client.Update(ctx, ctx.vmImage)).ToNot(HaveOccurred())
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
		// StorageClass specifies but not assigned to ResourceQuota
		if args.invalidStorageClass {
			ctx.vm.Spec.StorageClass = "invalid"
			rlName := builder.DummyStorageClassName + ".storageclass.storage.k8s.io/persistentvolumeclaims"
			resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
		}
		// StorageClass specified but no ResourceQuotas
		if args.invalidResourceQuota {
			ctx.vm.Spec.StorageClass = builder.DummyStorageClassName
		}
		// StorageClass specified and is assigned to ResourceQuota
		if args.validStorageClass {
			ctx.vm.Spec.StorageClass = builder.DummyStorageClassName
			storageClass := builder.DummyStorageClass()
			rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
			resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
		}
		if args.invalidReadinessNoProbe {
			ctx.vm.Spec.ReadinessProbe = &vmopv1.Probe{}
		}
		if args.invalidReadinessProbe {
			ctx.vm.Spec.ReadinessProbe = &vmopv1.Probe{
				TCPSocket:      &vmopv1.TCPSocketAction{},
				GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
			}
		}
		if args.isRestrictedNetworkEnv || args.isNonRestrictedNetworkEnv {
			configMapIn := setConfigMap(args.isRestrictedNetworkEnv)
			ctx.vm.Spec.ReadinessProbe = setReadinessProbe(args.isRestrictedNetworkValidProbePort)
			Expect(ctx.Client.Create(ctx, configMapIn)).To(Succeed())
		}

		// Please note this prevents the unit tests from running safely in
		// parallel.
		if args.isWCPFaultDomainsFSSEnabled {
			os.Setenv(lib.WcpFaultDomainsFSS, lib.TrueString)
		} else {
			os.Setenv(lib.WcpFaultDomainsFSS, "")
		}

		if args.isNoAvailabilityZones {
			// Delete the dummy AZ.
			Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
		}

		if args.isEmptyAvailabilityZone {
			delete(ctx.vm.Labels, topology.KubernetesTopologyZoneLabelKey)
		} else if args.isInvalidAvailabilityZone {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = "invalid"
		} else {
			zoneName := builder.DummyAvailabilityZoneName
			if !lib.IsWcpFaultDomainsFSSEnabled() {
				zoneName = topology.DefaultAvailabilityZoneName
			}
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}

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
		Expect(os.Setenv(lib.VmopNamespaceEnv, "namespace")).To(Succeed())
	})
	AfterEach(func() {
		ctx = nil
		Expect(os.Unsetenv(lib.VmopNamespaceEnv)).To(Succeed())
	})

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should deny invalid class name", createArgs{invalidClassName: true}, false, messages.ClassNotSpecified, nil),
		Entry("should deny invalid image name", createArgs{invalidImageName: true}, false, messages.ImageNotSpecified, nil),
		Entry("should fail when Readiness probe has multiple actions", createArgs{invalidReadinessProbe: true}, false, fmt.Sprintf(messages.ReadinessProbeOnlyOneAction), nil),
		Entry("should fail when Readiness probe has no actions", createArgs{invalidReadinessNoProbe: true}, false, fmt.Sprintf(messages.ReadinessProbeNoActions), nil),
		Entry("should deny invalid network name for VDS network type", createArgs{invalidNetworkName: true}, false, fmt.Sprintf(messages.NetworkNameNotSpecifiedFmt, 0), nil),
		Entry("should deny invalid network type", createArgs{invalidNetworkType: true}, false, fmt.Sprintf(messages.NetworkTypeNotSupportedFmt, 0, network.NsxtNetworkType, network.VdsNetworkType), nil),
		Entry("should deny invalid network card type", createArgs{invalidNetworkCardType: true}, false, fmt.Sprintf(messages.NetworkTypeEthCardTypeNotSupportedFmt, 0), nil),
		Entry("should deny connection of multiple network interfaces of a VM to the same network", createArgs{multipleNetIfToSameNetwork: true},
			false, fmt.Sprintf(messages.MultipleNetworkInterfacesNotSupportedFmt, 1), nil),
		Entry("should deny empty volume name", createArgs{emptyVolumeName: true}, false, fmt.Sprintf(messages.VolumeNameNotSpecifiedFmt, 0), nil),
		Entry("should deny invalid volume name", createArgs{invalidVolumeName: true}, false, fmt.Sprintf(messages.VolumeNameNotValidObjectNameFmt, 0, ""), nil),
		Entry("should deny duplicated volume names", createArgs{dupVolumeName: true}, false, fmt.Sprintf(messages.VolumeNameDuplicateFmt, 1), nil),
		Entry("should deny invalid volume source spec", createArgs{invalidVolumeSource: true}, false, fmt.Sprintf(messages.VolumeNotSpecifiedFmt, 0, 0), nil),
		Entry("should deny multiple volume source spec", createArgs{multipleVolumeSource: true}, false, fmt.Sprintf(messages.MultipleVolumeSpecifiedFmt, 0, 0), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false, fmt.Sprintf(messages.PersistentVolumeClaimNameNotSpecifiedFmt, 0), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCReadOnly: true}, false, fmt.Sprintf(messages.PersistentVolumeClaimNameReadOnlyFmt, 0), nil),
		Entry("should deny invalid PVC hardware verion", createArgs{invalidPVCHwVersion: true}, false, fmt.Sprintf(messages.PersistentVolumeClaimHardwareVersionNotSupportedFmt, builder.DummyImageName, 12, 13), nil),
		Entry("should deny invalid vsphere volume source spec", createArgs{invalidVsphereVolumeSource: true}, false, fmt.Sprintf(messages.VsphereVolumeSizeNotMBMultipleFmt, 0), nil),
		Entry("should deny invalid vm volume provisioning opts", createArgs{invalidVmVolumeProvOpts: true}, false, fmt.Sprintf(messages.EagerZeroedAndThinProvisionedNotSupported), nil),
		Entry("should deny invalid vmMetadata configmap", createArgs{invalidMetadataConfigMap: true}, false, messages.MetadataTransportConfigMapNotSpecified, nil),
		Entry("should deny invalid resource quota", createArgs{invalidResourceQuota: true}, false, fmt.Sprintf(messages.NoResourceQuotaFmt, ""), nil),
		Entry("should deny invalid storage class", createArgs{invalidStorageClass: true}, false, fmt.Sprintf(messages.StorageClassNotAssignedFmt, "invalid", ""), nil),
		Entry("should allow valid storage class and resource quota", createArgs{validStorageClass: true}, true, nil, nil),
		Entry("should fail when image is not compatible", createArgs{imageNonCompatible: true}, false, fmt.Sprintf(messages.VirtualMachineImageNotSupported), nil),
		Entry("should fail when restricted network env is set in provider config map and TCP port in readiness probe is not 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, false, fmt.Sprintf(messages.ReadinessProbePortNotSupportedFmt, 6443), nil),
		Entry("should allow when restricted network env is set in provider config map and TCP port in readiness probe is 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: true}, true, nil, nil),
		Entry("should allow when restricted network env is not set in provider config map and TCP port in readiness probe is not 6443", createArgs{isNonRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, true, nil, nil),

		Entry("should allow when VM specifies no availability zone, there are availability zones, and WCP FaultDomains FSS is disabled", createArgs{isEmptyAvailabilityZone: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isEmptyAvailabilityZone: true, isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is enabled", createArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),

		Entry("should allow when VM specifies valid availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),
		Entry("should allow when VM specifies valid availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isNoAvailabilityZones: true}, true, nil, nil),

		Entry("should deny when VM specifies invalid availability zone, there are availability zones, and WCP FaultDomains FSS is disabled", createArgs{isInvalidAvailabilityZone: true}, false, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isInvalidAvailabilityZone: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isInvalidAvailabilityZone: true, isNoAvailabilityZones: true}, false, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are no availability zones, and WCP FaultDomains FSS is enabled", createArgs{isInvalidAvailabilityZone: true, isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),

		Entry("should deny when there are no availability zones and WCP FaultDomains FSS is enabled", createArgs{isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		changeClassName      bool
		changeImageName      bool
		changeStorageClass   bool
		changeResourcePolicy bool
		changeZoneName       bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.changeClassName {
			ctx.vm.Spec.ClassName += updateSuffix
		}
		if args.changeImageName {
			ctx.vm.Spec.ImageName += updateSuffix
		}
		if args.changeStorageClass {
			ctx.vm.Spec.StorageClass += updateSuffix
		}
		if args.changeResourcePolicy {
			ctx.vm.Spec.ResourcePolicyName = updateSuffix
		}
		if args.changeZoneName {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] += updateSuffix
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
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

	DescribeTable("update table", validateUpdate,
		// Immutable Fields
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny class name change", updateArgs{changeClassName: true}, false, "updates to immutable fields are not allowed: [spec.className]", nil),
		Entry("should deny image name change", updateArgs{changeImageName: true}, false, "updates to immutable fields are not allowed: [spec.imageName]", nil),
		Entry("should deny storageClass change", updateArgs{changeStorageClass: true}, false, "updates to immutable fields are not allowed: [spec.storageClass]", nil),
		Entry("should deny resourcePolicy change", updateArgs{changeResourcePolicy: true}, false, "updates to immutable fields are not allowed: [spec.resourcePolicyName]", nil),
		Entry("should deny zone name change", updateArgs{changeZoneName: true}, false, "updates to immutable fields are not allowed: [metadata.labels."+topology.KubernetesTopologyZoneLabelKey+"]", nil),
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
