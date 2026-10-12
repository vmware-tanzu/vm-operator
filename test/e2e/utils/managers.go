package utils

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetCommandEnvVars(ctx context.Context, client ctrlclient.Client, ns, name, command string) (map[string]string, error) {
	deployment, err := GetDeployment(ctx, client, ns, name)
	Expect(err).To(BeNil())

	Expect(command).ToNot(BeEmpty())

	for _, container := range deployment.Spec.Template.Spec.Containers {
		if len(container.Command) != 0 && container.Command[0] == command {
			envMap := map[string]string{}

			for _, envVar := range container.Env {
				// TODO(thall): Fetching the env from ValueFrom is significantly more
				// complicated and we do not use them for now. Let's grab only the
				// simple string values.
				if envVar.ValueFrom == nil {
					envMap[envVar.Name] = envVar.Value
				}
			}

			return envMap, nil
		}
	}

	return nil, fmt.Errorf("unable to find container with command %q", command)
}
