// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	nextver "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	nextver_cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	nextver_common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	nextver_sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
)

func TestVirtualMachineConversion(t *testing.T) {
	g := NewWithT(t)

	hubSpokeHub := func(hub conversion.Hub, spoke conversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(conversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(conversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		hubAfter := hub.DeepCopyObject().(conversion.Hub)
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
	}

	spokeHubSpoke := func(spoke conversion.Convertible, hub conversion.Hub) {
		spokeBefore := spoke.DeepCopyObject().(conversion.Convertible)

		// First convert spoke to hub
		hubCopy := hub.DeepCopyObject().(conversion.Hub)
		g.Expect(spokeBefore.ConvertTo(hubCopy)).To(Succeed())

		// Convert hub back to spoke and check if the resulting spoke is equal to the spoke before the round trip
		spokeAfter := spoke.DeepCopyObject().(conversion.Convertible)
		g.Expect(spokeAfter.ConvertFrom(hubCopy)).To(Succeed())

		metaAfter := spokeAfter.(metav1.Object)
		delete(metaAfter.GetAnnotations(), utilconversion.AnnotationKey)

		g.Expect(apiequality.Semantic.DeepEqual(spokeBefore, spokeAfter)).To(BeTrue(), cmp.Diff(spokeBefore, spokeAfter))
	}

	t.Run("VirtualMachine hub-spoke-hub", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				ImageName:    "my-name",
				ClassName:    "my-class",
				StorageClass: "my-storage-class",
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					CloudInit: &nextver.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &nextver_common.SecretKeySelector{
							Name: "cloud-init-secret",
							Key:  "user-data",
						},
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
				},
				Network: &nextver.VirtualMachineNetworkSpec{
					HostName: "my-test-vm",
					Disabled: true,
					Interfaces: []nextver.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "vds-interface",
							Network: nextver_common.PartialObjectRef{
								TypeMeta: metav1.TypeMeta{
									Kind:       "Network",
									APIVersion: "netoperator.vmware.com/v1alpha1",
								},
								Name: "primary",
							},
						},
						{
							Name: "ncp-interface",
							Network: nextver_common.PartialObjectRef{
								TypeMeta: metav1.TypeMeta{
									Kind:       "VirtualNetwork",
									APIVersion: "vmware.com/v1alpha1",
								},
								Name: "segment1",
							},
						},
						{
							Name: "my-interface",
							Network: nextver_common.PartialObjectRef{
								TypeMeta: metav1.TypeMeta{
									Kind:       "Network",
									APIVersion: "netoperator.vmware.com/v1alpha1",
								},
								Name: "secondary",
							},
							Addresses:   []string{"1.1.1.11", "2.2.2.22"},
							DHCP4:       true,
							DHCP6:       true,
							Gateway4:    "1.1.1.1",
							Gateway6:    "2.2.2.2",
							MTU:         ptrOf[int64](9000),
							Nameservers: []string{"9.9.9.9"},
							Routes: []nextver.VirtualMachineNetworkRouteSpec{
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
				PowerState:      nextver.VirtualMachinePowerStateOff,
				PowerOffMode:    nextver.VirtualMachinePowerOpModeHard,
				SuspendMode:     nextver.VirtualMachinePowerOpModeTrySoft,
				NextRestartTime: "tomorrow",
				RestartMode:     nextver.VirtualMachinePowerOpModeSoft,
				Volumes: []nextver.VirtualMachineVolume{
					{
						Name: "my-volume",
						VirtualMachineVolumeSource: nextver.VirtualMachineVolumeSource{
							PersistentVolumeClaim: ptrOf(nextver.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-claim",
								},
							}),
						},
					},
					{
						Name: "my-volume-2",
						VirtualMachineVolumeSource: nextver.VirtualMachineVolumeSource{
							PersistentVolumeClaim: ptrOf(nextver.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-claim-2",
								},
								InstanceVolumeClaim: &nextver.InstanceVolumeClaimVolumeSource{
									StorageClass: "instance-storage-class",
									Size:         resource.MustParse("2048k"),
								},
							}),
						},
					},
				},
				ReadinessProbe: &nextver.VirtualMachineReadinessProbeSpec{
					TCPSocket: &nextver.TCPSocketAction{
						Host: "some-host",
						Port: intstr.FromString("https"),
					},
					GuestHeartbeat: &nextver.GuestHeartbeatAction{
						ThresholdStatus: nextver.RedHeartbeatStatus,
					},
					GuestInfo: []nextver.GuestInfoAction{
						{
							Key:   "guest-key",
							Value: "guest-value",
						},
					},
					TimeoutSeconds: 100,
					PeriodSeconds:  200,
				},
				Advanced: &nextver.VirtualMachineAdvancedSpec{
					BootDiskCapacity:              ptrOf(resource.MustParse("1024k")),
					DefaultVolumeProvisioningMode: nextver.VirtualMachineVolumeProvisioningModeThickEagerZero,
					ChangeBlockTracking:           true,
				},
				Reserved: &nextver.VirtualMachineReservedSpec{
					ResourcePolicyName: "my-resource-policy",
				},
				MinHardwareVersion: 42,
			},
		}

		hubSpokeHub(&hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined CloudInit", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					CloudInit: &nextver.VirtualMachineBootstrapCloudInitSpec{
						CloudConfig: &nextver_cloudinit.CloudConfig{
							Timezone: "my-tz",
							Users: []nextver_cloudinit.User{
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

		hubSpokeHub(&hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined Sysprep", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					Sysprep: &nextver.VirtualMachineBootstrapSysprepSpec{
						Sysprep: &nextver_sysprep.Sysprep{
							GUIRunOnce: nextver_sysprep.GUIRunOnce{
								Commands: []string{"echo", "hello"},
							},
							GUIUnattended: nextver_sysprep.GUIUnattended{
								AutoLogon: true,
							},
							Identification: nextver_sysprep.Identification{
								DomainAdmin: "my-admin",
							},
							UserData: nextver_sysprep.UserData{
								FullName: "vmware",
							},
						},
					},
				},
			},
		}

		hubSpokeHub(&hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with LinuxPrep and vAppConfig", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					LinuxPrep: &nextver.VirtualMachineBootstrapLinuxPrepSpec{
						HardwareClockIsUTC: true,
						TimeZone:           "my-tz",
					},
					VAppConfig: &nextver.VirtualMachineBootstrapVAppConfigSpec{
						Properties: []nextver_common.KeyValueOrSecretKeySelectorPair{
							{
								Key: "my-key",
								Value: nextver_common.ValueOrSecretKeySelector{
									Value: ptrOf("my-value"),
								},
							},
						},
						RawProperties: "my-secret",
					},
				},
			},
		}

		hubSpokeHub(&hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with vAppConfig", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					VAppConfig: &nextver.VirtualMachineBootstrapVAppConfigSpec{
						Properties: []nextver_common.KeyValueOrSecretKeySelectorPair{
							{
								Key: "my-key",
								Value: nextver_common.ValueOrSecretKeySelector{
									Value: ptrOf("my-value"),
								},
							},
						},
						RawProperties: "my-secret",
					},
				},
			},
		}

		hubSpokeHub(&hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke Status", func(t *testing.T) {
		hub := nextver.VirtualMachine{
			Status: nextver.VirtualMachineStatus{
				Network: &nextver.VirtualMachineNetworkStatus{
					PrimaryIP4: "192.168.1.10",
					Interfaces: []nextver.VirtualMachineNetworkInterfaceStatus{
						{
							Name: "eth0",
							IP: nextver.VirtualMachineNetworkInterfaceIPStatus{
								Addresses: []nextver.VirtualMachineNetworkInterfaceIPAddrStatus{
									{
										Address: "my-ip",
									},
								},
								MACAddr: "my-mac",
							},
						},
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:   nextver.VirtualMachineConditionCreated,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		spoke := &v1alpha1.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())

		g.Expect(spoke.Status.VmIp).To(Equal("192.168.1.10"))
		g.Expect(spoke.Status.NetworkInterfaces[0].MacAddress).To(Equal("my-mac"))
		g.Expect(spoke.Status.NetworkInterfaces[0].IpAddresses).To(Equal([]string{"my-ip"}))
		g.Expect(spoke.Status.Phase).To(Equal(v1alpha1.Created))
	})

	t.Run("VirtualMachine spoke-hub-spoke with TKG CP", func(t *testing.T) {
		spoke := v1alpha1.VirtualMachine{
			Spec: v1alpha1.VirtualMachineSpec{
				ClassName: "best-effort-small",
				ImageName: "vmi-d5973af773e94c1d8",
				NetworkInterfaces: []v1alpha1.VirtualMachineNetworkInterface{
					{
						NetworkName: "primary",
						NetworkType: "vsphere-distributed",
					},
				},
				PowerOffMode: v1alpha1.VirtualMachinePowerOpModeHard,
				PowerState:   v1alpha1.VirtualMachinePoweredOn,
				ReadinessProbe: &v1alpha1.Probe{
					TCPSocket: &v1alpha1.TCPSocketAction{
						Port: intstr.FromInt(6443),
						Host: "10.1.1.10",
					},
				},
				ResourcePolicyName: "tkg1",
				RestartMode:        v1alpha1.VirtualMachinePowerOpModeHard,
				StorageClass:       "wcpglobal-storage-profile",
				SuspendMode:        v1alpha1.VirtualMachinePowerOpModeHard,
				VmMetadata: &v1alpha1.VirtualMachineMetadata{
					SecretName: "tkg1-pg4ct-mvqgx",
					Transport:  v1alpha1.VirtualMachineMetadataCloudInitTransport,
				},
				Volumes: []v1alpha1.VirtualMachineVolume{
					{
						Name: "my-volume",
						PersistentVolumeClaim: &v1alpha1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "my-claim",
							},
						},
					},
				},
			},
			Status: v1alpha1.VirtualMachineStatus{
				Phase: v1alpha1.Unknown,
			},
		}

		spokeHubSpoke(&spoke, &nextver.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub-spoke with CloudInit EC", func(t *testing.T) {
		// This transport is really old and probably never used.
		spoke := v1alpha1.VirtualMachine{
			Spec: v1alpha1.VirtualMachineSpec{
				VmMetadata: &v1alpha1.VirtualMachineMetadata{
					Transport:  v1alpha1.VirtualMachineMetadataExtraConfigTransport,
					SecretName: "my-secret",
				},
			},
			Status: v1alpha1.VirtualMachineStatus{
				Phase: v1alpha1.Unknown,
			},
		}

		spokeHubSpoke(&spoke, &nextver.VirtualMachine{})
	})
}
