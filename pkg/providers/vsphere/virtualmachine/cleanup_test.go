// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func cleanupOnDeleteTests() {
	var (
		vcSimCtx *builder.TestContextForVCSim
		ctx      context.Context
		vcVM     *object.VirtualMachine
		vmCtx    pkgctx.VirtualMachineContext
		vm       *vmopv1.VirtualMachine
		tagMgr   *tags.Manager
	)

	BeforeEach(func() {
		vcSimCtx = builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()),
			builder.VCSimTestConfig{})
		ctx = vcSimCtx
		ctx = vmconfig.WithContext(ctx)
		ctx = pkgctx.WithRestClient(ctx, vcSimCtx.RestClient)

		tagMgr = tags.NewManager(vcSimCtx.RestClient)
	})

	JustBeforeEach(func() {
		var err error
		vcVM, err = vcSimCtx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		vm = builder.DummyVirtualMachine()
		logger := testutil.GinkgoLogr(5)
		vmCtx = pkgctx.VirtualMachineContext{
			Context: logr.NewContext(ctx, logger),
			Logger:  logger.WithValues("vmName", vcVM.Name()),
			VM:      vm,
		}
	})

	AfterEach(func() {
		vcSimCtx.AfterEach()
		ctx = nil
		vcVM = nil
		vm = nil
	})

	Context("CleanupVMServiceState", func() {
		var (
			initialExtraConfig []vimtypes.BaseOptionValue
			initialManagedBy   *vimtypes.ManagedByInfo
		)

		JustBeforeEach(func() {
			// Set up the VM with test data
			configSpec := vimtypes.VirtualMachineConfigSpec{
				ExtraConfig: initialExtraConfig,
				ManagedBy:   initialManagedBy,
			}

			task, err := vcVM.Reconfigure(vmCtx, configSpec)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.Wait(vmCtx)).To(Succeed())
		})

		Context("when VM has ExtraConfig and ManagedBy set", func() {
			BeforeEach(func() {
				initialExtraConfig = []vimtypes.BaseOptionValue{
					// Should be removed
					&vimtypes.OptionValue{
						Key:   constants.ExtraConfigVMServiceNamespacedName,
						Value: "test-namespace/test-vm",
					},
					// Should be removed
					&vimtypes.OptionValue{
						Key:   "guestinfo.metadata",
						Value: "metadata-content",
					},
					// Should be removed
					&vimtypes.OptionValue{
						Key:   "vmservice.vmi.labels",
						Value: "test-labels",
					},
					// Should be kept
					&vimtypes.OptionValue{
						Key:   "user.key",
						Value: "user-value",
					},
				}
				// Should be removed
				initialManagedBy = &vimtypes.ManagedByInfo{
					ExtensionKey: vmopv1.ManagedByExtensionKey,
					Type:         vmopv1.ManagedByExtensionType,
				}
			})

			It("should clean both ExtraConfig and ManagedBy", func() {
				// Run cleanup
				err := virtualmachine.CleanupVMServiceState(vmCtx, vcVM)
				Expect(err).NotTo(HaveOccurred())

				// Verify both were cleaned
				var moVM mo.VirtualMachine
				err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config"}, &moVM)
				Expect(err).NotTo(HaveOccurred())
				Expect(moVM.Config).ToNot(BeNil())

				// Check ExtraConfig
				ecList := object.OptionValueList(moVM.Config.ExtraConfig)
				val1, ok1 := ecList.Get(constants.ExtraConfigVMServiceNamespacedName)
				Expect(!ok1 || val1 == "").To(BeTrue(), "Expected vmservice.namespacedName to be removed or empty")

				val2, ok2 := ecList.Get("guestinfo.metadata")
				Expect(!ok2 || val2 == "").To(BeTrue(), "Expected guestinfo.metadata to be removed or empty")

				val3, ok3 := ecList.Get("vmservice.vmi.labels")
				Expect(!ok3 || val3 == "").To(BeTrue(), "Expected vmservice.vmi.labels to be removed or empty")

				val4, ok4 := ecList.Get("user.key")
				Expect(ok4).To(BeTrue())
				Expect(val4).To(Equal("user-value"))

				// Check ManagedBy
				Expect(moVM.Config.ManagedBy).To(BeNil())
			})
		})

		Context("when VM has no VM Operator state", func() {
			BeforeEach(func() {
				initialExtraConfig = []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   "user.key1",
						Value: "value1",
					},
					&vimtypes.OptionValue{
						Key:   "user.key2",
						Value: "value2",
					},
				}
				initialManagedBy = nil
			})

			It("should complete successfully without changes", func() {
				// Run cleanup
				err := virtualmachine.CleanupVMServiceState(vmCtx, vcVM)
				Expect(err).NotTo(HaveOccurred())

				// Verify state is unchanged
				var moVMAfter mo.VirtualMachine
				err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config"}, &moVMAfter)
				Expect(err).NotTo(HaveOccurred())

				ecList := object.OptionValueList(moVMAfter.Config.ExtraConfig)
				val1, ok1 := ecList.Get("user.key1")
				Expect(ok1).To(BeTrue())
				Expect(val1).To(Equal("value1"))

				val2, ok2 := ecList.Get("user.key2")
				Expect(ok2).To(BeTrue())
				Expect(val2).To(Equal("value2"))
			})
		})

		Context("when VM has tag associations", func() {
			var (
				policyTag1ID string
				policyTag2ID string
				userTagID    string
			)

			BeforeEach(func() {
				var err error

				// Create a category for policy tags
				policyCategoryID, err := tagMgr.CreateCategory(ctx, &tags.Category{
					Name:            "policy-category",
					Description:     "Category for policy tags",
					AssociableTypes: []string{"VirtualMachine"},
				})
				Expect(err).ToNot(HaveOccurred())

				// Create policy tags (managed by VM Operator)
				policyTag1ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "policy-tag-1",
					CategoryID: policyCategoryID,
				})
				Expect(err).ToNot(HaveOccurred())

				policyTag2ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "policy-tag-2",
					CategoryID: policyCategoryID,
				})
				Expect(err).ToNot(HaveOccurred())

				// Create a category for user tags
				userCategoryID, err := tagMgr.CreateCategory(ctx, &tags.Category{
					Name:            "user-category",
					Description:     "Category for user tags",
					AssociableTypes: []string{"VirtualMachine"},
				})
				Expect(err).ToNot(HaveOccurred())

				// Create a user tag (not managed by VM Operator)
				userTagID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "user-tag",
					CategoryID: userCategoryID,
				})
				Expect(err).ToNot(HaveOccurred())

				// Set up ExtraConfig with policy tags
				// This simulates what VM Operator does when it associates tags
				initialExtraConfig = []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   policy.ExtraConfigPolicyTagsKey,
						Value: policyTag1ID + "," + policyTag2ID,
					},
				}
				initialManagedBy = nil

				// Enable VSpherePolicies feature
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VSpherePolicies = true
				})
			})

			JustBeforeEach(func() {
				// Attach both policy tags and a user tag to the VM
				// Policy tags should be removed during cleanup, user tag should remain
				Expect(tagMgr.AttachMultipleTagsToObject(
					ctx,
					[]string{policyTag1ID, policyTag2ID, userTagID},
					vcVM.Reference(),
				)).To(Succeed())
			})

			It("should remove policy tag associations but preserve user tags", func() {
				// Verify tags are present before cleanup
				attachedTagsBefore, err := tagMgr.GetAttachedTags(ctx, vcVM.Reference())
				Expect(err).ToNot(HaveOccurred())
				attachedTagIDsBefore := make([]string, len(attachedTagsBefore))
				for i, tag := range attachedTagsBefore {
					attachedTagIDsBefore[i] = tag.ID
				}
				Expect(attachedTagIDsBefore).To(ContainElement(policyTag1ID))
				Expect(attachedTagIDsBefore).To(ContainElement(policyTag2ID))
				Expect(attachedTagIDsBefore).To(ContainElement(userTagID))

				// Verify ExtraConfig has policy tags recorded
				var moVMBefore mo.VirtualMachine
				err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config"}, &moVMBefore)
				Expect(err).NotTo(HaveOccurred())
				Expect(moVMBefore.Config).ToNot(BeNil())

				ecListBefore := object.OptionValueList(moVMBefore.Config.ExtraConfig)
				tagsBefore, ok := ecListBefore.GetString(policy.ExtraConfigPolicyTagsKey)
				Expect(ok).To(BeTrue())
				Expect(tagsBefore).To(ContainSubstring(policyTag1ID))
				Expect(tagsBefore).To(ContainSubstring(policyTag2ID))

				// Run cleanup
				err = virtualmachine.CleanupVMServiceState(vmCtx, vcVM)
				Expect(err).NotTo(HaveOccurred())

				// Verify ExtraConfig key was also removed
				var moVMAfter mo.VirtualMachine
				err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"config"}, &moVMAfter)
				Expect(err).NotTo(HaveOccurred())
				Expect(moVMAfter.Config).ToNot(BeNil())

				ecListAfter := object.OptionValueList(moVMAfter.Config.ExtraConfig)
				tagsAfter, ok := ecListAfter.GetString(policy.ExtraConfigPolicyTagsKey)
				Expect(!ok || tagsAfter == "").To(BeTrue(), "Expected policy tags ExtraConfig to be removed or empty")

				// TODO: once vcsim starts supporting TagSpecs in Reconfigure, we can verify
				// that the policy tags are removed from the VM and that the user tag remains
				// attached to the VM.
			})
		})
	})
}
