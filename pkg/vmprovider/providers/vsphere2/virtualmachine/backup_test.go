// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
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

	Context("Backup kube objects YAML", func() {
		BeforeEach(func() {
			vm := builder.DummyVirtualMachineA2()
			// Set the VM's UID and ResourceVersion to verify if the backup data is up-to-date.
			vm.UID = "vm-uid-test"
			vm.ResourceVersion = "0"
			vmCtx.VM = vm
		})

		When("No kube object is stored in ExtraConfig", func() {

			It("Should backup given kube objects as encoded, gzipped YAML in ExtraConfig", func() {
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:       vmCtx,
					VcVM:        vcVM,
					KubeObjects: []client.Object{vmCtx.VM},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				vmYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey, string(vmYAML))
			})
		})

		When("Kube objects data exists in ExtraConfig but is not up-to-date", func() {

			BeforeEach(func() {
				oldVM := vmCtx.VM.DeepCopy()
				oldVM.ObjectMeta.ResourceVersion = "1"
				oldVMYAML, err := yaml.Marshal(oldVM)
				Expect(err).NotTo(HaveOccurred())
				yamlEncoded, err := util.EncodeGzipBase64(string(oldVMYAML))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey,
							Value: yamlEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup kube objects YAML with the latest version", func() {
				vmCtx.VM.ObjectMeta.ResourceVersion = "2"
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:       vmCtx,
					VcVM:        vcVM,
					KubeObjects: []client.Object{vmCtx.VM},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				newVMYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey, string(newVMYAML))
			})
		})

		When("Kube objects data exists in ExtraConfig and is up-to-date", func() {
			var (
				kubeObjectsBackupStr = ""
			)

			BeforeEach(func() {
				vmCtx.VM.ObjectMeta.ResourceVersion = "3"
				vmYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				kubeObjectsBackupStr = string(vmYAML)
				encodedKubeObjectsBackup, err := util.EncodeGzipBase64(kubeObjectsBackupStr)
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey,
							Value: encodedKubeObjectsBackup,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should skip backing up VM kube data", func() {
				// Update the VM to verify its kube data is not backed up in ExtraConfig.
				vmCtx.VM.Labels = map[string]string{"foo": "bar"}
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:       vmCtx,
					VcVM:        vcVM,
					KubeObjects: []client.Object{vmCtx.VM},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupKubeObjectsYAMLExtraConfigKey, kubeObjectsBackupStr)
			})
		})
	})

	Context("Backup PVC Disk Data", func() {

		When("VM has no disks that are attached from PVCs", func() {

			It("Should skip backing up PVC disk data", func() {
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					DiskUUIDToPVC: nil,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupPVCDiskDataExtraConfigKey, "")
			})
		})

		When("VM has disks that are attached from PVCs", func() {

			It("Should backup PVC disk data as JSON in ExtraConfig", func() {
				dummyPVC := builder.DummyPersistentVolumeClaim()
				diskUUIDToPVC := map[string]corev1.PersistentVolumeClaim{
					vcSimDiskUUID: *dummyPVC,
				}

				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:         vmCtx,
					VcVM:          vcVM,
					DiskUUIDToPVC: diskUUIDToPVC,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

				diskData := []virtualmachine.PVCDiskData{
					{
						FileName:    vcSimDiskFileName,
						PVCName:     dummyPVC.Name,
						AccessModes: dummyPVC.Spec.AccessModes,
					},
				}
				diskDataJSON, err := json.Marshal(diskData)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupPVCDiskDataExtraConfigKey, string(diskDataJSON))
			})
		})
	})

	Context("Backup VM cloud-init instance ID data", func() {

		BeforeEach(func() {
			vmCtx.VM = builder.DummyVirtualMachineA2()
		})

		When("VM cloud-init instance ID already exists in ExtraConfig", func() {

			BeforeEach(func() {
				_, err := vcVM.Reconfigure(vmCtx, types.VirtualMachineConfigSpec{
					ExtraConfig: []types.BaseOptionValue{
						&types.OptionValue{
							Key:   vmopv1.VMBackupCloudInitInstanceIDExtraConfigKey,
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
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupCloudInitInstanceIDExtraConfigKey, "ec-instance-id")
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
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMBackupCloudInitInstanceIDExtraConfigKey, "annotation-instance-id")
			})
		})

		When("VM cloud-init instance ID does not exist in ExtraConfig and is not set in annotations", func() {

			BeforeEach(func() {
				vmCtx.VM.Annotations = nil
				vmCtx.VM.UID = "vm-uid"
			})

			It("Should backup the cloud-init instance ID from VM K8s resource UID", func() {
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}
				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
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
