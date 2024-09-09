module github.com/vmware-tanzu/vm-operator/api

go 1.22.7

require (
	github.com/google/go-cmp v0.6.0
	github.com/google/gofuzz v1.2.0
	github.com/google/uuid v1.6.0
	github.com/onsi/ginkgo/v2 v2.19.0
	github.com/onsi/gomega v1.33.1
	k8s.io/api v0.31.0
	k8s.io/apimachinery v0.31.0
	sigs.k8s.io/controller-runtime v0.19.0
)

require (
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/pprof v0.0.0-20240525223248-4bfdf5a9a2af // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	k8s.io/utils v0.0.0-20240711033017-18e509b52bc8 // indirect
)

require (
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/20
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/21
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/32
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/34
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/36
	golang.org/x/net v0.26.0 // indirect
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/22
	golang.org/x/text v0.16.0 // indirect; per
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
