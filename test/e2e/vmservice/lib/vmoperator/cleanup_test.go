// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmoperator_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

func TestCleanup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VM creation and cleanup")
}

type testClusterProxy struct {
	framework.ClusterProxyInterface
	client ctrlclient.Client
}

func (p testClusterProxy) GetClient() ctrlclient.Client { return p.client }
func (p testClusterProxy) GetKubeconfigPath() string    { return "unused.kubeconfig" }

var _ = Describe("CreateVirtualMachine", func() {
	DescribeTable("registers cleanup using the created object's identity", func(skipCleanup bool) {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		proxy := testClusterProxy{client: client}
		vm := &vmopv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{
			Namespace: "tenant-ns", Name: "test-vm",
		}}
		key := ctrlclient.ObjectKeyFromObject(vm)

		// Registered first so this runs after the helper's cleanup node.
		DeferCleanup(func(ctx context.Context) {
			err := client.Get(ctx, key, &vmopv1.VirtualMachine{})
			if skipCleanup {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "VM should be deleted by deferred cleanup")
			}
		})

		vmoperator.CreateVirtualMachine(ctx, &config.E2EConfig{}, proxy, vm, skipCleanup)
		Expect(client.Get(ctx, key, &vmopv1.VirtualMachine{})).To(Succeed())

		// Callers may reuse their object after creation; cleanup must retain
		// the original name and namespace.
		vm.Name = "another-vm"
		vm.Namespace = "another-namespace"
	},
		Entry("deletes the created VM", false),
		Entry("preserves the VM when cleanup is skipped", true),
	)
})
