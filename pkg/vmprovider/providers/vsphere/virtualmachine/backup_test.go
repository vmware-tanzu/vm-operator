// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func backupTests() {

	var (
		ctx   *builder.TestContextForVCSim
		vcVM  *object.VirtualMachine
		vmCtx context.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())

		vmCtx = context.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vcVM.Name()),
			VM:      builder.DummyVirtualMachine(),
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	JustBeforeEach(func() {
		if vmCtx.VM.Spec.VmMetadata == nil {
			return
		}

		dummyData := map[string]string{"foo": "bar"}

		if cmName := vmCtx.VM.Spec.VmMetadata.ConfigMapName; cmName != "" {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: vmCtx.VM.Namespace,
				},
				Data: dummyData,
			}
			Expect(ctx.Client.Create(ctx, cm)).To(Succeed())
		}

		if secretName := vmCtx.VM.Spec.VmMetadata.SecretName; secretName != "" {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: vmCtx.VM.Namespace,
				},
				StringData: dummyData,
			}
			Expect(ctx.Client.Create(ctx, secret)).To(Succeed())
		}
	})

	JustAfterEach(func() {
		if vmCtx.VM.Spec.VmMetadata == nil {
			return
		}

		if cmName := vmCtx.VM.Spec.VmMetadata.ConfigMapName; cmName != "" {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: vmCtx.VM.Namespace,
				},
			}
			Expect(ctx.Client.Delete(ctx, cm)).To(Succeed())
		}

		if secretName := vmCtx.VM.Spec.VmMetadata.SecretName; secretName != "" {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: vmCtx.VM.Namespace,
				},
			}
			Expect(ctx.Client.Delete(ctx, secret)).To(Succeed())
		}
	})

	Context("VM has no VM Metadata defined", func() {

		var (
			originalVMMetadata *vmopv1.VirtualMachineMetadata
		)

		BeforeEach(func() {
			originalVMMetadata = vmCtx.VM.Spec.VmMetadata
			vmCtx.VM.Spec.VmMetadata = nil
		})

		AfterEach(func() {
			vmCtx.VM.Spec.VmMetadata = originalVMMetadata
		})

		It("Should back up only VM kube data in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, ctx.Client)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx)
		})
	})

	Context("VM has neither ConfigMap or Secret defined in VM Metadata", func() {

		var (
			originalVMMetadata *vmopv1.VirtualMachineMetadata
		)

		BeforeEach(func() {
			originalVMMetadata = vmCtx.VM.Spec.VmMetadata
			vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
				Transport: "CloudInit",
			}
		})

		AfterEach(func() {
			vmCtx.VM.Spec.VmMetadata = originalVMMetadata
		})

		It("Should back up only VM kube data in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, ctx.Client)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx)

		})
	})

	Context("VM has bootstrap data defined in ConfigMap", func() {

		var (
			originalVMMetadata *vmopv1.VirtualMachineMetadata
		)

		BeforeEach(func() {
			originalVMMetadata = vmCtx.VM.Spec.VmMetadata
			vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
				ConfigMapName: "dummy-cm",
			}
		})

		AfterEach(func() {
			vmCtx.VM.Spec.VmMetadata = originalVMMetadata
		})

		It("Should back up both VM kube data and bootstrap data from ConfigMap in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, ctx.Client)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx)
		})

	})

	Context("VM has bootstrap data defined in Secret", func() {

		var (
			originalVMMetadata *vmopv1.VirtualMachineMetadata
		)

		BeforeEach(func() {
			originalVMMetadata = vmCtx.VM.Spec.VmMetadata
			vmCtx.VM.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
				SecretName: "dummy-secret",
			}
		})

		AfterEach(func() {
			vmCtx.VM.Spec.VmMetadata = originalVMMetadata
		})

		It("Should back up both VM kube data and bootstrap data from Secret in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, ctx.Client)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx)
		})
	})
}

// verifyBackupDataInExtraConfig verifies that the backup data is stored in VM's ExtraConfig.
// It gets the expected data from the VM CR and compares it with the actual data in ExtraConfig.
func verifyBackupDataInExtraConfig(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	vmCtx context.VirtualMachineContext) {
	// Get expected backup VM's kube data from the VM CR.
	vmCopy := vmCtx.VM.DeepCopy()
	vmCopy.Status = vmopv1.VirtualMachineStatus{}
	vmCopyYaml, err := yaml.Marshal(vmCopy)
	Expect(err).NotTo(HaveOccurred())
	Expect(vmCopyYaml).NotTo(BeEmpty())
	expectedKubeData, err := util.EncodeGzipBase64(string(vmCopyYaml))
	Expect(err).NotTo(HaveOccurred())

	// Get expected backup bootstrap data from VM's ConfigMap/Secret if defined.
	expectedBootstrapData := ""
	if vmCtx.VM.Spec.VmMetadata != nil {
		if cmName := vmCtx.VM.Spec.VmMetadata.ConfigMapName; cmName != "" {
			cm := &corev1.ConfigMap{}
			cmKey := ctrlruntime.ObjectKey{Name: cmName, Namespace: vmCtx.VM.Namespace}
			Expect(ctx.Client.Get(ctx, cmKey, cm)).To(Succeed())
			cm.APIVersion = "v1"
			cm.Kind = "ConfigMap"
			cmYaml, err := yaml.Marshal(cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cmYaml).NotTo(BeEmpty())
			expectedBootstrapData, err = util.EncodeGzipBase64(string(cmYaml))
			Expect(err).NotTo(HaveOccurred())
		}

		if secretName := vmCtx.VM.Spec.VmMetadata.SecretName; secretName != "" {
			secret := &corev1.Secret{}
			secretKey := ctrlruntime.ObjectKey{Name: secretName, Namespace: vmCtx.VM.Namespace}
			Expect(ctx.Client.Get(ctx, secretKey, secret)).To(Succeed())
			secret.APIVersion = "v1"
			secret.Kind = "Secret"
			secretYaml, err := yaml.Marshal(secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secretYaml).NotTo(BeEmpty())
			expectedBootstrapData, err = util.EncodeGzipBase64(string(secretYaml))
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// Get actual backup data from VM's ExtraConfig.
	moID := vcVM.Reference().Value
	objVM := ctx.GetVMFromMoID(moID)
	Expect(objVM).ToNot(BeNil())
	var moVM mo.VirtualMachine
	Expect(objVM.Properties(ctx, objVM.Reference(), []string{"config.extraConfig"}, &moVM)).To(Succeed())

	// Compare the backup data in ExtraConfig with the expected data.
	var kubeDataMatched, bootstrapDataMatched bool
	for _, ec := range moVM.Config.ExtraConfig {
		if ec.GetOptionValue().Key == constants.BackupVMBootstrapDataExtraConfigKey {
			Expect(ec.GetOptionValue().Value.(string)).To(Equal(expectedBootstrapData))
			bootstrapDataMatched = true
		} else if ec.GetOptionValue().Key == constants.BackupVMKubeDataExtraConfigKey {
			Expect(ec.GetOptionValue().Value.(string)).To(Equal(expectedKubeData))
			kubeDataMatched = true
		}

		if kubeDataMatched && bootstrapDataMatched {
			return
		}
	}

	Expect(kubeDataMatched).To(BeTrue())
	Expect(bootstrapDataMatched).To(BeTrue())
}
