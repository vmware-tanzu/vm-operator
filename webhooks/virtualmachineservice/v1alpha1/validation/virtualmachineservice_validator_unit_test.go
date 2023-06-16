// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking ValidateCreate", unitTestsValidateCreate)
	Describe("Invoking ValidateUpdate", unitTestsValidateUpdate)
	Describe("Invoking ValidateDelete", unitTestsValidateDelete)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vmService    *vmopv1.VirtualMachineService
	oldVMService *vmopv1.VirtualMachineService
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmService := builder.DummyVirtualMachineService()
	obj, err := builder.ToUnstructured(vmService)
	Expect(err).ToNot(HaveOccurred())

	var oldVMService *vmopv1.VirtualMachineService
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMService = vmService.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMService)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmService:                           vmService,
		oldVMService:                        oldVMService,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidDNSName        bool
		emptyType             bool
		invalidType           bool
		invalidPorts          bool
		invalidSelector       bool
		invalidClusterIP      bool
		invalidLBSourceRanges bool
		invalidExternalName   bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.invalidDNSName {
			// Name that doesn't conform to DNS Label format "[a-z0-9]([-a-z0-9]*[a-z0-9])?"
			ctx.vmService.Name = "MyVMService"
		}
		if args.emptyType {
			ctx.vmService.Spec.Type = ""
		}
		if args.invalidType {
			ctx.vmService.Spec.Type = "InvalidLB"
		}
		if args.invalidPorts {
			ctx.vmService.Spec.Ports = nil
		}
		if args.invalidSelector {
			ctx.vmService.Spec.Selector = map[string]string{"THIS_NOT_VALID!": "foo"}
		}
		if args.invalidClusterIP {
			ctx.vmService.Spec.ClusterIP = "100.1000.1.1"
		}
		if args.invalidLBSourceRanges {
			ctx.vmService.Spec.Type = vmopv1.VirtualMachineServiceTypeLoadBalancer
			ctx.vmService.Spec.LoadBalancerSourceRanges = []string{"10.1.1.1/42"}
		}
		if args.invalidExternalName {
			ctx.vmService.Spec.Type = vmopv1.VirtualMachineServiceTypeExternalName
			ctx.vmService.Spec.ExternalName = "InValid!"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmService)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should deny invalid name", createArgs{invalidDNSName: true}, false, "metadata.name: Invalid value: \"MyVMService\": a lowercase RFC 1123 label must consist of lower case alphanumeric character", nil),
		Entry("should deny no type", createArgs{emptyType: true}, false, "spec.type: Required value", nil),
		Entry("should deny invalid type", createArgs{invalidType: true}, false, "spec.type: Unsupported value: \"InvalidLB\":", nil),
		Entry("should deny invalid ports", createArgs{invalidPorts: true}, false, "spec.ports: Required value", nil),
		Entry("should deny invalid selector", createArgs{invalidSelector: true}, false, "spec.selector: Invalid value: \"THIS_NOT_VALID!\": name part must consist of alphanumeric characters", nil),
		Entry("should deny invalid ClusterIP", createArgs{invalidClusterIP: true}, false, "spec.clusterIP: Invalid value: \"100.1000.1.1\": must be a valid IP address", nil),
		Entry("should deny invalid LoadBalancerSourceRanges", createArgs{invalidLBSourceRanges: true}, false, "spec.loadBalancerSourceRanges: Invalid value: \"[10.1.1.1/42]", nil),
		Entry("should deny invalid ExternalName", createArgs{invalidExternalName: true}, false, "spec.externalName: Invalid value: \"InValid!\": a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters", nil),
	)

	validatePortCreate := func(expectedReason string, ports []vmopv1.VirtualMachineServicePort) {
		var err error

		ctx.vmService.Spec.Ports = ports

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmService)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		if expectedReason != "" {
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		} else {
			Expect(response.Allowed).To(BeTrue())
		}
	}

	DescribeTable("create service ports", validatePortCreate,
		Entry("should allow valid port", "",
			[]vmopv1.VirtualMachineServicePort{
				{
					Name:       "http",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 8080,
				},
			},
		),
		Entry("should deny invalid name", "spec.ports[0].name: Invalid value: \"INVALID\"",
			[]vmopv1.VirtualMachineServicePort{
				{
					Name: "INVALID",
				},
			},
		),
		Entry("should deny invalid port", "spec.ports[0].port: Invalid value: 100000:",
			[]vmopv1.VirtualMachineServicePort{
				{
					Port: 100000,
				},
			},
		),
		Entry("should deny invalid protocol", "spec.ports[0].protocol: Unsupported value: \"INVALID\"",
			[]vmopv1.VirtualMachineServicePort{
				{
					Protocol: "INVALID",
				},
			},
		),
		Entry("should deny invalid target port", "spec.ports[0].targetPort: Invalid value: 200000:",
			[]vmopv1.VirtualMachineServicePort{
				{
					TargetPort: 200000,
				},
			},
		),
		Entry("should deny duplicate names", "spec.ports[1].name: Duplicate value: \"port1\"",
			[]vmopv1.VirtualMachineServicePort{
				{
					Name:       "port1",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 8080,
				},
				{
					Name:       "port1",
					Protocol:   "TCP",
					Port:       433,
					TargetPort: 6443,
				},
			},
		),
		Entry("should deny duplicate protocol/port", "spec.ports[1]: Duplicate value: v1alpha1.VirtualMachineServicePort",
			[]vmopv1.VirtualMachineServicePort{
				{
					Name:       "port1",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 8080,
				},
				{
					Name:       "port2",
					Protocol:   "TCP",
					Port:       80,
					TargetPort: 8080,
				},
			},
		),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	type updateArgs struct {
		updateType      bool
		updateClusterIP bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		var err error

		if args.updateType {
			ctx.vmService.Spec.Type = vmopv1.VirtualMachineServiceTypeClusterIP
		}
		if args.updateClusterIP {
			ctx.vmService.Spec.ClusterIP = "9.9.9.9"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmService)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("update table", validateUpdate,
		Entry("should allow", updateArgs{}, true, nil, nil),
		Entry("should deny Type change", updateArgs{updateType: true}, false, "spec.type: Forbidden: field is immutable", nil),
		Entry("should deny ClusterIP change", updateArgs{updateClusterIP: true}, false, "spec.clusterIP: Forbidden: field is immutable", nil),
	)

	When("the update is performed while object deletion", func() {
		JustBeforeEach(func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	When("the delete is performed", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
