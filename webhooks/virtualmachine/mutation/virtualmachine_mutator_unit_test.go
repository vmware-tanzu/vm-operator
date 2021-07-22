// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func uniTests() {
	Describe("Invoking Mutate", unitTestsMutating)
}

type unitMutationWebhookContext struct {
	builder.UnitTestContextForMutatingWebhook
	vm *vmopv1.VirtualMachine
}

func newUnitTestContextForMutatingWebhook() *unitMutationWebhookContext {
	vm := builder.DummyVirtualMachine()
	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	return &unitMutationWebhookContext{
		UnitTestContextForMutatingWebhook: *suite.NewUnitTestContextForMutatingWebhook(obj),
		vm:                                vm,
	}
}

func unitTestsMutating() {
	var (
		ctx *unitMutationWebhookContext
	)

	type mutateArgs struct {
		isNoAvailabilityZones       bool
		isWCPFaultDomainsFSSEnabled bool
		isInvalidAvailabilityZone   bool
		isEmptyAvailabilityZone     bool
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
	})
	AfterEach(func() {
		ctx = nil
	})

	mutate := func(args mutateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		// Please note this prevents the unit tests from running safely in
		// parallel.
		if args.isWCPFaultDomainsFSSEnabled {
			os.Setenv(lib.WcpFaultDomainsFSS, lib.TrueString)
		} else {
			os.Setenv(lib.WcpFaultDomainsFSS, "")
		}

		if args.isNoAvailabilityZones {
			// Delete the dummy AZ.
			Expect(ctx.Client.Delete(ctx, builder.DummyAvailabilityZone())).To(Succeed())
		}

		if args.isEmptyAvailabilityZone {
			delete(ctx.vm.Labels, topology.KubernetesTopologyZoneLabelKey)
		} else if args.isInvalidAvailabilityZone {
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = "invalid"
		} else {
			zoneName := builder.DummyAvailabilityZoneName
			if !lib.IsWcpFaultDomainsFSSEnabled() {
				zoneName = topology.DefaultAvailabilityZoneName
			}
			ctx.vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.Mutate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	Describe("VirtualMachineMutator should admit updates when object is under deletion", func() {
		Context("when update request comes in while deletion in progress ", func() {
			It("should admit update operation", func() {
				t := metav1.Now()
				ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
				response := ctx.Mutate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	DescribeTable("Mutate allow/deny permutations", mutate,
		Entry("should allow when VM specifies no availability zone, there are availability zones, and WCP FaultDomains FSS is disabled", mutateArgs{isEmptyAvailabilityZone: true}, true, nil, nil),
		Entry("should allow when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", mutateArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true}, true, nil, nil),

		Entry("should allow when VM specifies valid availability zone, there are availability zones, and WCP FaultDomains FSS is enabled", mutateArgs{isWCPFaultDomainsFSSEnabled: true}, true, nil, nil),
		Entry("should allow when VM specifies valid availability zone, there are no availability zones, and WCP FaultDomains FSS is disabled", mutateArgs{isNoAvailabilityZones: true}, true, nil, nil),

		Entry("should deny when VM specifies no availability zone, there are no availability zones, and WCP FaultDomains FSS is enabled", mutateArgs{isEmptyAvailabilityZone: true, isNoAvailabilityZones: true, isWCPFaultDomainsFSSEnabled: true}, false, nil, nil),
	)

	Describe("Default placement strategy", func() {

		var (
			numberOfAvailabilityZones   int
			isWCPFaultDomainsFSSEnabled bool
		)

		BeforeEach(func() {
			// Delete all AZs.
			Expect(
				ctx.Client.Delete(
					ctx,
					builder.DummyAvailabilityZone(),
				),
			).To(Succeed())
		})

		AfterEach(func() {
			numberOfAvailabilityZones = 0
			isWCPFaultDomainsFSSEnabled = false
		})

		JustBeforeEach(func() {
			// Please note this prevents the unit tests from running safely in
			// parallel.
			if isWCPFaultDomainsFSSEnabled {
				os.Setenv(lib.WcpFaultDomainsFSS, lib.TrueString)
			} else {
				os.Setenv(lib.WcpFaultDomainsFSS, "")
			}

			// Create the dummy AZs.
			for i := 0; i < numberOfAvailabilityZones; i++ {
				az := builder.DummyAvailabilityZone()
				az.Name = fmt.Sprintf("az-%d", i)
				Expect(ctx.Client.Create(ctx, az)).To(Succeed())
			}
		})

		mutate := func(expectedZoneName string) {

			// Convert the VM to unstructured data as required by the webhook
			// context.
			var err error
			ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
			Expect(err).ToNot(HaveOccurred())

			// Issue the call to the mutation webhook and expect it to succeed.
			response := ctx.Mutate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())

			// If no zone was expected then there should be no patches.
			if expectedZoneName == "" {
				Expect(response.PatchType).To(BeNil())
				Expect(response.Patches).To(HaveLen(0))
				return
			}

			// The mutation webhook should result in a single JSON patch that
			// adds a label, topology.kubernetes.io/zone, to the VM. The
			// following assertions validate that patch.
			Expect(response.PatchType).ToNot(BeNil())
			Expect(*response.PatchType).To(Equal(admissionv1.PatchTypeJSONPatch))
			Expect(response.Patches).To(HaveLen(1))
			zonePatch := response.Patches[0]
			Expect(zonePatch.Operation).To(Equal("add"))
			Expect(zonePatch.Path).To(Equal("/metadata/labels"))
			Expect(zonePatch.Value).ToNot(BeNil())
			Expect(zonePatch.Value).To(HaveKeyWithValue(
				topology.KubernetesTopologyZoneLabelKey, expectedZoneName))
		}

		Context("WCP FaultDomains FSS enabled", func() {
			BeforeEach(func() {
				isWCPFaultDomainsFSSEnabled = true
			})
			Context("One availability zone", func() {
				BeforeEach(func() {
					numberOfAvailabilityZones = 1
				})
				Context("Submit five VirtualMachines sans label "+
					topology.KubernetesTopologyZoneLabelKey, func() {

					It("Should add label "+
						topology.KubernetesTopologyZoneLabelKey+
						" to each VM, with each VM in the same zone", func() {

						mutate("az-0")
						mutate("az-0")
						mutate("az-0")
						mutate("az-0")
						mutate("az-0")
					})
				})
			})
			Context("Three availability zones", func() {
				BeforeEach(func() {
					numberOfAvailabilityZones = 3
				})
				Context("Submit five VirtualMachines sans label "+
					topology.KubernetesTopologyZoneLabelKey, func() {

					It("Should add label "+
						topology.KubernetesTopologyZoneLabelKey+
						" to each VM, with each VM in a different zone", func() {

						// Starts at az-1 because the previous test incremented
						// the index.
						mutate("az-1")
						mutate("az-2")

						// The round-robin strategy resets because there are
						// only three AZs.
						mutate("az-0")
						mutate("az-1")
						mutate("az-2")
					})
				})
			})
		})

		Context("WCP FaultDomains FSS disabled", func() {
			Context("Submit five VirtualMachines sans label "+
				topology.KubernetesTopologyZoneLabelKey, func() {
				It("Should not add a zone", func() {
					mutate("")
					mutate("")
					mutate("")
					mutate("")
					mutate("")
				})
			})
		})
	})
}
