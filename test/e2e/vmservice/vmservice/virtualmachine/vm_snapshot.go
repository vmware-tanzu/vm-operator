// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	vmSnapshotSpecName = "v1alpha5-vmsnapshot"
)

type VMSnapshotSpecInput struct {
	Config           *e2eConfig.E2EConfig
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	WCPClient        wcp.WorkloadManagementAPI
	ArtifactFolder   string
	WCPNamespaceName string
}

func VMSnapshotSpec(ctx context.Context, inputGetter func() VMSnapshotSpecInput) {
	var (
		vmSnapshotInput       VMSnapshotSpecInput
		vmSvcClusterProxy     *common.VMServiceClusterProxy
		vmSvcE2EConfig        *e2eConfig.E2EConfig
		svClusterClient       ctrlclient.Client
		vmSvcClusterResources *e2eConfig.Resources
		vmSvcNamespace        string
		skipCleanup           bool

		randomString    string
		vmName          string
		vmSnapshot1Name string
		vmSnapshot2Name string
		vmSnapshot3Name string
	)

	vmSnapshotReference := func(name ...string) *vmopv1a5.VirtualMachineSnapshotReference {
		res := &vmopv1a5.VirtualMachineSnapshotReference{
			Type: vmopv1a5.VirtualMachineSnapshotReferenceTypeUnmanaged,
		}
		if len(name) > 0 {
			res = &vmopv1a5.VirtualMachineSnapshotReference{
				Type: vmopv1a5.VirtualMachineSnapshotReferenceTypeManaged,
				Name: name[0],
			}
		}

		return res
	}

	skipChecks := func() {
		skipper.SkipUnlessInfraIs(vmSnapshotInput.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, vmSvcClusterProxy, consts.VirtualMachineSnapshotCapabilityName)
		skipper.SkipUnlessSnapshotFSSEnabled(ctx, vmSvcClusterProxy, vmSvcE2EConfig)

		skipCleanup = false
	}

	BeforeEach(func() {
		vmSnapshotInput = inputGetter()
		Expect(vmSnapshotInput.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", vmSnapshotSpecName)
		Expect(vmSnapshotInput.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", vmSnapshotSpecName)
		Expect(vmSnapshotInput.Config.InfraConfig.ManagementClusterConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig.ManagementClusterConfig can't be nil when calling %s spec", vmSnapshotSpecName)
		Expect(vmSnapshotInput.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", vmSnapshotSpecName)
		Expect(vmSnapshotInput.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", vmSnapshotSpecName)
		Expect(os.MkdirAll(vmSnapshotInput.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", vmSnapshotSpecName)

		vmSvcClusterProxy = vmSnapshotInput.ClusterProxy.(*common.VMServiceClusterProxy)
		vmSvcE2EConfig = vmSnapshotInput.Config
		vmSvcClusterResources = vmSvcE2EConfig.InfraConfig.ManagementClusterConfig.Resources
		vmSvcNamespace = vmSnapshotInput.WCPNamespaceName
		svClusterClient = vmSvcClusterProxy.GetClient()
		skipCleanup = true

		skipChecks()

		vmSnapshotCancelPodWatches := framework.WatchPodLogsAndEventsInNamespaces(
			ctx,
			[]string{vmSvcE2EConfig.GetVariable("VMOPNamespace")},
			vmSvcClusterProxy.GetClientSet(),
			filepath.Join(vmSnapshotInput.ArtifactFolder, vmSnapshotSpecName))
		DeferCleanup(vmSnapshotCancelPodWatches)

		randomString = capiutil.RandomString(4)
		vmName = "vm-" + randomString
		vmSnapshot1Name = "sn-1-" + randomString
		vmSnapshot2Name = "sn-2-" + randomString
		vmSnapshot3Name = "sn-3-" + randomString
	})

	AfterEach(func() {
		if skipCleanup {
			return
		}

		vmoperator.VerifyVMDeleted(ctx, svClusterClient, vmSvcE2EConfig, vmSvcNamespace, vmName)
		vmoperator.EnsureVMSnapshotDeleted(ctx, vmSvcClusterProxy.GetClient(),
			vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot1Name,
			})
		vmoperator.EnsureVMSnapshotDeleted(ctx, vmSvcClusterProxy.GetClient(),
			vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot2Name,
			})
		vmoperator.EnsureVMSnapshotDeleted(ctx, vmSvcClusterProxy.GetClient(),
			vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot3Name,
			})
	})

	When("VM doesn't have PVC", func() {
		BeforeEach(func() {
			vmservice.DeployVMWithCloudInit(ctx, vmSvcClusterProxy, vmSvcClusterResources, vmSvcNamespace, vmName, "", nil)
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, vmName)
			vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, vmName, "PoweredOn")
		})

		It("successfully create, update, revert to, and delete vm snapshots", Label("smoke"), func() {
			By("create vm snapshot 1")
			vmservice.CreateVMSnapshot(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot1Name,
				VMName:    vmName,
				Memory:    true,
				Quiesce:   "10m",
			})

			By("Verifying Snapshot's status, power should be on since snapshot's spec.memory is true")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot1Name,
				vmopv1a5.VirtualMachinePowerStateOn,
				true,
				[]vmopv1a5.VirtualMachineSnapshotReference{})

			By("Verifying VM's Snapshot related status")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot1Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("update vm snapshot 1")
			vmservice.UpdateVMSnapshot(ctx, vmSvcClusterProxy, vmSvcE2EConfig,
				manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace:   vmSvcNamespace,
					Name:        vmSnapshot1Name,
					VMName:      vmName,
					Memory:      true,
					Quiesce:     "10m",
					Description: "updated snapshot 1",
				})

			By("create a new vm snapshot 2")
			vmservice.CreateVMSnapshot(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot2Name,
				VMName:    vmName,
			})

			By("Verifying Snapshot 2's status, power should be off since memory is false")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot2Name,
				vmopv1a5.VirtualMachinePowerStateOff,
				false,
				[]vmopv1a5.VirtualMachineSnapshotReference{})

			By("Verifying Snapshot 1's status, it should have snapshot 2 as child")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot1Name,
				vmopv1a5.VirtualMachinePowerStateOn,
				true,
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot2Name)})

			By("Verifying VM's Snapshot related status")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot2Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("create a new vm snapshot 3")
			vmservice.CreateVMSnapshot(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot3Name,
				VMName:    vmName,
			})

			By("Verifying Snapshot 3's status, power should be off since memory is false")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot3Name,
				vmopv1a5.VirtualMachinePowerStateOff,
				false,
				[]vmopv1a5.VirtualMachineSnapshotReference{})

			By("Verifying Snapshot 2's status, it should have snapshot 3 as child")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot2Name,
				vmopv1a5.VirtualMachinePowerStateOff,
				false,
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot3Name)})

			By("Verifying VM's Snapshot related status")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot3Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("Revert to vm snapshot 2")
			vmservice.RevertVMSnapshot(ctx, vmSvcClusterProxy, vmSvcE2EConfig,
				vmName, vmSvcNamespace, vmSnapshotReference(vmSnapshot2Name))

			By("Verifying VM's Snapshot related status, VM should be powered off")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot2Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOff)

			By("Revert to vm snapshot 1")
			vmservice.RevertVMSnapshot(ctx,
				vmSvcClusterProxy, vmSvcE2EConfig, vmName, vmSvcNamespace,
				vmSnapshotReference(vmSnapshot1Name))

			By("Verifying VM's Snapshot related status, VM should be powered on")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot1Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("Verifying Snapshot's quota usage")
			vmoperator.VerifyVMSnapshotQuotaUsage(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, vmSvcNamespace,
				vmSvcClusterResources.StorageClassName+"-vmsnapshot-usage",
				vmSnapshot1Name, vmSnapshot2Name, vmSnapshot3Name)

			By("Revert to current snapshot: vm snapshot 1")
			vmservice.RevertVMSnapshot(ctx,
				vmSvcClusterProxy, vmSvcE2EConfig, vmName, vmSvcNamespace,
				vmSnapshotReference(vmSnapshot1Name))

			By("Verifying VM's Snapshot related status, VM should be powered on")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot1Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("Delete vm snapshot 2")
			vmoperator.EnsureVMSnapshotDeleted(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace: vmSvcNamespace,
					Name:      vmSnapshot2Name,
				})

			By("Verifying Snapshot 1's status, it should not have snapshot 2 as child")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot1Name,
				vmopv1a5.VirtualMachinePowerStateOn,
				true,
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot3Name)})

			By("Delete vm")
			vmoperator.VerifyVMDeleted(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, vmSvcNamespace, vmName)

			By("Verifying Snapshot 1 and Snapshot 3 are gone because of garbage collection")
			vmoperator.VerifyVMSnapshotDeletion(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace: vmSvcNamespace,
					Name:      vmSnapshot1Name,
				})
			vmoperator.VerifyVMSnapshotDeletion(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace: vmSvcNamespace,
					Name:      vmSnapshot3Name,
				})

			By("Verifying Snapshot's quota usage")
			vmoperator.VerifyVMSnapshotQuotaUsage(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, vmSvcNamespace, vmSvcClusterResources.StorageClassName+"-vmsnapshot-usage")
		})

		It("successfully show unmanaged vm snapshot", func() {
			By("create a snapshot in vsphere")
			vmservice.CreateSnapshotInVC(ctx, vmSvcClusterProxy, vmSnapshot1Name, vmName, vmSvcNamespace)

			By("Verifying VM's Snapshot related status, it should show the unmanaged snapshot reference")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference()},
				vmopv1a5.VirtualMachinePowerStateOn)
		})

		It("successfully revert to imported Snapshot", func() {
			By("create imported vm snapshot 1 in vsphere")
			vmservice.CreateSnapshotInVC(ctx, vmSvcClusterProxy, vmSnapshot1Name, vmName, vmSvcNamespace)

			By("Create the snapshot CR with same name as the imported snapshot, with imported snapshot annotation")
			vmservice.CreateVMSnapshot(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace:        vmSvcNamespace,
				Name:             vmSnapshot1Name,
				VMName:           vmName,
				ImportedSnapshot: true,
			})

			By("Verifying VM's Snapshot related status")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot1Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("Set restart mode to hard")
			vmservice.UpdateVMRestartMode(ctx, vmSvcClusterProxy, vmSvcE2EConfig,
				vmName, vmSvcNamespace, vmopv1a5.VirtualMachinePowerOpModeHard)

			By("Revert to imported snapshot")
			vmservice.RevertVMSnapshot(ctx, vmSvcClusterProxy, vmSvcE2EConfig,
				vmName, vmSvcNamespace, vmSnapshotReference(vmSnapshot1Name))

			By("Verifying VM's restart mode is updated")
			vmoperator.VerifyVirtualMachineRestartMode(
				ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmopv1a5.VirtualMachinePowerOpModeHard)
		})
	})

	When("VM has PVC", func() {
		BeforeEach(func() {
			vmservice.DeployVMWithCloudInit(ctx, vmSvcClusterProxy, vmSvcClusterResources, vmSvcNamespace, vmName, "", []manifestbuilders.PVC{
				{
					VolumeName:       "test-vol" + randomString,
					ClaimName:        "test-claim" + randomString,
					StorageClassName: vmSvcClusterResources.StorageClassName,
					RequestSize:      "1Gi",
					Namespace:        vmSvcNamespace,
				},
			})
			vmoperator.WaitForVirtualMachineConditionCreated(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, vmName)
			vmoperator.WaitForVirtualMachinePowerState(ctx, vmSvcE2EConfig, svClusterClient, vmSvcNamespace, vmName, "PoweredOn")
		})

		It("successfully create, and delete vm snapshots", func() {
			By("create vm snapshot 1")
			vmservice.CreateVMSnapshot(ctx, vmSvcClusterProxy, manifestbuilders.VirtualMachineSnapshotYaml{
				Namespace: vmSvcNamespace,
				Name:      vmSnapshot1Name,
				VMName:    vmName,
			})

			By("Verifying Snapshot's status")
			vmoperator.VerifyVirtualMachineSnapshotCondition(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmSnapshot1Name,
				vmopv1a5.VirtualMachinePowerStateOff,
				false,
				[]vmopv1a5.VirtualMachineSnapshotReference{})

			By("Verifying VM's Snapshot related status")
			vmoperator.VerifySnapshotStatusOnVirtualMachine(ctx, vmSvcE2EConfig,
				vmSvcClusterProxy.GetClient(),
				vmSvcNamespace,
				vmName,
				vmSnapshotReference(vmSnapshot1Name),
				[]vmopv1a5.VirtualMachineSnapshotReference{*vmSnapshotReference(vmSnapshot1Name)},
				vmopv1a5.VirtualMachinePowerStateOn)

			By("Verifying Snapshot's quota usage")
			vmoperator.VerifyVMSnapshotQuotaUsage(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, vmSvcNamespace,
				vmSvcClusterResources.StorageClassName+"-vmsnapshot-usage",
				vmSnapshot1Name)

			By("Delete vm snapshot 1")
			vmoperator.EnsureVMSnapshotDeleted(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace: vmSvcNamespace,
					Name:      vmSnapshot1Name,
				})

			By("Verifying Snapshot 1 is deleted")
			vmoperator.VerifyVMSnapshotDeletion(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, manifestbuilders.VirtualMachineSnapshotYaml{
					Namespace: vmSvcNamespace,
					Name:      vmSnapshot1Name,
				})
			vmoperator.VerifyVMSnapshotQuotaUsage(ctx, vmSvcClusterProxy.GetClient(),
				vmSvcE2EConfig, vmSvcNamespace,
				vmSvcClusterResources.StorageClassName+"-vmsnapshot-usage")
		})
	})
}
