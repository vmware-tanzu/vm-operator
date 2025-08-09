module github.com/vmware-tanzu/vm-operator/api

go 1.24.0

toolchain go1.24.4

// The version of Ginkgo must match the one from ../go.mod.
require github.com/onsi/ginkgo/v2 v2.23.4

require (
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/onsi/gomega v1.36.3
	k8s.io/api v0.33.0
	k8s.io/apimachinery v0.33.0
	sigs.k8s.io/controller-runtime v0.19.0
	sigs.k8s.io/randfill v1.0.0
)

require go.uber.org/automaxprocs v1.6.0 // indirect

require (
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/22
	golang.org/x/text v0.23.0 // indirect; per
	golang.org/x/tools v0.31.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20241104100929-3ea5e8cea738 // indirect
	sigs.k8s.io/json v0.0.0-20241010143419-9aa6b5e7a4b3 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.6.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
