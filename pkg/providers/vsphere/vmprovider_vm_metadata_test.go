// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		zoneName string
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}

		if testConfig.WithContentLibrary {
			Expect(ctx.Client.Get(
				ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
				clusterVMI1)).To(Succeed())
		} else {
			vsphere.SkipVMImageCLProviderCheck = true
			clusterVMI1 = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
			Expect(ctx.Client.Create(ctx, clusterVMI1)).To(Succeed())
			conditions.MarkTrue(clusterVMI1, vmopv1.ReadyConditionType)
			Expect(ctx.Client.Status().Update(ctx, clusterVMI1)).To(Succeed())
		}

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName = ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		if vm != nil &&
			!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		vmClass = nil
		vm = nil

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
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
