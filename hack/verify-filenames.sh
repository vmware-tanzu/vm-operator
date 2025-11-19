#!/usr/bin/env bash

# Generic file naming convention checker
#
# This script provides an extensible framework for enforcing file naming conventions
# across the codebase. It uses a configuration-based approach where rules can be
# easily added or modified by editing the RULES section below.
#
# USAGE:
#   make verify-filenames
#   or directly: hack/verify-filenames.sh
#
# ADDING NEW RULES:
#   1. Find the appropriate section (WEBHOOK RULES, API RULES, CONTROLLER RULES, etc.)
#   2. Add a new check_naming_convention call with:
#      - Directory path to check
#      - Required filename pattern (shell glob pattern)
#      - Human-readable description
#      - (Optional) Exclude patterns (one per line)
#      - (Optional) Max depth (1 = only top-level, omit for recursive)
#
# EXAMPLE:
#   check_naming_convention \
#     "path/to/directory" \
#     "required_pattern*.go" \
#     "Description of expected pattern" \
#     "exclude_pattern1.go
# exclude_pattern2.go" \
#     "1"  # Optional: maxdepth
#
# PATTERNS:
#   - Use shell glob patterns: *.go, *_test.go, prefix_*.go
#   - Patterns are matched against filenames only (not paths)
#   - Multiple exclude patterns can be specified (one per line)

set -o errexit
set -o nounset
set -o pipefail

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

EXIT_CODE=0

# ============================================================================
# RULES CONFIGURATION
# ============================================================================
# Define naming rules here. Each rule has:
#   - DIR: Directory path to check (relative to project root)
#   - PATTERN: Required filename pattern (supports shell glob patterns)
#   - DESCRIPTION: Human-readable description of the pattern
#   - EXCLUDE: (Optional) Additional patterns to exclude from checking
# ============================================================================

check_naming_convention() {
  local dir="$1"
  local pattern="$2"
  local description="$3"
  local exclude_patterns="${4:-}"
  local maxdepth="${5:-}"
  
  if [ ! -d "${dir}" ]; then
    return 0
  fi
  
  # Build find command with exclusions
  local find_cmd="find \"${dir}\""
  
  # Add maxdepth if specified (to exclude subdirectories)
  if [ -n "${maxdepth}" ]; then
    find_cmd="${find_cmd} -maxdepth ${maxdepth}"
  fi
  
  find_cmd="${find_cmd} -name \"*.go\" -type f"
  
  # Exclude the required pattern
  find_cmd="${find_cmd} ! -name \"${pattern}\""
  
  # Add additional exclusions if provided
  if [ -n "${exclude_patterns}" ]; then
    while IFS= read -r exclude_pattern; do
      [ -n "${exclude_pattern}" ] && find_cmd="${find_cmd} ! -name \"${exclude_pattern}\""
    done <<< "${exclude_patterns}"
  fi
  
  # Execute find and capture invalid files
  local invalid_files
  invalid_files=$(eval "${find_cmd}" 2>/dev/null || true)
  
  if [ -n "${invalid_files}" ]; then
    echo "Error: Files in ${dir} must follow the naming convention:"
    echo "  ${description}"
    echo ""
    echo "Files that don't match the convention:"
    echo "${invalid_files}" | sed 's/^/  /'
    echo ""
    EXIT_CODE=1
  fi
}

# ============================================================================
# WEBHOOK RULES
# ============================================================================

# Generic webhook mutation pattern: {resource}_mutator.go
for webhook_dir in webhooks/*/mutation; do
  if [ -d "${webhook_dir}" ]; then
    resource=$(basename "$(dirname "${webhook_dir}")")
    pattern="${resource}_mutator*.go"
    check_naming_convention \
      "${webhook_dir}" \
      "${pattern}" \
      "${resource}_mutator.go (main file) or ${resource}_mutator_<feature>.go (feature-specific files)"
  fi
done

# Generic webhook validation pattern: {resource}_validator.go
for webhook_dir in webhooks/*/validation; do
  if [ -d "${webhook_dir}" ]; then
    resource=$(basename "$(dirname "${webhook_dir}")")
    pattern="${resource}_validator*.go"
    check_naming_convention \
      "${webhook_dir}" \
      "${pattern}" \
      "${resource}_validator.go (main file) or ${resource}_validator_<feature>.go (feature-specific files)"
  fi
done

# ============================================================================
# API RULES
# ============================================================================


# ============================================================================
# CONTROLLER RULES
# ============================================================================



# ============================================================================
# ADD MORE RULES HERE
# ============================================================================
# To add new rules, use the check_naming_convention function:
#
# check_naming_convention \
#   "path/to/directory" \
#   "required_pattern*.go" \
#   "Description of the pattern" \
#   "optional_exclude_pattern1.go
# optional_exclude_pattern2.go"
# ============================================================================

# ============================================================================
# OUTPUT RESULTS
# ============================================================================

if [ "${EXIT_CODE}" -eq 0 ]; then
  printf '\nCongratulations! All files follow the naming conventions!\n'
else
  # Please note the following heredoc uses leading tabs to allow
  # the contents to be indented with the if/fi statement. For
  # more information on indenting heredocs, please see the section
  # entitled "Mutli-line message, with tabs suppressed" from The
  # Linux Documentation Project (TLDP) at
  # https://tldp.org/LDP/abs/html/here-docs.html.
  cat <<-EOF
	Please rename the files listed above to follow the naming conventions.
	
	To add new naming rules, edit hack/verify-filenames.sh and add them
	in the RULES CONFIGURATION section.
	
	Thank you!
	EOF
fi

exit "${EXIT_CODE}"
