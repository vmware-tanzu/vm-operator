// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("BuildControllerKey", func() {

	It("shoud build controller key with type and bus number", func() {
		key := util.BuildControllerKey(vmopv1.VirtualControllerTypeSCSI, ptr.To[int32](2))
		Expect(key).To(Equal("SCSI:2"))
	})

	It("should build controller key with type only", func() {
		key := util.BuildControllerKey(vmopv1.VirtualControllerTypeNVME, nil)
		Expect(key).To(Equal("NVME"))
	})

	It("should build controller key with bus number only", func() {
		key := util.BuildControllerKey("", ptr.To[int32](3))
		Expect(key).To(Equal(":3"))
	})

	It("should return empty string when neither specified", func() {
		key := util.BuildControllerKey("", nil)
		Expect(key).To(Equal(""))
	})
})

var _ = DescribeTable("SanitizeCNSErrorMessage",
	func(original, expected string) {
		Expect(util.SanitizeCNSErrorMessage(original)).To(Equal(expected))
	},
	Entry("leave empty message untouched", "", ""),
	Entry("leave normal message with ':' untouched", "error: this is wrong", "error: this is wrong"),
	Entry("sanitize aweful error message", `failed to attach cns volume: \"88854b48-2b1c-43f8-8889-de4b5ca2cab5\" to node vm: \"VirtualMachine:vm-42
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
