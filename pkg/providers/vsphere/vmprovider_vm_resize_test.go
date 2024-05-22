// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmResizeTests() {

	var (
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
			WithVMResize:       true,
			WithNetworkEnv:     builder.NetworkEnvNamed,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	encodedConfigSpec := func(cs vimtypes.VirtualMachineConfigSpec) []byte {
		var w bytes.Buffer
		enc := vimtypes.NewJSONEncoder(&w)
		Expect(enc.Encode(cs)).To(Succeed())
		return w.Bytes()
	}

	createOrUpdateAndGetVcVM := func(
		ctx *builder.TestContextForVCSim,
		vm *vmopv1.VirtualMachine) (*object.VirtualMachine, error) {

		err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
		if err != nil {
			return nil, err
		}

		ExpectWithOffset(1, vm.Status.UniqueID).ToNot(BeEmpty())
		vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
		ExpectWithOffset(1, vcVM).ToNot(BeNil())
		return vcVM, nil
	}

	createVMClass := func(cs vimtypes.VirtualMachineConfigSpec, name ...string) *vmopv1.VirtualMachineClass {
		var class *vmopv1.VirtualMachineClass
		if len(name) == 1 {
			class = builder.DummyVirtualMachineClass(name[0])
		} else {
			class = builder.DummyVirtualMachineClassGenName()
		}
		class.Namespace = nsInfo.Namespace
		class.Spec.ConfigSpec = encodedConfigSpec(cs)
		class.Spec.Hardware.Cpus = int64(cs.NumCPUs)
		class.Spec.Hardware.Memory = resource.MustParse(fmt.Sprintf("%dMi", cs.MemoryMB))
		ExpectWithOffset(1, ctx.Client.Create(ctx, class)).To(Succeed())
		return class
	}

	Context("Resize VM", func() {

		var (
			vm         *vmopv1.VirtualMachine
			vmClass    *vmopv1.VirtualMachineClass
			configSpec vimtypes.VirtualMachineConfigSpec
		)

		BeforeEach(func() {
			vm = builder.DummyBasicVirtualMachine("test-vm", "")

			configSpec = vimtypes.VirtualMachineConfigSpec{}
			configSpec.NumCPUs = 1
			configSpec.MemoryMB = 512
		})

		JustBeforeEach(func() {
			vmClass = createVMClass(configSpec, "initial-class")

			clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, clusterVMImage)).To(Succeed())

			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = clusterVMImage.Name
			vm.Spec.Image.Kind = cvmiKind
			vm.Spec.Image.Name = clusterVMImage.Name
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			vm.Spec.StorageClass = ctx.StorageClassName

			_, err := createOrUpdateAndGetVcVM(ctx, vm)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("NumCPUs", func() {
			BeforeEach(func() {
				configSpec.NumCPUs = 2
			})

			It("Resizes", func() {
				cs := configSpec
				cs.NumCPUs = 42
				vm.Spec.ClassName = createVMClass(cs).Name

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(42))
			})
		})

		Context("MemoryMB", func() {
			BeforeEach(func() {
				configSpec.MemoryMB = 1024
			})

			It("Resizes", func() {
				cs := configSpec
				cs.MemoryMB = 8192
				vm.Spec.ClassName = createVMClass(cs).Name

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(8192))
			})
		})

		Context("Devops Overrides", func() {

			Context("ChangeBlockTracking", func() {
				BeforeEach(func() {
					configSpec.ChangeTrackingEnabled = vimtypes.NewBool(false)
				})

				It("Overrides", func() {
					vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
						ChangeBlockTracking: vimtypes.NewBool(true),
					}

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))
				})
			})
		})
	})
}
