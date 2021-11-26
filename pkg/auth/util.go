// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"os"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
)

const (
	KubeAdminUser = "kubernetes-admin"
)

// IsPODServiceAccountUser checks if user is POD Service Account.
func IsPODServiceAccountUser(userInfo authv1.UserInfo) bool {
	serviceAccountUserName := strings.Join(
		[]string{"system", "serviceaccount", os.Getenv("POD_NAMESPACE"), os.Getenv("POD_SERVICE_ACCOUNT_NAME")}, ":")
	return strings.EqualFold(userInfo.Username, serviceAccountUserName)
}

// IsKubernetesAdmin checks if the user is Kubernetes Administrator.
func IsKubernetesAdmin(userInfo authv1.UserInfo) bool {
	return strings.EqualFold(userInfo.Username, KubeAdminUser)
}
