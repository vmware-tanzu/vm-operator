// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package certman

import (
	"context"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/pkg/errors"
	admissionregv1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;patch

// New returns a new instance of a CertificateManager.
func New(options Options) (*CertificateManager, error) {
	if err := options.defaults(); err != nil {
		return nil, err
	}
	if err := options.validate(); err != nil {
		return nil, err
	}
	return &CertificateManager{opts: options}, nil
}

// CertificateManager is used to create and rotate the certificates required by
// a controller manager's webhook server.
type CertificateManager struct {
	opts Options
}

// Start activates the certificate manager server. This function does not return
// until the Done channel provided when creating the certificate manager is
// closed or an error occurs while rotating the certificates.
func (cm *CertificateManager) Start(done <-chan struct{}) error {
	ctx := context.Background()
	var rotationTime time.Time
	for {
		nextRotationTime, err := cm.rotateSecret(ctx, rotationTime)
		if err != nil {
			return err
		}
		cm.opts.Logger.Info("rotated secret", "next-rotation", nextRotationTime.String())
		rotationTime = nextRotationTime
		select {
		case <-time.After(time.Until(nextRotationTime)):
		case <-done:
			return nil
		}
	}
}

// CertDirReady returns a channel that is closed when there are certificates
// available in the configured certificate directory. If Options.CertDir is
// empty or the specified directory does not exist, then the returned channel
// is never closed.
func (cm *CertificateManager) CertDirReady() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		crtPath := path.Join(cm.opts.CertDir, ServerCertName)
		keyPath := path.Join(cm.opts.CertDir, ServerKeyName)
		for {
			if file, err := os.Stat(crtPath); err == nil {
				if file.Size() > 0 {
					if file, err := os.Stat(keyPath); err == nil {
						if file.Size() > 0 {
							close(done)
							return
						}
					}
				}
			}
			time.Sleep(time.Second * 1)
		}
	}()
	return done
}

func (cm *CertificateManager) rotateSecret(ctx context.Context, scheduledNextRotationTime time.Time) (time.Time, error) {
	// Determine now as early as possible.
	now := time.Now()
	cm.opts.Logger.Info("rotating certificates",
		"now", now.String(), "scheduled-rotation-time", scheduledNextRotationTime.String())

	// Get the webhook server's secret. Any error retrieving the secret is
	// cause for exiting the certificate manager server.
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: cm.opts.SecretNamespace, Name: cm.opts.SecretName}
	cm.opts.Logger.Info("getting secret",
		"secret-namespace", secretKey.Namespace, "secret-name", secretKey.Name)
	if err := cm.opts.Client.Get(ctx, secretKey, secret); err != nil {
		return time.Time{}, err
	}
	if len(secret.Annotations) == 0 {
		secret.Annotations = map[string]string{}
	}

	// If this is the first time rotateSecret was called since starting the
	// certificate manager, then initialize the webhook configurations with
	// the existing CA data (if any) in order to overwrite any reset CABundle
	// properties that may have been caused due to the generated webhook YAML.
	if scheduledNextRotationTime.IsZero() {
		if caCertData := secret.Data[CACertName]; len(caCertData) > 0 {
			if err := cm.updateWebhookConfigs(ctx, caCertData); err != nil {
				return time.Time{}, err
			}
		}
	}

	// If the secret already has a scheduled, next rotation time that does not
	// occur in the past, then reschedule this rotation since some other
	// certificate manager has already updated the secret.
	nextRotationTime, _ := cm.getNextRotationTime(secret)
	if !nextRotationTime.IsZero() && !nextRotationTime.Before(now) {
		if !scheduledNextRotationTime.Equal(nextRotationTime) {
			cm.opts.Logger.Info("rescheduling rotation early",
				"next-rotation-time", nextRotationTime.String())
			return nextRotationTime, nil
		}
	}

	// Determine the rotation interval.
	rotationInterval := defaultRotationInterval
	if value := secret.Annotations[cm.opts.RotationIntervalAnnotationKey]; value != "" {
		if interval, err := time.ParseDuration(value); err == nil {
			rotationInterval = interval
		}
	}
	cm.opts.Logger.Info("rotating certificates", "rotation-interval", rotationInterval.String())

	// Update the next rotation time.
	nextRotationTime = now.Add(rotationInterval)
	secret.Annotations[cm.opts.NextRotationAnnotationKey] = strconv.FormatInt(nextRotationTime.Unix(), 10)
	cm.opts.Logger.Info("rotating certificates", "next-rotation", nextRotationTime.String())

	// Increment the rotation count. If an error occurs just ignore it since
	// the rotation count is optional, primarily used for testing.
	var rotationCount int64
	if value := secret.Annotations[cm.opts.RotationCountAnnotationKey]; value != "" {
		rotationCount, _ = strconv.ParseInt(value, 10, 64)
	}
	rotationCount++
	secret.Annotations[cm.opts.RotationCountAnnotationKey] = strconv.FormatInt(rotationCount, 10)
	cm.opts.Logger.Info("rotating certificates", "rotation-count", rotationCount)

	// Update the secret data.
	notAfter := nextRotationTime.Add(certExpirationBuffer)
	cm.opts.Logger.Info("generating certificates")
	secretData, err := NewWebhookServerCertSecretData(notAfter, cm.opts.ServiceNamespace, cm.opts.ServiceName)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "failed to create new webhook server certificate secret data")
	}
	secret.Data = secretData

	// Update the secret on the API server.
	if nextRotationTime, err := cm.updateSecret(ctx, secret, secretKey); err != nil {
		if err == errRescheduled {
			return nextRotationTime, nil
		}
	}

	// Update the webhook configurations' CABundle properties with the updated
	// CA certificate.
	if err := cm.updateWebhookConfigs(ctx, secretData[CACertName]); err != nil {
		return time.Time{}, err
	}

	cm.opts.Logger.Info("rotated webhook server certificates")
	return nextRotationTime, nil
}

// errRescheduled is returned by updateSecret if a conflict occurs and an
// rescheduled rotation time is returned.
var errRescheduled = errors.New("rescheduled")

func (cm *CertificateManager) updateSecret(
	ctx context.Context,
	secret *corev1.Secret,
	secretKey client.ObjectKey) (time.Time, error) {

	cm.opts.Logger.Info("updating secret data",
		"secret-namespace", secretKey.Namespace, "secret-name", secretKey.Name)
	if err := cm.opts.Client.Update(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return time.Time{}, ErrSecretNotFound
		}

		// A conflict is an acceptable error because it means some other
		// controller manager has already updated the certificates in the
		// time since this function was entered.
		if apierrors.IsConflict(err) {

			cm.opts.Logger.Info("conflict occurred while updating secret data",
				"secret-namespace", secretKey.Namespace, "secret-name", secretKey.Name)
			// Since the secret was updated by someone else, we need to get the
			// secret in order to discover the actual next-rotation value.

			cm.opts.Logger.Info("getting secret again",
				"secret-namespace", secretKey.Namespace, "secret-name", secretKey.Name)
			if err := cm.opts.Client.Get(ctx, secretKey, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return time.Time{}, ErrSecretNotFound
				}
				return time.Time{}, err
			}

			nextRotationTime, err := cm.getNextRotationTime(secret)
			if err != nil {
				return time.Time{}, err
			}
			cm.opts.Logger.Info("rescheduling rotation after conflict",
				"next-rotation-time", nextRotationTime.String())

			// Return the next-rotation value.
			return nextRotationTime, errRescheduled
		}
		return time.Time{}, err
	}

	cm.opts.Logger.Info("rotated certificates")
	return time.Time{}, nil
}

func (cm *CertificateManager) updateWebhookConfigs(ctx context.Context, caCertData []byte) error {
	cm.opts.Logger.Info("updating webhook configs", "caCertData", string(caCertData))

	// Get a list of the validating webhook configurations that match
	// the provided label key.
	validatingWebhookConfigList := &admissionregv1.ValidatingWebhookConfigurationList{}
	matchWebhookConfigLabels := client.MatchingLabels(map[string]string{
		cm.opts.WebhookConfigLabelKey: webhookConfigLabelValue,
	})
	if err := cm.opts.Client.List(ctx, validatingWebhookConfigList, matchWebhookConfigLabels); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to get list of validating webhooks")
		}
	}

	// Update the validating webhook configurations with the new CA.
	for i := range validatingWebhookConfigList.Items {
		webhookConfig := validatingWebhookConfigList.Items[i]
		for i := range webhookConfig.Webhooks {
			cm.opts.Logger.Info("Updating validating webhook", "name", webhookConfig.Webhooks[i].Name)
			webhookConfig.Webhooks[i].ClientConfig.CABundle = caCertData
		}
		if err := cm.opts.Client.Update(ctx, &webhookConfig); err != nil {
			cm.opts.Logger.Error(err, "failed to update validating webhook configuration")
			if !apierrors.IsConflict(err) {
				return errors.Wrapf(
					err,
					"failed to update validating webhook configuration %s/%s",
					webhookConfig.Namespace, webhookConfig.Name)
			}
		}
	}

	// Get a list of the mutating webhook configurations that match the provided label key
	mutatingWebhookConfigList := &admissionregv1.MutatingWebhookConfigurationList{}
	if err := cm.opts.Client.List(ctx, mutatingWebhookConfigList, matchWebhookConfigLabels); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to get list of mutating webhooks")
		}
	}

	// Update the mutating webhook configurations with the new CA.
	for i := range mutatingWebhookConfigList.Items {
		webhookConfig := mutatingWebhookConfigList.Items[i]
		for i := range webhookConfig.Webhooks {
			cm.opts.Logger.Info("Updating mutating webhook", "name", webhookConfig.Webhooks[i].Name)
			webhookConfig.Webhooks[i].ClientConfig.CABundle = caCertData
		}
		if err := cm.opts.Client.Update(ctx, &webhookConfig); err != nil {
			cm.opts.Logger.Error(err, "failed to update mutating webhook configuration")
			if !apierrors.IsConflict(err) {
				return errors.Wrapf(
					err,
					"failed to update mutating webhook configuration %s/%s",
					webhookConfig.Namespace, webhookConfig.Name)
			}
		}
	}

	return nil
}

func (cm *CertificateManager) getNextRotationTime(secret *corev1.Secret) (time.Time, error) {
	// Get the next-rotation annotation
	szValue := secret.Annotations[cm.opts.NextRotationAnnotationKey]
	if szValue == "" {
		return time.Time{}, ErrMissingNextRotation
	}

	// The error is ignored since the value will always be >0.
	iValue, _ := strconv.ParseInt(szValue, 10, 64)
	if iValue <= 0 {
		return time.Time{}, ErrInvalidNextRotation
	}

	// Return the next-rotation value.
	return time.Unix(iValue, 0), nil
}
