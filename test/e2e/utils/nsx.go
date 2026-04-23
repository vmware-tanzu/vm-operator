package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vpcv1alpha1 "github.com/vmware-tanzu/vm-operator/external/nsx-operator/api/vpc/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
)

const (
	vpcAPIVersion = "crd.nsx.vmware.com/v1alpha1"
	SubnetKind    = "Subnet"
	SubnetSetKind = "SubnetSet"
	DHCPConfig    = "DHCP"
	CIDRConfig    = "CIDR"
)

func GetSecurityPolicy(ctx context.Context, client ctrlclient.Client, ns, name string) (*vpcv1alpha1.SecurityPolicy, error) {
	securitypolicy := &vpcv1alpha1.SecurityPolicy{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, securitypolicy)
	if err != nil {
		return nil, err
	}

	return securitypolicy, nil
}

func GetSubnet(ctx context.Context, client ctrlclient.Client, ns, name string) (*vpcv1alpha1.Subnet, error) {
	subnet := &vpcv1alpha1.Subnet{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, subnet)
	if err != nil {
		return nil, err
	}

	return subnet, nil
}

func GetSubnetSet(ctx context.Context, client ctrlclient.Client, ns, name string) (*vpcv1alpha1.SubnetSet, error) {
	subnetSet := &vpcv1alpha1.SubnetSet{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, subnetSet)
	if err != nil {
		return nil, err
	}

	return subnetSet, nil
}

func CreateSubnetOrSubnetSetYaml(kind, name, ns, networkConfigType string, private bool) (sYaml []byte) {
	s := manifestbuilders.SubnetOrSubnetSet{
		Namespace: ns,
		Name:      name,
	}

	switch kind {
	case SubnetKind:
		s.Kind = SubnetKind
	case SubnetSetKind:
		s.Kind = SubnetSetKind
	}

	switch networkConfigType {
	case CIDRConfig:
		sYaml = manifestbuilders.GetCIDRSubnetOrSubnetSetYaml(s, private)
	case DHCPConfig:
		sYaml = manifestbuilders.GetDHCPSubnetOrSubnetSetYaml(s, private)
	}

	return
}

func CreateSecurityPolicyYaml(name, ns string) (sYaml []byte) {
	s := manifestbuilders.SecurityPolicy{
		Namespace: ns,
		Name:      name,
	}

	sYaml = manifestbuilders.GetSecurityPolicyYaml(s)

	return
}

func GetSubnetConnectionBindingMap(ctx context.Context, client ctrlclient.Client, ns, name string) (*vpcv1alpha1.SubnetConnectionBindingMap, error) {
	bindingMap := &vpcv1alpha1.SubnetConnectionBindingMap{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, bindingMap)
	if err != nil {
		return nil, err
	}

	return bindingMap, nil
}
