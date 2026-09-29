// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
)

const (
	subnetKind    = "Subnet"
	subnetSetKind = "SubnetSet"

	// cidrSubnetSize is the IPv4 Subnet size of a CIDR Subnet or SubnetSet.
	cidrSubnetSize = 16
)

type SubnetOrSubnetSet struct {
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// GetDHCPSubnetOrSubnetSetYaml returns the manifest for the Subnet or
// SubnetSet built by DHCPSubnetOrSubnetSet.
func GetDHCPSubnetOrSubnetSetYaml(subnet SubnetOrSubnetSet, private bool) []byte {
	return subnetToYAML(must(DHCPSubnetOrSubnetSet(subnet, private)))
}

// GetCIDRSubnetOrSubnetSetYaml returns the manifest for the Subnet or
// SubnetSet built by CIDRSubnetOrSubnetSet.
func GetCIDRSubnetOrSubnetSetYaml(subnet SubnetOrSubnetSet, private bool) []byte {
	return subnetToYAML(must(CIDRSubnetOrSubnetSet(subnet, private)))
}

// DHCPSubnetOrSubnetSet returns a Subnet or SubnetSet, depending on
// subnet.Kind, that uses a DHCP server. An error is returned if subnet.Kind
// is neither Subnet nor SubnetSet.
func DHCPSubnetOrSubnetSet(subnet SubnetOrSubnetSet, private bool) (ctrlclient.Object, error) {
	return subnetOrSubnetSet(subnet, 0, vpcv1alpha1.DHCPConfigModeServer, private)
}

// CIDRSubnetOrSubnetSet returns a Subnet or SubnetSet, depending on
// subnet.Kind, with a fixed IPv4 Subnet size. An error is returned if
// subnet.Kind is neither Subnet nor SubnetSet.
func CIDRSubnetOrSubnetSet(subnet SubnetOrSubnetSet, private bool) (ctrlclient.Object, error) {
	return subnetOrSubnetSet(subnet, cidrSubnetSize, "", private)
}

func subnetOrSubnetSet(
	subnet SubnetOrSubnetSet,
	ipv4SubnetSize int,
	dhcpMode string,
	private bool) (ctrlclient.Object, error) {

	accessMode := vpcv1alpha1.AccessMode(vpcv1alpha1.AccessModePublic)
	if private {
		accessMode = vpcv1alpha1.AccessMode(vpcv1alpha1.AccessModePrivate)
	}
	dhcpConfig := vpcv1alpha1.SubnetDHCPConfig{
		Mode: vpcv1alpha1.DHCPConfigMode(dhcpMode),
	}
	objectMeta := metav1.ObjectMeta{
		Namespace: subnet.Namespace,
		Name:      subnet.Name,
	}

	switch subnet.Kind {
	case subnetKind:
		return &vpcv1alpha1.Subnet{
			TypeMeta:   typeMeta(vpcv1alpha1.GroupVersion.String(), subnetKind),
			ObjectMeta: objectMeta,
			Spec: vpcv1alpha1.SubnetSpec{
				IPv4SubnetSize:   ipv4SubnetSize,
				AccessMode:       accessMode,
				SubnetDHCPConfig: dhcpConfig,
			},
		}, nil
	case subnetSetKind:
		return &vpcv1alpha1.SubnetSet{
			TypeMeta:   typeMeta(vpcv1alpha1.GroupVersion.String(), subnetSetKind),
			ObjectMeta: objectMeta,
			Spec: vpcv1alpha1.SubnetSetSpec{
				IPv4SubnetSize:   ipv4SubnetSize,
				AccessMode:       accessMode,
				SubnetDHCPConfig: dhcpConfig,
			},
		}, nil
	default:
		return nil, fmt.Errorf("invalid kind %q: must be %s or %s",
			subnet.Kind, subnetKind, subnetSetKind)
	}
}

// subnetToYAML renders obj with its empty objects removed. The Subnet and
// SubnetSet specs have nested struct fields that are not pointers, so they
// marshal as empty objects, e.g. "advancedConfig: {}". An empty object is not
// the same as an absent one to the API server, which applies CRD defaults to
// it, so these are removed to send only the fields that were set.
func subnetToYAML(obj ctrlclient.Object) []byte {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		e2eframework.Failf("Failed to convert %T to unstructured: %v", obj, err)
	}
	if spec, ok := u["spec"].(map[string]any); ok {
		removeEmptyObjects(spec)
	}
	return ToYAML(&unstructured.Unstructured{Object: u})
}

// removeEmptyObjects removes from m every nested object that is empty or
// becomes empty after its own empty objects are removed.
func removeEmptyObjects(m map[string]any) {
	for k, v := range m {
		if child, ok := v.(map[string]any); ok {
			removeEmptyObjects(child)
			if len(child) == 0 {
				delete(m, k)
			}
		}
	}
}
