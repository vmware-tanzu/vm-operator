// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"
	"time"

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

	hubSpokeHub := func(g *WithT, hub conversion.Hub, spoke conversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(conversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(conversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		hubAfter := hub.DeepCopyObject().(conversion.Hub)
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
	}

	spokeHubSpoke := func(g *WithT, spoke conversion.Convertible, hub conversion.Hub) {
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
		g := NewWithT(t)

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

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with CloudInit", func(t *testing.T) {
		g := NewWithT(t)

		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					CloudInit: &nextver.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: &nextver_common.SecretKeySelector{
							Name: "cloudinit-secret",
							Key:  "my-key",
						},
						SSHAuthorizedKeys: []string{"my-ssh-key"},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined CloudInit", func(t *testing.T) {
		g := NewWithT(t)

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

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with Sysprep", func(t *testing.T) {
		g := NewWithT(t)

		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					Sysprep: &nextver.VirtualMachineBootstrapSysprepSpec{
						RawSysprep: &nextver_common.SecretKeySelector{
							Name: "sysprep-secret",
							Key:  "my-key",
						},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with inlined Sysprep", func(t *testing.T) {
		g := NewWithT(t)

		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					Sysprep: &nextver.VirtualMachineBootstrapSysprepSpec{
						Sysprep: &nextver_sysprep.Sysprep{
							GUIRunOnce: nextver_sysprep.GUIRunOnce{
								Commands: []string{"echo", "hello"},
							},
							GUIUnattended: &nextver_sysprep.GUIUnattended{
								AutoLogon: true,
							},
							Identification: &nextver_sysprep.Identification{
								DomainAdmin: "my-admin",
							},
							UserData: &nextver_sysprep.UserData{
								FullName: "vmware",
							},
						},
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with LinuxPrep and vAppConfig", func(t *testing.T) {
		g := NewWithT(t)

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

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with LinuxPrep", func(t *testing.T) {
		g := NewWithT(t)

		hub := nextver.VirtualMachine{
			Spec: nextver.VirtualMachineSpec{
				Bootstrap: &nextver.VirtualMachineBootstrapSpec{
					LinuxPrep: &nextver.VirtualMachineBootstrapLinuxPrepSpec{
						HardwareClockIsUTC: true,
						TimeZone:           "my-tz",
					},
				},
			},
		}

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with vAppConfig", func(t *testing.T) {
		g := NewWithT(t)

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

		hubSpokeHub(g, &hub, &v1alpha1.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke Status", func(t *testing.T) {
		g := NewWithT(t)

		now := time.Now()
		hub := nextver.VirtualMachine{
			Status: nextver.VirtualMachineStatus{
				Host:       "my-host",
				PowerState: nextver.VirtualMachinePowerStateOn,
				Conditions: []metav1.Condition{
					{
						Type:   nextver.VirtualMachineConditionCreated,
						Status: metav1.ConditionTrue,
					},
				},
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
				UniqueID:     "my-unique-id",
				BiosUUID:     "my-bios-uuid",
				InstanceUUID: "my-inst-id",
				Volumes: []nextver.VirtualMachineVolumeStatus{
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

		spoke := &v1alpha1.VirtualMachine{}
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())

		g.Expect(spoke.Status.Host).To(Equal(hub.Status.Host))
		g.Expect(spoke.Status.PowerState).To(Equal(v1alpha1.VirtualMachinePoweredOn))
		g.Expect(spoke.Status.VmIp).To(Equal(hub.Status.Network.PrimaryIP4))
		g.Expect(spoke.Status.NetworkInterfaces[0].MacAddress).To(Equal("my-mac"))
		g.Expect(spoke.Status.NetworkInterfaces[0].IpAddresses).To(Equal([]string{"my-ip"}))
		g.Expect(spoke.Status.Phase).To(Equal(v1alpha1.Created))
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
		spoke := v1alpha1.VirtualMachine{
			Status: v1alpha1.VirtualMachineStatus{
				Host:       "my-host",
				PowerState: v1alpha1.VirtualMachinePoweredOff,
				Phase:      v1alpha1.Created,
				Conditions: []v1alpha1.Condition{
					{
						Type:   "Cond",
						Status: corev1.ConditionTrue,
					},
				},
				VmIp:         "192.168.1.11",
				UniqueID:     "my-unique-id",
				BiosUUID:     "my-bios-uuid",
				InstanceUUID: "my-inst-id",
				Volumes: []v1alpha1.VirtualMachineVolumeStatus{
					{
						Name:     "my-disk",
						Attached: true,
						DiskUuid: "my-disk-uuid",
						Error:    "my-disk-error",
					},
				},
				ChangeBlockTracking: ptrOf(true),
				NetworkInterfaces: []v1alpha1.NetworkInterfaceStatus{
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

		hub := &nextver.VirtualMachine{}
		g.Expect(spoke.ConvertTo(hub)).To(Succeed())

		g.Expect(hub.Status.Host).To(Equal(spoke.Status.Host))
		g.Expect(hub.Status.PowerState).To(Equal(nextver.VirtualMachinePowerStateOff))
		g.Expect(hub.Status.Conditions).To(HaveLen(1))
		g.Expect(hub.Status.Conditions[0].Type).To(Equal("Cond"))
		g.Expect(hub.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(hub.Status.Conditions[0].Reason).To(Equal(string(metav1.ConditionTrue)))
		g.Expect(hub.Status.Network).ToNot(BeNil())
		g.Expect(hub.Status.Network.PrimaryIP4).To(Equal(spoke.Status.VmIp))
		g.Expect(hub.Status.Network.Interfaces).To(HaveLen(1))
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
			Type:               nextver.VirtualMachineConditionClassReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(1, 0, 0)),
		}
		vmImageCond := metav1.Condition{
			Type:               nextver.VirtualMachineConditionImageReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(2, 0, 0)),
		}
		vmSetRPCond := metav1.Condition{
			Type:               nextver.VirtualMachineConditionVMSetResourcePolicyReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(3, 0, 0)),
		}
		vmBSCond := metav1.Condition{
			Type:               nextver.VirtualMachineConditionBootstrapReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(now.AddDate(4, 0, 0)),
		}

		findCond := func(conditions []v1alpha1.Condition, t v1alpha1.ConditionType) *v1alpha1.Condition {
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

				hub := nextver.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond}

				spoke := &v1alpha1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(c.Reason).To(BeEmpty())
				g.Expect(c.Message).To(BeEmpty())
				g.Expect(c.LastTransitionTime).To(Equal(vmImageCond.LastTransitionTime))
			})

			t.Run("All Prereq Conditions", func(t *testing.T) {
				g := NewWithT(t)

				hub := nextver.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond, vmSetRPCond, vmBSCond}

				spoke := &v1alpha1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionTrue))
				g.Expect(c.Reason).To(BeEmpty())
				g.Expect(c.Message).To(BeEmpty())
				g.Expect(c.LastTransitionTime).To(Equal(vmImageCond.LastTransitionTime))
			})

			t.Run("Existing PrereqReady is in spoke", func(t *testing.T) {
				g := NewWithT(t)

				hub := nextver.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, vmImageCond, vmSetRPCond, vmBSCond}

				spoke := &v1alpha1.VirtualMachine{
					Status: v1alpha1.VirtualMachineStatus{
						Conditions: []v1alpha1.Condition{
							{
								Type:               v1alpha1.VirtualMachinePrereqReadyCondition,
								Status:             corev1.ConditionFalse,
								Reason:             "should be updated",
								LastTransitionTime: metav1.NewTime(now.AddDate(1000, 0, 0)),
							},
						},
					},
				}

				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
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

				hub := nextver.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{notReadyC}

				spoke := &v1alpha1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(v1alpha1.VirtualMachineClassNotFoundReason))
				g.Expect(c.Message).To(Equal("foobar"))
				g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))
			})

			t.Run("Image not ready", func(t *testing.T) {
				g := NewWithT(t)

				notReadyC := vmImageCond
				notReadyC.Status = metav1.ConditionFalse
				notReadyC.Message = "foobar"

				hub := nextver.VirtualMachine{}
				hub.Status.Conditions = []metav1.Condition{vmClassCond, notReadyC}

				spoke := &v1alpha1.VirtualMachine{}
				g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
				g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

				c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
				g.Expect(c).ToNot(BeNil())
				g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(c.Reason).To(Equal(v1alpha1.VirtualMachineImageNotFoundReason))
				g.Expect(c.Message).To(Equal("foobar"))
				g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))

				t.Run("Existing PrereqReady in spoke", func(t *testing.T) {
					spoke := &v1alpha1.VirtualMachine{
						Status: v1alpha1.VirtualMachineStatus{
							Conditions: []v1alpha1.Condition{
								{
									Type:               v1alpha1.VirtualMachinePrereqReadyCondition,
									Status:             corev1.ConditionFalse,
									Reason:             "should be updated",
									LastTransitionTime: metav1.NewTime(now.AddDate(1000, 0, 0)),
								},
							},
						},
					}

					g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
					g.Expect(spoke.Status.Conditions).To(HaveLen(len(hub.Status.Conditions) + 1))

					c := findCond(spoke.Status.Conditions, v1alpha1.VirtualMachinePrereqReadyCondition)
					g.Expect(c).ToNot(BeNil())
					g.Expect(c.Status).To(Equal(corev1.ConditionFalse))
					g.Expect(c.Reason).To(Equal(v1alpha1.VirtualMachineImageNotFoundReason))
					g.Expect(c.Message).To(Equal("foobar"))
					g.Expect(c.LastTransitionTime).To(Equal(notReadyC.LastTransitionTime))
				})
			})
		})
	})

	t.Run("VirtualMachine spoke-hub Status Conditions", func(t *testing.T) {

		t.Run("Converts Condition", func(t *testing.T) {
			g := NewWithT(t)

			now := metav1.Now()
			spoke := v1alpha1.VirtualMachine{
				Status: v1alpha1.VirtualMachineStatus{
					Conditions: []v1alpha1.Condition{
						{
							Type:               v1alpha1.GuestCustomizationCondition,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: now,
							Reason:             "Reason",
							Message:            "Message",
						},
					},
				},
			}

			hub := &nextver.VirtualMachine{}
			g.Expect(spoke.ConvertTo(hub)).To(Succeed())
			g.Expect(hub.Status.Conditions).To(HaveLen(1))

			c := hub.Status.Conditions[0]
			g.Expect(c.Type).To(Equal(nextver.GuestCustomizationCondition))
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.LastTransitionTime).To(Equal(now))
			g.Expect(c.Reason).To(Equal("Reason"))
			g.Expect(c.Message).To(Equal("Message"))
		})
	})

	t.Run("VirtualMachine spoke-hub-spoke with TKG CP", func(t *testing.T) {
		g := NewWithT(t)

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

		spokeHubSpoke(g, &spoke, &nextver.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub-spoke with CloudInit EC", func(t *testing.T) {
		g := NewWithT(t)

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

		spokeHubSpoke(g, &spoke, &nextver.VirtualMachine{})
	})
}
