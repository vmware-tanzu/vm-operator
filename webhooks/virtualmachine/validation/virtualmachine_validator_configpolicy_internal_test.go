// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// This file is package validation (not validation_test) -- an internal,
// white-box test -- because toFieldError/toFieldErrors are unexported and
// the scenario under test (a violation wrapped by an intermediate error)
// cannot be constructed through the public webhook entry points.
package validation

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/configpolicy"
)

var _ = Describe(
	"toFieldError",
	Label(testlabels.Validation, testlabels.Webhook),
	func() {
		var vm *vmopv1.VirtualMachine

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{}
		})

		When("v is a plain, unwrapped violation", func() {
			It("returns the matching field.Error", func() {
				v := &configpolicy.ErrIOMMUViolation{}
				fieldErr := toFieldError(vm, v)
				Expect(fieldErr).ToNot(BeNil())
				Expect(fieldErr.Field).To(Equal("spec.cpuAdvanced.iommuEnabled"))
			})
		})

		When("v wraps a violation via fmt.Errorf's %w", func() {
			It("still finds it with errors.As and returns the matching field", func() {
				violation := &configpolicy.ErrHugePagesViolation{}
				wrapped := fmt.Errorf("while reconfiguring: %w", violation)

				fieldErr := toFieldError(vm, wrapped)
				Expect(fieldErr).ToNot(BeNil())
				Expect(fieldErr.Field).To(Equal("spec.advanced.hugePages1GEnabled"))
			})
		})

		When("v wraps a violation two levels deep", func() {
			It("still finds it with errors.As", func() {
				violation := &configpolicy.ErrMemoryLockedToMaxViolation{}
				inner := fmt.Errorf("inner: %w", violation)
				outer := fmt.Errorf("outer: %w", inner)

				fieldErr := toFieldError(vm, outer)
				Expect(fieldErr).ToNot(BeNil())
				Expect(fieldErr.Field).To(
					Equal("spec.memoryAdvanced.reservationLockedToMax"))
			})
		})

		When("a violation's own Err field wraps a cause", func() {
			It("still finds it with errors.As", func() {
				cause := errors.New("some underlying cause")
				v := &configpolicy.ErrCPUCoresViolation{Got: 16, Err: cause}

				fieldErr := toFieldError(vm, v)
				Expect(fieldErr).ToNot(BeNil())
				Expect(fieldErr.Field).To(Equal("spec.resources.size.cpu"))
				Expect(fieldErr.Detail).To(ContainSubstring("some underlying cause"))
			})
		})

		When("v does not wrap any recognized violation type", func() {
			It("returns nil", func() {
				Expect(toFieldError(vm, errors.New("boom"))).To(BeNil())
			})
		})

		When("v is an ErrExtraConfigViolation wrapped by another error", func() {
			It("still locates the key's index in vm.Spec.Advanced.ExtraConfig", func() {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ExtraConfig: []vmopv1common.KeyValuePair{
						{Key: "guestinfo.a", Value: "1"},
						{Key: "guestinfo.b", Value: "2"},
					},
				}
				violation := &configpolicy.ErrExtraConfigViolation{
					Key: "guestinfo.b", Denied: true,
				}
				wrapped := fmt.Errorf("wrapped: %w", violation)

				fieldErr := toFieldError(vm, wrapped)
				Expect(fieldErr).ToNot(BeNil())
				Expect(fieldErr.Field).To(Equal("spec.advanced.extraConfig[1].key"))
			})
		})
	},
)

var _ = Describe(
	"toFieldErrors",
	Label(testlabels.Validation, testlabels.Webhook),
	func() {
		It("returns an empty list when violation is nil", func() {
			Expect(toFieldErrors(&vmopv1.VirtualMachine{}, nil)).To(BeEmpty())
		})

		It("handles a mix of wrapped and unwrapped violations, joined", func() {
			vm := &vmopv1.VirtualMachine{}

			unwrapped := &configpolicy.ErrIOMMUViolation{}
			wrapped := fmt.Errorf("wrapped: %w", &configpolicy.ErrHugePagesViolation{})
			joined := errors.Join(unwrapped, wrapped)

			allErrs := toFieldErrors(vm, joined)
			Expect(allErrs).To(HaveLen(2))

			var fields []string
			for _, fieldErr := range allErrs {
				fields = append(fields, fieldErr.Field)
			}
			Expect(fields).To(ConsistOf(
				"spec.cpuAdvanced.iommuEnabled",
				"spec.advanced.hugePages1GEnabled",
			))
		})

		It("drops an error that does not wrap any recognized violation type", func() {
			vm := &vmopv1.VirtualMachine{}

			recognized := &configpolicy.ErrIOMMUViolation{}
			unrecognized := errors.New("not a violation")
			joined := errors.Join(recognized, unrecognized)

			allErrs := toFieldErrors(vm, joined)
			Expect(allErrs).To(HaveLen(1))
			Expect(allErrs[0].Field).To(Equal("spec.cpuAdvanced.iommuEnabled"))
		})
	},
)
