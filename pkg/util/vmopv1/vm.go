// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

const (
	vmiKind           = "VirtualMachineImage"
	cvmiKind          = "Cluster" + vmiKind
	imgNotFoundFormat = "no VM image exists for %q in namespace or cluster scope"
)

// ErrImageNotFound is returned from ResolveImageName if the image cannot be
// found at the namespace or cluster scopes. This type will return true when
// provided to the apierrors.IsNotFound function.
type ErrImageNotFound struct {
	msg string
}

func (e ErrImageNotFound) Error() string {
	return e.msg
}

func (e ErrImageNotFound) Status() metav1.Status {
	return metav1.Status{
		Reason: metav1.StatusReasonNotFound,
		Code:   http.StatusNotFound,
	}
}

// ResolveImageName resolves the provided name of a VM image either to a
// VirtualMachineImage resource or ClusterVirtualMachineImage resource.
func ResolveImageName(
	ctx context.Context,
	k8sClient client.Client,
	namespace, imgName string) (client.Object, error) {

	// Return early if the VM image name is empty.
	if imgName == "" {
		return nil, fmt.Errorf("imgName is empty")
	}

	// Query the image from the object name in order to set the result in
	// spec.image.
	if strings.HasPrefix(imgName, "vmi-") {

		var obj client.Object

		obj = &vmopv1.VirtualMachineImage{}
		if err := k8sClient.Get(
			ctx,
			client.ObjectKey{Namespace: namespace, Name: imgName},
			obj); err != nil {

			if !apierrors.IsNotFound(err) {
				return nil, err
			}

			obj = &vmopv1.ClusterVirtualMachineImage{}
			if err := k8sClient.Get(
				ctx,
				client.ObjectKey{Name: imgName},
				obj); err != nil {

				if !apierrors.IsNotFound(err) {
					return nil, err
				}

				return nil, ErrImageNotFound{
					msg: fmt.Sprintf(imgNotFoundFormat, imgName)}
			}
		}

		return obj, nil
	}

	var obj client.Object

	// Check if a single namespace scope image exists by the status name.
	var vmiList vmopv1.VirtualMachineImageList
	if err := k8sClient.List(ctx, &vmiList, client.InNamespace(namespace),
		client.MatchingFields{
			"status.name": imgName,
		},
	); err != nil {
		return nil, err
	}
	switch len(vmiList.Items) {
	case 0:
		break
	case 1:
		obj = &vmiList.Items[0]
	default:
		return nil, errors.Errorf(
			"multiple VM images exist for %q in namespace scope", imgName)
	}

	// Check if a single cluster scope image exists by the status name.
	var cvmiList vmopv1.ClusterVirtualMachineImageList
	if err := k8sClient.List(ctx, &cvmiList, client.MatchingFields{
		"status.name": imgName,
	}); err != nil {
		return nil, err
	}
	switch len(cvmiList.Items) {
	case 0:
		break
	case 1:
		if obj != nil {
			return nil, errors.Errorf(
				"multiple VM images exist for %q in namespace and cluster scope",
				imgName)
		}
		obj = &cvmiList.Items[0]
	default:
		return nil, errors.Errorf(
			"multiple VM images exist for %q in cluster scope", imgName)
	}

	if obj == nil {
		return nil,
			ErrImageNotFound{msg: fmt.Sprintf(imgNotFoundFormat, imgName)}
	}

	return obj, nil
}
