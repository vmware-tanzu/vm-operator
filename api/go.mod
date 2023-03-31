module github.com/vmware-tanzu/vm-operator/api

go 1.18

require (
	github.com/google/go-cmp v0.5.9
	github.com/google/gofuzz v1.2.0
	github.com/onsi/gomega v1.24.1
	k8s.io/api v0.26.1
	k8s.io/apimachinery v0.26.1
	k8s.io/client-go v0.26.1
	k8s.io/utils v0.0.0-20221128185143-99ec85e7a448
	sigs.k8s.io/controller-runtime v0.14.5
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	// per the following dependabot alerts:
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/20
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/21
	golang.org/x/net v0.7.0 // indirect
	// per the following dependabot alerts:
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/22
	golang.org/x/text v0.7.0 // indirect; per
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)
