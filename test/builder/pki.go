// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

type pkiToolchain struct {
	caPublicKeyPEM  []byte
	caPrivateKeyPEM []byte
	publicKeyPEM    []byte
	privateKeyPEM   []byte
}

// generatePKIToolchain creates a certificate authority and uses it to sign
// a certificate for the webhook service.
// All generates certificates expire in one hour from time of creation.
func generatePKIToolchain() (pkiToolchain, error) {

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 1)
	subject := pkix.Name{
		Organization:  []string{"fake"},
		Country:       []string{"fake"},
		Province:      []string{"fake"},
		Locality:      []string{"fake"},
		StreetAddress: []string{"fake"},
		PostalCode:    []string{"fake"},
		CommonName:    "fake",
	}

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return pkiToolchain{}, err
	}

	caData, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return pkiToolchain{}, err
	}

	caCertPEM := &bytes.Buffer{}
	if err := pem.Encode(caCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caData,
	}); err != nil {
		return pkiToolchain{}, err
	}

	caPrivateKeyPEM := &bytes.Buffer{}
	if err := pem.Encode(caPrivateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivateKey),
	}); err != nil {
		return pkiToolchain{}, err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      subject,
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return pkiToolchain{}, err
	}

	certData, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivateKey.PublicKey, certPrivateKey)
	if err != nil {
		return pkiToolchain{}, err
	}

	certPEM := &bytes.Buffer{}
	if err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certData,
	}); err != nil {
		return pkiToolchain{}, err
	}

	certPrivateKeyPEM := &bytes.Buffer{}
	if err := pem.Encode(certPrivateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivateKey),
	}); err != nil {
		return pkiToolchain{}, err
	}

	return pkiToolchain{
		caPublicKeyPEM:  caCertPEM.Bytes(),
		caPrivateKeyPEM: caPrivateKeyPEM.Bytes(),
		publicKeyPEM:    certPEM.Bytes(),
		privateKeyPEM:   certPrivateKeyPEM.Bytes(),
	}, nil
}
