// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

func TestVirtualMachinePublishRequestConversion(t *testing.T) {

	t.Run("hub-spoke-hub", func(t *testing.T) {
		testCases := []struct {
			name string
			hub  ctrlconversion.Hub
		}{
			{
				name: "spec",
				hub: &vmopv1.VirtualMachinePublishRequest{
					Spec: vmopv1.VirtualMachinePublishRequestSpec{
						BackoffLimit: 5,
						Source: vmopv1.VirtualMachinePublishRequestSource{
							Name:       "my-vm",
							APIVersion: "imageregistry.vmware.com/v1alpha5",
							Kind:       "VirtualMachine",
						},
						Target: vmopv1.VirtualMachinePublishRequestTarget{
							Item: vmopv1.VirtualMachinePublishRequestTargetItem{
								Name:        "my-target",
								Description: "some description",
							},
							Location: vmopv1.VirtualMachinePublishRequestTargetLocation{
								Name:       "my-cl",
								APIVersion: "imageregistry.vmware.com/v1alpha2",
								Kind:       "ContentLibrary",
							},
						},
						TTLSecondsAfterFinished: ptrOf(int64(300)),
					},
				},
			},
		}

		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)

				after := &vmopv1.VirtualMachinePublishRequest{}
				spoke := &vmopv1a3.VirtualMachinePublishRequest{}

				// First convert hub to spoke
				g.Expect(spoke.ConvertFrom(tc.hub)).To(Succeed())

				// Convert spoke back to hub.
				g.Expect(spoke.ConvertTo(after)).To(Succeed())

				// Check that everything is equal.
				g.Expect(apiequality.Semantic.DeepEqual(tc.hub, after)).To(BeTrue(), cmp.Diff(tc.hub, after))
			})
		}
	})
}
