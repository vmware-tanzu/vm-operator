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

//nolint:gocyclo
func backupTests() {
	const (
		// These are the default values of the vcsim VM that are used to
		// construct the expected backup data in the following tests.
		vcSimVMPath       = "DC0_C0_RP0_VM0"
		vcSimDiskUUID     = "be8d2471-f32e-5c7e-a89b-22cb8e533890"
		vcSimDiskFileName = "[LocalDS_0] DC0_C0_RP0_VM0/disk1.vmdk"
		// dummy backup versions at timestamp t1, t2, t3.
		vT1 = "T1"
		vT2 = "T2"
		vT3 = "T3"
	)

	var (
		ctx        *builder.TestContextForVCSim
		vcVM       *object.VirtualMachine
		vmCtx      pkgctx.VirtualMachineContext
		testConfig builder.VCSimTestConfig
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	DescribeTableSubtree("Backup VM",
		func(IncrementalRestore bool) {

			BeforeEach(func() {
				if IncrementalRestore {
					testConfig.WithVMIncrementalRestore = true
				}
			})

			JustBeforeEach(func() {
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
				vmCtx = pkgctx.VirtualMachineContext{}
				vcVM = nil
			})

			Context("Backup VM Resource YAML", func() {
				JustBeforeEach(func() {
					vm := builder.DummyVirtualMachine()
					vmCtx.VM = vm
				})

				When("No VM resource is stored in ExtraConfig", func() {

					It("Should backup the current VM resource YAML in ExtraConfig", func() {
						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT1
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(vmYAML))

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT1)
						}
					})
				})

				When("VM resource exists in ExtraConfig and gets a spec change", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						oldVM := vmCtx.VM.DeepCopy()
						oldVM.ObjectMeta.Generation = 10
						oldVMYAML, err := yaml.Marshal(oldVM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

					It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
						// Update the generation to simulate a new spec update of the VM.
						vmCtx.VM.ObjectMeta.Generation = 11
						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(vmYAML))

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("VM resource exists in ExtraConfig and gets an annotation change", func() {

					JustBeforeEach(func() {
						oldVM := vmCtx.VM.DeepCopy()
						oldVM.ObjectMeta.Annotations = map[string]string{"foo": "bar"}

						if IncrementalRestore {
							oldVM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						oldVMYAML, err := yaml.Marshal(oldVM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

					It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
						vmCtx.VM.ObjectMeta.Annotations = map[string]string{"foo": "bar", "new-key": "new-val"}

						if IncrementalRestore {
							vmCtx.VM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						newVMYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(newVMYAML))

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("VM resource exists in ExtraConfig and gets a label change", func() {

					JustBeforeEach(func() {
						oldVM := vmCtx.VM.DeepCopy()
						oldVM.ObjectMeta.Labels = map[string]string{"foo": "bar"}

						if IncrementalRestore {
							oldVM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						oldVMYAML, err := yaml.Marshal(oldVM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(oldVMYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

					It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
						vmCtx.VM.ObjectMeta.Labels = map[string]string{"foo": "bar", "new-key": "new-val"}
						if IncrementalRestore {
							vmCtx.VM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}
						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						newVMYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, string(newVMYAML))
						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("VM resource exists in ExtraConfig and is up-to-date", func() {
					var (
						vmBackupStr = ""
					)

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						vmBackupStr = string(vmYAML)
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmBackupStr)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}
						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
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

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.VMResourceYAMLExtraConfigKey, vmBackupStr)

						if IncrementalRestore {
							// verify it doesn't get updated to vT3 and stays at vT2
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT2)
						}
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
				JustBeforeEach(func() {
					vm := builder.DummyVirtualMachine()
					vmCtx.VM = vm
				})

				When("No additional resource is stored in ExtraConfig", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}
						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

					It("Should backup the given additional resources YAML in ExtraConfig", func() {
						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx:               vmCtx,
							VcVM:                vcVM,
							AdditionalResources: []client.Object{secretRes},
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						resYAML, err := yaml.Marshal(secretRes)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, string(resYAML))
						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("Additional resource exists in ExtraConfig but is not up-to-date", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
						Expect(err).NotTo(HaveOccurred())

						oldRes := secretRes.DeepCopy()
						oldResYAML, err := yaml.Marshal(oldRes)
						Expect(err).NotTo(HaveOccurred())
						yamlEncoded, err := pkgutil.EncodeGzipBase64(string(oldResYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
							&vimtypes.OptionValue{
								Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
								Value: yamlEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
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

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						newResYAML, err := yaml.Marshal(secretRes)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, string(newResYAML))
						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("Additional resource exists in ExtraConfig and is up-to-date", func() {
					var (
						backupStr string
					)

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
						Expect(err).NotTo(HaveOccurred())

						resYAML, err := yaml.Marshal(secretRes)
						Expect(err).NotTo(HaveOccurred())
						backupStr = string(resYAML)
						yamlEncoded, err := pkgutil.EncodeGzipBase64(backupStr)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
							&vimtypes.OptionValue{
								Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
								Value: yamlEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
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

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, backupStr)

						if IncrementalRestore {
							// verify it doesn't get updated to vT3 and stays at vT2
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT2)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
						}
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

						if IncrementalRestore {
							backupOpts.BackupVersion = vT1
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						secretResYAML, err := yaml.Marshal(secretRes)
						Expect(err).NotTo(HaveOccurred())
						cmResYAML, err := yaml.Marshal(cmRes)
						Expect(err).NotTo(HaveOccurred())
						expectedYAML := string(secretResYAML) + "\n---\n" + string(cmResYAML)
						verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.AdditionalResourcesYAMLExtraConfigKey, expectedYAML)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT1)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))
						}
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

					JustBeforeEach(func() {
						vm := builder.DummyVirtualMachine()
						vmCtx.VM = vm

						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
						}

						vmYAML, err := yaml.Marshal(vmCtx.VM)
						Expect(err).NotTo(HaveOccurred())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   vmopv1.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   vmopv1.BackupVersionKey,
								Value: vT1,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

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

						if IncrementalRestore {
							backupOpts.BackupVersion = vT2
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

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT2)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
						}
					})
				})
			})

			if IncrementalRestore {
				Context("Incremental Restore only", func() {
					JustBeforeEach(func() {
						vm := builder.DummyVirtualMachine()
						vmCtx.VM = vm
					})

					When("When backup versions (ie) vm annotation and extra config key don't match", func() {
						JustBeforeEach(func() {
							// simulate old backup version t1 restored into the inventory.
							oldVM := vmCtx.VM.DeepCopy()
							oldVM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
							vmYAML, err := yaml.Marshal(oldVM)
							Expect(err).NotTo(HaveOccurred())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   vmopv1.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
							}

							if IncrementalRestore {
								extraConfig = append(extraConfig, &vimtypes.OptionValue{
									Key:   vmopv1.BackupVersionKey,
									Value: vT1,
								})
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("pause backup", func() {
							// current annotation is at latest version t3.
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT3
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: "t4",
							}

							err := virtualmachine.BackupVirtualMachine(backupOpts)
							Expect(err).To(HaveOccurred())

							// verify key doesn't get updated to "t4" and stays at "t1"
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT1)
							// verify annotation doesn't get updated to "t4" and stays at "t3"
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						})
					})

					When("When backup versions (ie) vm annotation and extra config key match", func() {
						JustBeforeEach(func() {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
							vmYAML, err := yaml.Marshal(vmCtx.VM)
							Expect(err).NotTo(HaveOccurred())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   vmopv1.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
							}

							if IncrementalRestore {
								extraConfig = append(extraConfig, &vimtypes.OptionValue{
									Key:   vmopv1.BackupVersionKey,
									Value: vT1,
								})
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("try backup", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: vT3,
							}

							// won't err to retry later since versions match and backup was tried
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							// annotation and key stay the same as there was no new changes to backup.
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT1)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))

							// Update the generation to simulate a new spec update of the VM.
							backupOpts.VMCtx.VM.ObjectMeta.Generation = 11
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT3)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						})
					})

					When("When vm has no backup version annotation", func() {
						JustBeforeEach(func() {
							vmYAML, err := yaml.Marshal(vmCtx.VM)
							Expect(err).NotTo(HaveOccurred())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(string(vmYAML))
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   vmopv1.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("do backup", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: vT2,
							}

							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							verifyBackupDataInExtraConfig(ctx, vcVM, vmopv1.BackupVersionKey, vT2)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
						})
					})
				})
			}
		},
		Entry("Brownfield Backup", false),
		Entry("With Incremental Restore", true),
	)
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
