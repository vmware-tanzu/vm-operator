// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Options defines the configuration used to create a new certificate manager.
type Options struct {
	// Client is used by the certificate manager to update the webhook secret.
	Client client.Client

	// CertDir is the path on the local filesystem where the certificates
	// should be created once the kubelet synchronizes the mounted secret's
	// contents. This value is only required if wanting to use the
	// CertDirReady() function that returns a channel that is closed when the
	// configured certificates are available locally.
	CertDir string

	// Logger is used by the certificate manager to emit log events.
	Logger logr.Logger

	// WebhookConfigLabelKey is the label used to search for a
	// k8s.io/api/admissionregistration/v1beta1.ValidatingWebhookConfigurationList
	// that contains the webhook configurations the certificate manager updates
	// with a generated certificate authority.
	// The value of the label must be set to "true".
	WebhookConfigLabelKey string

	// RotationIntervalAnnotationKey specifies the annotation on the
	// webhook server secret parseable by time.ParseDuration and controls
	// how often certificates are rotated.
	//
	// The generated certificates have their NotAfter property assigned
	// a value 30 minutes greater than RotationInterval. In other words,
	// if RotationInterval=6h, then the certificates expire after 6h30m.
	// This is to ensure a buffer between the generation of new
	// certificates and when the old ones expire.
	//
	// If the annotation is missing then the rotation interval defaults to
	// six hours.
	RotationIntervalAnnotationKey string

	// RotationCountAnnotationKey specifies an annotation on the webhook server
	// secret. The annotation's value is the number of times the certificates
	// have been rotated.
	RotationCountAnnotationKey string

	// NextRotationAnnotationKey specifies the annotation on the webhook server
	// secret and is the UNIX epoch which indicates when the next rotation
	// will occur.
	NextRotationAnnotationKey string

	// SecretNamespace is the namespace in which the Secret resource that
	// contains the webhook server's certificate data is located.
	SecretNamespace string

	// SecretName is the name of the Secret resource that contains the webhook
	// server's certificate data.
	SecretName string

	// ServiceNamespace is the namespace in which the webhook Service resource
	// is located.
	ServiceNamespace string

	// ServiceName is the name of the webhook Service.
	ServiceName string
}

func (o *Options) defaults() error {
	if o.Client == nil {
		restConfig, err := config.GetConfig()
		if err != nil {
			return err
		}
		c, err := client.New(restConfig, client.Options{})
		if err != nil {
			return err
		}
		o.Client = c
	}
	if o.Logger == nil {
		o.Logger = logf.Log
	}
	return nil
}

func (o *Options) validate() error {
	return nil
}
