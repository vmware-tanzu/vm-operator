// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("VirtualMachineService Validation", func() {
	var (
		vmServiceStrategy VirtualMachineServiceStrategy
		vmPort            VirtualMachineServicePort
	)

	BeforeEach(func() {
		vmServiceStrategy = VirtualMachineServiceStrategy{}
		vmPort = VirtualMachineServicePort{
			Name: "test-vms-port",
		}
	})

	Describe("When initialize", func() {
		It("it sets finalizer", func() {
			vmService := &VirtualMachineService{Spec: VirtualMachineServiceSpec{}}
			vmServiceStrategy.PrepareForCreate(context.Background(), vmService)
			Expect(vmService.Finalizers).Should(HaveLen(1))
			Expect(vmService.Finalizers[0]).Should(Equal("virtualmachineservice.vmoperator.vmware.com"))
		})
	})

	DescribeTable("should validate a VirtualMachineService",
		func(spec *VirtualMachineServiceSpec, expectedErrs *field.ErrorList) {
			vmService := VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-service-name",
					Annotations: nil,
				},
				Spec: *spec,
			}

			result := vmServiceStrategy.Validate(context.TODO(), &vmService)
			if expectedErrs == nil {
				Expect(result).Should(BeEmpty())
			} else {
				Expect(result).Should(Equal(*expectedErrs))
			}
		},
		Entry("with a valid spec", &VirtualMachineServiceSpec{
			Type:     "LoadBalancer",
			Ports:    []VirtualMachineServicePort{vmPort},
			Selector: map[string]string{"key1": "val1"},
		}, nil),
		Entry("empty", &VirtualMachineServiceSpec{}, &field.ErrorList{
			field.Required(field.NewPath("spec", "type"), ""),
			field.Required(field.NewPath("spec", "ports"), ""),
			field.Required(field.NewPath("spec", "selector"), ""),
		}),
		Entry("no selector", &VirtualMachineServiceSpec{
			Type:  "LoadBalancer",
			Ports: []VirtualMachineServicePort{vmPort},
		}, &field.ErrorList{
			field.Required(field.NewPath("spec", "selector"), ""),
		}),
		Entry("no type", &VirtualMachineServiceSpec{
			Type:     "",
			Ports:    []VirtualMachineServicePort{vmPort},
			Selector: map[string]string{"key1": "val1"},
		}, &field.ErrorList{
			field.Required(field.NewPath("spec", "type"), ""),
		}),
		Entry("no ports", &VirtualMachineServiceSpec{
			Type:     "LoadBalancer",
			Selector: map[string]string{"key1": "val1"},
		}, &field.ErrorList{
			field.Required(field.NewPath("spec", "ports"), ""),
		}),
	)
})
