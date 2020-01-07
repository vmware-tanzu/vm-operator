// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newVM() *VirtualMachineImage {
	return &VirtualMachineImage{
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-vm-name",
			Annotations:       map[string]string{},
			CreationTimestamp: v1.NewTime(time.Time{}),
		},
		Status: VirtualMachineImageStatus{
			Uuid:       "uuid",
			InternalId: "internal-id",
			PowerState: "powerstate",
		},
		Spec: VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
	}
}

var _ = Describe("VirtualMachineImage Validation", func() {
	var (
		vmImageStrategy       VirtualMachineImageStrategy
		vmImageStatusStrategy VirtualMachineImageStatusStrategy
	)

	BeforeEach(func() {
		vmImageStrategy = VirtualMachineImageStrategy{}
		vmImageStatusStrategy = VirtualMachineImageStatusStrategy{}
	})

	Context("When always", func() {
		It("virtual machine images are cluster-scoped", func() {
			Expect(vmImageStrategy.NamespaceScoped()).To(BeFalse())
		})
		It("virtual machine images status are cluster-scoped", func() {
			Expect(vmImageStatusStrategy.NamespaceScoped()).To(BeFalse())
		})
		It("virtual machine images are always valid", func() {
			Expect(vmImageStrategy.Validate(context.TODO(), newVM())).To(BeEmpty())
		})
	})
})
