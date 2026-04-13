package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

const (
	subnetFilePath = "test/e2e/fixtures/yaml/vmoperator/subnet"
)

type SubnetOrSubnetSet struct {
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// Util function to return a Namespaced Subnet or SubnetSet yaml from a templatized fixture.
func GetSubnetOrSubnetSetYaml(subnet SubnetOrSubnetSet) []byte {
	subnetYamlIn := fixtures.ReadFile(subnetFilePath, "subnet.yaml.in")
	subnetYaml := ReadSubnet(subnet, subnetYamlIn)

	return subnetYaml
}

func GetDHCPSubnetOrSubnetSetYaml(subnet SubnetOrSubnetSet, private bool) []byte {
	subnetYaml := GetSubnetOrSubnetSetYaml(subnet)
	// Add DHCP spec in separate to keep its template text as is.
	dhcpYaml := fixtures.ReadFileBytes(subnetFilePath, "subnetDHCP.yaml")
	// Determine the access mode YAML configuration.
	var accessModeYaml []byte
	if private {
		accessModeYaml = fixtures.ReadFileBytes(subnetFilePath, "subnetPrivateAccessMode.yaml")
	} else {
		accessModeYaml = fixtures.ReadFileBytes(subnetFilePath, "subnetPublicAccessMode.yaml")
	}

	return append(append(subnetYaml, dhcpYaml...), accessModeYaml...)
}

func GetCIDRSubnetOrSubnetSetYaml(subnet SubnetOrSubnetSet, private bool) []byte {
	subnetYaml := GetSubnetOrSubnetSetYaml(subnet)
	// Add CIDR spec in separate to keep its template text as is.
	cidrYaml := fixtures.ReadFileBytes(subnetFilePath, "subnetCIDR.yaml")
	// Determine the access mode YAML configuration.
	var accessModeYaml []byte
	if private {
		accessModeYaml = fixtures.ReadFileBytes(subnetFilePath, "subnetPrivateAccessMode.yaml")
	} else {
		accessModeYaml = fixtures.ReadFileBytes(subnetFilePath, "subnetPublicAccessMode.yaml")
	}

	return append(append(subnetYaml, cidrYaml...), accessModeYaml...)
}

func ReadSubnet(subnet SubnetOrSubnetSet, input string) []byte {
	tmpl := template.Must(template.New("subnet").Parse(input))

	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, subnet)
	if err != nil {
		e2eframework.Failf("Failed executing subnet template: %v", err)
	}

	return parsed.Bytes()
}
