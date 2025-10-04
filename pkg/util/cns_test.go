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
