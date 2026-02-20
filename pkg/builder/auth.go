// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	authv1 "k8s.io/api/authentication/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

type contextKey uint8

const (
	// RequestClientCertificateContextKey is the key used to store and extract
	// client cert data from http requests.
	RequestClientCertificateContextKey contextKey = iota

	// The apiserver client CN.
	apiserverCN = "apiserver-webhook-client"
)

var caCertPool atomic.Pointer[x509.CertPool]

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

// serviceAccountRx matches a user name that is a service account.
//
// The first grouped match is the domain name.
// The second grouped match is the user name.
var serviceAccountRx = regexp.MustCompile(`^system:serviceaccount:([^:]+):([^:]+)$`)

func InPrivilegedUsersList(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

	if ctx == nil {
		return false
	}

	// Users specified by Pod's environment variable "PRIVILEGED_USERS" are
	// considered privileged.
	privUsers := pkgcfg.StringToSlice(pkgcfg.FromContext(ctx).PrivilegedUsers)

	// Check if the authenticating user is a service account.
	authUserParts := serviceAccountRx.FindStringSubmatch(userInfo.Username)

	for _, privUser := range privUsers {

		// Determine if the current privileged user is a service account.
		privUserParts := serviceAccountRx.FindStringSubmatch(privUser)

		switch {

		case len(authUserParts) == 0 && len(privUserParts) == 0:
			//
			// Neither the authenticating user nor the current privileged user
			// is a service account, so compare the user names directly.
			//
			if userInfo.Username == privUser {
				return true
			}

		case len(authUserParts) > 0 && len(privUserParts) > 0:
			//
			// Both the authenticating user and the current privileged user are
			// service accounts, so compare the names more carefully.
			//
			var (
				authUserNamespace = authUserParts[1]
				authUserName      = authUserParts[2]
				privUserNamespace = privUserParts[1]
				privUserName      = privUserParts[2]
			)

			if strings.HasSuffix(privUserNamespace, "-HASH") {
				//
				// The privileged user namespace ends with -HASH, indicating it
				// should match any namespace that begins with the string's
				// prefix and ends with five alpha-numeric characters.
				//

				p := privUserNamespace[:len(privUserNamespace)-5]
				p = regexp.QuoteMeta(p)
				p = `^` + p + `\-[a-zA-Z0-9]{5}$`
				if ok, _ := regexp.MatchString(p, authUserNamespace); ok {
					if authUserName == privUserName {
						return true
					}
				}
			} else { //nolint:gocritic
				//
				// The privileged user namespace does *not* end with -HASH,
				// which means the authenticating user's namespace and name
				// should be compared literally to the privileged user's
				// namespace and name.
				//

				if authUserNamespace == privUserNamespace &&
					authUserName == privUserName {

					return true
				}
			}
		}
	}

	return false
}

func verifyPeerCertificate(
	ctx context.Context,
	connState *tls.ConnectionState) error {

	if connState == nil || len(connState.PeerCertificates) == 0 {
		return fmt.Errorf("no client certificate provided")
	}

	// The first certificate is the leaf certificate.
	cert := connState.PeerCertificates[0]
	if cert.Subject.CommonName != apiserverCN {
		// TODO: Check this after Verify()?
		return fmt.Errorf("unauthorized client CN: %s", cert.Subject.CommonName)
	}

	pool, err := getCACertPool(ctx)
	if err != nil {
		return err
	}

	opts := x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Add intermediates if present.
	if len(connState.PeerCertificates) > 1 {
		opts.Intermediates = x509.NewCertPool()
		for _, intermediate := range connState.PeerCertificates[1:] {
			opts.Intermediates.AddCert(intermediate)
		}
	}

	if _, err = cert.Verify(opts); err != nil {
		pemBlock := &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}
		pemBytes := pem.EncodeToMemory(pemBlock)
		pkglog.FromContextOrDefault(ctx).Error(err, "error verifying client certificate",
			"clientCert", base64.StdEncoding.EncodeToString(pemBytes),
			"peerCertCnt", len(connState.PeerCertificates))
		return err
	}

	return nil
}

func getCACertPool(ctx context.Context) (*x509.CertPool, error) {
	if certPool := caCertPool.Load(); certPool != nil {
		return certPool, nil
	}

	certDir := pkgcfg.FromContext(ctx).WebhookSecretVolumeMountPath
	if certDir == "" {
		certDir = pkgcfg.Default().WebhookSecretVolumeMountPath
	}
	caFilePath := filepath.Clean(filepath.Join(certDir, "client-ca", "ca.crt"))

	caCertPEM, err := os.ReadFile(caFilePath)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
		return nil, errors.New("failed to append CA certificate")
	}

	if caCertPool.CompareAndSwap(nil, certPool) {
		return certPool, nil
	}

	return caCertPool.Load(), nil
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

	return verifyPeerCertificate(ctx, verifiedChains)
}
