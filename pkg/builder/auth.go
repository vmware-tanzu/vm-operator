package builder

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	authv1 "k8s.io/api/authentication/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

type contextKey uint8

const (
	// RequestClientCertificateContextKey is the key used to store and extract
	// client cert data from http requests.
	RequestClientCertificateContextKey contextKey = iota

	// The path to ca cert used to validate client certs.
	caFilePath = "/tmp/k8s-webhook-server/serving-certs/client-ca/ca.crt"

	// The apiserver client CN.
	apiserverCN = "apiserver-webhook-client"
)

var caCertPool *x509.CertPool

func IsPrivilegedAccount(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	if ctx != nil {
		ctx.Logger.V(6).Info("Checking if account is privileged",
			"Webhook Name", ctx.Name,
			"Username", userInfo.Username,
			"UID", userInfo.UID,
			"Groups", userInfo.Groups,
			"Extra", userInfo.Extra)
	}

	return IsVMOperatorServiceAccount(ctx, userInfo) ||
		IsSystemMasters(ctx, userInfo) ||
		IsKubeAdmin(ctx, userInfo) ||
		InPrivilegedUsersList(ctx, userInfo)
}

func IsSystemMasters(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	// Per https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles,
	// any user that belongs to the group "system:masters" is a cluster-admin.
	for i := range userInfo.Groups {
		if userInfo.Groups[i] == "system:masters" {
			return true
		}
	}

	return false
}

func IsKubeAdmin(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	return strings.EqualFold(userInfo.Username, kubeAdminUser)
}

func IsVMOperatorServiceAccount(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	if ctx == nil {
		return false
	}

	serviceAccount := strings.Join(
		[]string{
			"system",
			"serviceaccount",
			ctx.Namespace,
			ctx.ServiceAccountName,
		}, ":")
	return strings.EqualFold(userInfo.Username, serviceAccount)
}

func InPrivilegedUsersList(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	if ctx == nil {
		return false
	}

	// Users specified by Pod's environment variable "PRIVILEGED_USERS" are
	// considered privileged.
	c := pkgcfg.FromContext(ctx)
	_, ok := pkgcfg.StringToSet(c.PrivilegedUsers)[userInfo.Username]
	return ok
}

func verifyPeerCertificate(connState *tls.ConnectionState) error {
	if err := loadCACert(caFilePath); err != nil {
		return err
	}

	if connState == nil || len(connState.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate provided")
	}

	// The first certificate is the leaf certificate.
	cert := connState.PeerCertificates[0]

	if cert.Subject.CommonName != apiserverCN {
		return fmt.Errorf("unauthorized client CN: %s", cert.Subject.CommonName)
	}

	opts := x509.VerifyOptions{
		Roots:         caCertPool,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Add intermediate if present
	for _, intermediate := range connState.PeerCertificates[1:] {
		opts.Intermediates.AddCert(intermediate)
	}

	_, err := cert.Verify(opts)
	return err
}

func loadCACert(path string) error {
	if caCertPool != nil {
		return nil
	}

	caCertPEM, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}

	caCertPool = x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCertPEM); !ok {
		return errors.New("failed to append CA certificate")
	}

	return nil
}

// contextWithClientCert augments the given context with the request's client certs.
func contextWithClientCert(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, RequestClientCertificateContextKey, r.TLS)
}

// clientCertFromContext returns the request's client cert stored in the given context.
func clientCertFromContext(ctx context.Context) (*tls.ConnectionState, error) {
	if v, ok := ctx.Value(RequestClientCertificateContextKey).(*tls.ConnectionState); ok {
		return v, nil
	}

	return nil, errors.New("no request client certificate found in context")
}

func VerifyWebhookRequest(ctx context.Context) error {
	// verify the request comes from apiserver only
	verifiedChains, err := clientCertFromContext(ctx)
	if err != nil {
		return err
	}
	return verifyPeerCertificate(verifiedChains)
}
