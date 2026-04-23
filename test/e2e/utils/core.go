// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetPod(ctx context.Context, client ctrlclient.Client, ns, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, pod)
	if err != nil {
		return nil, err
	}

	return pod, nil
}

func GetDeployment(ctx context.Context, client ctrlclient.Client, ns, name string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, deployment)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

func GetSecret(ctx context.Context, client ctrlclient.Client, ns, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// GetConfigMap is utility function to get a ConfigMap.
func GetConfigMap(ctx context.Context, client ctrlclient.Client, ns, name string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, configMap)
	if err != nil {
		return nil, err
	}

	return configMap, nil
}
