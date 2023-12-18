// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	nextver "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	nextver_common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
)

func TestVirtualMachineImageConversion(t *testing.T) {
	g := NewWithT(t)

	t.Run("VirtualMachineImage hub-spoke-hub", func(t *testing.T) {
		hub := &nextver.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-image",
				Namespace: "my-namespace",
			},
			Spec: nextver.VirtualMachineImageSpec{
				ProviderRef: nextver_common.LocalObjectRef{
					APIVersion: "vmware.com/v1",
					Kind:       "ImageProvider",
					Name:       "my-image",
				},
			},
		}

		spoke := &v1alpha1.VirtualMachineImage{}
		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())

		g.Expect(spoke.Spec.ProviderRef.APIVersion).To(Equal("vmware.com/v1"))
		g.Expect(spoke.Spec.ProviderRef.Kind).To(Equal("ImageProvider"))
		g.Expect(spoke.Spec.ProviderRef.Name).To(Equal("my-image"))
		g.Expect(spoke.Spec.ProviderRef.Namespace).To(Equal("my-namespace"))
	})
}
