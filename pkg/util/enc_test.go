// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

func TestTryToDecodeBase64Gzip(t *testing.T) {

	const helloWorld = "Hello, world."

	b64 := func(count int, data []byte) []byte {
		for i := 0; i <= count; i++ {
			data = []byte(base64.StdEncoding.EncodeToString(data))
		}
		return data
	}

	gz := func(data []byte) []byte {
		var w bytes.Buffer
		gzw := gzip.NewWriter(&w)
		//nolint:errcheck,gosec
		gzw.Write(data)
		//nolint:errcheck,gosec
		gzw.Close()
		return w.Bytes()
	}

	testCases := []struct {
		name      string
		data      []byte
		expString string
		expError  error
	}{
		{
			name:      "Plain text",
			data:      []byte(helloWorld),
			expString: helloWorld,
		},
		{
			name:      "Base64-encoded once",
			data:      b64(1, []byte(helloWorld)),
			expString: helloWorld,
		},
		{
			name:      "Base64-encoded twice",
			data:      b64(2, []byte(helloWorld)),
			expString: helloWorld,
		},
		{
			name:      "Base64-encoded thrice",
			data:      b64(3, []byte(helloWorld)),
			expString: helloWorld,
		},
		{
			name:      "Gzipped and base64-encoded once",
			data:      b64(1, gz([]byte(helloWorld))),
			expString: helloWorld,
		},
		{
			name:      "Gzipped and base64-encoded twice",
			data:      b64(2, gz([]byte(helloWorld))),
			expString: helloWorld,
		},
		{
			name:      "Gzipped and base64-encoded thrice",
			data:      b64(3, gz([]byte(helloWorld))),
			expString: helloWorld,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			s, err := util.TryToDecodeBase64Gzip(tc.data)
			if e, a := tc.expError, err; e != nil && a != nil && e.Error() != a.Error() {
				t.Errorf("expErr=%q != actErr=%q", e, a)
			}
			if e, a := tc.expString, s; e != a {
				t.Errorf("expStr=%q != actStr=%q", e, a)
			}
		})
	}

}
