module github.com/vmware-tanzu/vm-operator-api/hack/samples

go 1.13

// The generated client is not part of the release, so point to the local version that includes the generated code
// Note also that vm-operator-api is not specified in require the samples automatically pull in the latest
replace github.com/vmware-tanzu/vm-operator-api => ../../../vm-operator-api

require (
	github.com/vmware-tanzu/vm-operator-api v0.0.0
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.5.2
)
