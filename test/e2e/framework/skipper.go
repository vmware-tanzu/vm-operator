/*
Copyright 2014 The Kubernetes Authors.
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

// https://github.com/kubernetes/kubernetes/blob/8058942aa2552bcaf13208d1ea678e9f3f5f4605/test/e2e/framework/skipper/skipper.go
package framework

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/onsi/ginkgo/v2" //nolint:depguard // E2E skipper mirrors k8s framework and must call ginkgo.Skip.
	"k8s.io/kubernetes/test/e2e/framework"
)

// Skipper is a generic function called by other test-specific skipper.
func SkipInternalf(caller int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	framework.Logf("%s", msg)
	skip(msg, caller+1)
}

// SkipPanic is the value that will be panicked from Skip.
type SkipPanic struct {
	Message        string // The failure message passed to Fail
	Filename       string // The filename that is the source of the failure
	Line           int    // The line number of the filename that is the source of the failure
	FullStackTrace string // A full stack trace starting at the source of the failure
}

const ginkgoPanic = `
Your test failed.
Ginkgo panics to prevent subsequent assertions from running.
Normally Ginkgo rescues this panic so you shouldn't see it.

But, if you make an assertion in a goroutine, Ginkgo can't capture the panic.
To circumvent this, you should call

	defer GinkgoRecover()

at the top of the goroutine that caused this panic.
`

// String makes SkipPanic look like the old Ginkgo panic when printed.
func (SkipPanic) String() string { return ginkgoPanic }

// Skip wraps ginkgo.Skip so that it panics with more useful
// information about why the test is being skipped. This function will
// panic with a SkipPanic.
func skip(message string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip += callerSkip[0]
	}

	_, file, line, _ := runtime.Caller(skip)
	sp := SkipPanic{
		Message:        message,
		Filename:       file,
		Line:           line,
		FullStackTrace: pruneStack(skip),
	}

	defer func() {
		e := recover()
		if e != nil {
			panic(sp)
		}
	}()

	ginkgo.Skip(message, skip)
}

// ginkgo adds a lot of test running infrastructure to the stack, so
// we filter those out.
var stackSkipPattern = regexp.MustCompile(`onsi/ginkgo`)

func pruneStack(skip int) string {
	skip += 2 // one for pruneStack and one for debug.Stack
	stack := debug.Stack()
	scanner := bufio.NewScanner(bytes.NewBuffer(stack))

	var prunedStack []string

	// skip the top of the stack
	for i := 0; i < 2*skip+1; i++ {
		scanner.Scan()
	}

	for scanner.Scan() {
		if stackSkipPattern.Match(scanner.Bytes()) {
			scanner.Scan() // these come in pairs
		} else {
			prunedStack = append(prunedStack, scanner.Text())
			scanner.Scan() // these come in pairs
			prunedStack = append(prunedStack, scanner.Text())
		}
	}

	return strings.Join(prunedStack, "\n")
}

// Skipf skips with information about why the test is being skipped.
func Skipf(format string, args ...any) {
	SkipInternalf(1, format, args...)
}
