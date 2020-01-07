// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func newVirtualMachineClassResources(requests VirtualMachineResourceSpec, limits VirtualMachineResourceSpec) VirtualMachineClassResources {
	return VirtualMachineClassResources{
		Requests: requests,
		Limits:   limits,
	}
}

func newVirtualMachineClass(hardware VirtualMachineClassHardware, policies VirtualMachineClassPolicies) VirtualMachineClass {
	return VirtualMachineClass{
		Spec: VirtualMachineClassSpec{
			Hardware: hardware,
			Policies: policies,
		},
	}
}

var _ = Describe("VirtualMachineClass Validation", func() {
	var (
		vmMachineClassStrategy       = VirtualMachineClassStrategy{}
		vmMachineClassStatusStrategy = VirtualMachineClassStatusStrategy{}

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

	DescribeTable("should validate resources of the VirtualMachineClass",
		func(request *VirtualMachineResourceSpec, limit *VirtualMachineResourceSpec, expectedErr *field.Error) {
			vmClassResources := VirtualMachineClassResources{}
			if request != nil {
				vmClassResources.Requests = *request
			}
			if limit != nil {
				vmClassResources.Limits = *limit
			}
			policies := VirtualMachineClassPolicies{vmClassResources}
			vmClass := newVirtualMachineClass(hardware, policies)
			result := vmMachineClassStrategy.Validate(context.TODO(), &vmClass)
			if expectedErr == nil {
				Expect(result).Should(BeEmpty())
			} else {
				Expect(result).Should(HaveLen(1))
				Expect(result[0]).Should(Equal(expectedErr))
			}
		},
		Entry("empty", nil, nil, nil),
		Entry("no reservation or limit", &VirtualMachineResourceSpec{}, &VirtualMachineResourceSpec{}, nil),
		Entry("reservation < limit for CPU", &resourceSpecWithCpu1000, &resourceSpecWithCpu2000, nil),
		Entry("reservation < limit for memory", &resourceSpecWithMemory100, &resourceSpecWithMemory200, nil),
		Entry("reservation == limit for CPU", &resourceSpecWithCpu1000, &resourceSpecWithCpu1000, nil),
		Entry("reservation == limit for memory", &resourceSpecWithMemory100, &resourceSpecWithMemory100, nil),
		Entry("reservation > limit for CPU", &resourceSpecWithCpu2000, &resourceSpecWithCpu1000,
			field.Invalid(field.NewPath("spec", "policies", "resources", "requests", "cpu"), resourceSpecWithCpu2000.Cpu.Value(),
				"CPU request should not be larger than CPU limit")),
		Entry("reservation> limit for memory", &resourceSpecWithMemory200, &resourceSpecWithMemory100,
			field.Invalid(field.NewPath("spec", "policies", "resources", "requests", "memory"), resourceSpecWithMemory200.Memory.Value(),
				"Memory request should not be larger than Memory limit")),
	)

	Context("When old and new vm classes are the same", func() {
		It("should validate class update successfully", func() {
			policies := VirtualMachineClassPolicies{
				newVirtualMachineClassResources(resourceSpecWithCpu1000, resourceSpecWithCpu1000),
			}

			vmClass1 := newVirtualMachineClass(hardware, policies)
			vmClass2 := newVirtualMachineClass(hardware, policies)

			Expect(vmMachineClassStrategy.ValidateUpdate(context.TODO(), &vmClass1, &vmClass2)).Should(BeEmpty())
		})
	})

	Context("When old and new vm classes have changed", func() {
		It("should fail to validate class update", func() {

			policies1 := VirtualMachineClassPolicies{
				newVirtualMachineClassResources(resourceSpecWithCpuMemSmall, resourceSpecWithCpuMemSmall),
			}
			policies2 := VirtualMachineClassPolicies{
				newVirtualMachineClassResources(resourceSpecWithCpuMemLarge, resourceSpecWithCpuMemLarge),
			}

			vmClass1 := newVirtualMachineClass(hardwareSmall, policies1)
			vmClass2 := newVirtualMachineClass(hardwareLarge, policies2)

			result := vmMachineClassStrategy.ValidateUpdate(context.TODO(), &vmClass1, &vmClass2)
			Expect(result).Should(HaveLen(1))
			Expect(result[0]).Should(Equal(field.Forbidden(field.NewPath("spec"), "VM Classes are immutable")))

		})
	})

	Context("When always", func() {
		It("virtual machine classes are cluster-scoped", func() {
			Expect(vmMachineClassStrategy.NamespaceScoped()).To(BeFalse())
		})
		It("virtual machine classes status are cluster-scoped", func() {
			Expect(vmMachineClassStatusStrategy.NamespaceScoped()).To(BeFalse())
		})
	})
})
