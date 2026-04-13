package common

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/supervisor"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

type VMServiceClusterProxy struct {
	*wcpframework.WCPClusterProxy

	adminConfPath string
}

func NewVMServiceClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *VMServiceClusterProxy {
	baseClusterProxy := wcpframework.NewWCPClusterProxy(name, kubeconfigPath, scheme)

	proxy := &VMServiceClusterProxy{
		baseClusterProxy,
		"",
	}

	return proxy
}

// Create wraps `kubectl create ...` and prints the output so we can see what gets created to the cluster.
func (p *VMServiceClusterProxy) CreateWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Create")

	return framework.KubectlCreateWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
}

func (p *VMServiceClusterProxy) CreateRawWithArgs(ctx context.Context, resources []byte, args ...string) ([]byte, []byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Create")

	return framework.KubectlCreateRawWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
}

// Delete wraps `kubectl delete ...` and prints the output so we can see what gets deleted from the cluster.
func (p *VMServiceClusterProxy) DeleteWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Delete")

	return framework.KubectlDeleteWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
}

// Apply wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func (p *VMServiceClusterProxy) ApplyWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Apply")

	return framework.KubectlApplyWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
}

// Exec performs kubectl exec with following flags.
func (p *VMServiceClusterProxy) Exec(ctx context.Context, args ...string) ([]byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Exec")

	return framework.KubectlExec(ctx, p.GetKubeconfigPath(), args...)
}

// Label wraps `kubectl label ...` and prints the output so we can see what gets applied to the cluster.
func (p *VMServiceClusterProxy) Label(ctx context.Context, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Label")

	return framework.KubectlLabel(ctx, p.GetKubeconfigPath(), args...)
}

func getAPIServerAdminConf(ctx context.Context, svKubeConfig string) (string, error) {
	apiServerSSHCommandRunner, err := supervisor.GetAPIServerCommandRunner(ctx, svKubeConfig)
	if err != nil {
		return "", err
	}

	conf, err := apiServerSSHCommandRunner.RunCommand("cat /etc/kubernetes/admin.conf")
	if err != nil {
		return "", err
	}

	apiServerURL := kubectl.GetKubectlClusterForCurrentContext(ctx, svKubeConfig)

	parsedAPIServerURL, err := url.Parse(apiServerURL)
	if err != nil {
		return "", err
	}

	apiServerIP := parsedAPIServerURL.Hostname()

	e2eframework.Logf("Using %s for API server IP with admin.conf", apiServerIP)

	conf = bytes.Replace(conf, []byte("127.0.0.1"), []byte(apiServerIP), 1)

	f, err := os.CreateTemp("", "gce2e-admin.conf")
	if err != nil {
		return "", err
	}

	_, cerr := io.Copy(f, bytes.NewReader(conf))

	if err = f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}

	if cerr != nil {
		_ = os.Remove(f.Name())
		return "", cerr
	}

	return f.Name(), nil
}

func (p *VMServiceClusterProxy) NewAdminClusterProxy(ctx context.Context) (*VMServiceClusterProxy, error) {
	adminConfPath, err := getAPIServerAdminConf(ctx, p.GetKubeconfigPath())
	if err != nil {
		return nil, err
	}

	proxy := NewVMServiceClusterProxy("admin", adminConfPath, p.GetScheme())
	proxy.adminConfPath = adminConfPath

	return proxy, nil
}

func (p *VMServiceClusterProxy) GetAdminClient() (client.Client, error) {
	config := p.GetRESTConfig()

	// We replace 127.0.0.1 in admin.conf, but API server IP used may not be
	// one of the certificate IP SANs, causing TLS verify to fail.
	// Same as `kubectl --insecure-skip-tls-verify`
	config.Insecure = true
	config.CAData = nil

	return client.New(config, client.Options{Scheme: p.GetScheme()})
}

func (p *VMServiceClusterProxy) Dispose(ctx context.Context) {
	if p.adminConfPath != "" {
		_ = os.Remove(p.adminConfPath)
	}

	p.WCPClusterProxy.Dispose(ctx)
}

func (p *VMServiceClusterProxy) args(args []string) []string {
	if p.adminConfPath != "" {
		args = append(args, "--insecure-skip-tls-verify")
	}

	return args
}
