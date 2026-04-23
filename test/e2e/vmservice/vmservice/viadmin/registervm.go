// Copyright (c) 2024-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package viadmin

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware/govmomi/alarm"
	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vslm"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/appple2e/lib"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/dcli"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	config "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const trueString = "true"

type VIAdminRegisterVMSpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *config.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
	LinuxVMName      string
}

func VIAdminRegisterVMSpec(ctx context.Context, inputGetter func() VIAdminRegisterVMSpecInput) {
	const (
		specName = "register-vm"
	)

	var (
		input                         VIAdminRegisterVMSpecInput
		wcpClient                     wcp.WorkloadManagementAPI
		config                        *config.E2EConfig
		clusterProxy                  *common.VMServiceClusterProxy
		svClusterClient               ctrlclient.Client
		svClusterClientSet            *kubernetes.Clientset
		vmServiceBackupRestoreEnabled bool
		incrementalRestoreEnabled     bool
		linuxImageDisplayName         string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig.InfraConfig can't be nil when calling %s spec", specName)
		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.SVClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)
		Expect(input.LinuxVMName).ToNot(BeEmpty(), "Invalid argument. input.LinuxVMName can't be empty when calling %s spec", specName)

		wcpClient = input.WCPClient
		config = input.Config
		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClusterClient = clusterProxy.GetClient()
		svClusterClientSet = clusterProxy.GetClientSet()

		linuxImageDisplayName = vmservice.GetDefaultImageDisplayName(config.InfraConfig.ManagementClusterConfig.Resources)

		vmServiceBackupRestoreEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSVMServiceBackupRestore"))
		incrementalRestoreEnabled = utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable("EnvFSSIncrementalRestore"))
	})

	Context("Authorization test", func() {
		var (
			vCenterHostname                    string
			authTestWCPClient                  wcp.WorkloadManagementAPI
			vimClient                          *vim25.Client
			user                               *vcenter.User
			testUserWithoutPrivilege           = "test-user-without-privilege"
			password                           = "Password!23"
			testUserWithoutPrivilegeWithDomain = "test-user-without-privilege@vsphere.local"
		)

		BeforeEach(func() {
			vCenterAdminCreds := dcli.VCenterUserCredentials{Username: testbed.AdminUsername, Password: testbed.AdminPassword}
			vCenterHostname = vcenter.GetVCPNIDFromKubeconfig(context.TODO(), clusterProxy.GetKubeconfigPath())
			Expect(vCenterHostname).NotTo(BeZero(), "Unable to determine VC PNID")

			sshCommandRunner, _, _ := testutils.GetHelpersFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			user = vcenter.NewUser(testUserWithoutPrivilege, password).WithAdminCreds(vCenterAdminCreds).WithSSHCommandRunner(sshCommandRunner)

			var err error

			err = user.Create()
			Expect(err).ToNot(HaveOccurred())

			authTestWCPClient, err = wcp.NewWCPAPIClient(vCenterHostname, testUserWithoutPrivilegeWithDomain, password, testbed.RootUsername, testbed.RootPassword)
			Expect(err).NotTo(HaveOccurred())
			vimClient, err = vcenter.NewVimClient(vCenterHostname, testbed.AdminUsername, testbed.AdminPassword)
			Expect(err).NotTo(HaveOccurred())
			err = vcenter.AddToGroup(ctx, vimClient, testUserWithoutPrivilege, "ReadOnlyUsers")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Delete the SSO user.
			vcenter.DeleteUserOrFail(user)
		})

		It("A user without namespaces.Configure privilege should not be able to invoke RegisterVM API", Label("smoke"), func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			By("Invoke the RegisterVM API")

			taskID, err := authTestWCPClient.RegisterVM(input.WCPNamespaceName, "fake-vm-moid")
			Expect(taskID).To(BeEmpty())
			Expect(err).To(HaveOccurred())

			var dcliErr wcp.DcliError
			Expect(errors.As(err, &dcliErr)).Should(BeTrue())
			Expect(dcliErr.Response()).Should(ContainSubstring(lib.VapiUnauthorizedErrMsg))
		})
	})

	Context("RegisterVM with invalid params", func() {
		It("If the VM does not exist, returns not found error", func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			By("Invoke the RegisterVM API")

			taskID, err := wcpClient.RegisterVM(input.WCPNamespaceName, "non-exist-vm-moid")
			Expect(err).To(HaveOccurred())

			var dcliErr wcp.DcliError
			Expect(errors.As(err, &dcliErr)).Should(BeTrue())
			Expect(dcliErr.Response()).Should(ContainSubstring(lib.VapiNotFoundErrMsg))
			Expect(taskID).To(BeEmpty())
		})

		It("If the namespace does not exist, returns not found error", func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			By("Invoke the RegisterVM API")

			taskID, err := wcpClient.RegisterVM("non-existent-namespace", "vm-moid")
			Expect(err).To(HaveOccurred())

			var dcliErr wcp.DcliError
			Expect(errors.As(err, &dcliErr)).Should(BeTrue())
			Expect(dcliErr.Response()).Should(ContainSubstring(lib.VapiNotFoundErrMsg))
			Expect(taskID).To(BeEmpty())
		})

		It("If the VM is already registered, returns already in desired state error", func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			if incrementalRestoreEnabled {
				Skip("WCP_VMService_Incremental_Restore FSS is enabled")
			}

			By("Get an existing VM Service VM MoID in Supervisor")
			vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, input.LinuxVMName)
			existingVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, input.LinuxVMName)
			Expect(err).ToNot(HaveOccurred())
			Expect(existingVM.Status.UniqueID).ToNot(BeEmpty())

			By("Invoke the RegisterVM API")

			taskID, err := wcpClient.RegisterVM(input.WCPNamespaceName, existingVM.Status.UniqueID)
			Expect(err).To(HaveOccurred())

			var dcliErr wcp.DcliError
			Expect(errors.As(err, &dcliErr)).Should(BeTrue())
			Expect(dcliErr.Response()).Should(ContainSubstring(lib.VapiAlreadyInDesiredStateErrMsg))
			Expect(taskID).To(BeEmpty())
		})
	})

	Context("Incremental Restore - Register VM with pre-existing VM CR", func() {
		It("Should register VM successfully", func() {
			if !incrementalRestoreEnabled {
				Skip("WCP_VMService_Incremental_Restore FSS is not enabled")
			}

			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vCenterClient)

			vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			secretName := vmName + "-cloud-config-data"
			secret := manifestbuilders.Secret{
				Namespace: input.WCPNamespaceName,
				Name:      secretName,
			}
			secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
			Expect(clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

			resources := config.InfraConfig.ManagementClusterConfig.Resources

			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vmName,
				VMClassName:      resources.VMClassName,
				StorageClassName: resources.StorageClassName,
				ResourcePolicy:   resources.VMResourcePolicyName,
				ImageName:        linuxImageDisplayName,
				Bootstrap: manifestbuilders.Bootstrap{
					CloudInit: &manifestbuilders.CloudInit{
						RawCloudConfig: &manifestbuilders.KeySelector{
							Key:  "user-data",
							Name: secretName,
						},
					},
				},
				PowerState: "PoweredOn",
			}
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create Linux VM:\n%s", string(vmYaml))
			// End create new VM

			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)

			existingVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			// Wait for backup to complete before reading the backup data
			vmservice.WaitForBackupToComplete(ctx, existingVM, clusterProxy, config)

			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: existingVM.Status.UniqueID}
			vmObj := object.NewVirtualMachine(vCenterClient, vmMoRef)

			var vmMO mo.VirtualMachine

			var (
				backupVersion string
				resourceYAML  string
			)
			// VM Operator starts recording backup when disk promotion and volume registration has happened.
			propCollector := property.DefaultCollector(vCenterClient)
			Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
			Expect(vmMO.Config).ToNot(BeNil(), "VM Config should not be nil")

			ecList := object.OptionValueList(vmMO.Config.ExtraConfig)
			resourceYAML, _ = ecList.GetString(backupapi.VMResourceYAMLExtraConfigKey)
			backupVersion, _ = ecList.GetString(backupapi.BackupVersionExtraConfigKey)

			Expect(resourceYAML).ToNot(BeEmpty())
			Expect(backupVersion).ToNot(BeEmpty())

			By("Power off the VM")
			vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Add the pause annotation to VM")

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}

			vm.Annotations[vmopv1a3.PauseAnnotation] = trueString
			Expect(svClusterClient.Update(ctx, vm)).To(Succeed())

			// Collect all PVC names from the VM spec
			var pvcNames []string

			for _, volume := range vm.Spec.Volumes {
				if volume.PersistentVolumeClaim != nil {
					pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
				}
			}

			// Unregister all PVCs using the helper function
			vmservice.UnregisterPVCVolumes(ctx, svClusterClient, input.WCPNamespaceName, vmName, pvcNames, config)

			// reconfigBeforeRegister changes the VM's resource.yaml, backupVersion to the given value.
			reconfigBeforeRegister := func(value []string) {
				vmSpec := types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{Key: backupapi.VMResourceYAMLExtraConfigKey, Value: value[0]},
						&types.OptionValue{Key: backupapi.BackupVersionExtraConfigKey, Value: value[1]},
					},
				}

				task, err := vmObj.Reconfigure(ctx, vmSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())
			}

			By(fmt.Sprintf("Reconfigure VM with saved vm yaml %v: %s\n, backupVersion %v: %s\n",
				backupapi.VMResourceYAMLExtraConfigKey, resourceYAML,
				backupapi.BackupVersionExtraConfigKey, backupVersion))
			reconfigBeforeRegister([]string{resourceYAML, backupVersion})

			taskInfo, err := vmservice.InvokeRegisterVM(ctx, existingVM.Status.UniqueID, existingVM.Namespace, clusterProxy, wcpClient)

			By("Verify task state is success")
			Expect(err).ToNot(HaveOccurred())
			Expect(taskInfo).ToNot(BeNil())
			Expect(taskInfo.Error).To(BeNil())
			Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))

			vmservice.VerifyPostRegisterVM(ctx, existingVM.Name, existingVM.Namespace, nil, len(existingVM.Spec.Volumes), clusterProxy, config, svClusterClient, wcpClient)
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
		})
	})

	Context("Incremental Restore - Register VM with pre-existing VM CR and PVCs", func() {
		It("Should register VM successfully", func() {
			if !incrementalRestoreEnabled {
				Skip("WCP_VMService_Incremental_Restore FSS is not enabled")
			}

			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vCenterClient)

			vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			secretName := vmName + "-cloud-config-data"
			secret := manifestbuilders.Secret{
				Namespace: input.WCPNamespaceName,
				Name:      secretName,
			}
			secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
			Expect(clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

			resources := config.InfraConfig.ManagementClusterConfig.Resources
			pvcNameA := vmName + "-pvc-a"
			testutils.AssertCreatePVC(svClusterClientSet, pvcNameA, input.WCPNamespaceName, resources.StorageClassName)

			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vmName,
				VMClassName:      resources.VMClassName,
				StorageClassName: resources.StorageClassName,
				ResourcePolicy:   resources.VMResourcePolicyName,
				ImageName:        linuxImageDisplayName,
				Bootstrap: manifestbuilders.Bootstrap{
					CloudInit: &manifestbuilders.CloudInit{
						RawCloudConfig: &manifestbuilders.KeySelector{
							Key:  "user-data",
							Name: secretName,
						},
					},
				},
				PowerState: "PoweredOn",
				PVCNames:   []string{pvcNameA},
			}
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create Linux VM:\n%s", string(vmYaml))
			// End create new VM

			// Wait for IP, a valid moID and the PVC attachment.
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForPVCAttachment(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, pvcNameA)

			existingVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			// Wait for backup to complete before reading the backup data
			vmservice.WaitForBackupToComplete(ctx, existingVM, clusterProxy, config)

			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: existingVM.Status.UniqueID}
			vmObj := object.NewVirtualMachine(vCenterClient, vmMoRef)

			var vmMO mo.VirtualMachine

			// take a copy of the backed up vm resource and pvc backup data.
			By("Save original VM resource, backup version and PVC backup from ExtraConfig")

			propCollector := property.DefaultCollector(vCenterClient)
			Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
			Expect(vmMO.Config).ToNot(BeNil(), "VM Config should not be nil")

			var (
				backupVersion string
				resourceYAML  string
			)

			ecList := object.OptionValueList(vmMO.Config.ExtraConfig)
			resourceYAML, _ = ecList.GetString(backupapi.VMResourceYAMLExtraConfigKey)
			pvcBackup, _ := ecList.GetString(backupapi.PVCDiskDataExtraConfigKey)
			backupVersion, _ = ecList.GetString(backupapi.BackupVersionExtraConfigKey)

			Expect(resourceYAML).ToNot(BeEmpty())
			Expect(pvcBackup).ToNot(BeEmpty())
			Expect(backupVersion).ToNot(BeEmpty())

			// Create and attach another pvc to the VM.
			pvcNameB := vmName + "-pvc-b"
			testutils.AssertCreatePVC(svClusterClientSet, pvcNameB, input.WCPNamespaceName, resources.StorageClassName)

			// Use v1alpha3 here to make sure this doesn't blow up in product branches older than v1a5.
			By(fmt.Sprintf("Updating the VM with two PVCs: '%v'", vmParameters.PVCNames))

			vm, err := utils.GetVirtualMachineA3(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1a3.VirtualMachineVolume{
				Name: pvcNameB,
				VirtualMachineVolumeSource: vmopv1a3.VirtualMachineVolumeSource{
					PersistentVolumeClaim: &vmopv1a3.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcNameB,
						},
					},
				},
			})
			Expect(svClusterClient.Update(ctx, vm)).To(Succeed())

			vmoperator.WaitForPVCAttachment(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, pvcNameB)
			// Both PVC A and B are now attached to VM.

			By("Power off the VM")
			vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, "PoweredOff")

			By("Add the pause annotation to VM")

			vm, err = utils.GetVirtualMachineA3(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}

			vm.Annotations[vmopv1a3.PauseAnnotation] = trueString
			Expect(svClusterClient.Update(ctx, vm)).To(Succeed())

			// Collect all PVC names from the VM spec
			var pvcNames []string

			for _, volume := range vm.Spec.Volumes {
				if volume.PersistentVolumeClaim != nil {
					pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
				}
			}

			// Unregister all PVCs using the helper function
			vmservice.UnregisterPVCVolumes(ctx, svClusterClient, input.WCPNamespaceName, vmName, pvcNames, config)

			// reconfigBeforeRegister changes the VM's resource.yaml, backupVersion and PVC properties to the given value.
			reconfigBeforeRegister := func(value []string) {
				vmSpec := types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{Key: backupapi.VMResourceYAMLExtraConfigKey, Value: value[0]},
						&types.OptionValue{Key: backupapi.PVCDiskDataExtraConfigKey, Value: value[1]},
						&types.OptionValue{Key: backupapi.BackupVersionExtraConfigKey, Value: value[2]},
					},
				}

				task, err := vmObj.Reconfigure(ctx, vmSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())
			}

			By(fmt.Sprintf("Reconfigure VM with saved vm yaml %v: %s\n, backupVersion %v: %s\n, and PVC backup with one PVC (pvc-a) %v: %s\n",
				backupapi.VMResourceYAMLExtraConfigKey, resourceYAML,
				backupapi.BackupVersionExtraConfigKey, backupVersion,
				backupapi.PVCDiskDataExtraConfigKey, pvcBackup))
			reconfigBeforeRegister([]string{resourceYAML, pvcBackup, backupVersion})

			// Call registerVM on existing VM CR currently having two PVCs (a and b) with backup VM yaml having one PVC (a)
			taskInfo, err := vmservice.InvokeRegisterVM(ctx, existingVM.Status.UniqueID, existingVM.Namespace, clusterProxy, wcpClient)

			By("Verify task state is success")
			Expect(err).ToNot(HaveOccurred())
			Expect(taskInfo).ToNot(BeNil())
			Expect(taskInfo.Error).To(BeNil())
			Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))

			// Expected registered VM should have pvc-a-restored in vm.spec.volumes
			// There should be two restored volumes: one from classic disk, and one for pvc-a since pvc-b was added after backup.
			expectedRestoredPVCCount := 2
			vmservice.VerifyPostRegisterVM(ctx, existingVM.Name, existingVM.Namespace, nil, expectedRestoredPVCCount, clusterProxy, config, svClusterClient, wcpClient)
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
		})
	})

	Context("RegisterVM Alarm", func() {
		// Predefined Alarm definition added in main/9.0 (CLN 13918662)
		// If using a VC without the predefined alarm, create with:
		//  govc alarm.create -n WCPRegisterVMFailedAlarm \
		//   -d "registervm failed (for gce2e)" \
		//   -green com.vmware.wcp.RegisterVM.success \
		//   -yellow com.vmware.wcp.RegisterVM.failure
		// Note: "alarm." prefix can only be used in predefined SystemName
		const (
			alarmName    = "WCPRegisterVMFailedAlarm"
			eventPrefix  = "com.vmware.wcp.RegisterVM."
			eventSuccess = eventPrefix + "success"
			eventFailure = eventPrefix + "failure"
		)

		alarmMatches := func(info types.AlarmInfo) bool {
			return info.SystemName == "alarm."+alarmName || info.Name == alarmName
		}

		// Test summary:
		// - DeleteVMResource, removing the K8s CR
		// - Reconfig VM's resource.yaml to invalid
		// - InvokeRegisterVM, expecting to fail and trigger alarm
		// - Reconfig VM's resource.yaml to valid
		// - InvokeRegisterVM, expecting to succeed and clear triggered alarm
		// - VerifyPostRegisterVM, expecting VM is powered on, has IP, etc
		It("Should trigger on failure", func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vCenterClient)

			alarmManager := alarm.NewManager(vCenterClient)
			alarms, err := alarmManager.GetAlarm(ctx, vCenterClient.ServiceContent.RootFolder)
			Expect(err).NotTo(HaveOccurred())

			var wcpAlarm *mo.Alarm

			for _, alarm := range alarms {
				if alarmMatches(alarm.Info) {
					wcpAlarm = &alarm
					break
				}
			}

			if wcpAlarm == nil {
				Skip(alarmName + " not defined in this vCenter")
			}

			// Create a new VM (copy-n-paste of vmservicee2e.deployVMWithCloudInit)
			vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			vmsvcClusterProxy := input.ClusterProxy.(*common.VMServiceClusterProxy)
			secretName := vmName + "-cloud-config-data"
			secret := manifestbuilders.Secret{
				Namespace: input.WCPNamespaceName,
				Name:      secretName,
			}
			secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
			Expect(vmsvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

			resources := config.InfraConfig.ManagementClusterConfig.Resources
			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        input.WCPNamespaceName,
				Name:             vmName,
				VMClassName:      resources.VMClassName,
				StorageClassName: resources.StorageClassName,
				ResourcePolicy:   resources.VMResourcePolicyName,
				ImageName:        linuxImageDisplayName,
				Bootstrap: manifestbuilders.Bootstrap{
					CloudInit: &manifestbuilders.CloudInit{
						RawCloudConfig: &manifestbuilders.KeySelector{
							Key:  "user-data",
							Name: secretName,
						},
					},
				},
				PowerState: "PoweredOn",
			}
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(vmsvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create Linux VM:\n%s", string(vmYaml))

			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			existingVM, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			// Wait for backup to complete before reading the backup data
			vmservice.WaitForBackupToComplete(ctx, existingVM, clusterProxy, config)

			// Delete the VM Service VM CR, keeping the vCenter VM in inventory.
			vmMoID := vmservice.DeleteVMResource(ctx, existingVM.Name, existingVM.Namespace, nil, clusterProxy, config, svClusterClient)
			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}
			vmObj := object.NewVirtualMachine(vCenterClient, vmMoRef)

			var vmMO mo.VirtualMachine

			var resourceYAML string

			By("Save original VM ExtraConfig")

			Eventually(func(g Gomega) {
				propCollector := property.DefaultCollector(vCenterClient)
				g.Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
				g.Expect(vmMO.Config).ToNot(BeNil(), "VM Config should not be nil")
				ecList := object.OptionValueList(vmMO.Config.ExtraConfig)
				resourceYAML, _ = ecList.GetString(backupapi.VMResourceYAMLExtraConfigKey)
				g.Expect(resourceYAML).ToNot(BeEmpty())
			}, config.GetIntervals("default", "wait-backup-to-complete")...).
				Should(Succeed(), "Waiting for VM resource to be saved in ExtraConfig")

			// Create EventHistoryCollector for verifying events
			eventSpec := types.EventFilterSpec{
				EventTypeId: []string{eventSuccess, eventFailure},
				Entity: &types.EventFilterSpecByEntity{
					Entity:    vmMoRef,
					Recursion: types.EventFilterSpecRecursionOptionSelf,
				},
			}

			eventCollector, err := event.NewManager(vCenterClient).CreateCollectorForEvents(ctx, eventSpec)
			Expect(err).NotTo(HaveOccurred())

			defer func() { _ = eventCollector.Destroy(ctx) }()

			// latestEvents returns any new events of type spec.EventTypeId
			latestEvents := func() (map[string][]types.EventEx, error) {
				alarmEvents := make(map[string][]types.EventEx)

				for {
					events, err := eventCollector.ReadNextEvents(ctx, 10)
					if err != nil {
						return nil, err
					}

					if len(events) == 0 { // no more new events
						break
					}

					for i := range events {
						// spec.EventTypeId filters out other types
						event := events[i].(*types.EventEx)
						alarmEvents[event.EventTypeId] = append(alarmEvents[event.EventTypeId], *event)

						// Fields below set by the client PostEvent
						// calls in vapi/impl/wcp/registervm.go
						Expect(event.Message).ToNot(BeEmpty())
						Expect(event.EventTypeId).To(HavePrefix(eventPrefix))
						// This message set by VC for predefined alarms only, see:
						//  vpx/vpxd/extensions/VirtualCenter/locale/en/event.vmsg
						if wcpAlarm.Info.SystemName != "" {
							Expect(event.FullFormattedMessage).ToNot(BeEmpty())
						}
					}
				}

				return alarmEvents, nil
			}

			// triggeredAlarm gets the current triggeredAlarmState property and related info
			triggeredAlarm := func() *alarm.StateInfo {
				options := alarm.StateInfoOptions{Event: true}
				alarmStates, err := alarmManager.GetStateInfo(ctx, vmMoRef, options)
				Expect(err).NotTo(HaveOccurred())

				for _, state := range alarmStates {
					if alarmMatches(*state.Info) {
						return &state
					}
				}

				return nil
			}

			// reconfigResourceYAML changes the VM's resource.yaml property to the given value
			reconfigResourceYAML := func(value any) {
				vmSpec := types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{Key: backupapi.VMResourceYAMLExtraConfigKey, Value: value},
					},
				}

				task, err := vmObj.Reconfigure(ctx, vmSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())
			}

			By("Checking events before registervm")

			alarmEvents, err := latestEvents()
			Expect(err).NotTo(HaveOccurred())
			Expect(alarmEvents).To(HaveLen(0))
			By("Checking triggered alarms before registervm")
			Expect(triggeredAlarm()).To(BeNil())

			By("Reconfigure VM with invalid " + backupapi.VMResourceYAMLExtraConfigKey)
			reconfigResourceYAML("invalid-yaml")

			taskInfo, err := vmservice.InvokeRegisterVM(ctx, vmMoID, existingVM.Namespace, clusterProxy, wcpClient)

			By("Verify task state is error")
			Expect(err).NotTo(BeNil())
			Expect(taskInfo.Error).NotTo(BeNil())
			Expect(taskInfo.State).To(Equal(types.TaskInfoStateError))

			By("Verify failure event was emitted after registervm failure")
			Eventually(func(g Gomega) {
				alarmEvents, err = latestEvents()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(alarmEvents).To(HaveLen(1))
				g.Expect(alarmEvents[eventFailure]).To(HaveLen(1))
			}, config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "Timed out waiting for failure event")

			By("Verify alarm was triggered by failure event")

			warningAlarm := triggeredAlarm()
			Expect(warningAlarm).ToNot(BeNil())
			Expect(warningAlarm.OverallStatus).To(Equal(types.ManagedEntityStatusYellow))
			Expect(warningAlarm.Event).ToNot(BeNil())
			event := warningAlarm.Event.(*types.EventEx)
			Expect(event).ToNot(BeNil())
			Expect(event.EventTypeId).To(Equal(eventFailure))
			Expect(warningAlarm.EventKey).To(Equal(alarmEvents[eventFailure][0].Key))

			By("Reconfigure VM with original ExtraConfig")
			reconfigResourceYAML(resourceYAML)

			taskInfo, err = vmservice.InvokeRegisterVM(ctx, vmMoID, existingVM.Namespace, clusterProxy, wcpClient)

			By("Verify task state is success")
			Expect(err).ToNot(HaveOccurred())
			Expect(taskInfo).ToNot(BeNil())
			Expect(taskInfo.Error).To(BeNil())
			Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))

			vmservice.VerifyPostRegisterVM(ctx, existingVM.Name, existingVM.Namespace, nil, len(existingVM.Spec.Volumes), clusterProxy, config, svClusterClient, wcpClient)

			By("Verify success event was emitted after successful registervm")
			Eventually(func(g Gomega) {
				alarmEvents, err = latestEvents()
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(alarmEvents).To(HaveLen(1))
				g.Expect(alarmEvents[eventSuccess]).To(HaveLen(1))
			}, config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "Timed out waiting for success event")
			By("Verify triggered alarm was cleared by success event")
			Expect(triggeredAlarm()).To(BeNil())

			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
		})
	})

	Context("Restore disk only", func() {
		It("Should register restored disk", func() {
			if !vmServiceBackupRestoreEnabled {
				Skip("WCP_VMService_BackupRestore FSS is not enabled")
			}

			if !incrementalRestoreEnabled {
				Skip("WCP_VMService_Incremental_Restore FSS is not enabled")
			}

			adminClusterProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
			Expect(err).ToNot(HaveOccurred())

			defer adminClusterProxy.Dispose(ctx)

			vCenterHostname := vcenter.GetVCPNIDFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			adminClient, err := adminClusterProxy.GetAdminClient()
			Expect(err).ToNot(HaveOccurred())
			vmopSecret, err := utils.GetSecret(ctx, adminClient, "vmware-system-vmop", "wcp-vmop-sa-vc-auth")
			Expect(err).ToNot(HaveOccurred())
			vmopvCenterClient, err := vcenter.NewVimClient(vCenterHostname, string(vmopSecret.Data["username"]), string(vmopSecret.Data["password"]))
			Expect(err).ToNot(HaveOccurred())

			defer vcenter.LogoutVimClient(vmopvCenterClient)

			vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vCenterClient)

			// Create a new VM
			vmNamespace := input.WCPNamespaceName
			vmName := fmt.Sprintf("%s-%s", specName, capiutil.RandomString(4))
			vmsvcClusterProxy := input.ClusterProxy.(*common.VMServiceClusterProxy)
			secretName := vmName + "-cloud-config-data"
			secret := manifestbuilders.Secret{
				Namespace: vmNamespace,
				Name:      secretName,
			}
			secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
			Expect(vmsvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

			resources := config.InfraConfig.ManagementClusterConfig.Resources
			pvcNameA := vmName + "-pvc-a"
			testutils.AssertCreatePVC(svClusterClientSet, pvcNameA, input.WCPNamespaceName, resources.StorageClassName)

			vmParameters := manifestbuilders.VirtualMachineYaml{
				Namespace:        vmNamespace,
				Name:             vmName,
				VMClassName:      resources.VMClassName,
				StorageClassName: resources.StorageClassName,
				ResourcePolicy:   resources.VMResourcePolicyName,
				ImageName:        linuxImageDisplayName,
				Bootstrap: manifestbuilders.Bootstrap{
					CloudInit: &manifestbuilders.CloudInit{
						RawCloudConfig: &manifestbuilders.KeySelector{
							Key:  "user-data",
							Name: secretName,
						},
					},
				},
				PowerState: "PoweredOn",
				PVCNames:   []string{pvcNameA},
			}
			vmYaml := manifestbuilders.GetVirtualMachineYamlA2(vmParameters)
			Expect(vmsvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create Linux VM:\n%s", string(vmYaml))
			// End create new VM

			// Wait for IP, a valid moID and the PVC attachment.
			vmoperator.WaitForVirtualMachineCreation(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForVirtualMachineMOID(ctx, config, svClusterClient, input.WCPNamespaceName, vmName)
			vmoperator.WaitForPVCAttachment(ctx, config, svClusterClient, input.WCPNamespaceName, vmName, pvcNameA)

			existingVM, err := utils.GetVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)
			Expect(err).ToNot(HaveOccurred())

			// Wait for backup to complete before powering off the VM
			vmservice.WaitForBackupToComplete(ctx, existingVM, clusterProxy, config)

			vmMoID := existingVM.Status.UniqueID

			vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}
			vmObj := object.NewVirtualMachine(vmopvCenterClient, vmMoRef)

			By("Power off the VM")
			vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, "PoweredOff")
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, "PoweredOff")

			By("Add the pause annotation to VM")

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, input.WCPNamespaceName, vmName)
			Expect(err).ToNot(HaveOccurred())

			if vm.Annotations == nil {
				vm.Annotations = make(map[string]string)
			}

			vm.Annotations[vmopv1a3.PauseAnnotation] = trueString
			Expect(svClusterClient.Update(ctx, vm)).To(Succeed())

			var vmMO mo.VirtualMachine

			propCollector := property.DefaultCollector(vCenterClient)
			Expect(propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.files"}, &vmMO)).To(Succeed())

			// getVolumeHandle fetches the PVC, gets its PV, and returns the VolumeHandle
			getVolumeHandle := func(g Gomega, pvcName, namespace string) string {
				// Get the PVC
				pvc := &corev1.PersistentVolumeClaim{}
				pvcKey := ctrlclient.ObjectKey{
					Namespace: namespace,
					Name:      pvcName,
				}
				err := svClusterClient.Get(ctx, pvcKey, pvc)
				g.Expect(err).ToNot(HaveOccurred(), "Failed to get PVC %s in namespace %s", pvcName, namespace)
				g.Expect(pvc.Spec.VolumeName).ToNot(BeEmpty(), "PVC %s does not have a bound volume", pvcName)

				// Get the PV
				pv := &corev1.PersistentVolume{}
				pvKey := ctrlclient.ObjectKey{
					Name: pvc.Spec.VolumeName,
				}
				err = svClusterClient.Get(ctx, pvKey, pv)
				g.Expect(err).ToNot(HaveOccurred(), "Failed to get PV %s", pvc.Spec.VolumeName)

				// Get the VolumeHandle from the CSI PersistentVolumeSource
				g.Expect(pv.Spec.CSI).ToNot(BeNil(), "PV %s does not have a CSI source", pv.Name)
				g.Expect(pv.Spec.CSI.VolumeHandle).ToNot(BeEmpty(), "PV %s does not have a VolumeHandle", pv.Name)

				return pv.Spec.CSI.VolumeHandle
			}

			var (
				datastorePath, vmPath object.DatastorePath
				disk                  *types.VirtualDisk
				backing               *types.VirtualDiskFlatVer2BackingInfo
			)

			findDisk := func(g Gomega, pvcName string, shouldExist bool) {
				volumeHandle := getVolumeHandle(g, pvcName, vmNamespace)

				deviceList, err := vmObj.Device(ctx)
				g.Expect(err).ToNot(HaveOccurred())

				found := false

				for _, device := range deviceList.SelectByType((*types.VirtualDisk)(nil)) {
					// Find the disk that matches the VolumeHandle from the PVC/PV
					if vDiskID := device.(*types.VirtualDisk).VDiskId; vDiskID != nil {
						if vDiskID.Id == volumeHandle {
							disk = device.(*types.VirtualDisk)
							backing = disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)
							found = datastorePath.FromString(backing.FileName)

							break
						}
					}
				}

				g.Expect(found).To(Equal(shouldExist))
			}

			vmPath.FromString(vmMO.Config.Files.VmPathName)
			vmHome := path.Dir(vmPath.Path)

			findDisk(Default, pvcNameA, true)

			dir := path.Dir(datastorePath.Path)
			Expect(dir).ToNot(Equal(vmHome)) // "fcd" (or vsan object id) initially

			cnsClient, err := cns.NewClient(ctx, vCenterClient)
			Expect(err).ToNot(HaveOccurred())

			queryVolume := func() string {
				filter := cnstypes.CnsQueryFilter{
					VolumeIds: []cnstypes.CnsVolumeId{cnstypes.CnsVolumeId(*disk.VDiskId)},
				}
				res, err := cnsClient.QueryVolume(ctx, &filter)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Volumes).To(HaveLen(1))

				return res.Volumes[0].StoragePolicyId
			}
			storageProfileID := queryVolume()

			ds := object.NewDatastore(vCenterClient, *backing.Datastore)

			fcdManager := vslm.NewObjectManager(vCenterClient)
			// Validate FCD backing
			_, err = fcdManager.Retrieve(ctx, ds, disk.VDiskId.Id)
			Expect(err).ToNot(HaveOccurred())

			Expect(ds.FindInventoryPath(ctx)).To(Succeed())

			dc, err := find.NewFinder(vCenterClient).Datacenter(ctx, ds.DatacenterPath)
			Expect(err).ToNot(HaveOccurred())

			fileManager := ds.NewFileManager(dc, false)

			// "Delete" existing disk
			Expect(vmObj.RemoveDevice(ctx, true, disk)).To(Succeed())
			findDisk(Default, pvcNameA, false)

			// Create "new" disk
			dst := path.Join(vmHome, path.Base(datastorePath.Path))
			Expect(fileManager.Copy(ctx, datastorePath.Path, dst)).To(Succeed())
			Expect(fileManager.Delete(ctx, datastorePath.Path)).To(Succeed())

			// Expect to fail w/ orphaned FCD
			_, err = fcdManager.Retrieve(ctx, ds, disk.VDiskId.Id)
			Expect(err).To(HaveOccurred())
			Expect(fault.Is(err, &types.NotFound{})).To(BeTrue())

			// The Volume still exists
			queryVolume()

			// Attach new disk with storage profile (required for cns)
			datastorePath.Path = dst
			backing.FileName = datastorePath.String()

			profile := []types.BaseVirtualMachineProfileSpec{
				&types.VirtualMachineDefinedProfileSpec{
					ProfileId: storageProfileID,
				},
			}

			// Attach existing vmdk, rather than create a new backing
			disk.CapacityInKB = 0
			disk.CapacityInBytes = 0
			Expect(vmObj.AddDeviceWithProfile(ctx, profile, disk)).To(Succeed())

			// Since we deleted (moved) the disk backing, this causes the FCD and CNS Volume objects to be
			// removed on the vSphere side.. emulating what Veeam's restore flow does.
			task, err := fcdManager.ReconcileDatastoreInventory(ctx, ds.Reference())
			Expect(err).ToNot(HaveOccurred())
			err = task.Wait(ctx)
			Expect(err).ToNot(HaveOccurred())

			taskInfo, err := vmservice.InvokeRegisterVM(ctx, vmMoID, existingVM.Namespace, clusterProxy, wcpClient)

			By("Verify task state is success")
			Expect(err).ToNot(HaveOccurred())
			Expect(taskInfo).ToNot(BeNil())
			Expect(taskInfo.Error).To(BeNil())
			Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))

			e2eframework.Logf("VM has been restored: %v", vm)

			// Fetch the restored VM and verify that it has the expected number of volumes.
			restoredVM, err := utils.GetVirtualMachineA3(ctx, svClusterClient, existingVM.Namespace, existingVM.Name)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(restoredVM.Spec.Volumes)).To(Equal(2)) // one base disk and one PVC

			var restoredVol *vmopv1a3.VirtualMachineVolume

			for _, vol := range restoredVM.Spec.Volumes {
				// The volume with restored- prefix is the one that was restored.
				if vol.PersistentVolumeClaim != nil && strings.HasPrefix(vol.PersistentVolumeClaim.ClaimName, "restored-") {
					restoredVol = &vol
					break
				}
			}

			Expect(restoredVol).ToNot(BeNil())

			findDisk(Default, restoredVol.PersistentVolumeClaim.ClaimName, true)

			dir = path.Dir(datastorePath.Path)
			Expect(dir).To(Equal(vmHome))

			// Validate FCD backing is restored
			_, err = fcdManager.Retrieve(ctx, ds, disk.VDiskId.Id)
			Expect(err).ToNot(HaveOccurred())

			// Validate new Volume is created
			queryVolume()

			// We can't use len(existingVM.Spec.Volumes) here because we are only restoring one disk.
			vmservice.VerifyPostRegisterVM(ctx, existingVM.Name, existingVM.Namespace, nil, 1, clusterProxy, config, svClusterClient, wcpClient)
			Expect(clusterProxy.DeleteWithArgs(ctx, vmYaml)).To(Succeed(), "failed to delete virtualmachine")
		})
	})
}
