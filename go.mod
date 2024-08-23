module github.com/vmware-tanzu/vm-operator

go 1.22.5

replace (
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.4
	github.com/envoyproxy/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/vmware-tanzu/vm-operator/api => ./api
	github.com/vmware-tanzu/vm-operator/external/byok => ./external/byok
	github.com/vmware-tanzu/vm-operator/external/ncp => ./external/ncp
	github.com/vmware-tanzu/vm-operator/external/storage-policy-quota => ./external/storage-policy-quota
	github.com/vmware-tanzu/vm-operator/external/tanzu-topology => ./external/tanzu-topology
	github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels => ./pkg/constants/testlabels
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/go-logr/logr v1.4.2
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/onsi/ginkgo/v2 v2.19.0
	github.com/onsi/gomega v1.33.1
	github.com/prometheus/client_golang v1.19.1
	github.com/vmware-tanzu/image-registry-operator-api v0.0.0-20240509202721-f6552612433a
	github.com/vmware-tanzu/net-operator-api v0.0.0-20240523152550-862e2c4eb0e0
	github.com/vmware-tanzu/nsx-operator/pkg/apis v0.0.0-20240812063009-106e8b213b74
	github.com/vmware-tanzu/vm-operator/api v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/byok v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/ncp v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/storage-policy-quota v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/tanzu-topology v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels v0.0.0-00010101000000-000000000000
	github.com/vmware/govmomi v0.31.1-0.20240730173452-49b88eb9917f
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/24
	golang.org/x/text v0.16.0
	k8s.io/api v0.31.0
	k8s.io/apiextensions-apiserver v0.31.0
	k8s.io/apimachinery v0.31.0
	k8s.io/client-go v0.31.0
	k8s.io/klog/v2 v2.130.1
	sigs.k8s.io/controller-runtime v0.19.0
	sigs.k8s.io/yaml v1.4.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.4 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20240525223248-4bfdf5a9a2af // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/exp v0.0.0-20230515195305-f3d0a9c9a5cc // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/oauth2 v0.21.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/term v0.21.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	gomodules.xyz/jsonpatch/v2 v2.4.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	k8s.io/utils v0.0.0-20240711033017-18e509b52bc8 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
)
