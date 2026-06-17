// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package configpolicy

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2eConfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const vmConfigOptionsSpecName = "vmconfigoptions"

var vmConfigOptionsGVK = schema.GroupVersionKind{
	Group:   "vim.vmware.com",
	Version: "v1alpha1",
	Kind:    "VirtualMachineConfigOptions",
}

// VMConfigOptionsSpecInput is the input for the VirtualMachineConfigOptions E2E test spec.
type VMConfigOptionsSpecInput struct {
	ClusterProxy wcpframework.WCPClusterProxyInterface
	Config       *e2eConfig.E2EConfig
}

// VMConfigOptionsSpec validates the VirtualMachineConfigOptions admission webhook.
func VMConfigOptionsSpec(ctx context.Context, inputGetter func() VMConfigOptionsSpecInput) {
	var (
		input        VMConfigOptionsSpecInput
		clusterProxy *common.VMServiceClusterProxy
		svClient     ctrlclient.Client
		vmcoName     string
	)

	BeforeEach(func() {
		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", vmConfigOptionsSpecName)
		Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", vmConfigOptionsSpecName)

		clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
		svClient = clusterProxy.GetClient()
		vmcoName = "e2e-vmco-" + capiutil.RandomString(6)

		skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
		skipper.SkipUnlessSupervisorCapabilityEnabled(ctx, clusterProxy, consts.VirtualMachineConfigPolicyCapabilityName)
	})

	AfterEach(func() {
		vmco := newVMConfigOptionsUnstructured(vmcoName, "vmx-21")
		_ = svClient.Delete(ctx, vmco)
	})

	It("should allow creating a VirtualMachineConfigOptions with a valid hardwareVersion",
		Label("smoke", "core-functional"),
		func() {
			framework.Byf("Creating VirtualMachineConfigOptions %q with hardwareVersion vmx-21", vmcoName)

			vmco := newVMConfigOptionsUnstructured(vmcoName, "vmx-21")
			Expect(svClient.Create(ctx, vmco)).To(Succeed())
		},
	)

	It("should deny creating a VirtualMachineConfigOptions with an invalid hardwareVersion",
		Label("core-functional"),
		func() {
			framework.Byf("Attempting to create VirtualMachineConfigOptions %q with invalid hardwareVersion", vmcoName)

			vmco := newVMConfigOptionsUnstructured(vmcoName, "not-valid")
			err := svClient.Create(ctx, vmco)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hardwareVersion"))
		},
	)

	It("should deny updating the hardwareVersion of an existing VirtualMachineConfigOptions",
		Label("core-functional"),
		func() {
			framework.Byf("Creating VirtualMachineConfigOptions %q with hardwareVersion vmx-21", vmcoName)

			vmco := newVMConfigOptionsUnstructured(vmcoName, "vmx-21")
			Expect(svClient.Create(ctx, vmco)).To(Succeed())

			framework.Byf("Attempting to update hardwareVersion of %q to vmx-22", vmcoName)

			fetched := &unstructured.Unstructured{}
			fetched.SetGroupVersionKind(vmConfigOptionsGVK)
			Expect(svClient.Get(ctx, ctrlclient.ObjectKey{Name: vmcoName}, fetched)).To(Succeed())

			err := unstructured.SetNestedField(fetched.Object, "vmx-22", "spec", "hardwareVersion")
			Expect(err).ToNot(HaveOccurred())

			err = svClient.Update(ctx, fetched)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
		},
	)
}

// newVMConfigOptionsUnstructured returns a cluster-scoped VirtualMachineConfigOptions
// with the given hardwareVersion.
func newVMConfigOptionsUnstructured(name, hardwareVersion string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(vmConfigOptionsGVK)
	obj.SetName(name)
	err := unstructured.SetNestedField(obj.Object, hardwareVersion, "spec", "hardwareVersion")
	Expect(err).ToNot(HaveOccurred())

	return obj
}
