// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

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
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	vmopv1a2sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha2/sysprep"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha3/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
)

func TestVirtualMachineConversion(t *testing.T) {

	hubSpokeHub := func(g *WithT, hub, hubAfter ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
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
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
					LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{
						HardwareClockIsUTC: &[]bool{true}[0],
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
							},
							UserData: &vmopv1sysprep.UserData{
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
						AllowGuestControl: true,
						Connected:         true,
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
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
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
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
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
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
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
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

			expectedSpoke := vmopv1a2.VirtualMachine{
				Status: vmopv1a2.VirtualMachineStatus{
					Volumes: []vmopv1a2.VirtualMachineVolumeStatus{
						{
							Name: "vol1",
						},
					},
				},
			}

			g := NewWithT(t)
			g.Expect(utilconversion.MarshalData(&hub, &expectedSpoke)).To(Succeed())

			var spoke vmopv1a2.VirtualMachine
			g.Expect(spoke.ConvertFrom(hub.DeepCopy())).To(Succeed())
			g.Expect(apiequality.Semantic.DeepEqual(spoke, expectedSpoke)).To(BeTrue(), cmp.Diff(spoke, expectedSpoke))
		})

		t.Run("spoke-hub", func(t *testing.T) {

			spoke := vmopv1a2.VirtualMachine{
				Status: vmopv1a2.VirtualMachineStatus{
					Volumes: []vmopv1a2.VirtualMachineVolumeStatus{
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

	t.Run("VirtualMachine spoke-hub with image status", func(t *testing.T) {

		t.Run("generation == 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a2.VirtualMachine{
				Status: vmopv1a2.VirtualMachineStatus{
					Image: &vmopv1a2common.LocalObjectRef{
						Kind: "VirtualMachineImage",
						Name: "vmi-123",
					},
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).To(BeNil())
		})

		t.Run("generation > 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: vmopv1a2.VirtualMachineStatus{
					Image: &vmopv1a2common.LocalObjectRef{
						Kind: "VirtualMachineImage",
						Name: "vmi-123",
					},
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).ToNot(BeNil())
			g.Expect(hub.Spec.Image.Kind).To(Equal(spoke.Status.Image.Kind))
			g.Expect(hub.Spec.Image.Name).To(Equal(spoke.Status.Image.Name))
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
		var spoke vmopv1a2.VirtualMachine
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
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
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
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
	})

	t.Run("VirtualMachine and spec.network.domainName", func(t *testing.T) {

		const (
			domainNameFuBar      = "fu.bar"
			domainNameHelloWorld = "hello.world"
		)

		t.Run("hub-spoke-hub", func(t *testing.T) {
			g := NewWithT(t)
			hub := vmopv1.VirtualMachine{
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						DomainName: domainNameHelloWorld,
					},
				},
			}
			hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
		})

		t.Run("spoke-hub", func(t *testing.T) {

			newVmopv1BootstrapWithInlineSysprep := func() *vmopv1.VirtualMachineBootstrapSpec {
				return &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
						Sysprep: &vmopv1sysprep.Sysprep{
							Identification: &vmopv1sysprep.Identification{},
							GUIRunOnce:     &vmopv1sysprep.GUIRunOnce{},
						},
					},
				}
			}

			newVmopv1a2BootstrapWithInlineSysprep := func(jd string) *vmopv1a2.VirtualMachineBootstrapSpec {
				return &vmopv1a2.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1a2.VirtualMachineBootstrapSysprepSpec{
						Sysprep: &vmopv1a2sysprep.Sysprep{
							Identification: &vmopv1a2sysprep.Identification{
								JoinDomain: jd,
							},
						},
					},
				}
			}

			testCases := []struct {
				name   string
				spoke  vmopv1a2.VirtualMachine
				inHub  vmopv1.VirtualMachine
				outHub vmopv1.VirtualMachine
			}{
				{
					name: "joinDomain is empty and marshaled domainName is empty",
				},
				{
					name: "joinDomain is empty and marshaled domainName is non-empty",
					spoke: vmopv1a2.VirtualMachine{
						Spec: vmopv1a2.VirtualMachineSpec{
							Bootstrap: newVmopv1a2BootstrapWithInlineSysprep(""),
						},
					},
					inHub: vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Network: &vmopv1.VirtualMachineNetworkSpec{
								DomainName: domainNameHelloWorld,
							},
						},
					},
					outHub: vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Network: &vmopv1.VirtualMachineNetworkSpec{
								DomainName: domainNameHelloWorld,
							},
							Bootstrap: newVmopv1BootstrapWithInlineSysprep(),
						},
					},
				},
				{
					name: "joinDomain is non-empty and marshaled domainName is empty",
					spoke: vmopv1a2.VirtualMachine{
						Spec: vmopv1a2.VirtualMachineSpec{
							Bootstrap: newVmopv1a2BootstrapWithInlineSysprep(domainNameHelloWorld),
						},
					},
					outHub: vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Network: &vmopv1.VirtualMachineNetworkSpec{
								DomainName: domainNameHelloWorld,
							},
							Bootstrap: newVmopv1BootstrapWithInlineSysprep(),
						},
					},
				},
				{
					name: "joinDomain is non-empty and does equal marshaled domainName",
					spoke: vmopv1a2.VirtualMachine{
						Spec: vmopv1a2.VirtualMachineSpec{
							Bootstrap: newVmopv1a2BootstrapWithInlineSysprep(domainNameFuBar),
						},
					},
					inHub: vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Network: &vmopv1.VirtualMachineNetworkSpec{
								DomainName: domainNameHelloWorld,
							},
						},
					},
					outHub: vmopv1.VirtualMachine{
						Spec: vmopv1.VirtualMachineSpec{
							Network: &vmopv1.VirtualMachineNetworkSpec{
								DomainName: domainNameFuBar,
							},
							Bootstrap: newVmopv1BootstrapWithInlineSysprep(),
						},
					},
				},
			}

			for i := range testCases {
				tc := testCases[i]
				t.Run(tc.name, func(t *testing.T) {
					g := NewWithT(t)
					g.Expect(utilconversion.MarshalData(&tc.inHub, &tc.spoke)).To(Succeed())
					var hub vmopv1.VirtualMachine
					g.Expect(tc.spoke.ConvertTo(&hub)).To(Succeed())
					g.Expect(apiequality.Semantic.DeepEqual(hub, tc.outHub)).To(BeTrue(), cmp.Diff(hub, tc.outHub))
				})
			}
		})
	})
}
