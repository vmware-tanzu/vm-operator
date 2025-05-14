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
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha4/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha4/sysprep"
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
						ImageName:    "my-name",
						ClassName:    "my-class",
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
								HardwareClockIsUTC: ptrOf(true),
								TimeZone:           "my-tz",
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
								},
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
						Volumes: []vmopv1.VirtualMachineVolume{
							{
								Name: "my-volume",
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: ptrOf(vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "my-claim",
										},
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
							DefaultVolumeProvisioningMode: vmopv1.VirtualMachineVolumeProvisioningModeThickEagerZero,
							ChangeBlockTracking:           ptrOf(true),
						},
						Reserved: &vmopv1.VirtualMachineReservedSpec{
							ResourcePolicyName: "my-resource-policy",
						},
						MinHardwareVersion: 42,
						InstanceUUID:       uuid.NewString(),
						BiosUUID:           uuid.NewString(),
						GuestID:            "my-guest-id",
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
						PromoteDisksMode: vmopv1.VirtualMachinePromoteDisksModeOffline,
						BootOptions: &vmopv1.VirtualMachineBootOptions{
							BootDelay: &metav1.Duration{Duration: time.Second * 10},
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							BootRetryEnabled:     true,
							BootRetryDelay:       &metav1.Duration{Duration: time.Second * 10},
							EnterBIOSSetup:       true,
							EFISecureBootEnabled: false,
							NetworkBootProtocol:  vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
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
							BootDelay: &metav1.Duration{Duration: time.Second * 10},
							BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
								vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							BootRetryEnabled:     true,
							BootRetryDelay:       &metav1.Duration{Duration: time.Second * 10},
							EnterBIOSSetup:       true,
							EFISecureBootEnabled: false,
							NetworkBootProtocol:  vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
						},
						Affinity: &vmopv1.VirtualMachineAffinitySpec{
							ZoneAffinity: &vmopv1.VirtualMachineAffinityZoneAffinitySpec{
								RequiredDuringSchedulingIgnoredDuringExecution: []vmopv1.ZoneSelectorTerm{
									{
										MatchExpressions: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "foo",
												Operator: vmopv1.ZoneSelectorOpExists,
												Values: []string{
													"bar",
												},
											},
										},
										MatchFields: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "zone-code",
												Operator: vmopv1.ZoneSelectorOpGt,
												Values: []string{
													"1000",
												},
											},
										},
									},
								},
								PreferredDuringSchedulingIgnoredDuringExecution: []vmopv1.ZoneSelectorTerm{
									{
										MatchExpressions: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "foo",
												Operator: vmopv1.ZoneSelectorOpExists,
												Values: []string{
													"bar",
												},
											},
										},
										MatchFields: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "zone-code",
												Operator: vmopv1.ZoneSelectorOpGt,
												Values: []string{
													"1001",
												},
											},
										},
									},
								},
							},
							ZoneAntiAffinity: &vmopv1.VirtualMachineAntiAffinityZoneAffinitySpec{
								RequiredDuringSchedulingIgnoredDuringExecution: []vmopv1.ZoneSelectorTerm{
									{
										MatchExpressions: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "foo",
												Operator: vmopv1.ZoneSelectorOpExists,
												Values: []string{
													"bar",
												},
											},
										},
										MatchFields: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "zone-code",
												Operator: vmopv1.ZoneSelectorOpGt,
												Values: []string{
													"1000",
												},
											},
										},
									},
								},
								PreferredDuringSchedulingIgnoredDuringExecution: []vmopv1.ZoneSelectorTerm{
									{
										MatchExpressions: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "foo",
												Operator: vmopv1.ZoneSelectorOpExists,
												Values: []string{
													"bar",
												},
											},
										},
										MatchFields: []vmopv1.ZoneSelectorRequirement{
											{
												Key:      "zone-code",
												Operator: vmopv1.ZoneSelectorOpGt,
												Values: []string{
													"1001",
												},
											},
										},
									},
								},
							},
							VMAffinity: &vmopv1.VirtualMachineAffinityVMAffinitySpec{
								RequiredDuringSchedulingIgnoredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar",
											},
										},
										TopologyKey: "topology.kubernetes.io/abc",
									},
								},
								PreferredDuringSchedulingIgnoredDuringExecution: []vmopv1.VMAffinityTerm{
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
							VMAntiAffinity: &vmopv1.VirtualMachineAntiAffinityVMAffinitySpec{
								RequiredDuringSchedulingIgnoredDuringExecution: []vmopv1.VMAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app": "chess",
											},
										},
										TopologyKey: "topology.kubernetes.io/def",
									},
								},
								PreferredDuringSchedulingIgnoredDuringExecution: []vmopv1.VMAffinityTerm{
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
}
