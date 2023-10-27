package builder

import (
	"strings"

	authv1 "k8s.io/api/authentication/v1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

func IsPrivilegedAccount(ctx *context.WebhookContext, userInfo authv1.UserInfo) bool {
	username := userInfo.Username

	if strings.EqualFold(username, kubeAdminUser) {
		return true
	}

	// Users specified by Pod's environment variable "PRIVILEGED_USERS" are considered privileged.
	if lib.IsVMServiceBackupRestoreFSSEnabled() {
		privUsers := lib.GetPrivilegedUsers()
		if _, ok := privUsers[username]; ok {
			return true
		}
	}

	serviceAccount := strings.Join([]string{"system", "serviceaccount", ctx.Namespace, ctx.ServiceAccountName}, ":")
	return strings.EqualFold(username, serviceAccount)
}
