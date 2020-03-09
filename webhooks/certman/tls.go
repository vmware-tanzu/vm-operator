// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// NewWebhookServerCertSecretData returns a map with the data that belongs
// to the secret supplying the webhook server its CA and certificate.
func NewWebhookServerCertSecretData(notAfter time.Time, serviceNamespace, serviceName string, additionalDNSNames ...string) (map[string][]byte, error) {
	notBefore := time.Now()

	caPrivateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, err
	}
	caSerial := newSerial(time.Now())
	caTemplate := &x509.Certificate{
		SerialNumber: caSerial,
		Subject: pkix.Name{
			CommonName:   "self-signed CA",
			SerialNumber: caSerial.String(),
		},
		NotBefore:             notBefore.UTC(),
		NotAfter:              notAfter.UTC(),
		SubjectKeyId:          bigIntHash(caPrivateKey.N),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}
	ca, _ := x509.ParseCertificate(caBytes)

	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  pemTypeCertificate,
		Bytes: ca.Raw,
	})
	caPrivateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  pemTypeRSAPrivateKey,
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivateKey),
	})

	certSerial := newSerial(time.Now())
	certPrivateKey, err := rsa.GenerateKey(rand.Reader, rsaKeySize)
	if err != nil {
		return nil, err
	}
	certTemplate := &x509.Certificate{
		SerialNumber: certSerial,
		Subject:      pkix.Name{CommonName: serviceName},
		DNSNames:     serviceNames(serviceNamespace, serviceName),
		NotBefore:    notBefore.UTC(),
		NotAfter:     notAfter.UTC(),
		SubjectKeyId: bigIntHash(certPrivateKey.N),
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageDataEncipherment |
			x509.KeyUsageKeyEncipherment |
			x509.KeyUsageContentCommitment,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, certTemplate, ca, &certPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return nil, err
	}
	cert, _ := x509.ParseCertificate(certBytes)

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  pemTypeCertificate,
		Bytes: cert.Raw,
	})
	certPrivateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  pemTypeRSAPrivateKey,
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivateKey),
	})

	return map[string][]byte{
		CACertName:     caCertPEM,
		CAKeyName:      caPrivateKeyPEM,
		ServerCertName: certPEM,
		ServerKeyName:  certPrivateKeyPEM,
	}, nil
}

func newSerial(now time.Time) *big.Int {
	return big.NewInt(int64(now.Nanosecond()))
}

func bigIntHash(n *big.Int) []byte {
	h := sha256.New()
	_, _ = h.Write(n.Bytes())
	return h.Sum(nil)
}

func serviceNames(serviceNamespace, serviceName string) []string {
	return []string{
		serviceName,
		fmt.Sprintf("%s.%s", serviceName, serviceNamespace),
		fmt.Sprintf("%s.%s.svc", serviceName, serviceNamespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, serviceNamespace),
	}
}
