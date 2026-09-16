// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh

import "regexp"

// sensitiveFlagPattern matches CLI flags whose value is a password or other
// secret (e.g. +password 'x', --user-password 'x', --image-registry-password 'x'),
// so their value can be redacted before the command is logged.
var sensitiveFlagPattern = regexp.MustCompile(`(?i)([-+]{1,2}[\w-]*(?:password|secret|pwd|passwd)[\w-]*\s+)'[^']*'`)

// sensitiveOutputLinePattern matches "key: value" style lines commonly
// emitted by VC-side tooling (e.g. decryptK8Pwd.py prints "PWD: <secret>"),
// so the value can be redacted before command output is logged.
var sensitiveOutputLinePattern = regexp.MustCompile(`(?im)^([ \t]*(?:pwd|password|passwd)[ \t]*:[ \t]*)\S.*$`)

// RedactSensitiveFlags returns a copy of cmd with the values of any
// password- or secret-like flags replaced with '***'. It is safe to call
// on any CLI command string before printing or logging it.
func RedactSensitiveFlags(cmd string) string {
	return sensitiveFlagPattern.ReplaceAllString(cmd, "${1}'***'")
}

// RedactSensitiveOutput returns a copy of output with the values of any
// "pwd:"/"password:"/"passwd:" style lines replaced with '***'. It is safe
// to call on any command output before printing or logging it.
func RedactSensitiveOutput(output string) string {
	return sensitiveOutputLinePattern.ReplaceAllString(output, "${1}***")
}
