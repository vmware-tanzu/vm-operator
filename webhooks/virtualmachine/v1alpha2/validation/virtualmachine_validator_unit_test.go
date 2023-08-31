// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	updateSuffix = "-updated"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vm, oldVM *vmopv1.VirtualMachine
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachineA2()
	vm.Name = "dummy-vm-for-webhook-validation"
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

//nolint:gocyclo
func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		isServiceUser                     bool
		invalidImageName                  bool
		invalidClassName                  bool
		invalidVolumeName                 bool
		dupVolumeName                     bool
		invalidVolumeSource               bool
		invalidPVCName                    bool
		invalidPVCReadOnly                bool
		invalidStorageClass               bool
		notFoundStorageClass              bool
		validStorageClass                 bool
		withInstanceStorageVolumes        bool
		invalidReadinessProbe             bool
		isRestrictedNetworkEnv            bool
		isRestrictedNetworkValidProbePort bool
		isNonRestrictedNetworkEnv         bool
		isNoAvailabilityZones             bool
		isWCPFaultDomainsFSSEnabled       bool
		isInvalidAvailabilityZone         bool
		isEmptyAvailabilityZone           bool
		isBootstrapCloudInit              bool
		isBootstrapCloudInitInline        bool
		isBootstrapLinuxPrep              bool
		isSysprepFeatureEnabled           bool
		isBootstrapSysPrep                bool
		isBootstrapSysPrepInline          bool
		isBootstrapVAppConfig             bool
		isBootstrapVAppConfigInline       bool
		powerState                        vmopv1.VirtualMachinePowerState
		nextRestartTime                   string
		isForbiddenAnnotation             bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}

		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
		}
		if args.invalidClassName {
			ctx.vm.Spec.ClassName = ""
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
			storageClass := builder.DummyStorageClass()
			Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())
			ctx.vm.Spec.StorageClass = storageClass.Name

			rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"
			resourceQuota := builder.DummyResourceQuota(ctx.vm.Namespace, rlName)
			Expect(ctx.Client.Create(ctx, resourceQuota)).To(Succeed())
		}

		if args.withInstanceStorageVolumes {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumesA2()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}

		if args.invalidReadinessProbe {
			ctx.vm.Spec.ReadinessProbe = vmopv1.VirtualMachineReadinessProbeSpec{
				TCPSocket:      &vmopv1.TCPSocketAction{},
				GuestHeartbeat: &vmopv1.GuestHeartbeatAction{},
			}
		}
		if args.isRestrictedNetworkEnv || args.isNonRestrictedNetworkEnv {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.ProviderConfigMapName,
					Namespace: ctx.Namespace,
				},
				Data: make(map[string]string),
			}
			if args.isRestrictedNetworkEnv {
				cm.Data["IsRestrictedNetwork"] = "true"
			}
			Expect(ctx.Client.Create(ctx, cm)).To(Succeed())

			portValue := 6443
			if !args.isRestrictedNetworkValidProbePort {
				portValue = 443
			}
			ctx.vm.Spec.ReadinessProbe = vmopv1.VirtualMachineReadinessProbeSpec{
				TCPSocket: &vmopv1.TCPSocketAction{Port: intstr.FromInt(portValue)},
			}
		}

		if args.isWCPFaultDomainsFSSEnabled {
			Expect(os.Setenv(lib.WcpFaultDomainsFSS, "true")).To(Succeed())
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
			if !lib.IsWcpFaultDomainsFSSEnabled() {
				zoneName = topology.DefaultAvailabilityZoneName
			}
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}

		if args.isBootstrapCloudInit || args.isBootstrapCloudInitInline {
			ctx.vm.Spec.Bootstrap.CloudInit = &vmopv1.VirtualMachineBootstrapCloudInitSpec{}
			if args.isBootstrapCloudInit {
				ctx.vm.Spec.Bootstrap.CloudInit.RawCloudConfig.Key = "cloud-init-key"
			}
			if args.isBootstrapCloudInitInline {
				ctx.vm.Spec.Bootstrap.CloudInit.CloudConfig.Timezone = " dummy-tz"
			}
		}
		if args.isBootstrapLinuxPrep {
			ctx.vm.Spec.Bootstrap.LinuxPrep = &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{}
		}
		if args.isSysprepFeatureEnabled {
			Expect(os.Setenv(lib.WindowsSysprepFSS, "true")).To(Succeed())
		}
		if args.isBootstrapSysPrep || args.isBootstrapSysPrepInline {
			ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			ctx.vm.Spec.Bootstrap.Sysprep = &vmopv1.VirtualMachineBootstrapSysprepSpec{}
			if args.isBootstrapSysPrep {
				ctx.vm.Spec.Bootstrap.Sysprep.RawSysprep.Key = "sysprep-key"
			}
			if args.isBootstrapSysPrepInline {
				ctx.vm.Spec.Bootstrap.Sysprep.Sysprep.GUIRunOnce.Commands = []string{"hello"}
			}
		}
		if args.isBootstrapVAppConfig || args.isBootstrapVAppConfigInline {
			ctx.vm.Spec.Bootstrap.VAppConfig = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{}
			if args.isBootstrapVAppConfig {
				ctx.vm.Spec.Bootstrap.VAppConfig.RawProperties = "some-vapp-prop"
			}
			if args.isBootstrapVAppConfigInline {
				ctx.vm.Spec.Bootstrap.VAppConfig.Properties = []common.KeyValueOrSecretKeySelectorPair{
					{
						Key: "key",
					},
				}
			}
		}

		if args.isForbiddenAnnotation {
			ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = "some-value"
		}

		ctx.vm.Spec.PowerState = args.powerState
		ctx.vm.Spec.NextRestartTime = args.nextRestartTime

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
		Expect(os.Unsetenv(lib.WcpFaultDomainsFSS)).To(Succeed())
		Expect(os.Unsetenv(lib.WindowsSysprepFSS)).To(Succeed())
		ctx = nil
	})

	specPath := field.NewPath("spec")
	volPath := specPath.Child("volumes")
	nextRestartTimePath := specPath.Child("nextRestartTime")
	now := time.Now().UTC()
	annotationPath := field.NewPath("metadata", "annotations")

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),

		Entry("should deny invalid class name", createArgs{invalidClassName: true}, false,
			field.Required(specPath.Child("className"), "").Error(), nil),
		Entry("should deny invalid image name", createArgs{invalidImageName: true}, false,
			field.Required(specPath.Child("imageName"), "").Error(), nil),

		Entry("should fail when Readiness probe has multiple actions", createArgs{invalidReadinessProbe: true}, false,
			field.Forbidden(specPath.Child("readinessProbe"), "only one action can be specified").Error(), nil),

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
		Entry("should deny a StorageClass that does not exist", createArgs{notFoundStorageClass: true}, false,
			field.Invalid(specPath.Child("storageClass"), builder.DummyStorageClassName, fmt.Sprintf("Storage policy is not associated with the namespace %s", "")).Error(), nil),
		Entry("should deny a StorageClass that is not associated with the namespace", createArgs{invalidStorageClass: true}, false,
			field.Invalid(specPath.Child("storageClass"), builder.DummyStorageClassName, fmt.Sprintf("Storage policy is not associated with the namespace %s", "")).Error(), nil),
		Entry("should allow valid storage class and resource quota", createArgs{validStorageClass: true}, true, nil, nil),
		Entry("should deny when there are instance storage volumes and user is SSO user", createArgs{withInstanceStorageVolumes: true}, false,
			field.Forbidden(volPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow when there are instance storage volumes and user is service user", createArgs{isServiceUser: true, withInstanceStorageVolumes: true}, true, nil, nil),

		Entry("should deny when restricted network and TCP port in readiness probe is not 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, false,
			field.NotSupported(specPath.Child("readinessProbe", "tcpSocket", "port"), 443, []string{"6443"}).Error(), nil),
		Entry("should allow when restricted network and TCP port in readiness probe is 6443", createArgs{isRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: true}, true, nil, nil),
		Entry("should allow when not restricted network and TCP port in readiness probe is not 6443", createArgs{isNonRestrictedNetworkEnv: true, isRestrictedNetworkValidProbePort: false}, true, nil, nil),

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

		Entry("should allow CloudInit", createArgs{isBootstrapCloudInit: true}, true, nil, nil),
		Entry("should deny CloudInit with raw and inline", createArgs{isBootstrapCloudInit: true, isBootstrapCloudInitInline: true}, false, "cloudConfig and rawCloudConfig are mutually exclusive", nil),
		Entry("should deny CloudInit with LinuxPrep", createArgs{isBootstrapCloudInit: true, isBootstrapLinuxPrep: true}, false, "CloudInit may not be used with any other bootstrap provider", nil),
		Entry("should deny CloudInit with SysPrep", createArgs{isBootstrapCloudInit: true, isBootstrapSysPrep: true}, false, "CloudInit may not be used with any other bootstrap provider", nil),
		Entry("should deny CloudInit with vApp", createArgs{isBootstrapCloudInit: true, isBootstrapVAppConfig: true}, false, "CloudInit may not be used with any other bootstrap provider", nil),

		Entry("should allow LinuxPrep", createArgs{isBootstrapLinuxPrep: true}, true, nil, nil),
		Entry("should deny LinuxPrep with CloudInit", createArgs{isBootstrapLinuxPrep: true, isBootstrapCloudInit: true}, false, "LinuxPrep may not be used with either CloudInit or Sysprep bootstrap providers", nil),
		Entry("should deny LinuxPrep with SysPrep", createArgs{isBootstrapLinuxPrep: true, isBootstrapSysPrep: true}, false, "LinuxPrep may not be used with either CloudInit or Sysprep bootstrap providers", nil),

		Entry("should allow sysprep when FSS is enabled", createArgs{isSysprepFeatureEnabled: true, isBootstrapSysPrep: true}, true, nil, nil),
		Entry("should disallow sysprep when FSS is disabled", createArgs{isSysprepFeatureEnabled: false, isBootstrapSysPrep: true}, false,
			field.Invalid(specPath.Child("bootstrap", "sysprep"), "sysprep", "the sysprep feature is not enabled").Error(), nil),
		Entry("should deny sysprep with CloudInit", createArgs{isSysprepFeatureEnabled: true, isBootstrapSysPrep: true, isBootstrapCloudInit: true}, false, nil, nil),

		Entry("should allow vApp", createArgs{isBootstrapVAppConfig: true}, true, nil, nil),
		Entry("should deny with raw and inline", createArgs{isBootstrapVAppConfig: true, isBootstrapVAppConfigInline: true}, false, "properties and rawProperties are mutually exclusive", nil),
		Entry("should allow vApp with LinuxPrep", createArgs{isBootstrapVAppConfig: true, isBootstrapLinuxPrep: true}, true, nil, nil),
		Entry("should allow vApp with SysPrep", createArgs{isBootstrapVAppConfig: true, isSysprepFeatureEnabled: true, isBootstrapSysPrep: true}, true, nil, nil),

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
		Entry("should deny cloud-init-instance-id annotation set by user", createArgs{isForbiddenAnnotation: true}, false,
			field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "adding this annotation is not allowed").Error(), nil),
		Entry("should allow cloud-init-instance-id annotation set by service user", createArgs{isServiceUser: true, isForbiddenAnnotation: true}, true, nil, nil),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		isServiceUser               bool
		changeClassName             bool
		changeImageName             bool
		changeStorageClass          bool
		changeResourcePolicy        bool
		assignZoneName              bool
		changeZoneName              bool
		isSysprepFeatureEnabled     bool
		isSysprepTransportUsed      bool
		withInstanceStorageVolumes  bool
		changeInstanceStorageVolume bool
		oldPowerState               vmopv1.VirtualMachinePowerState
		newPowerState               vmopv1.VirtualMachinePowerState
		newPowerStateEmptyAllowed   bool
		nextRestartTime             string
		lastRestartTime             string
		isForbiddenAnnotation       bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.isServiceUser {
			ctx.IsPrivilegedAccount = true
		}

		if args.changeImageName {
			ctx.vm.Spec.ImageName += updateSuffix
		}
		if args.changeClassName {
			ctx.vm.Spec.ClassName += updateSuffix
		}
		if args.changeStorageClass {
			ctx.vm.Spec.StorageClass += updateSuffix
		}
		if args.changeResourcePolicy {
			ctx.vm.Spec.Reserved.ResourcePolicyName = updateSuffix
		}
		if args.assignZoneName {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
		}
		if args.changeZoneName {
			ctx.oldVM.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = builder.DummyAvailabilityZoneName + updateSuffix
		}

		if args.withInstanceStorageVolumes {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumesA2()
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}
		if args.changeInstanceStorageVolume {
			instanceStorageVolumes := builder.DummyInstanceStorageVirtualMachineVolumesA2()
			ctx.oldVM.Spec.Volumes = append(ctx.oldVM.Spec.Volumes, instanceStorageVolumes...)
			instanceStorageVolumes[0].Name += updateSuffix
			ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, instanceStorageVolumes...)
		}

		if args.isSysprepFeatureEnabled {
			Expect(os.Setenv(lib.WindowsSysprepFSS, "true")).To(Succeed())
		}
		if args.isSysprepTransportUsed {
			ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			ctx.vm.Spec.Bootstrap.Sysprep = &vmopv1.VirtualMachineBootstrapSysprepSpec{}
		}

		if args.oldPowerState != "" {
			ctx.oldVM.Spec.PowerState = args.oldPowerState
		}
		if args.newPowerState != "" || args.newPowerStateEmptyAllowed {
			ctx.vm.Spec.PowerState = args.newPowerState
		}

		if args.isForbiddenAnnotation {
			ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = "some-value"
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
		Expect(os.Unsetenv(lib.WcpFaultDomainsFSS)).To(Succeed())
		Expect(os.Unsetenv(lib.WindowsSysprepFSS)).To(Succeed())
	})

	msg := "field is immutable"
	volumesPath := field.NewPath("spec", "volumes")
	powerStatePath := field.NewPath("spec", "powerState")
	nextRestartTimePath := field.NewPath("spec", "nextRestartTime")
	annotationPath := field.NewPath("metadata", "annotations")

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),

		Entry("should deny image name change", updateArgs{changeImageName: true}, false, msg, nil),
		Entry("should deny class name change", updateArgs{changeClassName: true}, false, msg, nil),
		Entry("should deny storageClass change", updateArgs{changeStorageClass: true}, false, msg, nil),
		Entry("should deny resourcePolicy change", updateArgs{changeResourcePolicy: true}, false, msg, nil),

		Entry("should allow initial zone assignment", updateArgs{assignZoneName: true}, true, nil, nil),
		Entry("should allow zone name change when WCP FaultDomains FSS is disabled", updateArgs{changeZoneName: true}, true, nil, nil),

		Entry("should deny instance storage volume name change, when user is SSO user", updateArgs{changeInstanceStorageVolume: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should deny adding new instance storage volume, when user is SSO user", updateArgs{withInstanceStorageVolumes: true}, false,
			field.Forbidden(volumesPath, "adding or modifying instance storage volume claim(s) is not allowed").Error(), nil),
		Entry("should allow adding new instance storage volume, when user type is service user", updateArgs{withInstanceStorageVolumes: true, isServiceUser: true}, true, nil, nil),
		Entry("should allow instance storage volume name change, when user type is service user", updateArgs{changeInstanceStorageVolume: true, isServiceUser: true}, true, nil, nil),

		Entry("should allow sysprep when FSS is enabled", updateArgs{isSysprepFeatureEnabled: true, isSysprepTransportUsed: true}, true, nil, nil),
		Entry("should disallow sysprep when FSS is disabled", updateArgs{isSysprepFeatureEnabled: false, isSysprepTransportUsed: true}, false,
			field.Invalid(field.NewPath("spec", "bootstrap", "sysprep"), "sysprep", "the sysprep feature is not enabled").Error(), nil),
		Entry("should not error if sysprep FSS is disabled when sysprep is not used", updateArgs{isSysprepFeatureEnabled: false, isSysprepTransportUsed: false}, true, nil, nil),

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
		Entry("should deny cloud-init-instance-id annotation set by user", updateArgs{isForbiddenAnnotation: true}, false,
			field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "adding this annotation is not allowed").Error(), nil),
		Entry("should allow cloud-init-instance-id annotation set by service user", updateArgs{isServiceUser: true, isForbiddenAnnotation: true}, true, nil, nil),
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
