// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/vim25/mo"
)

func vmMetadataTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		pinVMToFirstZone(ctx, vm)
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	Context("ExtraConfig Transport", func() {
		var ec map[string]interface{}

		JustBeforeEach(func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "md-configmap-",
					Namespace:    vm.Namespace,
				},
				Data: map[string]string{
					"foo.bar":       "should-be-ignored",
					"guestinfo.Foo": "foo",
				},
			}
			Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())

			/*
				vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					ConfigMapName: configMap.Name,
					Transport:     vmopv1.VirtualMachineMetadataExtraConfigTransport,
				}
			*/
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			ec = map[string]interface{}{}
			for _, option := range o.Config.ExtraConfig {
				if val := option.GetOptionValue(); val != nil {
					ec[val.Key] = val.Value.(string)
				}
			}
		})

		AfterEach(func() {
			ec = nil
		})

		// TODO: As is we can't really honor "guestinfo.*" prefix
		XIt("Metadata data is included in ExtraConfig", func() {
			Expect(ec).ToNot(HaveKey("foo.bar"))
			Expect(ec).To(HaveKeyWithValue("guestinfo.Foo", "foo"))

			By("Should include default keys and values", func() {
				Expect(ec).To(HaveKeyWithValue("disk.enableUUID", "TRUE"))
				Expect(ec).To(HaveKeyWithValue("vmware.tools.gosc.ignoretoolscheck", "TRUE"))
			})
		})

		Context("JSON_EXTRA_CONFIG is specified", func() {
			BeforeEach(func() {
				b, err := json.Marshal(
					struct {
						Foo string
						Bar string
					}{
						Foo: "f00",
						Bar: "42",
					},
				)
				Expect(err).ToNot(HaveOccurred())
				testConfig.WithJSONExtraConfig = string(b)
			})

			It("Global config is included in ExtraConfig", func() {
				Expect(ec).To(HaveKeyWithValue("Foo", "f00"))
				Expect(ec).To(HaveKeyWithValue("Bar", "42"))
			})
		})
	})
}
