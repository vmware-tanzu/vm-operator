/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func genResources(requests VirtualMachineClassResourceSpec, limits VirtualMachineClassResourceSpec) VirtualMachineClassResources {
	return VirtualMachineClassResources{
		Requests: requests,
		Limits:   limits,
	}
}

func genClass(hardware VirtualMachineClassHardware, policies VirtualMachineClassPolicies) VirtualMachineClass {
	return VirtualMachineClass{
		Spec: VirtualMachineClassSpec{
			Hardware: hardware,
			Policies: policies,
		},
	}
}

var _ = Describe("VirtualMachineClass Validation", func() {

	Context("validateMemory", func() {
		It("empty class", func() {
			vmClass := VirtualMachineClass{}
			Expect(validateMemory(vmClass)).Should(BeEmpty())
		})

		It("no reservation or limit", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{}
			limits := VirtualMachineClassResourceSpec{}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateMemory(vmClass)).Should(BeEmpty())
		})

		It("limit larger than reservation", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("100Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("200Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateMemory(vmClass)).Should(BeEmpty())
		})

		It("reservation and limit equal", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("100Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("100Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateMemory(vmClass)).Should(BeEmpty())
		})

		It("reservation larger than limit", func() {

			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("2000Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Memory: resource.MustParse("1000Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			err := field.Invalid(field.NewPath("spec", "requests"), requests.Memory.Value(),
				"Memory Limits must be not be smaller than Memory Requests")

			vmClass := genClass(hardware, policies)
			Expect(validateMemory(vmClass)).ShouldNot(BeEmpty())
			Expect(validateMemory(vmClass)).Should(HaveLen(1))
			Expect(validateMemory(vmClass)[0]).Should(MatchError(err))
		})
	})

	Context("validateCPU", func() {
		It("empty class", func() {
			vmClass := VirtualMachineClass{}
			Expect(validateCPU(vmClass)).Should(BeEmpty())
		})

		It("no reservation or limit", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{}
			limits := VirtualMachineClassResourceSpec{}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateCPU(vmClass)).Should(BeEmpty())
		})

		It("limit larger than reservation", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("1000Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("2000Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateCPU(vmClass)).Should(BeEmpty())
		})

		It("reservation and limit equal", func() {
			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("1000Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("1000Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			vmClass := genClass(hardware, policies)
			Expect(validateCPU(vmClass)).Should(BeEmpty())
		})

		It("reservation larger than limit", func() {

			hardware := VirtualMachineClassHardware{}

			requests := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("2000Mi"),
			}
			limits := VirtualMachineClassResourceSpec{
				Cpu: resource.MustParse("1000Mi"),
			}
			policies := VirtualMachineClassPolicies{
				genResources(requests, limits),
				"",
			}

			err := field.Invalid(field.NewPath("spec", "requests"), requests.Cpu.Value(),
				"CPU Limits must be not be smaller than CPU Requests")

			vmClass := genClass(hardware, policies)
			Expect(validateCPU(vmClass)).ShouldNot(BeEmpty())
			Expect(validateCPU(vmClass)).Should(HaveLen(1))
			Expect(validateCPU(vmClass)[0]).Should(MatchError(err))
		})
	})

})
