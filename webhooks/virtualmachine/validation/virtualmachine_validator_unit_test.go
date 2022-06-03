// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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

func setConfigMap(namespace string, isRestrictedEnv bool) *corev1.ConfigMap {
	configMapIn := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.ProviderConfigMapName,
			Namespace: namespace,
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
		ctx                 *unitValidatingWebhookContext
		oldFaultDomainsFunc func() bool
	)

	const (
		bogusNetworkName = "bogus-network-name"
	)

	type createArgs struct {
		invalidImageName                     bool
		imageNotFound                        bool
		invalidClassName                     bool
		invalidNetworkName                   bool
		invalidNetworkType                   bool
		invalidNetworkCardType               bool
		multipleNetIfToSameNetwork           bool
		emptyVolumeName                      bool
		invalidVolumeName                    bool
		dupVolumeName                        bool
		invalidVolumeSource                  bool
		multipleVolumeSource                 bool
		invalidPVCName                       bool
		invalidPVCReadOnly                   bool
		invalidPVCHwVersion                  bool
		emptyMetadataResource                bool
		multipleMetadataResources            bool
		invalidVsphereVolumeSource           bool
		invalidVMVolumeProvOpts              bool
		invalidStorageClass                  bool
		notFoundStorageClass                 bool
		validStorageClass                    bool
		imageNonCompatible                   bool
		imageSupportCheckSkipAnnotation      bool
		imageNonCompatibleCloudInitTransport bool
		invalidReadinessNoProbe              bool
		invalidReadinessProbe                bool
		isRestrictedNetworkEnv               bool
		isRestrictedNetworkValidProbePort    bool
		isNonRestrictedNetworkEnv            bool
		isNoAvailabilityZones                bool
		isWCPFaultDomainsFSSEnabled          bool
		isUnifiedTKGFSSEnabled               bool
		isInvalidAvailabilityZone            bool
		isEmptyAvailabilityZone              bool
		isServiceUser                        bool
		addInstanceStorageVolumes            bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidClassName {
			ctx.vm.Spec.ClassName = ""
		}
		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
		}
		if args.imageNotFound {
			ctx.vm.Spec.ImageName = "image-does-not-exist"
		}
		if args.imageNonCompatible {
			ctx.vmImage.Status.ImageSupported = &[]bool{false}[0]
			Expect(ctx.Client.Status().Update(ctx, ctx.vmImage)).ToNot(HaveOccurred())
		}
		if args.imageSupportCheckSkipAnnotation {
			ctx.vm.Annotations[constants.VMOperatorImageSupportedCheckKey] = constants.VMOperatorImageSupportedCheckDisable
		}
		if args.imageNonCompatibleCloudInitTransport {
			ctx.vm.Spec.VmMetadata.Transport = vmopv1.VirtualMachineMetadataCloudInitTransport
		}
		if args.invalidNetworkName {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = ""
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = network.VdsNetworkType
		}
		if args.invalidNetworkType {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = bogusNetworkName
			ctx.vm.Spec.NetworkInterfaces[0].NetworkType = "bogusNetworkType"
		}
		if args.invalidNetworkCardType {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = bogusNetworkName
			ctx.vm.Spec.NetworkInterfaces[0].EthernetCardType = "bogusCardType"
		}
		if args.multipleNetIfToSameNetwork {
			ctx.vm.Spec.NetworkInterfaces[0].NetworkName = bogusNetworkName
			ctx.vm.Spec.NetworkInterfaces[1].NetworkName = bogusNetworkName
		}
		if args.emptyVolumeName {
			ctx.vm.Spec.Volumes[0].Name = ""
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
			ctx.vm.Spec.Volumes[0].VsphereVolume = nil
		}
		if args.multipleVolumeSource {
			ctx.vm.Spec.Volumes[0].PersistentVolumeClaim = &vmopv1.PersistentVolumeClaimVolumeSource{}
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
		if args.emptyMetadataResource {
			ctx.vm.Spec.VmMetadata.ConfigMapName = ""
			ctx.vm.Spec.VmMetadata.SecretName = ""
		}
		if args.multipleMetadataResources {
			ctx.vm.Spec.VmMetadata.ConfigMapName = "foo"
			ctx.vm.Spec.VmMetadata.SecretName = "bar"
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
		if args.invalidVMVolumeProvOpts {
			setProvOpts := true
			ctx.vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
				DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
					EagerZeroed:     &setProvOpts,
					ThinProvisioned: &setProvOpts,
				},
			}
		}
		if args.invalidStorageClass {
			// StorageClass specifies but not assigned to ResourceQuota.
			storageClass := builder.DummyStorageClass()
			ctx.vm.Spec.StorageClass = storageClass.Name
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
		}
		if args.notFoundStorageClass {
			// StorageClass specified but no ResourceQuotas.
			ctx.vm.Spec.StorageClass = builder.DummyStorageClassName
		}
		if args.validStorageClass {
			// StorageClass specified and is assigned to ResourceQuota.
			ctx.vm.Spec.StorageClass = builder.DummyStorageClassName
			storageClass := builder.DummyStorageClass()
			rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
			resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
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
			configMapIn := setConfigMap(ctx.Namespace, args.isRestrictedNetworkEnv)
			ctx.vm.Spec.ReadinessProbe = setReadinessProbe(args.isRestrictedNetworkValidProbePort)
			Expect(ctx.Client.Create(ctx, configMapIn)).To(Succeed())
		}
		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}
		if args.addInstanceStorageVolumes {
			instanceStorageVolume := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolume...)
		}
		// Please note this prevents the unit tests from running safely in parallel.
		lib.IsWcpFaultDomainsFSSEnabled = func() bool {
			return args.isWCPFaultDomainsFSSEnabled
		}
		lib.IsUnifiedTKGFSSEnabled = func() bool {
			return args.isUnifiedTKGFSSEnabled
		}
		if args.isNoAvailabilityZones {
			// Delete the dummy AZ.
			Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
		}
		// nolint:gocritic // Ignore linter complaint about converting to switch case since the following is more readable.
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
		oldFaultDomainsFunc = lib.IsWcpFaultDomainsFSSEnabled
	})

	AfterEach(func() {
		lib.IsWcpFaultDomainsFSSEnabled = oldFaultDomainsFunc
		ctx = nil
	})

	specPath := field.NewPath("spec")
	netIntPath := specPath.Child("networkInterfaces")
	volPath := specPath.Child("volumes")

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should deny invalid class name", createArgs{invalidClassName: true}, false,
			field.Required(specPath.Child("className"), "").Error(), nil),
		Entry("should deny invalid image name", createArgs{invalidImageName: true}, false,
			field.Required(specPath.Child("imageName"), "").Error(), nil),
		Entry("should deny image that does not exist", createArgs{imageNotFound: true}, false,
			field.Invalid(specPath.Child("imageName"), "image-does-not-exist", "").Error(), nil),
		Entry("should fail when Readiness probe has multiple actions", createArgs{invalidReadinessProbe: true}, false,
			field.Forbidden(specPath.Child("readinessProbe"), "only one action can be specified").Error(), nil),
		Entry("should fail when Readiness probe has no actions", createArgs{invalidReadinessNoProbe: true}, false,
			field.Forbidden(specPath.Child("readinessProbe"), "must specify an action").Error(), nil),

		Entry("should deny empty network name for VDS network type", createArgs{invalidNetworkName: true}, false,
			field.Required(netIntPath.Index(0).Child("networkName"), "").Error(), nil),
		Entry("should deny invalid network type", createArgs{invalidNetworkType: true}, false,
			field.NotSupported(netIntPath.Index(0).Child("networkType"), "bogusNetworkType", []string{network.NsxtNetworkType, network.VdsNetworkType}).Error(), nil),
		Entry("should deny invalid network card type", createArgs{invalidNetworkCardType: true}, false,
			field.NotSupported(netIntPath.Index(0).Child("ethernetCardType"), "bogusCardType", []string{"", "pcnet32", "e1000", "e1000e", "vmxnet2", "vmxnet3"}).Error(), nil),
		Entry("should deny connection of multiple network interfaces of a VM to the same network", createArgs{multipleNetIfToSameNetwork: true}, false,
			field.Duplicate(netIntPath.Index(1).Child("networkName"), bogusNetworkName).Error(), nil),

		Entry("should deny empty volume name", createArgs{emptyVolumeName: true}, false,
			field.Required(volPath.Index(0).Child("name"), "").Error(), nil),
		Entry("should deny invalid volume name", createArgs{invalidVolumeName: true}, false,
			field.Invalid(volPath.Index(0).Child("name"), "underscore_not_valid", validation.IsDNS1123Subdomain("underscore_not_valid")[0]).Error(), nil),
		Entry("should deny duplicated volume names", createArgs{dupVolumeName: true}, false,
			field.Duplicate(volPath.Index(1).Child("name"), "duplicate-name").Error(), nil),
		Entry("should deny invalid volume source spec", createArgs{invalidVolumeSource: true}, false,
			field.Forbidden(volPath.Index(0), "only one of persistentVolumeClaim or vsphereVolume must be specified").Error(), nil),
		Entry("should deny multiple volume source spec", createArgs{multipleVolumeSource: true}, false,
			field.Forbidden(volPath.Index(0), "only one of persistentVolumeClaim or vsphereVolume must be specified").Error(), nil),
		Entry("should deny invalid PVC name", createArgs{invalidPVCName: true}, false,
			field.Required(volPath.Index(0).Child("persistentVolumeClaim", "claimName"), "").Error(), nil),
		Entry("should deny invalid PVC read only", createArgs{invalidPVCReadOnly: true}, false,
			field.NotSupported(volPath.Index(0).Child("persistentVolumeClaim", "readOnly"), true, []string{"false"}).Error(), nil),
		Entry("should deny invalid PVC hardware version", createArgs{invalidPVCHwVersion: true}, false,
			field.Invalid(field.NewPath("spec", "imageName"), builder.DummyImageName, fmt.Sprintf("VirtualMachineImage has an unsupported hardware version %d for PersistentVolumes. Minimum supported hardware version %d", 12, constants.MinSupportedHWVersionForPVC)).Error(), nil),
		Entry("should deny invalid vsphere volume source spec", createArgs{invalidVsphereVolumeSource: true}, false,
			field.Invalid(volPath.Index(0).Child("vsphereVolume", "capacity", "ephemeral-storage"), resource.MustParse("1Ki"), "value must be a multiple of MB").Error(), nil),

		Entry("should deny invalid vm volume provisioning opts", createArgs{invalidVMVolumeProvOpts: true}, false,
			field.Forbidden(field.NewPath("spec", "advancedOptions", "defaultVolumeProvisioningOptions"), "Volume provisioning cannot have EagerZeroed and ThinProvisioning set. Eager zeroing requires thick provisioning").Error(), nil),

		Entry("should deny a storage class that does not exist", createArgs{notFoundStorageClass: true}, false,
			field.Invalid(specPath.Child("storageClass"), builder.DummyStorageClassName, fmt.Sprintf("Storage policy is not associated with the namespace %s", "")).Error(), nil),
		Entry("should deny a storage class that is not associated with the namespace", createArgs{invalidStorageClass: true}, false,
			field.Invalid(specPath.Child("storageClass"), builder.DummyStorageClassName, fmt.Sprintf("Storage policy is not associated with the namespace %s", "")).Error(), nil),
		Entry("should deny empty vmMetadata resource Names", createArgs{emptyMetadataResource: true}, false, "must specify either spec.vmMetadata.configMapName or spec.vmMetadata.secretName, but not both", nil),
		Entry("should deny when multiple vmMetadata resources are specified", createArgs{multipleMetadataResources: true}, false, "spec.vmMetadata.configMapName and spec.vmMetadata.secretName cannot be specified simultaneously", nil),
		Entry("should allow valid storage class and resource quota", createArgs{validStorageClass: true}, true, nil, nil),

		Entry("should fail when image is not compatible and UnifiedTKG FSS disabled", createArgs{imageNonCompatible: true}, false,
			field.Invalid(specPath.Child("imageName"), builder.DummyImageName, "VirtualMachineImage is not compatible with v1alpha1 or is not a TKG Image").Error(), nil),
		Entry("should allow despite incompatible image when UnifiedTKG FSS disabled and VMOperatorImageSupportedCheckKey is enabled", createArgs{imageSupportCheckSkipAnnotation: true, imageNonCompatible: true}, true, nil, nil),
		Entry("should allow despite incompatible image when UnifiedTKG FSS enabled", createArgs{isUnifiedTKGFSSEnabled: true, imageNonCompatible: true}, true, nil, nil),
		Entry("should allow despite incompatible image when VirtualMachineMetadataTransport is CloudInit", createArgs{imageNonCompatibleCloudInitTransport: true}, true, nil, nil),

		Entry("should fail when restricted network env is set in provider config map and TCP port in readiness probe is not 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, false,
			field.NotSupported(specPath.Child("readinessProbe", "tcpSocket", "port"), 443, []string{"6443"}).Error(), nil),
		Entry("should allow when restricted network env is set in provider config map and TCP port in readiness probe is 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: true}, true, nil, nil),
		Entry("should allow when restricted network env is not set in provider config map and TCP port in readiness probe is not 6443", createArgs{isNonRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, true, nil, nil),

		Entry("should allow when VM specifies no availability zone, there are availability zones, and WCP FaultDomains FSS is disabled", createArgs{isEmptyAvailabilityZone: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isEmptyAvailabilityZone: true, isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is enabled", createArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),

		Entry("should allow when VM specifies valid availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),
		Entry("should allow when VM specifies valid availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isNoAvailabilityZones: true}, true, nil, nil),

		Entry("should allow when VM specifies invalid availability zone, there are availability zones, and WCP FaultDomains FSS is disabled", createArgs{isInvalidAvailabilityZone: true}, true, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", createArgs{isInvalidAvailabilityZone: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),
		Entry("should allow when VM specifies invalid availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", createArgs{isInvalidAvailabilityZone: true, isNoAvailabilityZones: true}, true, nil, nil),
		Entry("should deny when VM specifies invalid availability zone, there are no availability zones, and WCP FaultDomains FSS is enabled", createArgs{isInvalidAvailabilityZone: true, isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),

		Entry("should deny when there are no availability zones and WCP FaultDomains FSS is enabled", createArgs{isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),
		Entry("should deny when there are instance storage volumes and user is SSO user", createArgs{addInstanceStorageVolumes: true}, false,
			field.Forbidden(volPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow when there are instance storage volumes and user is service user", createArgs{addInstanceStorageVolumes: true, isServiceUser: true}, true, nil, nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		changeClassName                 bool
		changeImageName                 bool
		changeStorageClass              bool
		changeResourcePolicy            bool
		assignZoneName                  bool
		changeZoneName                  bool
		changeInstanceStorageVolumeName bool
		isServiceUser                   bool
		addInstanceStorageVolume        bool
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
		if args.assignZoneName {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
		}
		if args.changeZoneName {
			ctx.oldVM.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName + updateSuffix
		}

		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}
		if args.addInstanceStorageVolume {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}
		if args.changeInstanceStorageVolumeName {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumes()
			ctx.oldVM.Spec.Volumes = append(ctx.oldVM.Spec.Volumes, instanceStorageVolumes...)
			instanceStorageVolumes[0].Name += updateSuffix
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}

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

	DescribeTable("update table", validateUpdate,
		// Immutable Fields
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny class name change", updateArgs{changeClassName: true}, false, msg, nil),
		Entry("should deny image name change", updateArgs{changeImageName: true}, false, msg, nil),
		Entry("should deny storageClass change", updateArgs{changeStorageClass: true}, false, msg, nil),
		Entry("should deny resourcePolicy change", updateArgs{changeResourcePolicy: true}, false, msg, nil),
		Entry("should allow initial zone assignment", updateArgs{assignZoneName: true}, true, nil, nil),
		Entry("should allow zone name change when WCP FaultDomains FSS is disabled", updateArgs{changeZoneName: true}, true, nil, nil),
		Entry("should deny instance storage volume name change, when user is SSO user", updateArgs{changeInstanceStorageVolumeName: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should deny adding new instance storage volume, when user is SSO user", updateArgs{addInstanceStorageVolume: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow adding new instance storage volume, when user type is service user", updateArgs{addInstanceStorageVolume: true, isServiceUser: true}, true, nil, nil),
		Entry("should allow instance storage volume name change, when user type is service user", updateArgs{changeInstanceStorageVolumeName: true, isServiceUser: true}, true, nil, nil),
	)

	When("the update is performed while object deletion", func() {
		It("should allow the request", func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
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
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
