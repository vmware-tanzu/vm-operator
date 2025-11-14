// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
)

func TestVirtualMachineConversion(t *testing.T) {

	t.Run("hub-spoke-hub", func(t *testing.T) {
		testCases := []struct {
			name string
			hub  ctrlconversion.Hub
		}{
			{
				name: "spec",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Image: &vmopv1.VirtualMachineImageRef{
							Name: "my-name",
						},
						ImageName: "my-name",
						ClassName: "my-class",
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							VTPMMode: vmopv1.VirtualMachineCryptoVTPMModeClone,
						},
						StorageClass: "my-storage-class",
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								CloudConfig: &vmopv1cloudinit.CloudConfig{
									Timezone: "my-tz",
									Users: []vmopv1cloudinit.User{
										{
											Name: "not-root",
										},
									},
								},
								RawCloudConfig: &vmopv1common.SecretKeySelector{
									Name: "cloud-init-secret",
									Key:  "user-data",
								},
								SSHAuthorizedKeys:               []string{"my-ssh-key"},
								UseGlobalNameserversAsDefault:   ptrOf(true),
								UseGlobalSearchDomainsAsDefault: ptrOf(true),
								WaitOnNetwork4:                  ptrOf(true),
								WaitOnNetwork6:                  ptrOf(false),
							},
							LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
								HardwareClockIsUTC:           ptrOf(true),
								TimeZone:                     "my-tz",
								ExpirePasswordAfterNextLogin: true,
								Password: &vmopv1common.PasswordSecretKeySelector{
									Name: "my-password-secret",
									Key:  "my-password-key",
								},
								ScriptText: &vmopv1common.ValueOrSecretKeySelector{
									From: &vmopv1common.SecretKeySelector{
										Name: "my-text-secret",
										Key:  "my-text-key",
									},
									Value: ptrOf("my-inline-script"),
								},
								CustomizeAtNextPowerOn: ptrOf(true),
							},
							Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
								Sysprep: &vmopv1sysprep.Sysprep{
									GUIRunOnce: &vmopv1sysprep.GUIRunOnce{
										Commands: []string{"echo", "hello"},
									},
									GUIUnattended: &vmopv1sysprep.GUIUnattended{
										AutoLogon: true,
									},
									Identification: &vmopv1sysprep.Identification{
										DomainAdmin: "my-admin",
										DomainOU:    "my-ou",
									},
									UserData: vmopv1sysprep.UserData{
										FullName: "vmware",
									},
									ExpirePasswordAfterNextLogin: true,
									ScriptText: &vmopv1common.ValueOrSecretKeySelector{
										From: &vmopv1common.SecretKeySelector{
											Name: "my-text-secret",
											Key:  "my-text-key",
										},
										Value: ptrOf("my-inline-script"),
									},
								},
								CustomizeAtNextPowerOn: ptrOf(true),
							},
							VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
								Properties: []vmopv1common.KeyValueOrSecretKeySelectorPair{
									{
										Key: "my-key",
										Value: vmopv1common.ValueOrSecretKeySelector{
											Value: ptrOf("my-value"),
										},
									},
								},
								RawProperties: "my-secret",
							},
						},
						Network: &vmopv1.VirtualMachineNetworkSpec{
							DomainName:    "my-domain.com",
							HostName:      "my-test-vm",
							Disabled:      true,
							Nameservers:   []string{"10.11.12.13", "9.9.9.9"},
							SearchDomains: []string{"foo.local", "bar.local"},
							Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "vds-interface",
									Network: &vmopv1common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											Kind:       "Network",
											APIVersion: "netoperator.vmware.com/v1alpha1",
										},
										Name: "primary",
									},
									GuestDeviceName: "eth10",
									MACAddr:         "00:11:22:33:44:55",
								},
								{
									Name: "ncp-interface",
									Network: &vmopv1common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											Kind:       "VirtualNetwork",
											APIVersion: "vmware.com/v1alpha1",
										},
										Name: "segment1",
									},
									GuestDeviceName: "eth20",
								},
								{
									Name: "nsx-vpc-subnet-interface",
									Network: &vmopv1common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											Kind:       "Subnet",
											APIVersion: "crd.nsx.vmware.com/v1alpha1",
										},
										Name: "segment-subnet",
									},
									GuestDeviceName: "eth30",
									MACAddr:         "00:11:22:33:44:56",
								},
								{
									Name: "nsx-vpc-subnetset-interface",
									Network: &vmopv1common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											Kind:       "SubnetSet",
											APIVersion: "crd.nsx.vmware.com/v1alpha1",
										},
										Name: "segment-subnetset",
									},
									MACAddr: "00:11:22:33:44:57",
								},
								{
									Name: "my-interface",
									Network: &vmopv1common.PartialObjectRef{
										TypeMeta: metav1.TypeMeta{
											Kind:       "Network",
											APIVersion: "netoperator.vmware.com/v1alpha1",
										},
										Name: "secondary",
									},
									GuestDeviceName: "eth40",
									MACAddr:         "00:11:22:33:44:58",
									Addresses:       []string{"1.1.1.11", "2.2.2.22"},
									DHCP4:           true,
									DHCP6:           true,
									Gateway4:        "1.1.1.1",
									Gateway6:        "2.2.2.2",
									MTU:             ptrOf[int64](9000),
									Nameservers:     []string{"9.9.9.9"},
									Routes: []vmopv1.VirtualMachineNetworkRouteSpec{
										{
											To:     "3.3.3.3",
											Via:    "1.1.1.111",
											Metric: 42,
										},
									},
									SearchDomains: []string{"vmware.com"},
								},
							},
						},
						PowerState:      vmopv1.VirtualMachinePowerStateOff,
						PowerOffMode:    vmopv1.VirtualMachinePowerOpModeHard,
						SuspendMode:     vmopv1.VirtualMachinePowerOpModeTrySoft,
						NextRestartTime: "tomorrow",
						RestartMode:     vmopv1.VirtualMachinePowerOpModeSoft,
						Hardware: &vmopv1.VirtualMachineHardwareSpec{
							Cdrom: []vmopv1.VirtualMachineCdromSpec{
								{
									Name: "cdrom1",
									Image: vmopv1.VirtualMachineImageRef{
										Name: "my-cdrom-image",
										Kind: "VirtualMachineImage",
									},
									AllowGuestControl: ptrOf(true),
									Connected:         ptrOf(true),
								},
							},
							IDEControllers: []vmopv1.IDEControllerSpec{
								{
									BusNumber: 0,
								},
								{
									BusNumber: 1,
								},
							},
							NVMEControllers: []vmopv1.NVMEControllerSpec{
								{
									BusNumber:     0,
									PCISlotNumber: ptrOf(int32(160)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
								},
								{
									BusNumber:     1,
									PCISlotNumber: ptrOf(int32(16)),
									SharingMode:   vmopv1.VirtualControllerSharingModePhysical,
								},
								{
									BusNumber:     2,
									PCISlotNumber: ptrOf(int32(162)),
									SharingMode:   vmopv1.VirtualControllerSharingModeVirtual,
								},
								{
									BusNumber:     3,
									PCISlotNumber: ptrOf(int32(163)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
								},
							},
							SATAControllers: []vmopv1.SATAControllerSpec{
								{
									BusNumber:     0,
									PCISlotNumber: ptrOf(int32(170)),
								},
								{
									BusNumber:     1,
									PCISlotNumber: ptrOf(int32(17)),
								},
								{
									BusNumber:     2,
									PCISlotNumber: ptrOf(int32(172)),
								},
								{
									BusNumber:     3,
									PCISlotNumber: ptrOf(int32(173)),
								},
							},
							SCSIControllers: []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     0,
									PCISlotNumber: ptrOf(int32(180)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
									Type:          vmopv1.SCSIControllerTypeParaVirtualSCSI,
								},
								{
									BusNumber:     1,
									PCISlotNumber: ptrOf(int32(18)),
									SharingMode:   vmopv1.VirtualControllerSharingModePhysical,
									Type:          vmopv1.SCSIControllerTypeParaVirtualSCSI,
								},
								{
									BusNumber:     2,
									PCISlotNumber: ptrOf(int32(182)),
									SharingMode:   vmopv1.VirtualControllerSharingModeVirtual,
									Type:          vmopv1.SCSIControllerTypeParaVirtualSCSI,
								},
								{
									BusNumber:     3,
									PCISlotNumber: ptrOf(int32(183)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
									Type:          vmopv1.SCSIControllerTypeParaVirtualSCSI,
								},
							},
						},
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "my-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: ptrOf(vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "my-claim",
										},
										ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
										ControllerType:      vmopv1.VirtualControllerTypeSCSI,
										ControllerBusNumber: ptrOf(int32(0)),
										DiskMode:            vmopv1.VolumeDiskModePersistent,
										UnitNumber:          ptrOf(int32(0)),
									}),
								},
							},
							{
								Name: "my-volume-2",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: ptrOf(vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "my-claim-2",
										},
										InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
											StorageClass: "instance-storage-class",
											Size:         resource.MustParse("2048k"),
										},
									}),
								},
							},
						},
						ReadinessProbe: &vmopv1.VirtualMachineReadinessProbeSpec{
							TCPSocket: &vmopv1.TCPSocketAction{
								Host: "some-host",
								Port: intstr.FromString("https"),
							},
							GuestHeartbeat: &vmopv1.GuestHeartbeatAction{
								ThresholdStatus: vmopv1.RedHeartbeatStatus,
							},
							GuestInfo: []vmopv1.GuestInfoAction{
								{
									Key:   "guest-key",
									Value: "guest-value",
								},
							},
							TimeoutSeconds: 100,
							PeriodSeconds:  200,
						},
						Advanced: &vmopv1.VirtualMachineAdvancedSpec{
							BootDiskCapacity:              ptrOf(resource.MustParse("1024k")),
							DefaultVolumeProvisioningMode: vmopv1.VolumeProvisioningModeThickEagerZero,
							ChangeBlockTracking:           ptrOf(true),
						},
						Reserved: &vmopv1.VirtualMachineReservedSpec{
							ResourcePolicyName: "my-resource-policy",
						},
						MinHardwareVersion: 42,
						InstanceUUID:       uuid.NewString(),
						BiosUUID:           uuid.NewString(),
						GuestID:            "my-guest-id",
						PromoteDisksMode:   vmopv1.VirtualMachinePromoteDisksModeOffline,
						BootOptions: &vmopv1.VirtualMachineBootOptions{
							Firmware:  vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
							BootDelay: &metav1.Duration{Duration: time.Second * 10},
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
							},
							BootRetry:           vmopv1.VirtualMachineBootOptionsBootRetryDisabled,
							BootRetryDelay:      &metav1.Duration{Duration: time.Second * 10},
							EFISecureBoot:       vmopv1.VirtualMachineBootOptionsEFISecureBootDisabled,
							NetworkBootProtocol: vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
						},
					},
				},
			},
			{
				name: "spec.bootstrap.cloudInit.waitOnNetwork4",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								WaitOnNetwork4: ptrOf(true),
							},
						},
					},
				},
			},
			{
				name: "spec.groupName",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						GroupName: "my-group",
					},
				},
			},
			{
				name: "spec.bootstrap.cloudInit.waitOnNetwork6",
				hub: &vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								WaitOnNetwork6: ptrOf(true),
							},
						},
						PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeOffline,
						BootOptions: &vmopv1.VirtualMachineBootOptions{
							Firmware:  vmopv1.VirtualMachineBootOptionsFirmwareTypeEFI,
							BootDelay: &metav1.Duration{Duration: time.Second * 10},
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
									Name: "disk-0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
									Name: "eth0",
								},
								{
									Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
								},
							},
							BootRetry:           vmopv1.VirtualMachineBootOptionsBootRetryDisabled,
							BootRetryDelay:      &metav1.Duration{Duration: time.Second * 10},
							EFISecureBoot:       vmopv1.VirtualMachineBootOptionsEFISecureBootDisabled,
							NetworkBootProtocol: vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
						},
						Affinity: &vmopv1.AffinitySpec{
							VMAffinity: &vmopv1.VMAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar",
											},
										},
										TopologyKey: "topology.kubernetes.io/abc",
									},
								},
								PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "trivia",
											},
										},
										TopologyKey: "topology.kubernetes.io/xyz",
									},
								},
							},
							VMAntiAffinity: &vmopv1.VMAntiAffinitySpec{
								RequiredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "chess",
											},
										},
										TopologyKey: "topology.kubernetes.io/def",
									},
								},
								PreferredDuringSchedulingPreferredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "football",
											},
										},
										TopologyKey: "topology.kubernetes.io/ghi",
									},
								},
							},
						},
					},
				},
			},
		}

		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)

				after := &vmopv1.VirtualMachine{}
				spoke := &vmopv1a3.VirtualMachine{}

				// First convert hub to spoke
				g.Expect(spoke.ConvertFrom(tc.hub)).To(Succeed())

				// Convert spoke back to hub.
				g.Expect(spoke.ConvertTo(after)).To(Succeed())

				// Check that everything is equal.
				g.Expect(apiequality.Semantic.DeepEqual(tc.hub, after)).To(BeTrue(), cmp.Diff(tc.hub, after))
			})
		}
	})

	t.Run("hub-spoke", func(t *testing.T) {

		testCases := []struct {
			name string
			hub  ctrlconversion.Hub
			exp  ctrlconversion.Convertible
		}{
			{
				name: "status.storage=nil",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: nil,
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: nil,
					},
				},
			},
			{
				name: "status.storage=empty",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: &vmopv1.VirtualMachineStorageStatus{},
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: &vmopv1a3.VirtualMachineStorageStatus{},
					},
				},
			},
			{
				name: "status.storage.requested=empty",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{},
						},
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: &vmopv1a3.VirtualMachineStorageStatus{},
					},
				},
			},
			{
				name: "status.storage.used=empty",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Used: &vmopv1.VirtualMachineStorageStatusUsed{},
						},
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: &vmopv1a3.VirtualMachineStorageStatus{},
					},
				},
			},
			{
				name: "status.storage.total",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptrOf(resource.MustParse("5.5Gi"))},
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: &vmopv1a3.VirtualMachineStorageStatus{
							Usage: &vmopv1a3.VirtualMachineStorageStatusUsage{
								Total: ptrOf(resource.MustParse("5.5Gi")),
							},
						},
					},
				},
			},
			{
				name: "status.storage",
				hub: &vmopv1.VirtualMachine{
					Status: vmopv1.VirtualMachineStatus{
						Storage: &vmopv1.VirtualMachineStorageStatus{
							Total: ptrOf(resource.MustParse("5.5Gi")),
							Requested: &vmopv1.VirtualMachineStorageStatusRequested{
								Disks: ptrOf(resource.MustParse("5Gi")),
							},
							Used: &vmopv1.VirtualMachineStorageStatusUsed{
								Disks: ptrOf(resource.MustParse("2Gi")),
								Other: ptrOf(resource.MustParse("512Mi")),
							},
						},
					},
				},
				exp: &vmopv1a3.VirtualMachine{
					Status: vmopv1a3.VirtualMachineStatus{
						Storage: &vmopv1a3.VirtualMachineStorageStatus{
							Usage: &vmopv1a3.VirtualMachineStorageStatusUsage{
								Total: ptrOf(resource.MustParse("5.5Gi")),
								Disks: ptrOf(resource.MustParse("5Gi")),
								Other: ptrOf(resource.MustParse("512Mi")),
							},
						},
					},
				},
			},
		}

		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)

				spoke := &vmopv1a3.VirtualMachine{}

				// First convert hub to spoke
				g.Expect(spoke.ConvertFrom(tc.hub)).To(Succeed())

				// Remove spoke's annotations.
				spoke.Annotations = nil

				// Check that everything is equal.
				g.Expect(apiequality.Semantic.DeepEqual(tc.exp, spoke)).To(BeTrue(), cmp.Diff(tc.exp, spoke))
			})
		}
	})

	t.Run("VirtualMachine and spec.crypto", func(t *testing.T) {

		hubSpokeHub := func(g *WithT, hub, hubAfter ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
			hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

			// First convert hub to spoke
			dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
			g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

			// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
			g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

			g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
		}

		t.Run("hub-spoke-hub", func(t *testing.T) {

			t.Run("spec.crypto is nil", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})

			t.Run("spec.crypto is empty", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})

			t.Run("spec.crypto.className is non-empty", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							EncryptionClassName: "fake",
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})

			t.Run("spec.crypto.useDefaultKeyProvider is true", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							UseDefaultKeyProvider: &[]bool{true}[0],
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})

			t.Run("spec.crypto.useDefaultKeyProvider is false", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							UseDefaultKeyProvider: &[]bool{false}[0],
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})

			t.Run("spec.crypto.vTPMMode is clone", func(t *testing.T) {
				g := NewWithT(t)
				hubBefore := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							EncryptionClassName: "my-class-1",
							VTPMMode:            vmopv1.VirtualMachineCryptoVTPMModeClone,
						},
					},
				}

				// First convert hub to spoke
				var spoke vmopv1a3.VirtualMachine
				g.Expect(spoke.ConvertFrom(&hubBefore)).To(Succeed())

				spoke.Spec.Crypto.EncryptionClassName = "my-class-2"

				var hubAfter vmopv1.VirtualMachine
				g.Expect(spoke.ConvertTo(&hubAfter)).To(Succeed())

				g.Expect(hubAfter.Spec.Crypto).ToNot(BeNil())
				g.Expect(hubAfter.Spec.Crypto.EncryptionClassName).To(Equal("my-class-2"))
				g.Expect(hubAfter.Spec.Crypto.VTPMMode).To(Equal(vmopv1.VirtualMachineCryptoVTPMModeClone))
			})

			t.Run("spec.crypto is completely filled out", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							EncryptionClassName:   "fake",
							UseDefaultKeyProvider: &[]bool{false}[0],
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a3.VirtualMachine{})
			})
		})
	})
}
