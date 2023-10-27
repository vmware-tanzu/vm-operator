// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"os"
	"strings"
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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	pkgbuilder "github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	updateSuffix          = "-updated"
	dummyInstanceIDVal    = "dummy-instance-id"
	dummyFirstBootDoneVal = "dummy-first-boot-done"
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
		ctx                           *unitValidatingWebhookContext
		oldVMServiceBackupRestoreFunc func() bool
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
		adminOnlyAnnotations              bool
		isPrivilegedUser                  bool
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

		if args.adminOnlyAnnotations {
			ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = updateSuffix
			ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = updateSuffix
		}

		if args.isPrivilegedUser {
			lib.IsVMServiceBackupRestoreFSSEnabled = func() bool {
				return true
			}

			fakeWCPUser := "sso:wcp-12345-fake-machineid-67890@vsphere.local"
			Expect(os.Setenv(lib.PrivilegedUsersEnv, fakeWCPUser)).To(Succeed())
			defer func() {
				Expect(os.Unsetenv(lib.PrivilegedUsersEnv)).To(Succeed())
			}()

			ctx.UserInfo.Username = fakeWCPUser
			ctx.IsPrivilegedAccount = pkgbuilder.IsPrivilegedAccount(ctx.WebhookContext, ctx.UserInfo)
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
		oldVMServiceBackupRestoreFunc = lib.IsVMServiceBackupRestoreFSSEnabled
	})

	AfterEach(func() {
		Expect(os.Unsetenv(lib.WcpFaultDomainsFSS)).To(Succeed())
		Expect(os.Unsetenv(lib.WindowsSysprepFSS)).To(Succeed())
		lib.IsVMServiceBackupRestoreFSSEnabled = oldVMServiceBackupRestoreFunc
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

		Entry("should disallow creating VM with admin-only annotations set by SSO user", createArgs{adminOnlyAnnotations: true}, false,
			strings.Join([]string{
				field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
				field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
			}, ", "), nil),
		Entry("should allow creating VM with admin-only annotations set by service user", createArgs{isServiceUser: true, adminOnlyAnnotations: true}, true, nil, nil),

		Entry("should allow creating VM with admin-only annotations set by WCP user when the Backup/Restore FSS is enabled", createArgs{adminOnlyAnnotations: true, isPrivilegedUser: true}, true, nil, nil),
	)

	Context("Network", func() {

		type testParams struct {
			setup         func(ctx *unitValidatingWebhookContext)
			validate      func(response admission.Response)
			expectAllowed bool
		}

		doTest := func(args testParams) {
			args.setup(ctx)

			var err error
			ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())

			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(Equal(args.expectAllowed))

			if args.validate != nil {
				args.validate(response)
			}
		}

		doValidateWithMsg := func(msgs ...string) func(admission.Response) {
			return func(response admission.Response) {
				reasons := strings.Split(string(response.Result.Reason), ", ")
				for _, m := range msgs {
					Expect(reasons).To(ContainElement(m))
				}
				// This may be overly strict in some cases but catches missed assertions.
				Expect(reasons).To(HaveLen(len(msgs)))
			}
		}

		DescribeTable("network create", doTest,
			Entry("allow default",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{}
					},
					expectAllowed: true,
				},
			),

			Entry("allow static",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							HostName:   "my-vm",
							DeviceName: "eth0",
							Addresses: []string{
								"192.168.1.100/24",
								"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
							},
							DHCP4:    false,
							DHCP6:    false,
							Gateway4: "192.168.1.1",
							Gateway6: "2605:a601:a0ba:720:2ce6::1",
							MTU:      pointer.Int64(9000),
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
						}
					},
					expectAllowed: true,
				},
			),

			Entry("allow dhcp",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							HostName:   "my-vm",
							DeviceName: "eth0",
							DHCP4:      true,
							DHCP6:      true,
						}
					},
					expectAllowed: true,
				},
			),

			Entry("disallow mixing static and dhcp",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							HostName:   "my-vm",
							DeviceName: "eth0",
							Addresses: []string{
								"192.168.1.100/24",
								"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
							},
							DHCP4:    true,
							DHCP6:    true,
							Gateway4: "192.168.1.1",
							Gateway6: "2605:a601:a0ba:720:2ce6::1",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.dhcp4: Invalid value: "192.168.1.100/24": dhcp4 cannot be used with IPv4 addresses in addresses field`,
						`spec.network.gateway4: Invalid value: "192.168.1.1": gateway4 is mutually exclusive with dhcp4`,
						`spec.network.dhcp6: Invalid value: "2605:a601:a0ba:720:2ce6:776d:8be4:2496/48": dhcp6 cannot be used with IPv6 addresses in addresses field`,
						`spec.network.gateway6: Invalid value: "2605:a601:a0ba:720:2ce6::1": gateway6 is mutually exclusive with dhcp6`,
					),
				},
			),

			Entry("validate addresses",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Addresses = []string{
							"1.1.",
							"1.1.1.1",
							"not-an-ip",
							"7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.addresses[0]: Invalid value: "1.1.": invalid CIDR address: 1.1.`,
						`spec.network.addresses[1]: Invalid value: "1.1.1.1": invalid CIDR address: 1.1.1.1`,
						`spec.network.addresses[2]: Invalid value: "not-an-ip": invalid CIDR address: not-an-ip`,
						`spec.network.addresses[3]: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": invalid CIDR address: 7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072`,
					),
				},
			),

			Entry("validate gateway4",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Gateway4 = "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072"
					},
					validate: doValidateWithMsg(
						`spec.network.gateway4: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": gateway4 must have an IPv4 address in the addresses field`,
						`spec.network.gateway4: Invalid value: "7936:39e1:d51b:39d2:05f8:1fb2:35cc:1072": must be a valid IPv4 address`,
					),
				},
			),

			Entry("validate gateway6",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Gateway6 = "192.168.1.1"
					},
					validate: doValidateWithMsg(
						`spec.network.gateway6: Invalid value: "192.168.1.1": gateway6 must have an IPv6 address in the addresses field`,
						`spec.network.gateway6: Invalid value: "192.168.1.1": must be a valid IPv6 address`,
					),
				},
			),

			Entry("validate nameservers",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Nameservers = []string{
							"not-an-ip",
							"192.168.1.1/24",
						}
					},
					validate: doValidateWithMsg(
						`spec.network.nameservers[0]: Invalid value: "not-an-ip": must be an IPv4 or IPv6 address`,
						`spec.network.nameservers[1]: Invalid value: "192.168.1.1/24": must be an IPv4 or IPv6 address`,
					),
				},
			),

			Entry("validate routes",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network.Routes = []vmopv1.VirtualMachineNetworkRouteSpec{
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
						`spec.network.routes[0].to: Invalid value: "10.100.10.1": invalid CIDR address: 10.100.10.1`,
						`spec.network.routes[0].via: Invalid value: "192.168.1": must be an IPv4 or IPv6 address`,
						`spec.network.routes[1].via: Invalid value: "2463:foobar": must be an IPv4 or IPv6 address`,
						`spec.network.routes[2]: Invalid value: "": cannot mix IP address families`,
					),
				},
			),

			Entry("validate interfaces",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							HostName: "my-vm",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth1",
									Addresses: []string{
										"192.168.1.100/24",
										"2605:a601:a0ba:720:2ce6:776d:8be4:2496/48",
									},
									Gateway4: "192.168.1.1",
									Gateway6: "2605:a601:a0ba:720:2ce6::1",
									MTU:      pointer.Int64(9000),
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

			Entry("disallow interfaces with default interface",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							HostName:  "my-vm",
							Addresses: []string{"192.168.1.10/24"},
							Gateway4:  "192.168.1.1",
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name:  "eth1",
									DHCP4: true,
								},
							},
						}
					},
					validate: doValidateWithMsg(
						`spec.network.interfaces: Invalid value: "null": interfaces are mutually exclusive with deviceName,network,addresses,dhcp4,dhcp6,gateway4,gateway6,mtu,nameservers,routes,searchDomains fields`,
					),
				},
			),

			Entry("disallow creating VM with network interfaces resulting in a non-DNS1123 combined network interface CR name/label (`vmName-networkName-interfaceName`)",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: fmt.Sprintf("%x", make([]byte, validation.DNS1123LabelMaxLength)),
									Network: common.PartialObjectRef{
										Name: "dummy-nw",
									},
								},
								{
									Name: "dummy_If",
									Network: common.PartialObjectRef{
										Name: "dummy-nw",
									},
								},
							},
						}
					},
					validate: doValidateWithMsg(
						fmt.Sprintf(`spec.network.interfaces[0].name: Invalid value: "%x": must be no more than 63 characters`, make([]byte, validation.DNS1123LabelMaxLength)),
						`spec.network.interfaces[1].name: Invalid value: "dummy_If": a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-'`,
						`and must start and end with an alphanumeric character (e.g. 'my-name'`,
						` or '123-abc'`,
						`regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')`,
					),
				},
			),
		)
	})
}

func unitTestsValidateUpdate() {
	var (
		ctx                           *unitValidatingWebhookContext
		oldVMServiceBackupRestoreFunc func() bool
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
		addAdminOnlyAnnotations     bool
		updateAdminOnlyAnnotations  bool
		removeAdminOnlyAnnotations  bool
		isPrivilegedUser            bool
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

		if args.addAdminOnlyAnnotations {
			ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
			ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
		}
		if args.updateAdminOnlyAnnotations {
			ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
			ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
			ctx.vm.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal + updateSuffix
			ctx.vm.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal + updateSuffix
		}
		if args.removeAdminOnlyAnnotations {
			ctx.oldVM.Annotations[vmopv1.InstanceIDAnnotation] = dummyInstanceIDVal
			ctx.oldVM.Annotations[vmopv1.FirstBootDoneAnnotation] = dummyFirstBootDoneVal
		}

		if args.isPrivilegedUser {
			lib.IsVMServiceBackupRestoreFSSEnabled = func() bool {
				return true
			}

			privilegedUsersEnvList := "  , foo ,bar , test,  "
			privilegedUser := "bar"
			Expect(os.Setenv(lib.PrivilegedUsersEnv, privilegedUsersEnvList)).To(Succeed())
			defer func() {
				Expect(os.Unsetenv(lib.PrivilegedUsersEnv)).To(Succeed())
			}()

			ctx.UserInfo.Username = privilegedUser
			ctx.IsPrivilegedAccount = pkgbuilder.IsPrivilegedAccount(ctx.WebhookContext, ctx.UserInfo)
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
		oldVMServiceBackupRestoreFunc = lib.IsVMServiceBackupRestoreFSSEnabled
	})

	AfterEach(func() {
		ctx = nil
		Expect(os.Unsetenv(lib.WcpFaultDomainsFSS)).To(Succeed())
		Expect(os.Unsetenv(lib.WindowsSysprepFSS)).To(Succeed())
		lib.IsVMServiceBackupRestoreFSSEnabled = oldVMServiceBackupRestoreFunc
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

		Entry("should disallow adding admin-only annotations by SSO user", updateArgs{addAdminOnlyAnnotations: true}, false,
			strings.Join([]string{
				field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
				field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
			}, ", "), nil),
		Entry("should disallow updating admin-only annotations by SSO user", updateArgs{updateAdminOnlyAnnotations: true}, false,
			strings.Join([]string{
				field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
				field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
			}, ", "), nil),
		Entry("should disallow removing admin-only annotations by SSO user", updateArgs{removeAdminOnlyAnnotations: true}, false,
			strings.Join([]string{
				field.Forbidden(annotationPath.Child(vmopv1.InstanceIDAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
				field.Forbidden(annotationPath.Child(vmopv1.FirstBootDoneAnnotation), "modifying this annotation is not allowed for non-admin users").Error(),
			}, ", "), nil),
		Entry("should allow adding admin-only annotations by service user", updateArgs{isServiceUser: true, addAdminOnlyAnnotations: true}, true, nil, nil),
		Entry("should allow adding admin-only annotations by service user", updateArgs{isServiceUser: true, updateAdminOnlyAnnotations: true}, true, nil, nil),
		Entry("should allow adding admin-only annotations by service user", updateArgs{isServiceUser: true, removeAdminOnlyAnnotations: true}, true, nil, nil),

		Entry("should allow adding admin-only annotations by privileged users", updateArgs{isPrivilegedUser: true, addAdminOnlyAnnotations: true}, true, nil, nil),
		Entry("should allow updating admin-only annotations by privileged users", updateArgs{isPrivilegedUser: true, updateAdminOnlyAnnotations: true}, true, nil, nil),
		Entry("should allow removing admin-only annotations by privileged users", updateArgs{isPrivilegedUser: true, removeAdminOnlyAnnotations: true}, true, nil, nil),
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
