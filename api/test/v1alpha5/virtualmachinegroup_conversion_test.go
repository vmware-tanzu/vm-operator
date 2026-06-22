// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineGroupConversion(t *testing.T) {
	t.Run("hub-spoke-hub powerOffDelay on single boot order group", func(t *testing.T) {
		delay := metav1.Duration{Duration: 30 * time.Second}
		hub := &vmopv1.VirtualMachineGroup{
			Spec: vmopv1.VirtualMachineGroupSpec{
				BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: "VirtualMachine", Name: "db-vm"},
						},
						PowerOffDelay: &delay,
					},
				},
			},
		}

		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineGroup{}
		after := &vmopv1.VirtualMachineGroup{}

		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(hub, after)).To(BeTrue(), cmp.Diff(hub, after))
	})

	t.Run("hub-spoke-hub powerOffDelay on multiple boot order groups", func(t *testing.T) {
		delay1 := metav1.Duration{Duration: 10 * time.Second}
		delay2 := metav1.Duration{Duration: 45 * time.Second}
		hub := &vmopv1.VirtualMachineGroup{
			Spec: vmopv1.VirtualMachineGroupSpec{
				BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
					{
						Members: []vmopv1.GroupMember{
							{Kind: "VirtualMachine", Name: "app-vm"},
						},
						PowerOffDelay: &delay1,
					},
					{
						Members: []vmopv1.GroupMember{
							{Kind: "VirtualMachine", Name: "db-vm"},
						},
					},
					{
						Members: []vmopv1.GroupMember{
							{Kind: "VirtualMachine", Name: "infra-vm"},
						},
						PowerOffDelay: &delay2,
					},
				},
			},
		}

		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineGroup{}
		after := &vmopv1.VirtualMachineGroup{}

		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(hub, after)).To(BeTrue(), cmp.Diff(hub, after))
	})

	t.Run("hub-spoke-hub no boot order groups", func(t *testing.T) {
		hub := &vmopv1.VirtualMachineGroup{
			Spec: vmopv1.VirtualMachineGroupSpec{},
		}

		g := NewWithT(t)
		spoke := &vmopv1a5.VirtualMachineGroup{}
		after := &vmopv1.VirtualMachineGroup{}

		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(hub, after)).To(BeTrue(), cmp.Diff(hub, after))
	})
}
