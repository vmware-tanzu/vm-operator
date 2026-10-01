// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package redact provides helpers to mask password- and secret-like values
// out of CLI command strings and command output before they are printed or
// logged. It has no dependencies beyond the standard library so that any
// E2E helper — SSH-based, exec.Command-based, or otherwise — can use it
// without pulling in an unrelated transport package.
package redact

import "regexp"

// sensitiveFlagPattern matches CLI flags whose value is a password or other
// secret (e.g. +password 'x', --user-password 'x', --image-registry-password 'x'),
// so their value can be redacted before the command is logged.
var sensitiveFlagPattern = regexp.MustCompile(`(?i)([-+]{1,2}[\w-]*(?:password|secret|pwd|passwd)[\w-]*\s+)'[^']*'`)

// sensitiveVarAssignmentPattern matches unquoted key=value pairs whose key is
// a password or other secret (e.g. Packer's "-var ssh_password=x" style,
// produced by exec.Cmd.String()), so the value can be redacted before the
// command is logged.
var sensitiveVarAssignmentPattern = regexp.MustCompile(`(?i)([\w-]*(?:password|secret|pwd|passwd)[\w-]*=)\S+`)

// sensitiveOutputLinePattern matches "key: value" style lines commonly
// emitted by VC-side tooling (e.g. decryptK8Pwd.py prints "PWD: <secret>"),
// so the value can be redacted before command output is logged.
var sensitiveOutputLinePattern = regexp.MustCompile(`(?im)^([ \t]*(?:pwd|password|passwd)[ \t]*:[ \t]*)\S.*$`)

// RedactSensitiveFlags returns a copy of cmd with the values of any
// password- or secret-like flags replaced with '***'. It is safe to call
// on any CLI command string before printing or logging it.
func RedactSensitiveFlags(cmd string) string {
	cmd = sensitiveFlagPattern.ReplaceAllString(cmd, "${1}'***'")
	return sensitiveVarAssignmentPattern.ReplaceAllString(cmd, "${1}***")
}

// RedactSensitiveOutput returns a copy of output with the values of any
// "pwd:"/"password:"/"passwd:" style lines replaced with '***'. It is safe
// to call on any command output before printing or logging it.
func RedactSensitiveOutput(output string) string {
	return sensitiveOutputLinePattern.ReplaceAllString(output, "${1}***")
}
