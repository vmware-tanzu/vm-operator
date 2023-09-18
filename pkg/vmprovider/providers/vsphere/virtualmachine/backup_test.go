// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func backupTests() {
	var (
		ctx           *builder.TestContextForVCSim
		vcVM          *object.VirtualMachine
		vmCtx         context.VirtualMachineContext
		bootstrapData map[string]string
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

	Context("VM bootstrap data is NOT provided", func() {

		It("Should back up only VM kube data in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx, nil)
		})
	})

	Context("VM bootstrap data is provided", func() {

		BeforeEach(func() {
			bootstrapData = map[string]string{"foo": "bar"}
		})

		AfterEach(func() {
			bootstrapData = nil
		})

		It("Should back up both VM kube data and bootstrap data in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, bootstrapData)).To(Succeed())
			verifyBackupDataInExtraConfig(ctx, vcVM, vmCtx, bootstrapData)
		})
	})
}

// verifyBackupDataInExtraConfig verifies that the backup data is stored in VM's ExtraConfig.
// It gets the expected data and compares it with the actual data in ExtraConfig.
func verifyBackupDataInExtraConfig(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	vmCtx context.VirtualMachineContext,
	bootstrapData map[string]string) {
	// Get expected backup VM's kube data from the VM CR.
	vmCopy := vmCtx.VM.DeepCopy()
	vmCopy.Status = vmopv1.VirtualMachineStatus{}
	vmCopyYaml, err := yaml.Marshal(vmCopy)
	Expect(err).NotTo(HaveOccurred())
	Expect(vmCopyYaml).NotTo(BeEmpty())
	expectedKubeData, err := util.EncodeGzipBase64(string(vmCopyYaml))
	Expect(err).NotTo(HaveOccurred())

	var expectedBootstrapData string
	if bootstrapData != nil {
		bootstrapDataYaml, err := yaml.Marshal(bootstrapData)
		Expect(err).NotTo(HaveOccurred())
		expectedBootstrapData, err = util.EncodeGzipBase64(string(bootstrapDataYaml))
		Expect(err).NotTo(HaveOccurred())
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
		if ec.GetOptionValue().Key == constants.BackupVMKubeDataExtraConfigKey {
			Expect(ec.GetOptionValue().Value.(string)).To(Equal(expectedKubeData))
			kubeDataMatched = true
		} else if ec.GetOptionValue().Key == constants.BackupVMBootstrapDataExtraConfigKey {
			Expect(ec.GetOptionValue().Value.(string)).To(Equal(expectedBootstrapData))
			bootstrapDataMatched = true
		}

		if kubeDataMatched && bootstrapDataMatched {
			return
		}
	}

	Expect(kubeDataMatched).To(BeTrue(), "Encoded VM kube data is not found in VM's ExtraConfig")
	Expect(bootstrapDataMatched).To(BeTrue(), "Encoded VM bootstrap data is not found in VM's ExtraConfig")
}
