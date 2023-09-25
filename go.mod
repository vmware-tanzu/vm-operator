module github.com/vmware-tanzu/vm-operator

go 1.20

replace (
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.4
	github.com/envoyproxy/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/vmware-tanzu/vm-operator/api => ./api
)

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/envoyproxy/go-control-plane v0.10.3
	github.com/go-logr/logr v1.2.4
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.1
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.27.10
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.16.0
	github.com/vmware-tanzu/image-registry-operator-api v0.0.0-20230526154708-f67dac7c805f
	github.com/vmware-tanzu/vm-operator/api v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/ncp v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/tanzu-topology v0.0.0-00010101000000-000000000000
	github.com/vmware/govmomi v0.28.1-0.20230918130735-a83ec3f76646
	// per the following dependabot alerts:
	// * https://github.com/vmware-tanzu/vm-operator/security/dependabot/24
	golang.org/x/net v0.13.0 // indirect
	golang.org/x/text v0.11.0
	gomodules.xyz/jsonpatch/v2 v2.4.0
	google.golang.org/grpc v1.54.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.28.0
	k8s.io/apiextensions-apiserver v0.28.0
	k8s.io/apimachinery v0.28.0
	k8s.io/client-go v0.28.0
	k8s.io/klog/v2 v2.100.1
	k8s.io/utils v0.0.0-20230406110748-d93618cff8a2
	sigs.k8s.io/controller-runtime v0.15.0
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cncf/udpa/go v0.0.0-20220112060539-c52dc94e7fbe // indirect
	github.com/cncf/xds/go v0.0.0-20230105202645-06c439db220b // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v0.9.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/oauth2 v0.8.0 // indirect
	golang.org/x/sys v0.11.0 // indirect
	golang.org/x/term v0.10.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230526161137-0005af68ea54 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230525234035-dd9d682886f9 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230525234030-28d5490b6b19 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/component-base v0.28.0 // indirect
	k8s.io/kube-openapi v0.0.0-20230717233707-2695361300d9 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace github.com/vmware-tanzu/vm-operator/external/tanzu-topology => ./external/tanzu-topology

replace github.com/vmware-tanzu/vm-operator/external/ncp => ./external/ncp
