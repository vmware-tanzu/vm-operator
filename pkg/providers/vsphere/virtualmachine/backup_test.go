// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

//nolint:gocyclo
func backupTests() {
	const (
		// These are the default values of the vcsim VM that are used to
		// construct the expected backup data in the following tests.
		vcSimVMPath       = "DC0_C0_RP0_VM0"
		vcSimDiskFileName = "[LocalDS_0] DC0_C0_RP0_VM0/disk1.vmdk"

		// dummy backup versions at timestamp t1, t2, t3.
		vT1 = "1001"
		vT2 = "1002"
		vT3 = "1003"

		backupVersionMsg = "Backup version: %s"
	)

	var (
		ctx           *builder.TestContextForVCSim
		vcVM          *object.VirtualMachine
		vmCtx         pkgctx.VirtualMachineContext
		testConfig    builder.VCSimTestConfig
		vcSimDiskUUID string
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)

		dskMgr := object.NewVirtualDiskManager(ctx.VCClient.Client)
		var err error
		vcSimDiskUUID, err = dskMgr.QueryVirtualDiskUuid(ctx, vcSimDiskFileName, ctx.Datacenter)
		Expect(err).ToNot(HaveOccurred())
		Expect(vcSimDiskUUID).ToNot(BeZero())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	DescribeTableSubtree("Backup VM",
		func(IncrementalRestore bool) {

			var fakeManagedFields = []metav1.ManagedFieldsEntry{
				{
					Manager: "fake-manager",
				},
			}

			BeforeEach(func() {
				if IncrementalRestore {
					testConfig.WithVMIncrementalRestore = true
				}
			})

			JustBeforeEach(func() {
				var err error
				vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
				Expect(err).NotTo(HaveOccurred())

				logger := testutil.GinkgoLogr(5)
				vmCtx = pkgctx.VirtualMachineContext{
					Context: logr.NewContext(ctx, logger),
					Logger:  logger.WithValues("vmName", vcVM.Name()),
					VM:      builder.DummyVirtualMachine(),
				}

				// Add last-apply annotation and managed fields to verify they are not backed up in VM's ExtraConfig.
				vmCtx.VM.Annotations[corev1.LastAppliedConfigAnnotation] = "last-applied-vm"
				vmCtx.VM.ManagedFields = fakeManagedFields
			})

			AfterEach(func() {
				// Backing up the YAML should not result in an update op being flagged.
				Expect(ctxop.IsUpdate(vmCtx)).To(BeFalse())
			})

			Context("Backup VM Resource YAML", func() {

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
						expectedVMYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.VMResourceYAMLExtraConfigKey, expectedVMYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT1, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT1)))
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
						oldVMYAML := getExpectedBackupObjectYAML(oldVM)
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(oldVMYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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
						expectedVMYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.VMResourceYAMLExtraConfigKey, expectedVMYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT3)))
						}
					})
				})

				When("VM resource exists in ExtraConfig and gets an annotation change", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
							vmCtx.VM.Status.Conditions = []metav1.Condition{
								{Type: vmopv1.VirtualMachineBackupUpToDateCondition, Status: "True"},
							}
						}

						oldVM := vmCtx.VM.DeepCopy()
						oldVM.Annotations["foo"] = "bar"
						oldVMYAML := getExpectedBackupObjectYAML(oldVM)
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(oldVMYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
								Value: vT2,
							})
						}

						_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
							ExtraConfig: extraConfig,
						})
						Expect(err).NotTo(HaveOccurred())
					})

					It("Should backup VM resource YAML with the latest version in ExtraConfig", func() {
						vmCtx.VM.Annotations["foo"] = "baz"

						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						// getExpectedBackupObjectYaml empties the expected VM yaml's status
						expectedVMYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.VMResourceYAMLExtraConfigKey, expectedVMYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT3)))
						}
					})
				})

				When("VM resource exists in ExtraConfig and gets a label change", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						oldVM := vmCtx.VM.DeepCopy()
						oldVM.ObjectMeta.Labels = map[string]string{"foo": "bar"}
						oldVMYAML := getExpectedBackupObjectYAML(oldVM)
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(oldVMYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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

						backupOpts := virtualmachine.BackupVirtualMachineOptions{
							VMCtx: vmCtx,
							VcVM:  vcVM,
						}

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						expectedVMYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.VMResourceYAMLExtraConfigKey, expectedVMYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT3)))
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

						vmBackupStr = getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmBackupStr)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.VMResourceYAMLExtraConfigKey, vmBackupStr, true)

						if IncrementalRestore {
							// verify it doesn't get updated to vT3 and stays at vT2
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
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
							// Add last-apply annotation and managed fields to verify they are not backed up in ExtraConfig.
							Annotations: map[string]string{
								corev1.LastAppliedConfigAnnotation: "last-applied-secret",
							},
							ManagedFields: fakeManagedFields,
						},
					}
				)

				When("No additional resource is stored in ExtraConfig", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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
						resYAML := getExpectedBackupObjectYAML(secretRes)
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.AdditionalResourcesYAMLExtraConfigKey, resYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})

				When("Additional resource exists in ExtraConfig but is not up-to-date", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
						}

						vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
						Expect(err).NotTo(HaveOccurred())

						oldResYAML := getExpectedBackupObjectYAML(secretRes.DeepCopy())
						Expect(err).NotTo(HaveOccurred())
						yamlEncoded, err := pkgutil.EncodeGzipBase64(oldResYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
							&vimtypes.OptionValue{
								Key:   backupapi.AdditionalResourcesYAMLExtraConfigKey,
								Value: yamlEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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
						newResYAML := getExpectedBackupObjectYAML(secretRes.DeepCopy())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.AdditionalResourcesYAMLExtraConfigKey, newResYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
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

						vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
						Expect(err).NotTo(HaveOccurred())

						backupStr = getExpectedBackupObjectYAML(secretRes.DeepCopy())
						yamlEncoded, err := pkgutil.EncodeGzipBase64(backupStr)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
							&vimtypes.OptionValue{
								Key:   backupapi.AdditionalResourcesYAMLExtraConfigKey,
								Value: yamlEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.AdditionalResourcesYAMLExtraConfigKey, backupStr, true)

						if IncrementalRestore {
							// verify it doesn't get updated to vT3 and stays at vT2
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
						}
					})
				})

				When("Multiple additional resources are given", func() {

					It("Should backup the additional resources YAML with '---' separator in ExtraConfig", func() {
						cmRes := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name: "vm-configMap",
								// Add last-apply annotation and managed fields to verify they are not backed up in ExtraConfig.
								Annotations: map[string]string{
									corev1.LastAppliedConfigAnnotation: "last-applied-configMap",
								},
								ManagedFields: fakeManagedFields,
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
						secretResYAML := getExpectedBackupObjectYAML(secretRes.DeepCopy())
						cmResYAML := getExpectedBackupObjectYAML(cmRes.DeepCopy())
						expectedYAML := secretResYAML + "\n---\n" + cmResYAML
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.AdditionalResourcesYAMLExtraConfigKey, expectedYAML, true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT1, false)
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
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.PVCDiskDataExtraConfigKey, "", true)
					})
				})

				When("VM has disks that are attached from PVCs", func() {

					JustBeforeEach(func() {
						if IncrementalRestore {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
						}

						vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
						vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
						Expect(err).NotTo(HaveOccurred())

						extraConfig := []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   backupapi.VMResourceYAMLExtraConfigKey,
								Value: vmYAMLEncoded,
							},
						}

						if IncrementalRestore {
							extraConfig = append(extraConfig, &vimtypes.OptionValue{
								Key:   backupapi.BackupVersionExtraConfigKey,
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

						diskData := []backupapi.PVCDiskData{
							{
								FileName:    vcSimDiskFileName,
								PVCName:     dummyPVC.Name,
								AccessModes: backupapi.ToPersistentVolumeAccessModes(dummyPVC.Spec.AccessModes),
							},
						}
						diskDataJSON, err := json.Marshal(diskData)
						Expect(err).NotTo(HaveOccurred())
						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.PVCDiskDataExtraConfigKey, string(diskDataJSON), true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
						}
					})

					It("Should clear PVC disk data in ExtraConfig when no PVCs", func() {
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

						backupOpts.DiskUUIDToPVC = nil

						if IncrementalRestore {
							backupOpts.BackupVersion = vT3
						}

						Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

						verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.PVCDiskDataExtraConfigKey, "", true)

						if IncrementalRestore {
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
						}
					})
				})
			})

			if IncrementalRestore {
				Context("Backup Version", func() {

					When("backup versions (ie) vm annotation and extra config key don't match", func() {
						JustBeforeEach(func() {
							// simulate an old VM with backup version at t2 restored into the inventory.
							oldVM := vmCtx.VM.DeepCopy()
							oldVM.ObjectMeta.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT2
							vmYAML := getExpectedBackupObjectYAML(oldVM)
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   backupapi.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
								&vimtypes.OptionValue{
									Key:   backupapi.BackupVersionExtraConfigKey,
									Value: vT2,
								},
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("pause backup if annotation is newer than backup extra config version", func() {
							// simulate current annotation is at latest version t3.
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT3
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: "1004",
							}

							err := virtualmachine.BackupVirtualMachine(backupOpts)
							Expect(err).ToNot(HaveOccurred())

							// verify key doesn't get updated to "t4" and stays at "t1"
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
							// verify annotation doesn't get updated to "t4" and stays at "t3"
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBackupPausedReason))
							Expect(c.Message).To(Equal("A restore was detected"))
						})

						It("do backup if annotation is older than backup extra config version", func() {
							// current annotation is at an older version t1.
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: "1004",
							}

							err := virtualmachine.BackupVirtualMachine(backupOpts)
							Expect(err).ToNot(HaveOccurred())

							// verify key gets updated to "1004"
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, "1004", false)
							// verify annotation gets updated to "t4"
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal("1004"))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, "1004")))
						})
					})

					When("backup versions (ie) vm annotation and extra config key match", func() {
						JustBeforeEach(func() {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
							vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   backupapi.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
								&vimtypes.OptionValue{
									Key:   backupapi.BackupVersionExtraConfigKey,
									Value: vT1,
								},
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
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT1, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))

							// Update the generation to simulate a new spec update of the VM.
							backupOpts.VMCtx.VM.ObjectMeta.Generation = 11
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT3, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT3))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT3)))
						})
					})

					When("vm has no backup version annotation", func() {
						JustBeforeEach(func() {
							vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   backupapi.VMResourceYAMLExtraConfigKey,
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
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT2))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionTrue))
							Expect(c.Message).To(Equal(fmt.Sprintf(backupVersionMsg, vT2)))
						})
					})

					When("vm has invalid backup version annotation", func() {
						JustBeforeEach(func() {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = "invalid"
							vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   backupapi.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
								&vimtypes.OptionValue{
									Key:   backupapi.BackupVersionExtraConfigKey,
									Value: vT1,
								},
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("backup fails", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: vT2,
							}

							err := virtualmachine.BackupVirtualMachine(backupOpts)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(Equal(`strconv.ParseInt: parsing "invalid": invalid syntax`))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBackupFailedReason))
							Expect(c.Message).To(Equal(`Failed to backup VM. err: strconv.ParseInt: parsing "invalid": invalid syntax`))
						})
					})

					When("vm has invalid backup version extraConfig key", func() {
						JustBeforeEach(func() {
							vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = vT1
							vmYAML := getExpectedBackupObjectYAML(vmCtx.VM.DeepCopy())
							vmYAMLEncoded, err := pkgutil.EncodeGzipBase64(vmYAML)
							Expect(err).NotTo(HaveOccurred())

							extraConfig := []vimtypes.BaseOptionValue{
								&vimtypes.OptionValue{
									Key:   backupapi.VMResourceYAMLExtraConfigKey,
									Value: vmYAMLEncoded,
								},
								&vimtypes.OptionValue{
									Key:   backupapi.BackupVersionExtraConfigKey,
									Value: "invalid",
								},
							}

							_, err = vcVM.Reconfigure(vmCtx, vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: extraConfig,
							})
							Expect(err).NotTo(HaveOccurred())
						})

						It("backup fails", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:         vmCtx,
								VcVM:          vcVM,
								BackupVersion: vT2,
							}

							err := virtualmachine.BackupVirtualMachine(backupOpts)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(Equal(`strconv.ParseInt: parsing "invalid": invalid syntax`))
							c := conditions.Get(vmCtx.VM, vmopv1.VirtualMachineBackupUpToDateCondition)
							Expect(c).NotTo(BeNil())
							Expect(c.Status).To(Equal(metav1.ConditionFalse))
							Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBackupFailedReason))
							Expect(c.Message).To(Equal(`Failed to backup VM. err: strconv.ParseInt: parsing "invalid": invalid syntax`))
						})
					})
				})

				Context("Backup Classic Disk Data", func() {
					When("VM has no static disks attached", func() {
						It("Should skip backing up classic disk data", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: nil,
							}
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.ClassicDiskDataExtraConfigKey, "", false)
						})
					})

					When("VM has classic disks attached", func() {
						It("Should backup classic disk data", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: map[string]struct{}{vcSimDiskUUID: {}},
								BackupVersion:    vT1,
							}

							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
							diskData := []backupapi.ClassicDiskData{
								{
									FileName: vcSimDiskFileName,
								},
							}
							diskDataJSON, err := json.Marshal(diskData)
							Expect(err).NotTo(HaveOccurred())
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.ClassicDiskDataExtraConfigKey, string(diskDataJSON), true)
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT1, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))
						})
					})

					When("VM already has classic disk data backup in ExtraConfig", func() {
						JustBeforeEach(func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: map[string]struct{}{vcSimDiskUUID: {}},
								BackupVersion:    vT1,
							}
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						})

						It("Should skip backing up classic disk data", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: map[string]struct{}{vcSimDiskUUID: {}},
								BackupVersion:    vT2,
							}
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

							// Verify the backup version remains the same which indicates the backup was skipped.
							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT1, false)
							Expect(vmCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]).To(Equal(vT1))
						})
					})

					When("VM already has classic disk data backup in ExtraConfig", func() {
						JustBeforeEach(func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: map[string]struct{}{vcSimDiskUUID: {}},
								BackupVersion:    vT1,
							}
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())
						})

						It("Should clear classic disk data backup if no classic disks in ExtraConfig", func() {
							backupOpts := virtualmachine.BackupVirtualMachineOptions{
								VMCtx:            vmCtx,
								VcVM:             vcVM,
								ClassicDiskUUIDs: nil,
								BackupVersion:    vT2,
							}
							Expect(virtualmachine.BackupVirtualMachine(backupOpts)).To(Succeed())

							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.ClassicDiskDataExtraConfigKey, "", true)

							verifyBackupDataInExtraConfig(ctx, vcVM, backupapi.BackupVersionExtraConfigKey, vT2, false)
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
	expectedKey, expectedVal string, decode bool) {

	// Get the VM's ExtraConfig and convert it to map.
	moID := vcVM.Reference().Value
	objVM := ctx.GetVMFromMoID(moID)
	Expect(objVM).NotTo(BeNil())
	var moVM mo.VirtualMachine
	Expect(objVM.Properties(ctx, objVM.Reference(), []string{"config.extraConfig"}, &moVM)).To(Succeed())
	ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()

	// Verify the expected key doesn't exist in ExtraConfig if the expected value is empty.
	if expectedVal == "" {
		Expect(ecMap).NotTo(HaveKey(expectedKey))
		return
	}

	// Verify the expected key exists in ExtraConfig and the decoded values match.
	Expect(ecMap).To(HaveKey(expectedKey))
	ecValRaw := ecMap[expectedKey]

	if decode {
		ecValDecoded, err := pkgutil.TryToDecodeBase64Gzip([]byte(ecValRaw))
		Expect(err).NotTo(HaveOccurred())
		Expect(ecValDecoded).To(Equal(expectedVal))
	} else {
		Expect(ecValRaw).To(Equal(expectedVal))
	}
}

func getExpectedBackupObjectYAML(obj client.Object) string {
	// Remove fields that are trimmed before persisting in the ExtraConfig.
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		delete(annotations, corev1.LastAppliedConfigAnnotation)
		obj.SetAnnotations(annotations)
	}
	obj.SetManagedFields(nil)

	// Ignore the VM status field as the backup condition will be added after
	// persisting the VM YAML in ExtraConfig.
	if vm, ok := obj.(*vmopv1.VirtualMachine); ok {
		vm.Status = vmopv1.VirtualMachineStatus{}
		obj = vm
	}

	objYAML, err := yaml.Marshal(obj)
	Expect(err).NotTo(HaveOccurred())
	return string(objYAML)
}
