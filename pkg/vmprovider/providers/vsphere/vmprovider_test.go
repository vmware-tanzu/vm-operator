// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

// TODO: ResVmToVirtualMachineImage isn't currently used. Just remove it?
var _ = Describe("ResVmToVirtualMachineImage", func() {

	It("returns a VirtualMachineImage object from an inventory VM with annotations", func() {
		simulator.Test(func(ctx context.Context, c *vim25.Client) {
			svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
			obj := object.NewVirtualMachine(c, svm.Reference())

			resVM, err := res.NewVMFromObject(obj)
			Expect(err).To(BeNil())

			// TODO: Need to convert this VM to a vApp (and back).
			// annotations := map[string]string{}
			// annotations[versionKey] = versionVal

			image, err := vsphere.ResVMToVirtualMachineImage(ctx, resVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal(obj.Name()))
			// Expect(image.Annotations).ToNot(BeEmpty())
			// Expect(image.Annotations).To(HaveKeyWithValue(versionKey, versionVal))
		})
	})
})
