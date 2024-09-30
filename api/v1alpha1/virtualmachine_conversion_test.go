// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

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

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
)

func TestVirtualMachineConversion(t *testing.T) {

	hubSpokeHub := func(g *WithT, hub ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		hubAfter := hub.DeepCopyObject().(ctrlconversion.Hub)
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
	}

	spokeHubSpoke := func(g *WithT, spoke ctrlconversion.Convertible, hub ctrlconversion.Hub) {
		spokeBefore := spoke.DeepCopyObject().(ctrlconversion.Convertible)

		// First convert spoke to hub
		hubCopy := hub.DeepCopyObject().(ctrlconversion.Hub)
		g.Expect(spokeBefore.ConvertTo(hubCopy)).To(Succeed())

		// Convert hub back to spoke and check if the resulting spoke is equal to the spoke before the round trip
		spokeAfter := spoke.DeepCopyObject().(ctrlconversion.Convertible)
		g.Expect(spokeAfter.ConvertFrom(hubCopy)).To(Succeed())

		metaAfter := spokeAfter.(metav1.Object)
		delete(metaAfter.GetAnnotations(), utilconversion.AnnotationKey)

		g.Expect(apiequality.Semantic.DeepEqual(spokeBefore, spokeAfter)).To(BeTrue(), cmp.Diff(spokeBefore, spokeAfter))
	}

	t.Run("VirtualMachine hub-spoke-hub", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image: &vmopv1.VirtualMachineImageRef{
					Name: "my-name",
				},
				ImageName:    "my-name",
				ClassName:    "my-class",
				StorageClass: "my-storage-class",
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &vmopv1common.SecretKeySelector{
							Name: "cloud-init-secret",
							Key:  "user-data",
						},
						SSHAuthorizedKeys: []string{"my-ssh-key"},
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
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with CloudInit", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &vmopv1common.SecretKeySelector{
							Name: "cloudinit-secret",
							Key:  "my-key",
						},
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined CloudInit", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
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
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with CloudInit w/ just SSHAuthorizedKeys", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with LinuxPrep", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
						HardwareClockIsUTC: &[]bool{true}[0],
						TimeZone:           "my-tz",
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with LinuxPrep and vAppConfig", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
						HardwareClockIsUTC: &[]bool{true}[0],
						TimeZone:           "my-tz",
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
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with Sysprep", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &vmopv1common.SecretKeySelector{
							Name: "sysprep-secret",
							Key:  "my-key",
						},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined Sysprep", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
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
							},
							UserData: &vmopv1sysprep.UserData{
								FullName: "vmware",
							},
						},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with Sysprep and vAppConfig", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &vmopv1common.SecretKeySelector{
							Name: "sysprep-secret",
							Key:  "my-key",
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
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with vAppConfig", func(t *testing.T) {
		g := NewWithT(t)

		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
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
			},
		}

		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke Status", func(t *testing.T) {
		g := NewWithT(t)

		now := time.Now()
		hub := vmopv1.VirtualMachine{
			Status: vmopv1.VirtualMachineStatus{
				Host:       "my-host",
				PowerState: vmopv1.VirtualMachinePowerStateOn,
				Conditions: []metav1.Condition{
					{
						Type:   vmopv1.VirtualMachineConditionCreated,
						Status: metav1.ConditionTrue,
					},
				},
				Network: &vmopv1.VirtualMachineNetworkStatus{
					PrimaryIP4: "192.168.1.10",
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceStatus{
						{
							Name: "eth0",
							IP: &vmopv1.VirtualMachineNetworkInterfaceIPStatus{
								Addresses: []vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus{
									{
										Address: "my-ip",
									},
								},
								MACAddr: "my-mac",
							},
							DNS: &vmopv1.VirtualMachineNetworkDNSStatus{
								DHCP:          true,
								DomainName:    "vmware.com",
								HostName:      "my-host",
								Nameservers:   []string{"8.8.8.8"},
								SearchDomains: []string{"broadcom.com"},
							},
						},
					},
				},
				UniqueID:     "my-unique-id",
				BiosUUID:     "my-bios-uuid",
				InstanceUUID: "my-inst-id",
				Volumes: []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "my-disk",
						Attached: true,
						DiskUUID: "my-disk-uuid",
						Error:    "my-disk-error",
					},
				},
				ChangeBlockTracking: ptrOf(true),
				Zone:                "my-zone",
				LastRestartTime:     ptrOf(metav1.NewTime(now)),
				HardwareVersion:     42,
			},
		}

		spoke := &vmopv1a1.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())

		g.Expect(spoke.Status.Host).To(Equal(hub.Status.Host))
		g.Expect(spoke.Status.PowerState).To(Equal(vmopv1a1.VirtualMachinePoweredOn))
		g.Expect(spoke.Status.VmIp).To(Equal(hub.Status.Network.PrimaryIP4))
		g.Expect(spoke.Status.NetworkInterfaces[0].MacAddress).To(Equal("my-mac"))
		g.Expect(spoke.Status.NetworkInterfaces[0].IpAddresses).To(Equal([]string{"my-ip"}))
		g.Expect(spoke.Status.Phase).To(Equal(vmopv1a1.Created))
		g.Expect(spoke.Status.UniqueID).To(Equal(hub.Status.UniqueID))
		g.Expect(spoke.Status.BiosUUID).To(Equal(hub.Status.BiosUUID))
		g.Expect(spoke.Status.InstanceUUID).To(Equal(hub.Status.InstanceUUID))
		g.Expect(spoke.Status.Volumes).To(HaveLen(1))
		g.Expect(spoke.Status.Volumes[0].Name).To(Equal(hub.Status.Volumes[0].Name))
		g.Expect(spoke.Status.Volumes[0].Attached).To(Equal(hub.Status.Volumes[0].Attached))
		g.Expect(spoke.Status.Volumes[0].DiskUuid).To(Equal(hub.Status.Volumes[0].DiskUUID))
		g.Expect(spoke.Status.Volumes[0].Error).To(Equal(hub.Status.Volumes[0].Error))
		g.Expect(spoke.Status.ChangeBlockTracking).To(Equal(ptrOf(true)))
		g.Expect(spoke.Status.Zone).To(Equal(hub.Status.Zone))
		g.Expect(spoke.Status.LastRestartTime).To(Equal(hub.Status.LastRestartTime))
		g.Expect(spoke.Status.HardwareVersion).To(Equal(hub.Status.HardwareVersion))
	})

	t.Run("VirtualMachine spoke-hub Status", func(t *testing.T) {
		g := NewWithT(t)

		now := time.Now()
		spoke := vmopv1a1.VirtualMachine{
			Status: vmopv1a1.VirtualMachineStatus{
				Host:       "my-host",
				PowerState: vmopv1a1.VirtualMachinePoweredOff,
				Phase:      vmopv1a1.Created,
				Conditions: []vmopv1a1.Condition{
					{
						Type:   "Cond",
						Status: corev1.ConditionTrue,
					},
				},
				VmIp:         "192.168.1.11",
				UniqueID:     "my-unique-id",
				BiosUUID:     "my-bios-uuid",
				InstanceUUID: "my-inst-id",
				Volumes: []vmopv1a1.VirtualMachineVolumeStatus{
					{
						Name:     "my-disk",
						Attached: true,
						DiskUuid: "my-disk-uuid",
						Error:    "my-disk-error",
					},
				},
				ChangeBlockTracking: ptrOf(true),
				NetworkInterfaces: []vmopv1a1.NetworkInterfaceStatus{
					{
						Connected:   true,
						MacAddress:  "my-mac",
						IpAddresses: []string{"172.42.99.10"},
					},
				},
				Zone:            "my-zone",
				LastRestartTime: ptrOf(metav1.NewTime(now)),
				HardwareVersion: 42,
			},
		}

		hub := &vmopv1.VirtualMachine{}
		g.Expect(spoke.ConvertTo(hub)).To(Succeed())

		g.Expect(hub.Status.Host).To(Equal(spoke.Status.Host))
		g.Expect(hub.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
		g.Expect(hub.Status.Conditions).To(HaveLen(1))
		g.Expect(hub.Status.Conditions[0].Type).To(Equal("Cond"))
		g.Expect(hub.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(hub.Status.Conditions[0].Reason).To(Equal(string(metav1.ConditionTrue)))
		g.Expect(hub.Status.Network).ToNot(BeNil())
		g.Expect(hub.Status.Network.PrimaryIP4).To(Equal(spoke.Status.VmIp))
		g.Expect(hub.Status.Network.Interfaces).To(HaveLen(1))
		g.Expect(hub.Status.Network.Interfaces[0].IP).ToNot(BeNil())
		g.Expect(hub.Status.Network.Interfaces[0].IP.MACAddr).To(Equal(spoke.Status.NetworkInterfaces[0].MacAddress))
		g.Expect(hub.Status.Network.Interfaces[0].IP.Addresses).To(HaveLen(1))
		g.Expect(hub.Status.Network.Interfaces[0].IP.Addresses[0].Address).To(Equal(spoke.Status.NetworkInterfaces[0].IpAddresses[0]))
		g.Expect(hub.Status.UniqueID).To(Equal(spoke.Status.UniqueID))
		g.Expect(hub.Status.BiosUUID).To(Equal(spoke.Status.BiosUUID))
		g.Expect(hub.Status.InstanceUUID).To(Equal(spoke.Status.InstanceUUID))
		g.Expect(hub.Status.Volumes).To(HaveLen(1))
		g.Expect(hub.Status.Volumes[0].Name).To(Equal(spoke.Status.Volumes[0].Name))
		g.Expect(hub.Status.UniqueID).To(Equal(spoke.Status.UniqueID))
		g.Expect(hub.Status.BiosUUID).To(Equal(spoke.Status.BiosUUID))
		g.Expect(hub.Status.InstanceUUID).To(Equal(spoke.Status.InstanceUUID))
		g.Expect(hub.Status.Volumes).To(HaveLen(1))
		g.Expect(hub.Status.Volumes[0].Name).To(Equal(spoke.Status.Volumes[0].Name))
		g.Expect(hub.Status.Volumes[0].Attached).To(Equal(spoke.Status.Volumes[0].Attached))
		g.Expect(hub.Status.Volumes[0].DiskUUID).To(Equal(spoke.Status.Volumes[0].DiskUuid))
		g.Expect(hub.Status.Volumes[0].Error).To(Equal(spoke.Status.Volumes[0].Error))
		g.Expect(hub.Status.ChangeBlockTracking).To(Equal(ptrOf(true)))
		g.Expect(hub.Status.Zone).To(Equal(spoke.Status.Zone))
		g.Expect(hub.Status.LastRestartTime).To(Equal(spoke.Status.LastRestartTime))
		g.Expect(hub.Status.HardwareVersion).To(Equal(spoke.Status.HardwareVersion))
	})

	t.Run("VirtualMachine hub-spoke Status Conditions", func(t *testing.T) {
		loc, err := time.LoadLocation("UTC")
		NewWithT(t).Expect(err).ToNot(HaveOccurred())
		now := metav1.Date(2000, time.January, 1, 0, 0, 0, 0, loc)

		vmClassCond := metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionClassReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(1, 0, 0)),
		}
		vmImageCond := metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionImageReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(2, 0, 0)),
		}
		vmSetRPCond := metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(3, 0, 0)),
		}
		vmBSCond := metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionBootstrapReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(4, 0, 0)),
		}

		findCond := func(conditions []vmopv1a1.Condition, t vmopv1a1.ConditionType) *vmopv1a1.Condition {
			for _, c := range conditions {
				if c.Type == t {
					return &c
				}
			}
			return nil
		}

		t.Run("Prereqs are ready", func(t *testing.T) {

			t.Run("Class and Image Conditions", func(t *testing.T) {
				g := NewWithT(t)

				hub := vmopv1.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond}

				spoke := &vmopv1a1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(c.Reason).To(BeEmpty())
				g.Expect(c.Message).To(BeEmpty())
				g.Expect(c.LastTransitionTime).To(Equal(vmImageCond.LastTransitionTime))
			})

			t.Run("All Prereq Conditions", func(t *testing.T) {
				g := NewWithT(t)

				hub := vmopv1.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond, vmSetRPCond, vmBSCond}

				spoke := &vmopv1a1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(c.Reason).To(BeEmpty())
				g.Expect(c.Message).To(BeEmpty())
				g.Expect(c.LastTransitionTime).To(Equal(vmImageCond.LastTransitionTime))
			})

			t.Run("Existing PrereqReady is in spoke", func(t *testing.T) {
				g := NewWithT(t)

				hub := vmopv1.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond, vmSetRPCond, vmBSCond}

				spoke := &vmopv1a1.VirtualMachine{
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								Reason:             "should be updated",
								LastTransitionTime: metav1.NewTime(now.AddDate(1000, 0, 0)),
							},
						},
					},
				}

				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(c.Reason).To(BeEmpty())
				g.Expect(c.Message).To(BeEmpty())
				g.Expect(c.LastTransitionTime).To(Equal(vmImageCond.LastTransitionTime))
			})
		})

		t.Run("Prereqs are not ready", func(t *testing.T) {

			t.Run("Class not ready", func(t *testing.T) {
				g := NewWithT(t)

				notReadyC := vmClassCond
				notReadyC.Status = metav1.ConditionFalse
				notReadyC.Message = "foobar"

				hub := vmopv1.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{notReadyC}

				spoke := &vmopv1a1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineClassNotFoundReason))
				g.Expect(c.Message).To(Equal("foobar"))
				g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))
			})

			t.Run("Image not ready", func(t *testing.T) {
				g := NewWithT(t)

				notReadyC := vmImageCond
				notReadyC.Status = metav1.ConditionFalse
				notReadyC.Message = "foobar"

				hub := vmopv1.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, notReadyC}

				spoke := &vmopv1a1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineImageNotFoundReason))
				g.Expect(c.Message).To(Equal("foobar"))
				g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))

				t.Run("Existing PrereqReady in spoke", func(t *testing.T) {
					spoke := &vmopv1a1.VirtualMachine{
						Status: vmopv1a1.VirtualMachineStatus{
							Conditions: []vmopv1a1.Condition{
								{
									Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
									Status:             corev1.ConditionFalse,
									Reason:             "should be updated",
									LastTransitionTime: metav1.NewTime(now.AddDate(1000, 0, 0)),
								},
							},
						},
					}

					g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
					g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

					c := findCond(spoke.Status.Conditions, vmopv1a1.VirtualMachinePrereqReadyCondition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
					g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineImageNotFoundReason))
					g.Expect(c.Message).To(Equal("foobar"))
					g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))
				})
			})
		})
	})

	t.Run("VirtualMachine spoke-hub Status Conditions", func(t *testing.T) {

		findCond := func(conditions []metav1.Condition, ConditionType string) *metav1.Condition {
			for _, c := range conditions {
				if c.Type == ConditionType {
					return &c
				}
			}
			return nil
		}

		t.Run("Converts Condition", func(t *testing.T) {
			g := NewWithT(t)

			now := metav1.Now()
			spoke := vmopv1a1.VirtualMachine{
				Status: vmopv1a1.VirtualMachineStatus{
					Conditions: []vmopv1a1.Condition{
						{
							Type:               vmopv1a1.GuestCustomizationCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: now,
							Reason:             "Reason",
							Message:            "Message",
						},
					},
				},
			}

			hub := &vmopv1.VirtualMachine{}
			g.Expect(spoke.ConvertTo(hub)).To(Succeed())
			g.Expect(hub.Status.Conditions).To(HaveLen(1))

			c := hub.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(vmopv1.GuestCustomizationCondition))
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			g.Expect(c.Reason).To(Equal("Reason"))
			g.Expect(c.Message).To(Equal("Message"))
		})

		t.Run("Converts PrereqReady Condition", func(t *testing.T) {
			t.Run("PrereqReady is True with no prior nextver conditions", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady is True with no vmMetadata or resource policy set", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(2))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with VMClass not Found reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineClassNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineClassNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady is False because VMClass not found, with no vmMetadata or resource policy in spec", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineClassNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(2))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineClassNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with VMClass Binding not Found reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineClassBindingNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineClassBindingNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with VMImage not Found reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineImageNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineImageNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with VMImage not Ready reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineImageNotReadyReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.VirtualMachineImageNotReadyReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with ContentSourceBinding Found reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.ContentSourceBindingNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.ContentSourceBindingNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with ContentLibraryProvider not found reason", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.ContentLibraryProviderNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(4))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(vmopv1a1.ContentLibraryProviderNotFoundReason))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[3]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionBootstrapReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionUnknown)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is True with prior nextver conditions", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
							},
							{
								Type:               vmopv1.VirtualMachineConditionClassReady,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionTrue),
							},
							{
								Type:               vmopv1.VirtualMachineConditionImageReady,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionTrue),
							},
							{
								Type:               vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionTrue),
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(3))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})

			t.Run("PrereqReady Condition is False with prior nextver conditions", func(t *testing.T) {
				g := NewWithT(t)

				now := metav1.Now()
				spoke := vmopv1a1.VirtualMachine{
					Spec: vmopv1a1.VirtualMachineSpec{
						VmMetadata: &vmopv1a1.VirtualMachineMetadata{
							Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
							SecretName: "my-secret",
						},
						ResourcePolicyName: "my-policy",
					},
					Status: vmopv1a1.VirtualMachineStatus{
						Conditions: []vmopv1a1.Condition{
							{
								Type:               vmopv1a1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             vmopv1a1.VirtualMachineClassNotFoundReason,
								Severity:           vmopv1a1.ConditionSeverityError,
							},
							{
								Type:               vmopv1.VirtualMachineConditionClassReady,
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionFalse),
							},
							{
								Type:               vmopv1.VirtualMachineConditionImageReady,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionTrue),
							},
							{
								Type:               vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
								Status:             corev1.ConditionTrue,
								LastTransitionTime: now,
								Reason:             string(corev1.ConditionTrue),
							},
						},
					},
				}

				hub := &vmopv1.VirtualMachine{}
				g.Expect(spoke.ConvertTo(hub)).To(Succeed())
				p := findCond(hub.Status.Conditions, string(vmopv1a1.VirtualMachinePrereqReadyCondition))
				g.Expect(p).To(BeNil())

				g.Expect(hub.Status.Conditions).To(HaveLen(3))

				c := hub.Status.Conditions[0]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionClassReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionFalse)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[1]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionImageReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))

				c = hub.Status.Conditions[2]
				g.Expect(c.Type).To(Equal(vmopv1.VirtualMachineConditionVMSetResourcePolicyReady))
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(c.Reason).To(Equal(string(metav1.ConditionTrue)))
				g.Expect(c.LastTransitionTime).To(Equal(now))
			})
		})
	})

	t.Run("VirtualMachine spoke-hub-spoke with TKG CP", func(t *testing.T) {
		g := NewWithT(t)

		spoke := vmopv1a1.VirtualMachine{
			Spec: vmopv1a1.VirtualMachineSpec{
				ClassName: "best-effort-small",
				ImageName: "vmi-d5973af773e94c1d8",
				NetworkInterfaces: []vmopv1a1.VirtualMachineNetworkInterface{
					{
						NetworkName: "primary",
						NetworkType: "vsphere-distributed",
					},
				},
				PowerOffMode: vmopv1a1.VirtualMachinePowerOpModeHard,
				PowerState:   vmopv1a1.VirtualMachinePoweredOn,
				ReadinessProbe: &vmopv1a1.Probe{
					TCPSocket: &vmopv1a1.TCPSocketAction{
						Port: intstr.FromInt(6443),
						Host: "10.1.1.10",
					},
				},
				ResourcePolicyName: "tkg1",
				RestartMode:        vmopv1a1.VirtualMachinePowerOpModeHard,
				StorageClass:       "wcpglobal-storage-profile",
				SuspendMode:        vmopv1a1.VirtualMachinePowerOpModeHard,
				VmMetadata: &vmopv1a1.VirtualMachineMetadata{
					SecretName: "tkg1-pg4ct-mvqgx",
					Transport:  vmopv1a1.VirtualMachineMetadataCloudInitTransport,
				},
				Volumes: []vmopv1a1.VirtualMachineVolume{
					{
						Name: "my-volume",
						PersistentVolumeClaim: &vmopv1a1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-claim",
							},
						},
					},
				},
			},
			Status: vmopv1a1.VirtualMachineStatus{
				Phase: vmopv1a1.Unknown,
			},
		}

		spokeHubSpoke(g, &spoke, &vmopv1.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub-spoke with ExtraConfig", func(t *testing.T) {
		g := NewWithT(t)

		// This transport is really old and probably never used.
		spoke := vmopv1a1.VirtualMachine{
			Spec: vmopv1a1.VirtualMachineSpec{
				VmMetadata: &vmopv1a1.VirtualMachineMetadata{
					Transport:  vmopv1a1.VirtualMachineMetadataExtraConfigTransport,
					SecretName: "my-secret",
				},
			},
			Status: vmopv1a1.VirtualMachineStatus{
				Phase: vmopv1a1.Unknown,
			},
		}

		spokeHubSpoke(g, &spoke, &vmopv1.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub-spoke with ConfigMapName", func(t *testing.T) {
		g := NewWithT(t)

		spoke := vmopv1a1.VirtualMachine{
			Spec: vmopv1a1.VirtualMachineSpec{
				VmMetadata: &vmopv1a1.VirtualMachineMetadata{
					Transport:     vmopv1a1.VirtualMachineMetadataCloudInitTransport,
					ConfigMapName: "my-configMap",
				},
			},
			Status: vmopv1a1.VirtualMachineStatus{
				Phase: vmopv1a1.Unknown,
			},
		}

		spokeHubSpoke(g, &spoke, &vmopv1.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub with ConfigMapName", func(t *testing.T) {
		g := NewWithT(t)

		spoke := vmopv1a1.VirtualMachine{
			Spec: vmopv1a1.VirtualMachineSpec{
				VmMetadata: &vmopv1a1.VirtualMachineMetadata{
					Transport:     vmopv1a1.VirtualMachineMetadataCloudInitTransport,
					ConfigMapName: "my-configMap",
				},
			},
			Status: vmopv1a1.VirtualMachineStatus{
				Phase: vmopv1a1.Unknown,
			},
		}

		var hub vmopv1.VirtualMachine
		g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
		g.Expect(hub.Spec.Bootstrap.CloudInit.RawCloudConfig)
		anno := hub.GetAnnotations()
		g.Expect(anno).ToNot(BeNil())
		g.Expect(anno[vmopv1.V1alpha1ConfigMapTransportAnnotation]).To(Equal("true"))
	})

	t.Run("VirtualMachine hub-spoke-hub with spec.image", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image: &vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "vmi-123",
				},
				ClassName:    "my-class",
				StorageClass: "my-storage-class",
			},
		}
		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub with image name", func(t *testing.T) {

		t.Run("generation == 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a1.VirtualMachine{
				Spec: vmopv1a1.VirtualMachineSpec{
					ImageName: "vmi-123",
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).To(BeNil())
			g.Expect(hub.Spec.ImageName).To(Equal(spoke.Spec.ImageName))
		})

		t.Run("generation > 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Spec: vmopv1a1.VirtualMachineSpec{
					ImageName: "vmi-123",
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).ToNot(BeNil())
			g.Expect(hub.Spec.Image.Kind).To(BeEmpty())
			g.Expect(hub.Spec.Image.Name).To(Equal(spoke.Spec.ImageName))
			g.Expect(hub.Spec.ImageName).To(Equal(spoke.Spec.ImageName))
		})
	})

	t.Run("VirtualMachine hub-spoke with empty image name", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image: &vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "vmi-123",
				},
			},
		}
		var spoke vmopv1a1.VirtualMachine
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
		g.Expect(spoke.Spec.ImageName).To(Equal("vmi-123"))
	})

	t.Run("VirtualMachine hub-spoke-hub with bios UUID", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				BiosUUID: "123",
			},
		}
		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with cloud-init instance ID", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						InstanceID: "123",
					},
				},
			},
		}
		hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
	})

	t.Run("VirtualMachine status.storage", func(t *testing.T) {
		t.Run("hub-spoke-hub", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachine{
				Status: vmopv1.VirtualMachineStatus{
					Storage: &vmopv1.VirtualMachineStorageStatus{
						Committed:   ptrOf(resource.MustParse("10Gi")),
						Uncommitted: ptrOf(resource.MustParse("20Gi")),
						Unshared:    ptrOf(resource.MustParse("9Gi")),
					},
				},
			}
			hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
		})
	})

	t.Run("VirtualMachine status.volumes", func(t *testing.T) {
		t.Run("hub-spoke-hub", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachine{
				Status: vmopv1.VirtualMachineStatus{
					Volumes: []vmopv1.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
							Type: vmopv1.VirtualMachineStorageDiskTypeManaged,
						},
						{
							Name: "vol2",
							Type: vmopv1.VirtualMachineStorageDiskTypeClassic,
						},
					},
				},
			}
			hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
		})
		t.Run("hub-spoke", func(t *testing.T) {
			hub := vmopv1.VirtualMachine{
				Status: vmopv1.VirtualMachineStatus{
					Volumes: []vmopv1.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
							Type: vmopv1.VirtualMachineStorageDiskTypeManaged,
						},
						{
							Name: "vol2",
							Type: vmopv1.VirtualMachineStorageDiskTypeClassic,
						},
					},
				},
			}

			expectedSpoke := vmopv1a1.VirtualMachine{
				Status: vmopv1a1.VirtualMachineStatus{
					Phase: vmopv1a1.Unknown,
					Volumes: []vmopv1a1.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
						},
					},
				},
			}

			g := NewWithT(t)
			g.Expect(utilconversion.MarshalData(&hub, &expectedSpoke)).To(Succeed())

			var spoke vmopv1a1.VirtualMachine
			g.Expect(spoke.ConvertFrom(hub.DeepCopy())).To(Succeed())
			g.Expect(apiequality.Semantic.DeepEqual(spoke, expectedSpoke)).To(BeTrue(), cmp.Diff(spoke, expectedSpoke))
		})

		t.Run("spoke-hub", func(t *testing.T) {

			spoke := vmopv1a1.VirtualMachine{
				Status: vmopv1a1.VirtualMachineStatus{
					Volumes: []vmopv1a1.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
						},
					},
				},
			}

			expectedHub := vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Status: vmopv1.VirtualMachineStatus{
					Volumes: []vmopv1.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
							Type: vmopv1.VirtualMachineStorageDiskTypeManaged,
						},
					},
				},
			}

			g := NewWithT(t)
			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(apiequality.Semantic.DeepEqual(hub, expectedHub)).To(BeTrue(), cmp.Diff(hub, expectedHub))
		})
	})

	t.Run("VirtualMachine and spec.crypto", func(t *testing.T) {

		t.Run("hub-spoke-hub", func(t *testing.T) {

			t.Run("spec.crypto is nil", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{}
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
			})

			t.Run("spec.crypto is empty", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
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
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
			})

			t.Run("spec.crypto.useDefaultKeyProvider is &true", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							UseDefaultKeyProvider: &[]bool{true}[0],
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
			})

			t.Run("spec.crypto.useDefaultKeyProvider is &false", func(t *testing.T) {
				g := NewWithT(t)
				hub := vmopv1.VirtualMachine{
					Spec: vmopv1.VirtualMachineSpec{
						Crypto: &vmopv1.VirtualMachineCryptoSpec{
							UseDefaultKeyProvider: &[]bool{false}[0],
						},
					},
				}
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
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
				hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
			})
		})
	})

	t.Run("VirtualMachine pauseAnnotation rename", func(t *testing.T) {
		t.Run("VirtualMachine hub-spoke-hub after pauseAnnotation rename", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{vmopv1.PauseAnnotation: "true"},
				},
			}
			hubSpokeHub(g, &hub, &vmopv1a1.VirtualMachine{})
		})

		t.Run("VirtualMachine hub-spoke pauseAnnotation rename", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{vmopv1.PauseAnnotation: "true"},
				},
			}

			var spoke vmopv1a1.VirtualMachine
			g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
			anno := hub.GetAnnotations()
			g.Expect(anno).ToNot(BeNil())
			g.Expect(anno[vmopv1a1.PauseAnnotation]).To(Equal("true"))
			g.Expect(anno).ShouldNot(HaveKey(vmopv1.PauseAnnotation))
		})
	})
}
