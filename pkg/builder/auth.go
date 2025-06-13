package builder

import (
	"strings"

	authv1 "k8s.io/api/authentication/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func IsPrivilegedAccount(
	ctx *pkgctx.WebhookContext,
	userInfo authv1.UserInfo) bool {

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
