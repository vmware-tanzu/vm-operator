// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package flexvmclasses

import (
	"context"
	"fmt"
	"os"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

const (
	notSupportedYetMsg   = "not supported yet"
	requiresClasslessMsg = "requires the VMServiceClasslessVM capability"
)

// VMAPISpec verifies the Flex VM API surface on the VM resource: the installed
// CRD schema and the VM validating webhook gate that rejects the new spec
// fields until the controller supports them. Every gate check is paired with a
// check that VMs without the new fields, and older API versions, are still
// admitted. Controller population of these fields is covered by later stories.
func VMAPISpec(ctx context.Context, inputGetter func() SpecInput) {
	const specName = "flex-vm-api"

	var (
		input        SpecInput
		client       ctrlclient.Client
		namespace    string
		linuxVMIName string
		className    string
		storageClass string
	)

	ginkgo.BeforeEach(func() {
		// The staged operator upgrade run (FLEX_UPGRADE_PHASE) selects the
		// staged upgrade specs only.
		if phase := os.Getenv("FLEX_UPGRADE_PHASE"); phase != "" {
			ginkgo.Skip(fmt.Sprintf("FLEX_UPGRADE_PHASE=%s selects the staged upgrade specs only", phase))
		}

		input = inputGetter()
		Expect(input.Config).ToNot(BeNil(),
			"Invalid argument. input.Config can't be nil when calling %s spec", specName)
		Expect(input.ClusterProxy).ToNot(BeNil(),
			"Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.WCPNamespaceName).ToNot(BeEmpty(),
			"Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

		client = input.ClusterProxy.GetClient()
		namespace = input.WCPNamespaceName
	})

	// loadResources resolves the namespace's default image, class and storage
	// class. It is only needed by the admission specs.
	loadResources := func() {
		resources := input.Config.InfraConfig.ManagementClusterConfig.Resources
		className = resources.VMClassName
		storageClass = resources.StorageClassName
		linuxVMIName = vmoperator.WaitForVirtualMachineImageName(
			ctx, &input.Config.Config, client, namespace,
			vmservice.GetDefaultImageDisplayName(resources))
	}

	newVM := func(prefix string) *vmopv1.VirtualMachine {
		return &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", prefix, capiutil.RandomString(4)),
				Namespace: namespace,
			},
			Spec: vmopv1.VirtualMachineSpec{
				ClassName:    className,
				ImageName:    linuxVMIName,
				StorageClass: storageClass,
				PowerState:   vmopv1.VirtualMachinePowerStateOff,
			},
		}
	}

	// New behavior: the installed CRD publishes the Flex VM fields.
	ginkgo.It("publishes the Flex VM fields in the installed schema",
		ginkgo.Label(FeatureLabel, "extended-functional", "experimental"), func() {
			vm := flexAPISchema(ctx, client, "virtualmachines.vmoperator.vmware.com", "v1alpha6")
			Expect(flexAPIField(vm, "spec", "reservedProfileName")).To(HaveKeyWithValue("type", "string"))
			vgpu := flexAPIField(vm, "spec", "hardware", "vGPUs")
			directPath := flexAPIField(vm, "spec", "hardware", "dynamicDirectPathIODevices")
			for _, tc := range []struct {
				devices  map[string]any
				required []string
			}{
				{vgpu, []string{"name", "profileName"}},
				{directPath, []string{"name"}},
			} {
				devices, wantRequired := tc.devices, tc.required
				Expect(devices).To(HaveKeyWithValue("x-kubernetes-list-type", "map"))
				Expect(devices).To(HaveKeyWithValue("x-kubernetes-list-map-keys", []any{"name"}))
				items, found, err := unstructured.NestedMap(devices, "items")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				required, found, err := unstructured.NestedStringSlice(items, "required")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(required).To(ConsistOf(wantRequired))
				Expect(flexAPIField(items, "name")).To(HaveKeyWithValue("type", "string"))
			}
			Expect(flexAPIField(vm, "status", "hardware", "firmware")).To(HaveKeyWithValue("enum", []any{"bios", "efi"}))
			Expect(flexAPIField(vm, "status", "changeBlockTracking")).To(HaveKeyWithValue("type", "boolean"))
			Expect(flexAPIField(vm, "status", "hardware", "cpu", "hotRemoveEnabled")).To(HaveKeyWithValue("type", "boolean"))
			for _, field := range []string{"sevEnabled", "sevSnpEnabled", "tdxEnabled", "smcPresent"} {
				Expect(flexAPIField(vm, "status", "hardware", field)).To(HaveKeyWithValue("type", "boolean"))
			}
		})

	// New behavior: the webhook gate. The VM validating webhook declares
	// sideEffects None, so a server-side dry run exercises it without
	// creating anything. The update path is covered by webhook unit tests.
	ginkgo.Context("webhook gate on the Flex VM fields", func() {
		ginkgo.BeforeEach(loadResources)

		for _, tc := range []struct {
			name  string
			field string
			set   func(*vmopv1.VirtualMachine)
		}{
			{
				name:  "spec.reservedProfileName",
				field: "spec.reservedProfileName",
				set: func(vm *vmopv1.VirtualMachine) {
					vm.Spec.ReservedProfileName = ptr.To("e2e-reserved-profile")
				},
			},
			{
				name:  "one spec.hardware.vGPUs entry",
				field: "spec.hardware.vGPUs",
				set: func(vm *vmopv1.VirtualMachine) {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						VGPUs: []vmopv1.VirtualMachineVGPUDevice{{
							Name:        "gpu0",
							ProfileName: "e2e-profile",
						}},
					}
				},
			},
			{
				name:  "one spec.hardware.dynamicDirectPathIODevices entry",
				field: "spec.hardware.dynamicDirectPathIODevices",
				set: func(vm *vmopv1.VirtualMachine) {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						DynamicDirectPathIODevices: []vmopv1.VirtualMachineDynamicDirectPathIODevice{{
							Name:     "dev0",
							VendorID: ptr.To(int64(0x10de)),
							DeviceID: ptr.To(int64(0x1234)),
						}},
					}
				},
			},
		} {
			ginkgo.It(fmt.Sprintf("rejects creating a v1alpha6 VM that sets %s", tc.name),
				ginkgo.Label(FeatureLabel, "extended-functional", "experimental"), func() {
					vm := newVM("flex-gate")
					tc.set(vm)

					err := client.Create(ctx, vm, ctrlclient.DryRunAll)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(tc.field))
					Expect(err.Error()).To(ContainSubstring(notSupportedYetMsg))

					// The capability message is reported independently, and
					// only when the capability is off.
					if input.Config.SupervisorCapabilities[consts.VMServiceClasslessVMCapabilityName] {
						Expect(err.Error()).ToNot(ContainSubstring(requiresClasslessMsg))
					} else {
						Expect(err.Error()).To(ContainSubstring(requiresClasslessMsg))
					}
				})
		}
	})

	// Existing behavior: the gate must not affect VMs that do not use the new
	// fields, nor older API clients.
	ginkgo.Context("existing VM admission is unaffected", func() {
		ginkgo.BeforeEach(func() {
			skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)
			loadResources()
		})

		ginkgo.It("admits a v1alpha6 VM without the Flex VM fields",
			ginkgo.Label(FeatureLabel, "extended-functional", "experimental"), func() {
				Expect(client.Create(ctx, newVM("flex-existing-a6"), ctrlclient.DryRunAll)).To(Succeed())
			})

		ginkgo.It("admits a v1alpha5 VM",
			ginkgo.Label(FeatureLabel, "extended-functional", "experimental"), func() {
				vm := &vmopv1a5.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("flex-existing-a5-%s", capiutil.RandomString(4)),
						Namespace: namespace,
					},
					Spec: vmopv1a5.VirtualMachineSpec{
						ClassName:    className,
						ImageName:    linuxVMIName,
						StorageClass: storageClass,
						PowerState:   vmopv1a5.VirtualMachinePowerStateOff,
					},
				}
				Expect(client.Create(ctx, vm, ctrlclient.DryRunAll)).To(Succeed())
			})
	})
}

func flexAPISchema(ctx context.Context, client ctrlclient.Client, name, version string) map[string]any {
	ginkgo.GinkgoHelper()
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	Expect(client.Get(ctx, ctrlclient.ObjectKey{Name: name}, crd)).To(Succeed())
	versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())
	for _, entry := range versions {
		item := entry.(map[string]any)
		if item["name"] == version {
			schema, found, err := unstructured.NestedMap(item, "schema", "openAPIV3Schema")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			return schema
		}
	}
	ginkgo.Fail("CRD " + name + " does not serve " + version)
	return nil
}

func flexAPIField(schema map[string]any, fields ...string) map[string]any {
	ginkgo.GinkgoHelper()
	path := make([]string, 0, 2*len(fields))
	for _, field := range fields {
		path = append(path, "properties", field)
	}
	value, found, err := unstructured.NestedMap(schema, path...)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "missing schema field %v", fields)
	return value
}
