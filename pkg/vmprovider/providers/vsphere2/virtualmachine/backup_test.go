// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"encoding/json"

	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func backupTests() {
	var (
		ctx   *builder.TestContextForVCSim
		vcVM  *object.VirtualMachine
		vmCtx context.VirtualMachineContextA2
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		vmCtx = context.VirtualMachineContextA2{
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
			vmCtx.VM = builder.DummyVirtualMachineA2()
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
				Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())

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
				Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMKubeDataExtraConfigKey, kubeDataBackup)
			})
		})
	})

	Context("VM bootstrap data", func() {

		It("Should back up bootstrap data as JSON in ExtraConfig", func() {
			bootstrapDataRaw := map[string]string{"foo": "bar"}
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, bootstrapDataRaw)).To(Succeed())

			bootstrapDataJSON, err := json.Marshal(bootstrapDataRaw)
			Expect(err).NotTo(HaveOccurred())
			verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMBootstrapDataExtraConfigKey, string(bootstrapDataJSON))
		})
	})

	Context("VM Disk data", func() {

		It("Should backup VM disk data as JSON in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())

			// Use the default disk info from the vcSim VM for testing.
			diskData := []virtualmachine.VMDiskData{
				{
					VDiskID:  "",
					FileName: "[LocalDS_0] DC0_C0_RP0_VM0/disk1.vmdk",
				},
			}
			diskDataJSON, err := json.Marshal(diskData)
			Expect(err).NotTo(HaveOccurred())
			verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMDiskDataExtraConfigKey, string(diskDataJSON))
		})
	})

	Context("VM cloud-init instance ID data", func() {

		BeforeEach(func() {
			vmCtx.VM = builder.DummyVirtualMachineA2()
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

			It("Should skip backing up the cloud-init instance ID", func() {
				Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())
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
				Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMCloudInitInstanceIDExtraConfigKey, "annotation-instance-id")
			})
		})

		When("VM cloud-init instance ID does not exist in ExtraConfig and is not set in annotations", func() {

			BeforeEach(func() {
				vmCtx.VM.Annotations = nil
				vmCtx.VM.UID = "vm-uid"
			})

			It("Should backup the cloud-init instance ID from annotations", func() {
				Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMCloudInitInstanceIDExtraConfigKey, "vm-uid")
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

	// Verify the expected key exists in ExtraConfig and the decoded values match.
	Expect(ecMap).To(HaveKey(expectedKey))
	ecValRaw := ecMap[expectedKey]
	ecValDecoded, err := util.TryToDecodeBase64Gzip([]byte(ecValRaw))
	Expect(err).NotTo(HaveOccurred())
	Expect(ecValDecoded).To(Equal(expectedValDecoded))
}
