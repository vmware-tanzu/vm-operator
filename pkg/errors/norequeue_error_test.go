// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package errors_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"

	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
)

var _ = Describe("NoRequeueError", func() {

	DescribeTable("Error",
		func(e error, expErr string) {
			Expect(e).To(MatchError(expErr))
		},

		Entry(
			"no message",
			pkgerr.NoRequeueError{},
			"no requeue",
		),

		Entry(
			"with message",
			pkgerr.NoRequeueError{Message: "hi"},
			"hi",
		),
	)

	Describe("AggregateOrNoRequeue", func() {
		Context("when given empty error slice", func() {
			It("should return nil", func() {
				result := pkgerr.AggregateOrNoRequeue([]error{})
				Expect(result).To(BeNil())
			})
		})

		Context("when given nil error slice", func() {
			It("should return nil", func() {
				result := pkgerr.AggregateOrNoRequeue(nil)
				Expect(result).To(BeNil())
			})
		})

		Context("when all errors are NoRequeueError", func() {
			It("should return a NoRequeueError with aggregated message", func() {
				errs := []error{
					pkgerr.NoRequeueError{Message: "first error"},
					pkgerr.NoRequeueError{Message: "second error"},
					pkgerr.NoRequeueError{Message: "third error"},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())

				var noRequeueErr pkgerr.NoRequeueError
				Expect(result).To(BeAssignableToTypeOf(noRequeueErr))
				Expect(result.Error()).To(ContainSubstring("first error"))
				Expect(result.Error()).To(ContainSubstring("second error"))
				Expect(result.Error()).To(ContainSubstring("third error"))
			})

			It("should handle NoRequeueError with empty messages", func() {
				errs := []error{
					pkgerr.NoRequeueError{},
					pkgerr.NoRequeueError{Message: "has message"},
					pkgerr.NoRequeueError{},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(result.Error()).To(ContainSubstring("has message"))
			})

			It("should handle NoRequeueError with DoNotErr flag", func() {
				errs := []error{
					pkgerr.NoRequeueError{Message: "error1", DoNotErr: true},
					pkgerr.NoRequeueError{Message: "error2", DoNotErr: false},
					pkgerr.NoRequeueError{Message: "error3", DoNotErr: true},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(result.Error()).To(ContainSubstring("error1"))
				Expect(result.Error()).To(ContainSubstring("error2"))
				Expect(result.Error()).To(ContainSubstring("error3"))
			})

			It("should set DoNotErr to false when any error has DoNotErr=false", func() {
				errs := []error{
					pkgerr.NoRequeueError{Message: "error1", DoNotErr: true},
					pkgerr.NoRequeueError{Message: "error2", DoNotErr: false},
					pkgerr.NoRequeueError{Message: "error3", DoNotErr: true},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(pkgerr.IsNoRequeueNoError(result)).To(BeFalse())
			})

			It("should set DoNotErr to true when all errors have DoNotErr=true", func() {
				errs := []error{
					pkgerr.NoRequeueError{Message: "error1", DoNotErr: true},
					pkgerr.NoRequeueError{Message: "error2", DoNotErr: true},
					pkgerr.NoRequeueError{Message: "error3", DoNotErr: true},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(pkgerr.IsNoRequeueNoError(result)).To(BeTrue())
			})
		})

		Context("when some errors are not NoRequeueError", func() {
			It("should return an aggregate error", func() {
				errs := []error{
					pkgerr.NoRequeueError{Message: "no requeue error"},
					errors.New("regular error"),
					pkgerr.NoRequeueError{Message: "another no requeue error"},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeFalse())
				Expect(result.Error()).To(ContainSubstring("no requeue error"))
				Expect(result.Error()).To(ContainSubstring("regular error"))
				Expect(result.Error()).To(ContainSubstring("another no requeue error"))
			})
		})

		Context("when given nested errors", func() {
			It("should handle nested NoRequeueError correctly", func() {
				nestedErr := pkgerr.NoRequeueError{Message: "nested no requeue"}
				wrappedErr := fmt.Errorf("wrapped: %w", nestedErr)

				errs := []error{
					wrappedErr,
					pkgerr.NoRequeueError{Message: "direct no requeue"},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(result.Error()).To(ContainSubstring("nested no requeue"))
				Expect(result.Error()).To(ContainSubstring("direct no requeue"))
			})

			It("should handle nested NoRequeueError with DoNotErr flags", func() {
				nestedErr := pkgerr.NoRequeueError{Message: "nested no requeue", DoNotErr: false}
				wrappedErr := fmt.Errorf("wrapped: %w", nestedErr)

				errs := []error{
					wrappedErr,
					pkgerr.NoRequeueError{Message: "direct no requeue", DoNotErr: true},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(pkgerr.IsNoRequeueNoError(result)).To(BeFalse()) // Should be false because nested error has DoNotErr=false
				Expect(result.Error()).To(ContainSubstring("nested no requeue"))
				Expect(result.Error()).To(ContainSubstring("direct no requeue"))
			})

			It("should handle deeply nested NoRequeueError", func() {
				deepNestedErr := pkgerr.NoRequeueError{Message: "deep nested", DoNotErr: true}
				midNestedErr := fmt.Errorf("mid: %w", deepNestedErr)
				outerErr := fmt.Errorf("outer: %w", midNestedErr)

				errs := []error{
					outerErr,
					pkgerr.NoRequeueError{Message: "direct no requeue", DoNotErr: true},
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
				Expect(pkgerr.IsNoRequeueNoError(result)).To(BeTrue()) // Should be true because all have DoNotErr=true
				Expect(result.Error()).To(ContainSubstring("deep nested"))
				Expect(result.Error()).To(ContainSubstring("direct no requeue"))
			})

			It("should flatten nested aggregate errors properly", func() {
				errs := []error{
					apierrorsutil.NewAggregate([]error{
						apierrorsutil.NewAggregate([]error{
							pkgerr.NoRequeueError{Message: "nested no requeue"},
						}),
					}),
				}

				result := pkgerr.AggregateOrNoRequeue(errs)
				Expect(result).ToNot(BeNil())
				Expect(pkgerr.IsNoRequeueError(result)).To(BeTrue())
			})
		})
	})
})
