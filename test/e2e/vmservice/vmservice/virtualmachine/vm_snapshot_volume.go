// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	vmSnapshotVolumeSpecName = "vm-snapshot-volume"
)

type VMSnapshotVolumeSpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	ArtifactFolder   string
	WCPNamespaceName string
}

func VMSnapshotVolumeSpec(ctx context.Context, inputGetter func() VMSnapshotVolumeSpecInput) {
	var (
		input                 VMSnapshotVolumeSpecInput
		vmSvcClusterProxy     *common.VMServiceClusterProxy
		vmSvcE2EConfig        *e2eConfig.E2EConfig
		svClusterClient       ctrlclient.Client
		vmSvcClusterResources *e2eConfig.Resources
		vmSvcNamespace        string
		skipCleanup           bool

		randomString    string
		sourceVMName    string
		dataMoverVMName string
		snapshotName    string
	)

	skipChecks := func() {
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.VirtualMachineSnapshotCapabilityName)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.CSIBackupAPICapabilityName)
		skipCleanup = false
	}

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", vmSnapshotVolumeSpecName)
		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", vmSnapshotVolumeSpecName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", vmSnapshotVolumeSpecName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", vmSnapshotVolumeSpecName)

		vmSvcClusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		vmSvcE2EConfig = input.Config
		vmSvcClusterResources = vmSvcE2EConfig.InfraConfig.ManagementClusterConfig.Resources
		vmSvcNamespace = input.WCPNamespaceName
		svClusterClient = vmSvcClusterProxy.GetClient()
		skipCleanup = true

		skipChecks()

		vmSnapshotCancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{vmSvcE2EConfig.GetVariable("VMOPNamespace")},
			vmSvcClusterProxy.GetClientSet(),
			filepath.Join(input.ArtifactFolder, vmSnapshotVolumeSpecName))
		DeferCleanup(vmSnapshotCancelPodWatches)

		randomString = capiutil.RandomString(4)
		sourceVMName = "src-vm-" + randomString
		dataMoverVMName = "dm-vm-" + randomString
		snapshotName = "snap-" + randomString
	})

	AfterEach(func() {
		if skipCleanup {
			return
		}

		vmoperator.VerifyVMDeleted(ctx, svClusterClient, vmSvcE2EConfig, vmSvcNamespace, dataMoverVMName)
		vmoperator.VerifyVMDeleted(ctx, svClusterClient, vmSvcE2EConfig, vmSvcNamespace, sourceVMName)
		vmoperator.EnsureVMSnapshotDeleted(ctx, svClusterClient, vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
			Namespace: vmSvcNamespace,
			Name:      snapshotName,
		})
	})

	Context("VirtualMachineSnapshot volume mounts", func() {
		BeforeEach(func() {
			By("Deploying source VM")
			vmservice.DeployVMWithCloudInitA6(ctx, vmSvcClusterProxy, vmSvcE2EConfig, vmSvcClusterResources, vmSvcNamespace, sourceVMName, "", nil)
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, sourceVMName)
			vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, sourceVMName, "PoweredOn")

			asyncSupervisorFSSEnabled, err := utils.CheckSupervisorCapabilitiesCRDSupport(ctx, svClusterClient)
			Expect(err).NotTo(HaveOccurred())
			allDisksArePVCapabilityEnabled := utils.IsSupervisorCapabilityEnabled(
				ctx,
				vmSvcClusterProxy.GetClientSet(),
				vmSvcClusterProxy.GetDynamicClient(),
				consts.AllDisksArePVCapabilityName,
				asyncSupervisorFSSEnabled)

			if allDisksArePVCapabilityEnabled {
				vmoperator.WaitForBootDiskPVC(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, sourceVMName, nil)
				vmoperator.WaitForVMCnsRegisterVolumesRegistered(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, sourceVMName)
			}

			By("Creating VirtualMachineSnapshot")
			vmservice.CreateVMSnapshotA6(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      snapshotName,
				VMName:    sourceVMName,
			})

			By("Waiting for VirtualMachineSnapshot to be ready and disks populated")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				svClusterClient,
				vmSvcNamespace,
				snapshotName,
				vmopv1.VirtualMachinePowerStateOff,
				false,
				[]vmopv1.VirtualMachineSnapshotReference{})

			Eventually(func(g Gomega) {
				vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
				err := svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: snapshotName}, vmSnapshot)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vmSnapshot.Status.Disks).ToNot(BeEmpty(), "Expected snapshot disks to be populated")
			}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-condition")...).Should(Succeed())
		})

		It("Mount a snapshot disk on a data-mover VM (happy path) and then detach it", Label("vmservice", "storage", "snapshot"), func() {
			var diskID string
			By("Getting snapshot disk ID", func() {
				vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: snapshotName}, vmSnapshot)).To(Succeed())
				Expect(vmSnapshot.Status.Disks).ToNot(BeEmpty())
				diskID = vmSnapshot.Status.Disks[0].ID
			})

			By("Deploying data-mover VM with snapshot volume", func() {
				secretName := "cloud-config-data-" + capiutil.RandomString(4)
				secret := manifestbuilders.Secret{
					Namespace: vmSvcNamespace,
					Name:      secretName,
				}
				secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
				Expect(vmSvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed())

				linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(vmSvcClusterResources)
				linuxVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &vmSvcE2EConfig.Config, svClusterClient, vmSvcNamespace, linuxImageDisplayName)

				vmParameters := manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             dataMoverVMName,
					ImageName:        linuxVMIName,
					VMClassName:      vmSvcClusterResources.VMClassName,
					StorageClassName: vmSvcClusterResources.StorageClassName,
					SecretName:       secretName,
				}
				vmYaml := manifestbuilders.GetVirtualMachineYamlA6(vmParameters)

				vm := &vmopv1.VirtualMachine{}
				Expect(yaml.Unmarshal(vmYaml, vm)).To(Succeed())

				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   snapshotName,
							DiskID: diskID,
						},
					},
					DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
					Removable: ptr.To(true),
				})

				Expect(svClusterClient.Create(ctx, vm)).To(Succeed())
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, dataMoverVMName)
				vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, dataMoverVMName, "PoweredOn")
			})

			By("Asserting snapshot volume is attached", func() {
				Eventually(func(g Gomega) {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: dataMoverVMName}, vm)).To(Succeed())

					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range vm.Status.Volumes {
						if v.Name == "snap-vol" {
							snapVolStatus = &vm.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil())
					g.Expect(snapVolStatus.Attached).To(BeTrue())
					g.Expect(snapVolStatus.DiskUUID).ToNot(BeEmpty())
					g.Expect(snapVolStatus.Error).To(BeEmpty())
				}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
			})

			By("Detaching snapshot disk", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: dataMoverVMName}, vm)).To(Succeed())

				// Remove the snapshot volume
				var newVolumes []vmopv1.VirtualMachineVolume
				for _, v := range vm.Spec.Volumes {
					if v.Name != "snap-vol" {
						newVolumes = append(newVolumes, v)
					}
				}
				vm.Spec.Volumes = newVolumes
				Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			})

			By("Asserting snapshot volume is detached", func() {
				Eventually(func(g Gomega) {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: dataMoverVMName}, vm)).To(Succeed())

					for _, v := range vm.Status.Volumes {
						g.Expect(v.Name).ToNot(Equal("snap-vol"))
					}
				}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
			})
		})

		It("Mount a snapshot disk on the source VM (same disk UUID)", Label("vmservice", "storage", "snapshot"), func() {
			var diskID string
			By("Getting snapshot disk ID", func() {
				vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: snapshotName}, vmSnapshot)).To(Succeed())
				Expect(vmSnapshot.Status.Disks).ToNot(BeEmpty())
				diskID = vmSnapshot.Status.Disks[0].ID
			})

			By("Attaching snapshot volume to the source VM", func() {
				vm := &vmopv1.VirtualMachine{}
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: sourceVMName}, vm)).To(Succeed())

				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "snap-vol-same-uuid",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   snapshotName,
							DiskID: diskID,
						},
					},
					DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
					Removable: ptr.To(true),
				})
				Expect(svClusterClient.Update(ctx, vm)).To(Succeed())
			})

			By("Asserting snapshot volume is attached", func() {
				Eventually(func(g Gomega) {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: sourceVMName}, vm)).To(Succeed())

					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range vm.Status.Volumes {
						if v.Name == "snap-vol-same-uuid" {
							snapVolStatus = &vm.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil())
					g.Expect(snapVolStatus.Attached).To(BeTrue())
					g.Expect(snapVolStatus.DiskUUID).ToNot(BeEmpty())
					g.Expect(snapVolStatus.Error).To(BeEmpty())
				}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
			})
		})

		It("Mount a snapshot disk on a data-mover VM in a different namespace", Label("vmservice", "storage", "snapshot"), func() {
			var diskID string
			By("Getting snapshot disk ID", func() {
				vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
				Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: snapshotName}, vmSnapshot)).To(Succeed())
				Expect(vmSnapshot.Status.Disks).ToNot(BeEmpty())
				diskID = vmSnapshot.Status.Disks[0].ID
			})

			var otherNamespace string
			By("Creating a different namespace", func() {
				otherNamespace = "other-ns-" + capiutil.RandomString(4)
				nsObj := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: otherNamespace,
					},
				}
				Expect(svClusterClient.Create(ctx, nsObj)).To(Succeed())
			})

			By("Deploying data-mover VM in the other namespace with snapshot volume", func() {
				secretName := "cloud-config-data-" + capiutil.RandomString(4)
				secret := manifestbuilders.Secret{
					Namespace: otherNamespace,
					Name:      secretName,
				}
				secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
				Expect(vmSvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed())

				linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(vmSvcClusterResources)
				linuxVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &vmSvcE2EConfig.Config, svClusterClient, otherNamespace, linuxImageDisplayName)

				vmParameters := manifestbuilders.VirtualMachineYaml{
					Namespace:        otherNamespace,
					Name:             dataMoverVMName,
					ImageName:        linuxVMIName,
					VMClassName:      vmSvcClusterResources.VMClassName,
					StorageClassName: vmSvcClusterResources.StorageClassName,
					SecretName:       secretName,
				}
				vmYaml := manifestbuilders.GetVirtualMachineYamlA6(vmParameters)

				vm := &vmopv1.VirtualMachine{}
				Expect(yaml.Unmarshal(vmYaml, vm)).To(Succeed())

				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "snap-vol-cross-ns",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Namespace: vmSvcNamespace,
							Name:      snapshotName,
							DiskID:    diskID,
						},
					},
					DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
					Removable: ptr.To(true),
				})

				Expect(svClusterClient.Create(ctx, vm)).To(Succeed())
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, otherNamespace, dataMoverVMName)
				vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, otherNamespace, dataMoverVMName, "PoweredOn")
			})

			By("Asserting snapshot volume is attached", func() {
				Eventually(func(g Gomega) {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: otherNamespace, Name: dataMoverVMName}, vm)).To(Succeed())

					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range vm.Status.Volumes {
						if v.Name == "snap-vol-cross-ns" {
							snapVolStatus = &vm.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil())
					g.Expect(snapVolStatus.Attached).To(BeTrue())
					g.Expect(snapVolStatus.DiskUUID).ToNot(BeEmpty())
					g.Expect(snapVolStatus.Error).To(BeEmpty())
				}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
			})
			
			By("Cleaning up the other namespace", func() {
				vmoperator.VerifyVMDeleted(ctx, svClusterClient, vmSvcE2EConfig, otherNamespace, dataMoverVMName)
				nsObj := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: otherNamespace,
					},
				}
				Expect(svClusterClient.Delete(ctx, nsObj)).To(Succeed())
			})
		})

		It("Invalid snapshot reference", Label("vmservice", "storage", "snapshot"), func() {
			By("Deploying VM with invalid snapshot volume", func() {
				secretName := "cloud-config-data-" + capiutil.RandomString(4)
				secret := manifestbuilders.Secret{
					Namespace: vmSvcNamespace,
					Name:      secretName,
				}
				secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
				Expect(vmSvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed())

				linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(vmSvcClusterResources)
				linuxVMIName := vmoperator.WaitForVirtualMachineImageName(ctx, &vmSvcE2EConfig.Config, svClusterClient, vmSvcNamespace, linuxImageDisplayName)

				vmParameters := manifestbuilders.VirtualMachineYaml{
					Namespace:        vmSvcNamespace,
					Name:             dataMoverVMName,
					ImageName:        linuxVMIName,
					VMClassName:      vmSvcClusterResources.VMClassName,
					StorageClassName: vmSvcClusterResources.StorageClassName,
					SecretName:       secretName,
				}
				vmYaml := manifestbuilders.GetVirtualMachineYamlA6(vmParameters)

				vm := &vmopv1.VirtualMachine{}
				Expect(yaml.Unmarshal(vmYaml, vm)).To(Succeed())

				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "invalid-snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "non-existent-snapshot",
							DiskID: "invalid-disk-id",
						},
					},
					DiskMode:  vmopv1.VolumeDiskModeIndependentNonPersistent,
					Removable: ptr.To(true),
				})

				Expect(svClusterClient.Create(ctx, vm)).To(Succeed())
				vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, dataMoverVMName)
				vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, dataMoverVMName, "PoweredOn")
			})

			By("Asserting status.volumes[].error is populated", func() {
				Eventually(func(g Gomega) {
					vm := &vmopv1.VirtualMachine{}
					g.Expect(svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSvcNamespace, Name: dataMoverVMName}, vm)).To(Succeed())

					var snapVolStatus *vmopv1.VirtualMachineVolumeStatus
					for i, v := range vm.Status.Volumes {
						if v.Name == "invalid-snap-vol" {
							snapVolStatus = &vm.Status.Volumes[i]
							break
						}
					}
					g.Expect(snapVolStatus).ToNot(BeNil())
					g.Expect(snapVolStatus.Error).ToNot(BeEmpty())
				}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
			})
		})
	})
}
