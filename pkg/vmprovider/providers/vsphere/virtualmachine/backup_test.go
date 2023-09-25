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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
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

		It("Should backup VM kube data YAML without status field in ExtraConfig", func() {
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, nil)).To(Succeed())

			vmCopy := vmCtx.VM.DeepCopy()
			vmCopy.Status = vmopv1.VirtualMachineStatus{}
			vmCopyYaml, err := yaml.Marshal(vmCopy)
			Expect(err).NotTo(HaveOccurred())
			verifyBackupDataInExtraConfig(ctx, vcVM, constants.BackupVMKubeDataExtraConfigKey, string(vmCopyYaml))
		})
	})

	Context("VM bootstrap data", func() {

		It("Should back up bootstrap data as JSON in ExtraConfig", func() {
			bootstrapData := map[string]string{"foo": "bar"}
			Expect(virtualmachine.BackupVirtualMachine(vmCtx, vcVM, bootstrapData)).To(Succeed())

			bootstrapDataJSON, err := json.Marshal(bootstrapData)
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
	ecMap := session.ExtraConfigToMap(moVM.Config.ExtraConfig)

	// Verify the expected key exists in ExtraConfig and the decoded values match.
	Expect(ecMap).To(HaveKey(expectedKey))
	ecValRaw := ecMap[expectedKey]
	ecValDecoded, err := util.TryToDecodeBase64Gzip([]byte(ecValRaw))
	Expect(err).NotTo(HaveOccurred())
	Expect(ecValDecoded).To(Equal(expectedValDecoded))
}
