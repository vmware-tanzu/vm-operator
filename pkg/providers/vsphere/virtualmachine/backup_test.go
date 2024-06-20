// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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
		vmCtx pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      &vmopv1.VirtualMachine{},
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Backup VM Resource YAML", func() {
		BeforeEach(func() {
			vm := builder.DummyVirtualMachine()
			vmCtx.VM = vm
		})

		When("No VM resource is stored in ExtraConfig", func() {

			It("Should backup the current VM resource YAML in ExtraConfig", func() {
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				vmYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(vmYAML))
			})
		})

		When("VM resource exists in ExtraConfig and gets a spec change", func() {

			BeforeEach(func() {
				oldVM := vmCtx.VM.DeepCopy()
				oldVM.ObjectMeta.Generation = 1
				oldVMYAML, err := yaml.Marshal(oldVM)
				Expect(err).NotTo(HaveOccurred())
				vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.VMResourceYAMLExtraConfigKey,
							Value: vmYAMLEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
				// Update the generation to simulate a new spec update of the VM.
				vmCtx.VM.ObjectMeta.Generation++
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				newVMYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(newVMYAML))
			})
		})

		When("VM resource exists in ExtraConfig and gets an annotation change", func() {

			BeforeEach(func() {
				oldVM := vmCtx.VM.DeepCopy()
				oldVM.ObjectMeta.Annotations = map[string]string{"foo": "bar"}
				oldVMYAML, err := yaml.Marshal(oldVM)
				Expect(err).NotTo(HaveOccurred())
				vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.VMResourceYAMLExtraConfigKey,
							Value: vmYAMLEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
				vmCtx.VM.ObjectMeta.Annotations = map[string]string{"foo": "bar", "new-key": "new-val"}
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				newVMYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(newVMYAML))
			})
		})

		When("VM resource exists in ExtraConfig and gets a label change", func() {

			BeforeEach(func() {
				oldVM := vmCtx.VM.DeepCopy()
				oldVM.ObjectMeta.Labels = map[string]string{"foo": "bar"}
				oldVMYAML, err := yaml.Marshal(oldVM)
				Expect(err).NotTo(HaveOccurred())
				vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.VMResourceYAMLExtraConfigKey,
							Value: vmYAMLEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
				vmCtx.VM.ObjectMeta.Labels = map[string]string{"foo": "bar", "new-key": "new-val"}
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				newVMYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(newVMYAML))
			})
		})

		When("VM resource exists in ExtraConfig and is up-to-date", func() {
			var (
				vmBackupStr = ""
			)

			BeforeEach(func() {
				vmYAML, err := yaml.Marshal(vmCtx.VM)
				Expect(err).NotTo(HaveOccurred())
				vmBackupStr = string(vmYAML)
				vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmBackupStr)
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.VMResourceYAMLExtraConfigKey,
							Value: vmYAMLEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should skip backing up VM resource YAML in ExtraConfig", func() {
				// Update VM status field to verify its resource YAML is not backed up in ExtraConfig.
				vmCtx.VM.Status.Conditions = []metav1.Condition{
					{Type: "New-Condition", Status: "True"},
				}
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx: vmCtx,
					VcVM:  vcVM,
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, vmBackupStr)
			})
		})
	})

	Context("Backup Additional Resources YAML", func() {
		var (
			secretRes = &corev1.Secret{
				// The typeMeta may not be populated when getting the resource from client.
				// It's required in test for getting the resource version to check if the backup is up-to-date.
				TypeMeta: metav1.TypeMeta{
					Kind: "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					UID:             "secret-uid",
					ResourceVersion: "0",
					Name:            "vm-secret",
				},
			}
		)
		BeforeEach(func() {
			vm := builder.DummyVirtualMachine()
			vmCtx.VM = vm
		})

		When("No additional resource is stored in ExtraConfig", func() {

			It("Should backup the given additional resources YAML in ExtraConfig", func() {
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:               vmCtx,
					VcVM:                vcVM,
					AdditionalResources: []client.Object{secretRes},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				resYAML, err := yaml.Marshal(secretRes)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, string(resYAML))
			})
		})

		When("Additional resource exists in ExtraConfig but is not up-to-date", func() {

			BeforeEach(func() {
				oldRes := secretRes.DeepCopy()
				oldResYAML, err := yaml.Marshal(oldRes)
				Expect(err).NotTo(HaveOccurred())
				yamlEncoded, err := pkgutil.EncodeGzipBase64(string(oldResYAML))
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
							Value: yamlEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should backup the given additional resources YAML with the latest version in ExtraConfig", func() {
				secretRes.ObjectMeta.ResourceVersion = "1"
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:               vmCtx,
					VcVM:                vcVM,
					AdditionalResources: []client.Object{secretRes},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				newResYAML, err := yaml.Marshal(secretRes)
				Expect(err).NotTo(HaveOccurred())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, string(newResYAML))
			})
		})

		When("Additional resource exists in ExtraConfig and is up-to-date", func() {
			var (
				backupStr string
			)

			BeforeEach(func() {
				resYAML, err := yaml.Marshal(secretRes)
				Expect(err).NotTo(HaveOccurred())
				backupStr = string(resYAML)
				yamlEncoded, err := pkgutil.EncodeGzipBase64(backupStr)
				Expect(err).NotTo(HaveOccurred())

				_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
							Value: yamlEncoded,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should skip backing up the additional resource YAML in ExtraConfig", func() {
				// Update the resource without changing its resourceVersion to verify the backup is skipped.
				secretRes.Labels = map[string]string{"foo": "bar"}
				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:               vmCtx,
					VcVM:                vcVM,
					AdditionalResources: []client.Object{secretRes},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, backupStr)
			})
		})

		When("Multiple additional resources are given", func() {

			It("Should backup the additional resources YAML with '---' separator in ExtraConfig", func() {
				cmRes := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vm-configMap",
					},
				}

				backupOpts := virtualmachine.BackupVirtualMachineOptions{
					VMCtx:               vmCtx,
					VcVM:                vcVM,
					AdditionalResources: []client.Object{secretRes, cmRes},
				}

				Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
				secretResYAML, err := yaml.Marshal(secretRes)
				Expect(err).NotTo(HaveOccurred())
				cmResYAML, err := yaml.Marshal(cmRes)
				Expect(err).NotTo(HaveOccurred())
				expectedYAML := string(secretResYAML) + "\n---\n" + string(cmResYAML)
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, expectedYAML)
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
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.PVCDiskDataExtraConfigKey, "")
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
				verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.PVCDiskDataExtraConfigKey, string(diskDataJSON))
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
	ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()

	// Verify the expected key doesn't exist in ExtraConfig if the expected value is empty.
	if expectedValDecoded == "" {
		Expect(ecMap).NotTo(HaveKey(expectedKey))
		return
	}

	// Verify the expected key exists in ExtraConfig and the decoded values match.
	Expect(ecMap).To(HaveKey(expectedKey))
	ecValRaw := ecMap[expectedKey]
	ecValDecoded, err := pkgutil.TryToDecodeBase64Gzip([]byte(ecValRaw))
	Expect(err).NotTo(HaveOccurred())
	Expect(ecValDecoded).To(Equal(expectedValDecoded))
}
