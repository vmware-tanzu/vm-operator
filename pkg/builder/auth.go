package builder

import (
	"strings"

	authv1 "k8s.io/api/authentication/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func isPrivilegedAccount(ctx *context.WebhookContext, userInfo authv1.UserInfo) bool {
	username := userInfo.Username

	if strings.EqualFold(username, kubeAdminUser) {
		return true
	}

	serviceAccount := strings.Join([]string{"system", "serviceaccount", ctx.Namespace, ctx.ServiceAccountName}, ":")
	return strings.EqualFold(username, serviceAccount)
}
