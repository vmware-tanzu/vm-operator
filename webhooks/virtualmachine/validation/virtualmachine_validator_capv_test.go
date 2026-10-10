// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"
)

func TestIsCAPVServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		// Old style: svc-tkg-domain-<cluster-mob-id>
		{
			name:     "old tkg namespace with cluster mob id, default sa",
			username: "system:serviceaccount:svc-tkg-domain-c52:default",
			want:     true,
		},
		{
			name:     "old tkg namespace with cluster mob id, capv-manager sa",
			username: "system:serviceaccount:svc-tkg-domain-c52:capv-manager",
			want:     true,
		},
		// New style: svc-tkg-<random>
		{
			name:     "new tkg namespace with random hash, default sa",
			username: "system:serviceaccount:svc-tkg-123ab:default",
			want:     true,
		},
		{
			name:     "new tkg namespace with random hash, capv-manager sa",
			username: "system:serviceaccount:svc-tkg-123ab:capv-manager",
			want:     true,
		},
		// Negative cases
		{
			name:     "vks prefix namespace (not used, should not match)",
			username: "system:serviceaccount:svc-vks-123ab:capv-manager",
			want:     false,
		},
		{
			name:     "unrelated supervisor service, default sa",
			username: "system:serviceaccount:svc-configuration-123ab:default",
			want:     false,
		},
		{
			name:     "unrelated supervisor service, capv-manager sa",
			username: "system:serviceaccount:svc-configuration-123ab:capv-manager",
			want:     false,
		},
		{
			name:     "tkg namespace with wrong sa",
			username: "system:serviceaccount:svc-tkg-123ab:other-sa",
			want:     false,
		},
		{
			name:     "default namespace",
			username: "system:serviceaccount:default:default",
			want:     false,
		},
		{
			name:     "sso user",
			username: "sso:admin@vsphere.local",
			want:     false,
		},
		{
			name:     "empty username",
			username: "",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCAPVServiceAccount(tc.username); got != tc.want {
				t.Errorf("isCAPVServiceAccount(%q) = %v, want %v", tc.username, got, tc.want)
			}
		})
	}
}
