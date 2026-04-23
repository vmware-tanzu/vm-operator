module github.com/vmware-tanzu/vm-operator/test/e2e

go 1.26.2

replace (
	github.com/onsi/ginkgo/v2 => github.com/onsi/ginkgo/v2 v2.28.1
	github.com/onsi/gomega => github.com/onsi/gomega v1.39.1
	github.com/vmware-tanzu/vm-operator => ../../
	github.com/vmware-tanzu/vm-operator/api => ../../api
	github.com/vmware-tanzu/vm-operator/external/appplatform => ../../external/appplatform
	github.com/vmware-tanzu/vm-operator/external/byok => ../../external/byok
	github.com/vmware-tanzu/vm-operator/external/capabilities => ../../external/capabilities
	github.com/vmware-tanzu/vm-operator/external/image-registry-operator => ../../external/image-registry-operator
	github.com/vmware-tanzu/vm-operator/external/infra => ../../external/infra
	github.com/vmware-tanzu/vm-operator/external/mobility-operator => ../../external/mobility-operator
	github.com/vmware-tanzu/vm-operator/external/ncp => ../../external/ncp
	github.com/vmware-tanzu/vm-operator/external/net-operator => ../../external/net-operator
	github.com/vmware-tanzu/vm-operator/external/nsx-operator => ../../external/nsx-operator
	github.com/vmware-tanzu/vm-operator/external/storage-policy-quota => ../../external/storage-policy-quota
	github.com/vmware-tanzu/vm-operator/external/tanzu-topology => ../../external/tanzu-topology
	github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver => ../../external/vsphere-csi-driver
	github.com/vmware-tanzu/vm-operator/external/vsphere-policy => ../../external/vsphere-policy
	github.com/vmware-tanzu/vm-operator/pkg/backup/api => ../../pkg/backup/api
	github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels => ../../pkg/constants/testlabels

	k8s.io/api => k8s.io/api v0.35.3
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.35.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.35.3
	k8s.io/apiserver => k8s.io/apiserver v0.35.3
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.35.3
	k8s.io/client-go => k8s.io/client-go v0.35.3
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.35.3
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.35.3
	k8s.io/component-base => k8s.io/component-base v0.35.3
	k8s.io/component-helpers => k8s.io/component-helpers v0.35.3
	k8s.io/controller-manager => k8s.io/controller-manager v0.35.3
	k8s.io/cri-api => k8s.io/cri-api v0.35.3
	k8s.io/cri-client => k8s.io/cri-client v0.35.3
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.35.3
	k8s.io/endpointslice => k8s.io/endpointslice v0.35.3
	k8s.io/externaljwt => k8s.io/externaljwt v0.35.3
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.35.3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.35.3
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.35.3
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.35.3
	k8s.io/kubectl => k8s.io/kubectl v0.35.3
	k8s.io/mount-utils => k8s.io/mount-utils v0.35.3
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.35.3
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.35.3

	sigs.k8s.io/structured-merge-diff/v4 => sigs.k8s.io/structured-merge-diff/v4 v4.7.0
)

require (
	github.com/antchfx/htmlquery v1.3.6
	github.com/google/goexpect v0.0.0-20210430020637-ab937bf7fd6f
	github.com/onsi/ginkgo/v2 v2.28.1
	github.com/onsi/gomega v1.39.1
	github.com/sirupsen/logrus v1.9.3
	github.com/vmware-tanzu/vm-operator/api v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/image-registry-operator v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/mobility-operator v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/ncp v0.0.0-20260414073537-dff710623a20
	github.com/vmware-tanzu/vm-operator/external/net-operator v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/nsx-operator v0.0.0-00010101000000-000000000000
	github.com/vmware-tanzu/vm-operator/external/storage-policy-quota v0.0.0-20260414073537-dff710623a20
	github.com/vmware-tanzu/vm-operator/external/tanzu-topology v0.0.0-20260414073537-dff710623a20
	github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver v0.0.0-20260414073537-dff710623a20
	github.com/vmware-tanzu/vm-operator/pkg/backup/api v0.0.0-20260414073537-dff710623a20
	github.com/vmware/govmomi v0.53.0
	golang.org/x/crypto v0.50.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.35.3
	k8s.io/apiextensions-apiserver v0.35.0
	k8s.io/apimachinery v0.35.3
	k8s.io/client-go v0.35.3
	k8s.io/klog/v2 v2.140.0
	k8s.io/kubernetes v0.0.0-00010101000000-000000000000
	sigs.k8s.io/cluster-api v1.12.5
	sigs.k8s.io/controller-runtime v0.23.3
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/pkg/errors v0.9.1 // indirect
	k8s.io/utils v0.0.0-20260319190234-28399d86e0b5 // indirect
)

require (
	cel.dev/expr v0.25.1 // indirect
	cyphar.com/go-pathrs v0.2.4 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/antchfx/xpath v1.3.6 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cyphar/filepath-securejoin v0.6.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/gkampitakis/go-snaps v0.5.21 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/jsonreference v0.21.5 // indirect
	github.com/go-openapi/swag v0.25.5 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.5 // indirect
	github.com/go-openapi/swag/conv v0.25.5 // indirect
	github.com/go-openapi/swag/fileutils v0.25.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.5 // indirect
	github.com/go-openapi/swag/loading v0.25.5 // indirect
	github.com/go-openapi/swag/mangling v0.25.5 // indirect
	github.com/go-openapi/swag/netutils v0.25.5 // indirect
	github.com/go-openapi/swag/stringutils v0.25.5 // indirect
	github.com/go-openapi/swag/typeutils v0.25.5 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.5 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/cel-go v0.26.0 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/goterm v0.0.0-20190703233501-fc88cf888a3f // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/selinux v1.13.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stoewer/go-strcase v1.3.1 // indirect
	github.com/vmware-tanzu/vm-operator v1.9.0
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.68.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260401024825-9d38bb4040a9 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260401024825-9d38bb4040a9 // indirect
	google.golang.org/grpc v1.80.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/apiserver v0.35.3 // indirect
	k8s.io/component-base v0.35.3 // indirect
	k8s.io/component-helpers v0.35.3 // indirect
	k8s.io/controller-manager v0.0.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260330154417-16be699c7b31 // indirect
	k8s.io/kubectl v0.0.0 // indirect
	k8s.io/kubelet v0.0.0 // indirect
	k8s.io/mount-utils v0.0.0 // indirect
	k8s.io/pod-security-admission v0.0.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.34.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.2 // indirect
)

replace (
	github.com/gobuffalo/flect => github.com/gobuffalo/flect v1.0.2
	github.com/golang/glog => github.com/golang/glog v1.1.0
	github.com/google/gofuzz => github.com/google/gofuzz v1.2.0
	github.com/kr/pretty => github.com/kr/pretty v0.3.1
	github.com/moby/sys/mountinfo => github.com/moby/sys/mountinfo v0.6.2
	github.com/tidwall/gjson => github.com/tidwall/gjson v1.14.4
	github.com/tidwall/pretty => github.com/tidwall/pretty v1.2.1
	github.com/tidwall/sjson => github.com/tidwall/sjson v1.2.5
)

replace (
	github.com/go-logr/zapr => github.com/go-logr/zapr v1.3.0
	github.com/go-task/slim-sprig/v3 => github.com/go-task/slim-sprig/v3 v3.0.0
	github.com/stretchr/testify => github.com/stretchr/testify v1.9.0

	go.uber.org/goleak => go.uber.org/goleak v1.3.0
	gopkg.in/check.v1 => gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
)

replace github.com/stretchr/objx => github.com/stretchr/objx v0.5.2

replace github.com/tidwall/match => github.com/tidwall/match v1.1.1

replace (
	github.com/dougm/pretty => github.com/dougm/pretty v0.0.0-20160325215624-add1dbc86daf
	github.com/inconshreveable/mousetrap => github.com/inconshreveable/mousetrap v1.1.0
	github.com/joshdk/go-junit => github.com/joshdk/go-junit v1.0.0
	github.com/mfridman/tparse => github.com/mfridman/tparse v0.18.0
)

replace (
	github.com/gkampitakis/ciinfo => github.com/gkampitakis/ciinfo v0.3.4
	github.com/goccy/go-yaml => github.com/goccy/go-yaml v1.19.2
	github.com/maruel/natural => github.com/maruel/natural v1.3.0
)

replace (
	google.golang.org/genproto/googleapis/api => google.golang.org/genproto/googleapis/api v0.0.0-20240814211410-ddb44dafa142
	google.golang.org/genproto/googleapis/rpc => google.golang.org/genproto/googleapis/rpc v0.0.0-20240123012728-ef4313101c80
)

replace k8s.io/kubelet => k8s.io/kubelet v0.35.3

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.35.3

replace k8s.io/kubernetes => k8s.io/kubernetes v1.35.3

replace github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.1

replace go.uber.org/zap => go.uber.org/zap v1.26.0

replace go.uber.org/multierr => go.uber.org/multierr v1.10.0

replace github.com/kylelemons/godebug => github.com/kylelemons/godebug v1.1.0

replace github.com/armon/go-socks5 => github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5

replace github.com/pmezard/go-difflib => github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2

replace github.com/google/cel-go => github.com/google/cel-go v0.22.1
