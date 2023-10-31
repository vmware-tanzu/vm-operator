// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func backupTests() {
	const (
		// These are the default values of the vcsim VM that are used to
		// construct the expected backup data in the following tests.
		vcSimVMPath       = "DC0_C0_RP0_VM0"
		vcSimDiskUUID     = "be8d2471-f32e-5c7e-a89b-22cb8e533890"
		vcSimDiskFileName = "[LocalDS_0] DC0_C0_RP0_VM0/disk1.vmdk"
	)

	var (
		ctx   *builder.TestContextForVCSim
		vcVM  *object.VirtualMachine
		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, vcSimVMPath)
		Expect(err).NotTo(HaveOccurred())

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      &vmopv1.VirtualMachine{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("VM Kube data", func() {
		BeforeEach(func() {
			vmCtx.VM = builder.DummyVirtualMachine()
		})

		When("VM kube data exists in ExtraConfig but is not up-to-date", func() {

			BeforeEach(func() {
				oldVM := vmCtx.VM.DeepCopy()
				oldVM.ObjectMeta.Generation = 1
				oldVMYaml, err := yaml.Marshal(oldVM)
				Expect(err).NotTo(HaveOccurred())
				backupVMYamlEncoded, err := util.EncodeGzipBase64(string(oldVMYaml))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   constants.BackupVMKubeDataExtraConfigKey,
							Value: backupVMYamlEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup VM kube data YAML with the latest spec", func() {
				vmCtx.VM.ObjectMeta.Generation = 2
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())

				vmCopy := vmCtx.VM.DeepCopy()
				vmCopy.Status = vmopv1.VirtualMachineStatus{}
				vmCopyYaml, err := yaml.Marshal(vmCopy)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMKubeDataExtraConfigKey, string(vmCopyYaml))
			})
		})

		When("VM kube data exists in ExtraConfig and is up-to-date", func() {
			var (
				kubeDataBackup = ""
			)

			BeforeEach(func() {
				vmYaml, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				kubeDataBackup = string(vmYaml)
				encodedKubeDataBackup, err := util.EncodeGzipBase64(kubeDataBackup)
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   constants.BackupVMKubeDataExtraConfigKey,
							Value: encodedKubeDataBackup,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should skip backing up VM kube data", func() {
				// Update the VM to verify its kube data is not backed up in ExtraConfig.
				vmCtx.VM.Labels = map[string]string{"foo": "bar"}
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMKubeDataExtraConfigKey, kubeDataBackup)
			})
		})
	})

	Context("VM bootstrap data", func() {

		It("Should back up bootstrap data as JSON in ExtraConfig", func() {
			bootstrapDataRaw := map[string]string{"foo": "bar"}
			backupVMCtx := context.BackupVirtualMachineContext{
				VMCtx:         vmCtx,
				VcVM:          vcVM,
				BootstrapData: bootstrapDataRaw,
				DiskUUIDToPVC: nil,
			}
			Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())

			bootstrapDataJSON, err := json.Marshal(bootstrapDataRaw)
			Expect(err).NotTo(HaveOccurred())
			verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMBootstrapDataExtraConfigKey, string(bootstrapDataJSON))
		})
	})

	Context("VM Disk data", func() {

		When("VM has no disks that are attached from PVCs", func() {

			It("Should skip backing up VM disk data", func() {
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMDiskDataExtraConfigKey, "")
			})
		})

		When("VM has disks that are attached from PVCs", func() {

			It("Should backup VM disk data as JSON in ExtraConfig", func() {
				dummyPVC := builder.DummyPersistentVolumeClaim()
				diskUUIDToPVC := map[string]corev1.PersistentVolumeClaim{
					vcSimDiskUUID: *dummyPVC,
				}

				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: diskUUIDToPVC,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())

				diskData := []virtualmachine.VMDiskData{
					{
						FileName:    vcSimDiskFileName,
						PVCName:     dummyPVC.Name,
						AccessModes: dummyPVC.Spec.AccessModes,
					},
				}
				diskDataJSON, err := json.Marshal(diskData)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMDiskDataExtraConfigKey, string(diskDataJSON))
			})
		})
	})

	Context("VM cloud-init instance ID data", func() {

		BeforeEach(func() {
			vmCtx.VM = builder.DummyVirtualMachine()
		})

		When("VM cloud-init instance ID already exists in ExtraConfig", func() {

			BeforeEach(func() {
				_, err := vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   constants.BackupVMCloudInitInstanceIDExtraConfigKey,
							Value: "ec-instance-id",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				vmCtx.VM.Annotations = map[string]string{
					vmopv1.InstanceIDAnnotation: "other-instance-id",
				}
			})

			It("Should not change the cloud-init instance ID in VM's ExtraConfig", func() {
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMCloudInitInstanceIDExtraConfigKey, "ec-instance-id")
			})
		})

		When("VM cloud-init instance ID does not exist in ExtraConfig and is set in annotations", func() {

			BeforeEach(func() {
				vmCtx.VM.Annotations = map[string]string{
					vmopv1.InstanceIDAnnotation: "annotation-instance-id",
				}
				vmCtx.VM.UID = "vm-uid"
			})

			It("Should backup the cloud-init instance ID from annotations", func() {
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMCloudInitInstanceIDExtraConfigKey, "annotation-instance-id")
			})
		})

		When("VM cloud-init instance ID does not exist in ExtraConfig and is not set in annotations", func() {

			BeforeEach(func() {
				vmCtx.VM.Annotations = nil
				vmCtx.VM.UID = "vm-uid"
			})

			It("Should backup the cloud-init instance ID from VM K8s resource UID", func() {
				backupVMCtx := context.BackupVirtualMachineContext{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					BootstrapData: nil,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupVMCtx)).To(Succeed())
			})
		})
	})
}

func verifyBackupDataInExtraConfig(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	expectedKey, expectedValDecoded string) {

	// Get the VM's ExtraConfig and convert it to map.
	moID := vcVM.Reference().Value
	objVM := ctx.GetVMFromMoID(moID)
	Expect(objVM).NotTo(BeNil())
	var moVM mo.VirtualMachine
	Expect(objVM.Properties(ctx, objVM.Reference(), []string{"config.extraConfig"}, &moVM)).To(Succeed())
	ecMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)

	// Verify the expected key doesn't exist in ExtraConfig if the expected value is empty.
	if expectedValDecoded == "" {
		Expect(ecMap).NotTo(HaveKey(expectedKey))
		return
	}

	// Verify the expected key exists in ExtraConfig and the decoded values match.
	Expect(ecMap).To(HaveKey(expectedKey))
	ecValRaw := ecMap[expectedKey]
	ecValDecoded, err := util.TryToDecodeBase64Gzip([]byte(ecValRaw))
	Expect(err).NotTo(HaveOccurred())
	Expect(ecValDecoded).To(Equal(expectedValDecoded))
}
