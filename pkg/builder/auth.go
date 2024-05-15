package builder

import (
	"strings"

	authv1 "k8s.io/api/authentication/v1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func IsPrivilegedAccount(
	ctx *pkgctx.WebhookContext, userInfo authv1.UserInfo) bool {

	// Per https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles,
	// any user that belongs to the group "system:masters" is a cluster-admin.
	for i := range userInfo.Groups {
		if userInfo.Groups[i] == "system:masters" {
			return true
		}
	}

	username := userInfo.Username
	if strings.EqualFold(username, kubeAdminUser) {
		return true
	}

	if ctx == nil {
		return false
	}

	// Users specified by Pod's environment variable "PRIVILEGED_USERS" are
	// considered privileged.
	c := pkgcfg.FromContext(ctx)
	if _, ok := pkgcfg.StringToSet(c.PrivilegedUsers)[username]; ok {
		return true
	}

	serviceAccount := strings.Join(
		[]string{
			"system",
			"serviceaccount",
			ctx.Namespace,
			ctx.ServiceAccountName,
		}, ":")
	return strings.EqualFold(username, serviceAccount)
}
