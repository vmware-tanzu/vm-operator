/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ = Describe("VirtualMachine Validation", func() {

	type ValidateFunc func(*VirtualMachine) field.ErrorList

	Context("validate network type", func() {
		DescribeTable("should reject network interface without name",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).ShouldNot(BeEmpty())

			},
			Entry("nsx-t network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkType: "nsx-t"}}),
			Entry("empty network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkType: ""}}),
		)

		DescribeTable("should reject network interface with invalid type",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).ShouldNot(BeEmpty())
			},
			Entry("invalid network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "invalid-type"}}),
		)

		DescribeTable("should accept valid network interface",
			func(validateFunc ValidateFunc, vnifs []VirtualMachineNetworkInterface) {
				vm := &VirtualMachine{
					Spec: VirtualMachineSpec{
						NetworkInterfaces: vnifs,
					},
				}
				Expect(validateFunc(vm)).Should(BeEmpty())
			},
			Entry("nsx-t network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: "nsx-t"}}),
			Entry("empty network type", validateNetworkType, []VirtualMachineNetworkInterface{{NetworkName: "dummy-network-name", NetworkType: ""}}),
			Entry("no network interface", validateNetworkType, nil),
		)
	})
})
