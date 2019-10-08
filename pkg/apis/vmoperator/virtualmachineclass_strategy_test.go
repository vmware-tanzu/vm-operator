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

func aVirtualMachineClassResources(requests VirtualMachineResourceSpec, limits VirtualMachineResourceSpec) VirtualMachineClassResources {
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

		vmClass = VirtualMachineClass{}

		hardware      = VirtualMachineClassHardware{}
		hardwareSmall = VirtualMachineClassHardware{
			Cpus:   1,
			Memory: resource.MustParse("1Gi"),
		}
		hardwareLarge = VirtualMachineClassHardware{
			Cpus:   2,
			Memory: resource.MustParse("2Gi"),
		}

		resourceSpecWithMemory100 = VirtualMachineResourceSpec{
			Memory: resource.MustParse("100Mi"),
		}
		resourceSpecWithMemory200 = VirtualMachineResourceSpec{
			Memory: resource.MustParse("200Mi"),
		}
		resourceSpecWithCpu1000 = VirtualMachineResourceSpec{
			Cpu: resource.MustParse("1000Mi"),
		}
		resourceSpecWithCpu2000 = VirtualMachineResourceSpec{
			Cpu: resource.MustParse("2000Mi"),
		}

		resourceSpecWithCpuMemSmall = VirtualMachineResourceSpec{
			Cpu:    resource.MustParse("1000Mi"),
			Memory: resource.MustParse("100Mi"),
		}
		resourceSpecWithCpuMemLarge = VirtualMachineResourceSpec{
			Cpu:    resource.MustParse("2000Mi"),
			Memory: resource.MustParse("200Mi"),
		}
	)

	type ValidateFunc func(VirtualMachineClass) field.ErrorList
	type ValidateUpdateFunc func(VirtualMachineClass, VirtualMachineClass) field.ErrorList

	BeforeEach(func() {
		requests := VirtualMachineResourceSpec{}
		limits := VirtualMachineResourceSpec{}
		policies = VirtualMachineClassPolicies{
			aVirtualMachineClassResources(requests, limits),
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
		func(validateFunc ValidateFunc, request *VirtualMachineResourceSpec, limit *VirtualMachineResourceSpec) {
			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*request, *limit),
			}

			vmClass := aVirtualMachineClass(hardware, policies)
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory100, &resourceSpecWithMemory200),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu1000, &resourceSpecWithCpu2000),
	)

	DescribeTable("should validate with reservation and limit equal",
		func(validateFunc ValidateFunc, requestAndLimit *VirtualMachineResourceSpec) {
			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*requestAndLimit, *requestAndLimit),
			}
			vmClass := aVirtualMachineClass(hardware, policies)
			Expect(validateFunc(vmClass)).Should(BeEmpty())
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory100),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu1000),
	)

	DescribeTable("should fail if reservation larger than limit",
		func(validateFunc ValidateFunc, request *VirtualMachineResourceSpec, limit *VirtualMachineResourceSpec, expectedErr *field.Error) {

			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*request, *limit),
			}

			vmClass := aVirtualMachineClass(hardware, policies)
			result := validateFunc(vmClass)

			Expect(result).Should(HaveLen(1))
			Expect(result[0]).Should(Equal(expectedErr))
		},
		Entry("for memory resources", validateMemory, &resourceSpecWithMemory200, &resourceSpecWithMemory100,
			field.Invalid(field.NewPath("spec", "policies", "resources", "requests", "memory"), resourceSpecWithMemory200.Memory.Value(),
				"Memory request should not be larger than Memory limit")),
		Entry("for CPU resources", validateCPU, &resourceSpecWithCpu2000, &resourceSpecWithCpu1000,
			field.Invalid(field.NewPath("spec", "policies", "resources", "requests", "cpu"), resourceSpecWithCpu2000.Cpu.Value(),
				"CPU request should not be larger than Memory limit")),
	)
	DescribeTable("should pass if old and new vm classes are the same",
		func(validateFunc ValidateUpdateFunc, request *VirtualMachineResourceSpec, limit *VirtualMachineResourceSpec) {

			policies := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*request, *limit),
			}

			vmClass1 := aVirtualMachineClass(hardware, policies)
			vmClass2 := aVirtualMachineClass(hardware, policies)

			Expect(validateFunc(vmClass1, vmClass2)).Should(BeEmpty())
		},
		Entry("For changes", validateClassUpdate, &resourceSpecWithCpuMemSmall, &resourceSpecWithCpuMemLarge),
	)
	DescribeTable("should fail if old and new vm classes have changed",
		func(validateFunc ValidateUpdateFunc, res1 *VirtualMachineResourceSpec, res2 *VirtualMachineResourceSpec) {

			policies1 := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*res1, *res1),
			}
			policies2 := VirtualMachineClassPolicies{
				aVirtualMachineClassResources(*res2, *res2),
			}

			vmClass1 := aVirtualMachineClass(hardwareSmall, policies1)
			vmClass2 := aVirtualMachineClass(hardwareLarge, policies2)

			result := validateFunc(vmClass1, vmClass2)
			Expect(result).Should(Not(BeEmpty()))
		},
		Entry("For changes", validateClassUpdate, &resourceSpecWithCpuMemSmall, &resourceSpecWithCpuMemLarge),
	)
})
