#!/usr/bin/env bash

# Check if any tests have a focus.

set -o errexit
set -o nounset
set -o pipefail

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."


make unfocus || exit_code="${?}"
if [ "${exit_code:-0}" -ne 0 ]; then

  # Please note the following heredoc uses leading tabs to allow
  # the contents to be indented with the if/fi statement. For
  # more information on indenting heredocs, please see the section
  # entitled "Mutli-line message, with tabs suppressed" from The
  # Linux Documentation Project (TLDP) at
  # https://tldp.org/LDP/abs/html/here-docs.html.
  cat <<-EOF
	Please investigate why the target is failing by running the
	following command locally from the root of the project:

	    make unfocus

	Thank you!
	EOF

  exit "${exit_code}"
fi

if git diff --exit-code './**/*_test.go'; then
  printf '\nCongratulations! No tests have a focus!\n'
else
  exit_code="${?}"

  # Please note the following heredoc uses leading tabs to allow
  # the contents to be indented with the if/fi statement. For
  # more information on indenting heredocs, please see the section
  # entitled "Mutli-line message, with tabs suppressed" from The
  # Linux Documentation Project (TLDP) at
  # https://tldp.org/LDP/abs/html/here-docs.html.
  cat <<-EOF
	Please run the following command locally to remove all focuses from
	tests:

	    make unfocus

	Thank you!
	EOF

  exit "${exit_code}"
fi

