// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman

import (
	"time"
)

const (
	// CAKeyName is the name of the CA private key
	CAKeyName = "ca.key"
	// CACertName is the name of the CA certificate
	CACertName = "ca.crt"
	// ServerKeyName is the name of the server private key
	ServerKeyName = "tls.key"
	// ServerCertName is the name of the serving certificate
	ServerCertName = "tls.crt"

	// rsaKeySize is the size of the generated private keys.
	rsaKeySize = 2048

	// defaultRotationInterval is the default interval at which certificates
	// are rotated. This value is used if the webhook server secret is missing
	// the annotation that specifies the rotation interval.
	defaultRotationInterval = time.Hour * 6

	// certExpirationBuffer specifies the amount of time in addition to the
	// rotation interval that generated certificates expire.
	certExpirationBuffer = time.Minute * 30

	// webhookConfigLabelValue is the value that a label must be assigned to
	// indicate a webhook configuration is managed by a certificate manager.
	webhookConfigLabelValue = "true"

	// pemTypeCertificate is the type used when encoding public keys as PEM
	// data.
	pemTypeCertificate = "CERTIFICATE"
	// pemTypeRSAPrivateKey is the type used when encoding private keys as PEM
	// data.
	pemTypeRSAPrivateKey = "RSA PRIVATE KEY"
)
