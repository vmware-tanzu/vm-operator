// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

type SecurityPolicy struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// GetSecurityPolicyYaml returns a SecurityPolicy that allows ingress traffic
// to VMs labeled role=allow-ingress.
func GetSecurityPolicyYaml(securitypolicy SecurityPolicy) []byte {
	return ToYAML(&vpcv1alpha1.SecurityPolicy{
		TypeMeta: typeMeta(vpcv1alpha1.GroupVersion.String(), "SecurityPolicy"),
		ObjectMeta: metav1.ObjectMeta{
			Name:      securitypolicy.Name,
			Namespace: securitypolicy.Namespace,
		},
		Spec: vpcv1alpha1.SecurityPolicySpec{
			Priority: 10,
			AppliedTo: []vpcv1alpha1.SecurityPolicyTarget{
				{
					VMSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"role": "allow-ingress"},
					},
				},
			},
			Rules: []vpcv1alpha1.SecurityPolicyRule{
				{
					Direction: ptr.To(vpcv1alpha1.RuleDirection("in")),
					Action:    ptr.To(vpcv1alpha1.RuleAction("allow")),
				},
			},
		},
	})
}
