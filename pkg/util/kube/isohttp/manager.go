// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package isohttp

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	containerName = "iso-httpserver"
	containerPort = 8080
	servicePort   = 80

	// dataRootDir is the root directory under which every referenced
	// Secret is mounted, one subdirectory per Secret name, so that the
	// server can serve /<secretName>/<key> as /data/<secretName>/<key>.
	dataRootDir = "/data"
)

// Manager creates and tears down the ephemeral HTTP server used to serve
// spec.bootstrap.iso.assets to a VM during automated ISO installation.
//
// The Pod and Service names are derived deterministically from the VM's own
// name -- callers never supply or see the name, since it is purely an
// implementation detail of how Assets are made available to the VM.
type Manager struct {
	client ctrlclient.Client
	vm     *vmopv1.VirtualMachine
}

// NewManager returns a Manager that creates and tears down the ephemeral
// bootstrap HTTP server for vm.
func NewManager(client ctrlclient.Client, vm *vmopv1.VirtualMachine) *Manager {
	return &Manager{client: client, vm: vm}
}

// ErrAssetNotFound indicates a Secret or key referenced by
// spec.bootstrap.iso.assets could not be found.
type ErrAssetNotFound struct {
	SecretName string
	Key        string
	Err        error
}

func (e *ErrAssetNotFound) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("secret %q not found", e.SecretName)
	}
	return fmt.Sprintf("key %q not found in secret %q", e.Key, e.SecretName)
}

func (e *ErrAssetNotFound) Unwrap() error {
	return e.Err
}

// objectName returns the deterministic name shared by the Pod and Service
// created for vm.
func objectName(vm *vmopv1.VirtualMachine) string {
	return vm.Name + "-iso-bootstrap"
}

// EnsureReady validates the Secrets/keys referenced by
// spec.bootstrap.iso.assets, then creates (or, on subsequent calls,
// idempotently confirms) the ephemeral Pod and Service.
//
// EnsureReady never blocks waiting for the Service to be assigned an
// address: if one is not yet available, ready is false and err is nil --
// callers should requeue and call EnsureReady again later.
func (m *Manager) EnsureReady(ctx context.Context) (address string, port int, ready bool, err error) {
	secretNames, err := m.validateAssets(ctx)
	if err != nil {
		return "", 0, false, err
	}

	if err := m.ensurePod(ctx, secretNames); err != nil {
		return "", 0, false, fmt.Errorf("failed to ensure iso bootstrap pod: %w", err)
	}

	svc, err := m.ensureService(ctx)
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to ensure iso bootstrap service: %w", err)
	}

	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return "", 0, false, nil
	}

	ingress := svc.Status.LoadBalancer.Ingress[0]
	address = ingress.IP
	if address == "" {
		address = ingress.Hostname
	}
	if address == "" {
		return "", 0, false, nil
	}

	return address, servicePort, true, nil
}

// Teardown deletes the Pod and Service created for this VM, if they exist.
func (m *Manager) Teardown(ctx context.Context) error {
	name := objectName(m.vm)

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: m.vm.Namespace}}
	if err := m.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete iso bootstrap pod: %w", err)
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: m.vm.Namespace}}
	if err := m.client.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete iso bootstrap service: %w", err)
	}

	return nil
}

// validateAssets confirms every Secret and key referenced by
// spec.bootstrap.iso.assets exists, returning the sorted, de-duplicated
// list of referenced Secret names.
func (m *Manager) validateAssets(ctx context.Context) ([]string, error) {
	iso := m.vm.Spec.Bootstrap.ISO
	if iso == nil {
		return nil, nil
	}

	secrets := map[string]*corev1.Secret{}

	for _, asset := range iso.Assets {
		secret, ok := secrets[asset.Name]
		if !ok {
			secret = &corev1.Secret{}
			key := ctrlclient.ObjectKey{Namespace: m.vm.Namespace, Name: asset.Name}
			if err := m.client.Get(ctx, key, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return nil, &ErrAssetNotFound{SecretName: asset.Name, Err: err}
				}
				return nil, fmt.Errorf("failed to get secret %q: %w", asset.Name, err)
			}
			secrets[asset.Name] = secret
		}

		if _, ok := secret.Data[asset.Key]; !ok {
			return nil, &ErrAssetNotFound{SecretName: asset.Name, Key: asset.Key}
		}
	}

	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)

	return names, nil
}

// ensurePod creates or confirms the ephemeral HTTP server Pod, mounting
// each of secretNames as its own volume under dataRootDir.
func (m *Manager) ensurePod(ctx context.Context, secretNames []string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName(m.vm),
			Namespace: m.vm.Namespace,
		},
	}

	image := pkgcfg.FromContext(ctx).ISOHTTPServerImage

	_, err := controllerutil.CreateOrPatch(ctx, m.client, pod, func() error {
		if err := controllerutil.SetControllerReference(m.vm, pod, m.client.Scheme()); err != nil {
			return err
		}

		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}
		pod.Labels[pkgconst.ISOBootstrapVMLabelKey] = m.vm.Name

		// A Pod's spec is almost entirely immutable after creation, so it
		// is only ever set here, never patched on an existing Pod.
		if pod.ResourceVersion == "" {
			pod.Spec = newPodSpec(secretNames, image)
		}

		return nil
	})

	return err
}

// newPodSpec returns the PodSpec for the ephemeral HTTP server, mounting
// each of secretNames as its own read-only volume under dataRootDir.
func newPodSpec(secretNames []string, image string) corev1.PodSpec {
	volumes := make([]corev1.Volume, 0, len(secretNames))
	mounts := make([]corev1.VolumeMount, 0, len(secretNames))

	for i, name := range secretNames {
		volName := fmt.Sprintf("secret-%d", i)
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: name},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: dataRootDir + "/" + name,
			ReadOnly:  true,
		})
	}

	return corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyNever,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			RunAsUser:    ptr.To(int64(65532)),
			RunAsGroup:   ptr.To(int64(65532)),
			FSGroup:      ptr.To(int64(65532)),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		Volumes: volumes,
		Containers: []corev1.Container{
			{
				Name:  containerName,
				Image: image,
				Env: []corev1.EnvVar{
					{Name: "SERVER_ROOT", Value: dataRootDir},
					{Name: "SERVER_BIND_ADDRESS", Value: "0.0.0.0"},
					{Name: "SERVER_PORT", Value: fmt.Sprintf("%d", containerPort)},
				},
				Ports: []corev1.ContainerPort{
					{ContainerPort: containerPort, Protocol: corev1.ProtocolTCP},
				},
				VolumeMounts: mounts,
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   ptr.To(true),
					AllowPrivilegeEscalation: ptr.To(false),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{"ALL"},
					},
				},
			},
		},
	}
}

// ensureService creates or confirms the Service fronting the ephemeral HTTP
// server Pod, returning its current state (including Status).
func (m *Manager) ensureService(ctx context.Context) (*corev1.Service, error) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectName(m.vm),
			Namespace: m.vm.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, m.client, svc, func() error {
		if err := controllerutil.SetControllerReference(m.vm, svc, m.client.Scheme()); err != nil {
			return err
		}

		if svc.Labels == nil {
			svc.Labels = map[string]string{}
		}
		svc.Labels[pkgconst.ISOBootstrapVMLabelKey] = m.vm.Name

		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.Selector = map[string]string{
			pkgconst.ISOBootstrapVMLabelKey: m.vm.Name,
		}
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Protocol:   corev1.ProtocolTCP,
				Port:       servicePort,
				TargetPort: intstr.FromInt32(containerPort),
			},
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return svc, nil
}
