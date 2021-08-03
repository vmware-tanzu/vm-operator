module github.com/vmware-tanzu/vm-operator

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/envoyproxy/go-control-plane v0.9.4
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pkg/errors v0.9.1
	github.com/vmware-tanzu/vm-operator-api v0.1.4-0.20210729193442-29f3169f67b5
	github.com/vmware/govmomi v0.26.0
	google.golang.org/grpc v1.27.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/yaml v1.2.0
)
