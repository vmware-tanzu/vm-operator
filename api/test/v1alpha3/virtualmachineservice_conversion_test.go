// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func TestVirtualMachineServiceConversion(t *testing.T) {
	t.Run("hub-spoke-hub conditions field", func(t *testing.T) {
		hub := &vmopv1.VirtualMachineService{
			Spec: vmopv1.VirtualMachineServiceSpec{
				Type:     vmopv1.VirtualMachineServiceTypeClusterIP,
				Ports:    []vmopv1.VirtualMachineServicePort{{Name: "http", Protocol: "TCP", Port: 80, TargetPort: 8080}},
				Selector: map[string]string{"app": "test"},
			},
			Status: vmopv1.VirtualMachineServiceStatus{
				LoadBalancer: vmopv1.LoadBalancerStatus{
					Ingress: []vmopv1.LoadBalancerIngress{
						{IP: "192.168.1.100"},
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:               vmopv1.ServiceReadyConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Now().Rfc3339Copy(),
						Reason:             "ServiceReady",
						Message:            "Service is ready",
					},
				},
			},
		}

		g := NewWithT(t)
		after := &vmopv1.VirtualMachineService{}
		spoke := &vmopv1a3.VirtualMachineService{}

		g.Expect(spoke.ConvertFrom(hub)).To(Succeed())
		g.Expect(spoke.ConvertTo(after)).To(Succeed())
		g.Expect(apiequality.Semantic.DeepEqual(hub, after)).To(BeTrue(), cmp.Diff(hub, after))
	})
}
