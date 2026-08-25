package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/kubectl"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/supervisor"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// isRetryableKubectlError reports whether stdout/stderr/err look like a
// transient connectivity failure worth retrying, as opposed to a real error
// from kubectl or the API server. Shares its classification (and retry
// interval/timeout, see retryKubectlOnTransientError below) with the
// controller-runtime client retry in framework.NewRetryableClient, so both
// paths recognize the same class of outage.
func isRetryableKubectlError(stdout, stderr []byte, err error) bool {
	if err == nil {
		return false
	}

	return framework.IsRetryableTransientErrString(string(stdout) + " " + string(stderr) + " " + err.Error())
}

// alreadySucceededFunc reports whether a retried write actually landed on a
// previous attempt before its connection dropped, e.g. kubectl create sees
// AlreadyExists, kubectl delete sees NotFound.
//
// These predicates assume a single-document manifest (no "---" separators),
// which holds for every caller of CreateWithArgs/DeleteWithArgs today -- see
// manifestbuilders. If a multi-document manifest is ever passed through
// here, a benign AlreadyExists/NotFound for one document could mask a real
// failure on another, since kubectl folds all documents' stderr together
// and this only checks that ANY of it matches.
type alreadySucceededFunc func(stdout, stderr []byte) bool

func kubectlCreateAlreadySucceeded(_, stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "AlreadyExists") || strings.Contains(s, "already exists")
}

func kubectlDeleteAlreadySucceeded(_, stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "NotFound") || strings.Contains(s, "not found")
}

// retryKubectlOnTransientError runs fn, retrying every
// framework.TransientRetryInterval for up to framework.TransientRetryTimeout,
// but only while fn's error is classified as a transient connectivity
// failure by isRetryableKubectlError. Any other error is returned
// immediately.
//
// succeeded, when non-nil, is checked on attempts after the first: if fn
// fails but succeeded reports the write already landed (e.g. AlreadyExists
// on a retried create, NotFound on a retried delete), the earlier failure is
// treated as success rather than surfaced as a hard error -- the object may
// have been persisted before the connection reporting the failure dropped.
// On the first attempt, that same signal is a real failure (a name
// collision, or a delete target that never existed) and is not suppressed.
func retryKubectlOnTransientError(
	ctx context.Context,
	op string,
	succeeded alreadySucceededFunc,
	fn func() (stdout, stderr []byte, err error),
) ([]byte, []byte, error) {
	var (
		stdout, stderr []byte
		err            error
		attempt        int
	)

	start := time.Now()

	pollErr := wait.PollUntilContextTimeout(ctx, framework.TransientRetryInterval, framework.TransientRetryTimeout, true,
		func(pollCtx context.Context) (bool, error) {
			attempt++

			if pollCtx.Err() != nil {
				err = pollCtx.Err()
				return false, err
			}

			stdout, stderr, err = fn()
			if err == nil {
				return true, nil
			}

			if attempt > 1 && succeeded != nil && succeeded(stdout, stderr) {
				e2eframework.Logf("kubectl %s: attempt %d landed before a transient error was reported, treating as success: %v",
					op, attempt, err)
				err = nil
				return true, nil
			}

			if !isRetryableKubectlError(stdout, stderr, err) {
				return false, err
			}

			e2eframework.Logf("kubectl %s: attempt %d failed with a transient error, retrying in %s (elapsed %s): %v\nstderr: %s",
				op, attempt, framework.TransientRetryInterval, time.Since(start).Round(time.Second), err, string(stderr))

			return false, nil
		})

	switch {
	case err != nil:
		return stdout, stderr, err
	case pollErr != nil:
		return stdout, stderr, pollErr
	default:
		return stdout, stderr, nil
	}
}

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

// Create wraps `kubectl create` and prints the output so we can see what gets created to the cluster.
// Overrides the embedded WCPClusterProxy.Create so this also gets the retry behavior of CreateWithArgs.
func (p *VMServiceClusterProxy) Create(ctx context.Context, resources []byte) error {
	return p.CreateWithArgs(ctx, resources)
}

// CreateWithArgs wraps `kubectl create ...` and prints the output so we can see what gets created to
// the cluster. It retries on transient connectivity errors (e.g. the conversion webhook being briefly
// unreachable); see retryKubectlOnTransientError for the retry and classification rules.
func (p *VMServiceClusterProxy) CreateWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Create")

	stdout, stderr, err := retryKubectlOnTransientError(ctx, "create", kubectlCreateAlreadySucceeded,
		func() ([]byte, []byte, error) {
			return framework.KubectlCreateRawWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
		})
	if err != nil {
		fmt.Println(string(stderr))
		return fmt.Errorf("%w: %s", err, string(stderr))
	}

	fmt.Println(string(stdout))

	return nil
}

// CreateRawWithArgs is the non-retrying counterpart to CreateWithArgs: it returns stdout/stderr/err
// as-is from a single attempt, for callers that need to inspect an expected failure (e.g. asserting
// on a specific admission error message) rather than have it retried away.
func (p *VMServiceClusterProxy) CreateRawWithArgs(ctx context.Context, resources []byte, args ...string) ([]byte, []byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Create")

	return framework.KubectlCreateRawWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
}

// Delete wraps `kubectl delete` and prints the output so we can see what gets deleted from the cluster.
// Overrides the embedded WCPClusterProxy.Delete so this also gets the retry behavior of DeleteWithArgs.
func (p *VMServiceClusterProxy) Delete(ctx context.Context, resources []byte) error {
	return p.DeleteWithArgs(ctx, resources)
}

// DeleteWithArgs wraps `kubectl delete ...` and prints the output so we can see what gets deleted from
// the cluster. It retries on transient connectivity errors (e.g. the conversion webhook being briefly
// unreachable); see retryKubectlOnTransientError for the retry and classification rules.
func (p *VMServiceClusterProxy) DeleteWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Delete")

	stdout, stderr, err := retryKubectlOnTransientError(ctx, "delete", kubectlDeleteAlreadySucceeded,
		func() ([]byte, []byte, error) {
			return framework.KubectlRawWithArgs(ctx, p.GetKubeconfigPath(), "delete", resources, p.args(args)...)
		})
	if err != nil {
		fmt.Println(string(stderr))
		return fmt.Errorf("%w: %s", err, string(stderr))
	}

	fmt.Println(string(stdout))

	return nil
}

// ApplyWithArgs wraps `kubectl apply ...` and prints the output so we can see what gets applied to the
// cluster. It retries on transient connectivity errors (e.g. the conversion webhook being briefly
// unreachable); see retryKubectlOnTransientError for the retry and classification rules. Apply is
// naturally idempotent, so no "already succeeded" check is needed.
func (p *VMServiceClusterProxy) ApplyWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeEmpty(), "resources is required for Apply")

	stdout, stderr, err := retryKubectlOnTransientError(ctx, "apply", nil,
		func() ([]byte, []byte, error) {
			return framework.KubectlApplyRawWithArgs(ctx, p.GetKubeconfigPath(), resources, p.args(args)...)
		})
	if err != nil {
		fmt.Println(string(stderr))
		return fmt.Errorf("%w: %s", err, string(stderr))
	}

	fmt.Println(string(stdout))

	return nil
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

// GetAdminClient returns a controller-runtime client for the cluster using the
// admin identity. Like GetClient, the returned client retries on transient
// connectivity errors; see framework.NewRetryableClient.
func (p *VMServiceClusterProxy) GetAdminClient() (client.Client, error) {
	config := p.GetRESTConfig()

	// We replace 127.0.0.1 in admin.conf, but API server IP used may not be
	// one of the certificate IP SANs, causing TLS verify to fail.
	// Same as `kubectl --insecure-skip-tls-verify`
	config.Insecure = true
	config.CAData = nil

	c, err := client.New(config, client.Options{Scheme: p.GetScheme()})
	if err != nil {
		return nil, err
	}

	return framework.NewRetryableClient(c), nil
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
