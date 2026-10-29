// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// VMFlexAPISpec verifies the installed VM and Zone API contracts. Controller
// population of these fields is covered by the later governance stories.
func VMFlexAPISpec(ctx context.Context, clientGetter func() ctrlclient.Client) {
	It("publishes the Flex VM and Zone fields in the installed schemas",
		Label("extended-functional", "experimental"), func() {
			client := clientGetter()
			vm := flexAPISchema(ctx, client, "virtualmachines.vmoperator.vmware.com", "v1alpha6")
			Expect(flexAPIField(vm, "spec", "reservedProfileName")).To(HaveKeyWithValue("type", "string"))
			Expect(flexAPIField(vm, "status", "governingClassNames")).To(HaveKeyWithValue("x-kubernetes-list-type", "set"))
			vgpu := flexAPIField(vm, "spec", "hardware", "vGPUs")
			directPath := flexAPIField(vm, "spec", "hardware", "dynamicDirectPathIODevices")
			for _, devices := range []map[string]any{vgpu, directPath} {
				Expect(devices).To(HaveKeyWithValue("x-kubernetes-list-type", "map"))
				Expect(devices).To(HaveKeyWithValue("x-kubernetes-list-map-keys", []any{"name"}))
				items, found, err := unstructured.NestedMap(devices, "items")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				required, found, err := unstructured.NestedStringSlice(items, "required")
				Expect(err).NotTo(HaveOccurred())
				Expect(found).To(BeTrue())
				Expect(required).To(Equal([]string{"name"}))
				Expect(flexAPIField(items, "name")).To(HaveKeyWithValue("type", "string"))
			}
			Expect(flexAPIField(vm, "status", "hardware", "firmware")).To(HaveKeyWithValue("enum", []any{"bios", "efi"}))
			Expect(flexAPIField(vm, "status", "changeBlockTracking")).To(HaveKeyWithValue("type", "boolean"))
			Expect(flexAPIField(vm, "status", "hardware", "cpu", "hotRemoveEnabled")).To(HaveKeyWithValue("type", "boolean"))
			for _, field := range []string{"sevEnabled", "sevSnpEnabled", "tdxEnabled", "smcPresent"} {
				Expect(flexAPIField(vm, "status", "hardware", field)).To(HaveKeyWithValue("type", "boolean"))
			}
			zone := flexAPISchema(ctx, client, "zones.topology.tanzu.vmware.com", "v1alpha1")
			Expect(flexAPIField(zone, "status", "governingClasses")).To(HaveKeyWithValue("x-kubernetes-list-type", "set"))
		})
}

func flexAPISchema(ctx context.Context, client ctrlclient.Client, name, version string) map[string]any {
	GinkgoHelper()
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
	Fail("CRD " + name + " does not serve " + version)
	return nil
}

func flexAPIField(schema map[string]any, fields ...string) map[string]any {
	GinkgoHelper()
	path := make([]string, 0, 2*len(fields))
	for _, field := range fields {
		path = append(path, "properties", field)
	}
	value, found, err := unstructured.NestedMap(schema, path...)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "missing schema field %v", fields)
	return value
}
