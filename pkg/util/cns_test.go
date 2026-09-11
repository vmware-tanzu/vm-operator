// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = DescribeTable("SanitizeCNSErrorMessage",
	func(original, expected string) {
		Expect(util.SanitizeCNSErrorMessage(original)).To(Equal(expected))
	},
	Entry("leave empty message untouched", "", ""),
	Entry("leave normal message with ':' untouched", "error: this is wrong", "error: this is wrong"),
	Entry("sanitize awful error message", `failed to attach cns volume: \"88854b48-2b1c-43f8-8889-de4b5ca2cab5\" to node vm: \"VirtualMachine:vm-42
[VirtualCenterHost: vc.vmware.com, UUID: 42080725-d6b0-c045-b24e-29c4dadca6f2, Datacenter: Datacenter
[Datacenter: Datacenter:datacenter, VirtualCenterHost: vc.vmware.com]]\".
fault: \"(*vimtypes.LocalizedMethodFault)(0xc003d9b9a0)({\\n DynamicData: (vimtypes.DynamicData)
{\\n },\\n Fault: (*vimtypes.ResourceInUse)(0xc002e69080)({\\n VimFault: (vimtypes.VimFault)
{\\n MethodFault: (vimtypes.MethodFault) {\\n FaultCause: (*vimtypes.LocalizedMethodFault)(\u003cnil\u003e),\\n
FaultMessage: ([]vimtypes.LocalizableMessage) \u003cnil\u003e\\n }\\n },\\n Type: (string) \\\"\\\",\\n Name:
(string) (len=6) \\\"volume\\\"\\n }),\\n LocalizedMessage: (string) (len=32)
\\\"The resource 'volume' is in use.\\\"\\n})\\n\". opId: \"67d69c68\""
`, "failed to attach cns volume"),
)

var _ = Describe("GetCnsNodeVMAttachmentsForVM", func() {

	const namespace = "test-namespace"

	// nodeUUIDIndexerFunc mirrors the field indexer registered by the
	// volume/volumebatch controllers' AddToManager, which normalizes
	// spec.nodeuuid to lowercase before indexing.
	nodeUUIDIndexerFunc := func(obj client.Object) []string {
		attachment := obj.(*cnsv1alpha1.CnsNodeVmAttachment)
		return []string{strings.ToLower(attachment.Spec.NodeUUID)}
	}

	newClientWithAttachment := func(attachmentNodeUUID string) client.Client {
		attachment := &cnsv1alpha1.CnsNodeVmAttachment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-attachment",
				Namespace: namespace,
			},
			Spec: cnsv1alpha1.CnsNodeVmAttachmentSpec{
				NodeUUID: attachmentNodeUUID,
			},
		}
		return fake.NewClientBuilder().
			WithScheme(builder.NewScheme()).
			WithObjects(attachment).
			WithIndex(&cnsv1alpha1.CnsNodeVmAttachment{}, "spec.nodeuuid", nodeUUIDIndexerFunc).
			Build()
	}

	DescribeTable("finds attachments regardless of BiosUUID/NodeUUID letter casing",
		func(vmBiosUUID, attachmentNodeUUID string, expectMatch bool) {
			vm := &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
				Status:     vmopv1.VirtualMachineStatus{BiosUUID: vmBiosUUID},
			}

			attachments, err := util.GetCnsNodeVMAttachmentsForVM(
				context.Background(), newClientWithAttachment(attachmentNodeUUID), vm)
			Expect(err).ToNot(HaveOccurred())

			if expectMatch {
				Expect(attachments).To(HaveLen(1))
				Expect(attachments).To(HaveKey("my-attachment"))
			} else {
				Expect(attachments).To(BeEmpty())
			}
		},
		Entry("identical casing matches",
			"4212a4b6-9f8e-4b1a-9b1e-0123456789ab",
			"4212a4b6-9f8e-4b1a-9b1e-0123456789ab",
			true),
		Entry("uppercase BiosUUID matches lowercase NodeUUID",
			"4212A4B6-9F8E-4B1A-9B1E-0123456789AB",
			"4212a4b6-9f8e-4b1a-9b1e-0123456789ab",
			true),
		Entry("lowercase BiosUUID matches uppercase NodeUUID",
			"4212a4b6-9f8e-4b1a-9b1e-0123456789ab",
			"4212A4B6-9F8E-4B1A-9B1E-0123456789AB",
			true),
		Entry("mixed-case BiosUUID matches mixed-case NodeUUID",
			"4212A4b6-9F8e-4b1A-9b1E-0123456789Ab",
			"4212a4B6-9f8E-4B1a-9B1e-0123456789aB",
			true),
		Entry("non-matching UUIDs do not match",
			"4212a4b6-9f8e-4b1a-9b1e-0123456789ab",
			"ffffffff-ffff-ffff-ffff-ffffffffffff",
			false),
	)
})

var _ = Describe("GetCnsDiskModeFromDiskMode", func() {
	DescribeTable("disk mode conversion",
		func(volumeDiskMode vmopv1.VolumeDiskMode, expectedCnsDiskMode cnsv1alpha1.DiskMode, expectError bool) {
			cnsDiskMode, err := util.GetCnsDiskModeFromDiskMode(volumeDiskMode)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported disk mode"))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(cnsDiskMode).To(Equal(expectedCnsDiskMode))
			}
		},
		Entry("Persistent mode",
			vmopv1.VolumeDiskModePersistent,
			cnsv1alpha1.Persistent,
			false,
		),
		Entry("IndependentPersistent mode",
			vmopv1.VolumeDiskModeIndependentPersistent,
			cnsv1alpha1.IndependentPersistent,
			false,
		),
		Entry("NonPersistent mode",
			vmopv1.VolumeDiskModeNonPersistent,
			cnsv1alpha1.DiskMode(cnsv1alpha1.NonPersistent),
			false,
		),
		Entry("IndependentNonPersistent mode",
			vmopv1.VolumeDiskModeIndependentNonPersistent,
			cnsv1alpha1.DiskMode(cnsv1alpha1.IndependentNonPersistent),
			false,
		),
		Entry("empty mode is unsupported",
			vmopv1.VolumeDiskMode(""),
			cnsv1alpha1.DiskMode(""),
			true,
		),
		Entry("unknown mode is unsupported",
			vmopv1.VolumeDiskMode("unknown_mode"),
			cnsv1alpha1.DiskMode(""),
			true,
		),
	)
})

var _ = Describe("GetCnsSharingModeFromSharingMode", func() {
	DescribeTable("sharing mode conversion",
		func(volumeSharingMode vmopv1.VolumeSharingMode, expectedCnsSharingMode cnsv1alpha1.SharingMode, expectError bool) {
			cnsSharingMode, err := util.GetCnsSharingModeFromSharingMode(volumeSharingMode)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported sharing mode"))
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(cnsSharingMode).To(Equal(expectedCnsSharingMode))
			}
		},
		Entry("None mode",
			vmopv1.VolumeSharingModeNone,
			cnsv1alpha1.SharingNone,
			false,
		),
		Entry("MultiWriter mode",
			vmopv1.VolumeSharingModeMultiWriter,
			cnsv1alpha1.SharingMultiWriter,
			false,
		),
		Entry("empty mode is unsupported",
			vmopv1.VolumeSharingMode(""),
			cnsv1alpha1.SharingMode(""),
			true,
		),
		Entry("unknown mode is unsupported",
			vmopv1.VolumeSharingMode("unknown_sharing"),
			cnsv1alpha1.SharingMode(""),
			true,
		),
	)
})
