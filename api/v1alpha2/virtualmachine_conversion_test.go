// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a2common "github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

func TestVirtualMachineConversion(t *testing.T) {

	hubSpokeHub := func(g *WithT, hub, hubAfter ctrlconversion.Hub, spoke ctrlconversion.Convertible) {
		hubBefore := hub.DeepCopyObject().(ctrlconversion.Hub)

		// First convert hub to spoke
		dstCopy := spoke.DeepCopyObject().(ctrlconversion.Convertible)
		g.Expect(dstCopy.ConvertFrom(hubBefore)).To(Succeed())

		// Convert spoke back to hub and check if the resulting hub is equal to the hub before the round trip
		g.Expect(dstCopy.ConvertTo(hubAfter)).To(Succeed())

		g.Expect(apiequality.Semantic.DeepEqual(hubBefore, hubAfter)).To(BeTrue(), cmp.Diff(hubBefore, hubAfter))
	}

	t.Run("VirtualMachine hub-spoke-hub with spec.image", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image: &vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "vmi-123",
				},
				ClassName:    "my-class",
				StorageClass: "my-storage-class",
			},
		}
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
	})

	t.Run("VirtualMachine spoke-hub with image status", func(t *testing.T) {

		t.Run("generation == 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a2.VirtualMachine{
				Status: vmopv1a2.VirtualMachineStatus{
					Image: &vmopv1a2common.LocalObjectRef{
						Kind: "VirtualMachineImage",
						Name: "vmi-123",
					},
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).To(BeNil())
		})

		t.Run("generation > 0", func(t *testing.T) {
			g := NewWithT(t)

			spoke := vmopv1a2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: vmopv1a2.VirtualMachineStatus{
					Image: &vmopv1a2common.LocalObjectRef{
						Kind: "VirtualMachineImage",
						Name: "vmi-123",
					},
				},
			}

			var hub vmopv1.VirtualMachine
			g.Expect(spoke.ConvertTo(&hub)).To(Succeed())
			g.Expect(hub.Spec.Image).ToNot(BeNil())
			g.Expect(hub.Spec.Image.Kind).To(Equal(spoke.Status.Image.Kind))
			g.Expect(hub.Spec.Image.Name).To(Equal(spoke.Status.Image.Name))
		})
	})

	t.Run("VirtualMachine hub-spoke with empty image name", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Image: &vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "vmi-123",
				},
			},
		}
		var spoke vmopv1a2.VirtualMachine
		g.Expect(spoke.ConvertFrom(&hub)).To(Succeed())
		g.Expect(spoke.Spec.ImageName).To(Equal("vmi-123"))
	})

	t.Run("VirtualMachine hub-spoke-hub with bios UUID", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				BiosUUID: "123",
			},
		}
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
	})

	t.Run("VirtualMachine hub-spoke-hub with cloud-init instance ID", func(t *testing.T) {
		g := NewWithT(t)
		hub := vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						InstanceID: "123",
					},
				},
			},
		}
		hubSpokeHub(g, &hub, &vmopv1.VirtualMachine{}, &vmopv1a2.VirtualMachine{})
	})

}
