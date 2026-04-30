// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineServiceConversion(t *testing.T) {
	t.Run("hub-spoke-hub dual-stack fields", func(t *testing.T) {
		policy := corev1.IPFamilyPolicyPreferDualStack
		hub := &vmopv1.VirtualMachineService{
			Spec: vmopv1.VirtualMachineServiceSpec{
				Type:                     vmopv1.VirtualMachineServiceTypeLoadBalancer,
				IPFamilies:               []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
				IPFamilyPolicy:           &policy,
				Ports:                    []vmopv1.VirtualMachineServicePort{{Name: "http", Protocol: "TCP", Port: 80, TargetPort: 8080}},
				Selector:                 map[string]string{"app": "test"},
				LoadBalancerIP:           "",
				LoadBalancerSourceRanges: nil,
				ClusterIP:                "",
				ExternalName:             "",
			},
		}

		g := NewWithT(t)
		after := &vmopv1.VirtualMachineService{}
		spoke := &vmopv1a5.VirtualMachineService{}

		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(hub, after)).To(BeTrue(), cmp.Diff(hub, after))
	})
}
