/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func aVirtualMachineClassResources(requests VirtualMachineClassResourceSpec, limits VirtualMachineClassResourceSpec) VirtualMachineClassResources {
	return VirtualMachineClassResources{
		Requests: requests,
		Limits:   limits,
	}
}

func aVirtualMachineClass(hardware VirtualMachineClassHardware, policies VirtualMachineClassPolicies) VirtualMachineClass {
	return VirtualMachineClass{
		Spec: VirtualMachineClassSpec{
			Hardware: hardware,
			Policies: policies,
		},
	}
}

var _ = Describe("VirtualMachineClass Validation", func() {
	var (
		policies VirtualMachineClassPolicies

		vmClass  = VirtualMachineClass{}
		hardware = VirtualMachineClassHardware{}

		resourceSpecWithMemory100 = VirtualMachineClassResourceSpec{
			Memory: resource.MustParse("100Mi"),
		}
		resourceSpecWithMemory200 = VirtualMachineClassResourceSpec{
			Memory: resource.MustParse("200Mi"),
		}
		resourceSpecWithCpu1000 = VirtualMachineClassResourceSpec{
			Cpu: resource.MustParse("1000Mi"),
		}
		resourceSpecWithCpu2000 = VirtualMachineClassResourceSpec{
			Cpu: resource.MustParse("2000Mi"),
		}
	)

	type ValidateFunc func(VirtualMachineClass) field.ErrorList

	BeforeEach(func() {
		requests := VirtualMachineClassResourceSpec{}
		limits := VirtualMachineClassResourceSpec{}
		policies = VirtualMachineClassPolicies{
			aVirtualMachineClassResources(requests, limits),
			"",
		}
	})

	DescribeTable("should validate with empty",
		func(validateFunc ValidateFunc) {
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory),
		Entry("for CPU resources", validateCPU),
	)

	DescribeTable("should validate when no reservation or limit",
		func(validateFunc ValidateFunc) {
			vmClass := aVirtualMachineClass(hardware, policies)
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory),
		Entry("for CPU resources", validateCPU),
	)
	DescribeTable("should validate with limit larger than reservation",
		func(validateFunc ValidateFunc, request *VirtualMachineClassResourceSpec, limit *VirtualMachineClassResourceSpec) {
			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*request, *limit),
				"",
			}

			vmClass := aVirtualMachineClass(hardware, policies)
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory100, &resourceSpecWithMemory200),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu1000, &resourceSpecWithCpu2000),
	)

	DescribeTable("should validate with reservation and limit equal",
		func(validateFunc ValidateFunc, requestAndLimit *VirtualMachineClassResourceSpec) {
			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*requestAndLimit, *requestAndLimit),
				"",
			}
			vmClass := aVirtualMachineClass(hardware, policies)
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory100),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu1000),
	)

	DescribeTable("should fail if reservation larger than limit",
		func(validateFunc ValidateFunc, request *VirtualMachineClassResourceSpec, limit *VirtualMachineClassResourceSpec, expectedErr *field.Error) {

			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*request, *limit),
				"",
			}

			vmClass := aVirtualMachineClass(hardware, policies)
			result := validateFunc(vmClass)

			Expect(result).Should(HaveLen(1))
			Expect(result[0]).Should(Equal(expectedErr))
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory200, &resourceSpecWithMemory100,
			field.Invalid(field.NewPath("spec", "requests"), resourceSpecWithMemory200.Memory.Value(),
				"Memory Limits must not be smaller than Memory Requests")),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu2000, &resourceSpecWithCpu1000,
			field.Invalid(field.NewPath("spec", "requests"), resourceSpecWithCpu2000.Cpu.Value(),
				"CPU Limits must not be smaller than CPU Requests")),
	)
})
