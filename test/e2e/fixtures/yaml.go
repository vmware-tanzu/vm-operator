/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package fixtures

import (
	"os"
	"path/filepath"
	"runtime"

	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

func ReadFile(path, file string) string {
	return string(ReadFileBytes(path, file))
}

func ReadFileBytes(path, file string) []byte {
	from := filepath.Join(path, file)

	out, err := testfiles.Read(from)
	if err != nil {
		_, thisFile, _, ok := runtime.Caller(0)
		if ok {
			repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))

			absPath := filepath.Join(repoRoot, from)
			if out, err2 := os.ReadFile(absPath); err2 == nil {
				return out
			}
		}

		panic(err)
	}

	return out
}
