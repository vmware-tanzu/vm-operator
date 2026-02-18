#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Change directories to the parent directory of the one in which this
# script is located.
cd "$(dirname "${BASH_SOURCE[0]}")/.."

################################################################################
##                                  USAGE
################################################################################

usage() {
  cat <<EOF
Usage: $(basename "${0}") [OPTIONS] VM_YAML_FILE

Generates a plain-text timeline report from a VirtualMachine YAML file.

ARGUMENTS:
  VM_YAML_FILE    Path to the VM YAML file to analyze

OPTIONS:
  -o OUTPUT       Output file path (default: stdout)
  -h              Show this help message

EXAMPLES:
  # Generate timeline to stdout
  $(basename "${0}") vm.yaml

  # Generate timeline to a file
  $(basename "${0}") -o timeline.txt vm.yaml

  # Pipe VM from kubectl
  kubectl get vm my-vm -o yaml | $(basename "${0}") /dev/stdin

EOF
  exit 1
}

################################################################################
##                                   FUNCS
################################################################################

# error stores exit code, writes arguments to STDERR, and returns stored exit code
# fatal is like error except it will exit program if exit code >0
error() {
  exit_code="${?}"
  echo "${@}" 1>&2
  return "${exit_code}"
}
fatal() {
  error "${@}"
  exit 1
}
check_command() {
  command -v "${1}" >/dev/null 2>&1 || fatal "${1} is required"
}
check_dependencies() {
  check_command jq
  check_command yq
}


# Parse timestamp to epoch seconds (works on both macOS and Linux)
parse_timestamp() {
  local ts="${1}"
  if date --version &>/dev/null; then
    # GNU date (Linux)
    date -d "${ts}" "+%s" 2>/dev/null || echo "0"
  else
    # BSD date (macOS)
    date -j -f "%Y-%m-%dT%H:%M:%SZ" "${ts}" "+%s" 2>/dev/null || echo "0"
  fi
}

# Calculate duration between two epoch timestamps
calc_duration() {
  local start="${1}"
  local end="${2}"
  local diff=$((end - start))

  if [ "${diff}" -lt 0 ]; then
    echo "invalid"
    return
  fi

  local days=$((diff / 86400))
  local hours=$(((diff % 86400) / 3600))
  local minutes=$(((diff % 3600) / 60))
  local seconds=$((diff % 60))

  local result=""
  [ "${days}" -gt 0 ] && result="${days}d "
  [ "${hours}" -gt 0 ] && result="${result}${hours}h "
  [ "${minutes}" -gt 0 ] && result="${result}${minutes}m "
  [ "${seconds}" -gt 0 ] || [ -z "${result}" ] && result="${result}${seconds}s"

  echo "${result% }"
}

# Format relative time from creation
format_relative_time() {
  local diff="${1}"

  if [ "${diff}" -eq 0 ]; then
    echo "T+0s"
    return
  fi

  local days=$((diff / 86400))
  local hours=$(((diff % 86400) / 3600))
  local minutes=$(((diff % 3600) / 60))
  local seconds=$((diff % 60))

  local result="T+"
  [ "${days}" -gt 0 ] && result="${result}${days}d "
  [ "${hours}" -gt 0 ] && result="${result}${hours}h "
  [ "${minutes}" -gt 0 ] && result="${result}${minutes}m "
  [ "${seconds}" -gt 0 ] || [ -z "${result#T+}" ] && result="${result}${seconds}s"

  echo "${result% }"
}

generate_timeline() {
  local vm_file="${1}"
  local output_file="${2}"

  # Extract VM name and creation timestamp
  local vm_name
  local creation_ts
  vm_name=$(yq eval '.metadata.name // "unknown"' "${vm_file}")
  creation_ts=$(yq eval '.metadata.creationTimestamp // ""' "${vm_file}")

  if [ -z "${creation_ts}" ]; then
    echo "Error: Could not find metadata.creationTimestamp in ${vm_file}" >&2
    exit 1
  fi

  local creation_epoch
  creation_epoch=$(parse_timestamp "${creation_ts}")

  # Extract all conditions with their timestamps and build a sorted timeline
  local conditions_json
  conditions_json=$(yq eval -o=json '.status.conditions // []' "${vm_file}")

  # Create a temporary file to store timeline entries
  local tmpfile
  tmpfile=$(mktemp)

  # Add creation time - use special conditions that are always set at creation
  local creation_conditions=""
  if echo "${conditions_json}" | jq -e '.[] | select(.type == "VirtualMachineClassReady" and .lastTransitionTime == "'"${creation_ts}"'")' >/dev/null 2>&1; then
    creation_conditions="VirtualMachineClassReady: True"
  fi
  if echo "${conditions_json}" | jq -e '.[] | select(.type == "VirtualMachineStorageReady" and .lastTransitionTime == "'"${creation_ts}"'")' >/dev/null 2>&1; then
    [ -n "${creation_conditions}" ] && creation_conditions="${creation_conditions};"
    creation_conditions="${creation_conditions}VirtualMachineStorageReady: True"
  fi
  if [ -z "${creation_conditions}" ]; then
    creation_conditions="VM resource created"
  fi
  echo "${creation_epoch}|VM Created (metadata.creationTimestamp)||${creation_conditions}" >> "${tmpfile}"

  # Process all conditions
  local num_conditions
  num_conditions=$(echo "${conditions_json}" | jq 'length')

  for ((i = 0; i < num_conditions; i++)); do
    local condition
    condition=$(echo "${conditions_json}" | jq -r ".[$i]")

    local cond_type cond_status cond_reason cond_message cond_ts
    cond_type=$(echo "${condition}" | jq -r '.type // "Unknown"')
    cond_status=$(echo "${condition}" | jq -r '.status // "Unknown"')
    cond_reason=$(echo "${condition}" | jq -r '.reason // ""')
    cond_message=$(echo "${condition}" | jq -r '.message // ""')
    cond_ts=$(echo "${condition}" | jq -r '.lastTransitionTime // ""')

    if [ -z "${cond_ts}" ]; then
      continue
    fi

    local cond_epoch
    cond_epoch=$(parse_timestamp "${cond_ts}")

    # Skip pre-creation timestamps (like VirtualMachineImageReady)
    if [ "${cond_epoch}" -lt "${creation_epoch}" ]; then
      continue
    fi

    # Skip creation-time conditions we already added
    if [ "${cond_epoch}" -eq "${creation_epoch}" ] && { [ "${cond_type}" = "VirtualMachineClassReady" ] || [ "${cond_type}" = "VirtualMachineStorageReady" ]; }; then
      continue
    fi

    # Build condition description
    local cond_desc="${cond_type}: ${cond_status}"
    [ -n "${cond_reason}" ] && [ "${cond_reason}" != "True" ] && cond_desc="${cond_desc} (${cond_reason})"
    [ -n "${cond_message}" ] && cond_desc="${cond_desc}^^${cond_message}"

    # Add to timeline
    echo "${cond_epoch}|${cond_type}||${cond_desc}" >> "${tmpfile}"
  done

  # Sort and group by timestamp
  local sorted_timeline
  sorted_timeline=$(sort -t'|' -k1 -n "${tmpfile}")

  # Group entries by timestamp
  local grouped_tmpfile
  grouped_tmpfile=$(mktemp)

  local prev_ts=""
  local prev_event=""
  local prev_conditions=""

  while IFS='|' read -r ts event empty conditions; do
    if [ "${ts}" = "${prev_ts}" ]; then
      # Same timestamp, merge conditions
      prev_conditions="${prev_conditions};${conditions}"
    else
      # Different timestamp, output previous entry if exists
      if [ -n "${prev_ts}" ]; then
        echo "${prev_ts}|${prev_event}||${prev_conditions}" >> "${grouped_tmpfile}"
      fi
      prev_ts="${ts}"
      prev_event="${event}"
      prev_conditions="${conditions}"
    fi
  done <<< "${sorted_timeline}"

  # Output last entry
  if [ -n "${prev_ts}" ]; then
    echo "${prev_ts}|${prev_event}||${prev_conditions}" >> "${grouped_tmpfile}"
  fi

  rm -f "${tmpfile}"

  # Generate output
  {
    echo "================================================================================"
    echo "VM Timeline for ${vm_name}"
    echo "================================================================================"
    echo ""
    echo "Creation Time: ${creation_ts}"
    echo ""
    echo "--------------------------------------------------------------------------------"
    echo "Timeline of Events"
    echo "--------------------------------------------------------------------------------"
    echo ""

    local prev_epoch=0
    while IFS='|' read -r ts_epoch event_name empty conditions; do
      # Calculate durations
      local from_creation=$((ts_epoch - creation_epoch))

      # Format timestamp
      local abs_ts
      if date --version &>/dev/null; then
        # GNU date (Linux)
        abs_ts=$(date -d "@${ts_epoch}" "+%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")
      else
        # BSD date (macOS)
        abs_ts=$(date -j -r "${ts_epoch}" "+%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")
      fi

      # Generate event name from conditions if needed
      if [ "${event_name}" != "VM Created (metadata.creationTimestamp)" ]; then
        # Create a meaningful event name from the conditions
        local cond_types=""
        IFS=';' read -ra COND_ARRAY <<< "${conditions}"
        for cond in "${COND_ARRAY[@]}"; do
          local ctype=$(echo "${cond}" | cut -d':' -f1 | sed 's/VirtualMachine//g' | sed 's/Condition//g')
          [ -n "${cond_types}" ] && cond_types="${cond_types} & "
          cond_types="${cond_types}${ctype}"
        done
        event_name="${cond_types}"
      fi

      # Output event
      echo "$(format_relative_time ${from_creation}) (${abs_ts})"
      echo "    Event: ${event_name}"
      echo "    Duration from creation: $(calc_duration ${creation_epoch} ${ts_epoch})"
      if [ "${prev_epoch}" -eq 0 ]; then
        echo "    Duration from previous: —"
      else
        echo "    Duration from previous: $(calc_duration ${prev_epoch} ${ts_epoch})"
      fi

      # Output conditions
      if [ -n "${conditions}" ]; then
        echo "    Conditions set:"
        IFS=';' read -ra COND_ARRAY <<< "${conditions}"
        for cond in "${COND_ARRAY[@]}"; do
          if [[ "${cond}" == *"^^"* ]]; then
            local cond_name=$(echo "${cond}" | cut -d'^' -f1)
            local cond_msg=$(echo "${cond}" | cut -d'^' -f3)
            echo "        • ${cond_name}"
            echo "          Message: ${cond_msg}"
          else
            echo "        • ${cond}"
          fi
        done
      fi

      echo ""
      prev_epoch="${ts_epoch}"
    done < "${grouped_tmpfile}"

    rm -f "${grouped_tmpfile}"

    # Generate summary
    local total_duration
    total_duration=$(calc_duration ${creation_epoch} ${prev_epoch})

    echo "--------------------------------------------------------------------------------"
    echo "Summary"
    echo "--------------------------------------------------------------------------------"
    echo ""
    echo "Total elapsed time:     ${total_duration}"

    # Check current power state
    local power_state
    power_state=$(yq eval '.status.powerState // "Unknown"' "${vm_file}")
    echo "Current power state:    ${power_state}"

    # Check for any false conditions
    local false_conditions
    false_conditions=$(yq eval '.status.conditions[] | select(.status == "False") | .type' "${vm_file}" | tr '\n' ', ' | sed 's/,$//')
    if [ -n "${false_conditions}" ]; then
      echo "Conditions not ready:   ${false_conditions}"
    fi

    echo ""
    echo "================================================================================"
  } > "${output_file}"
}


################################################################################
##                                      MAIN
################################################################################

# Verify the required dependencies are met
check_dependencies

# Default values
OUTPUT_FILE="/dev/stdout"

while getopts ":o:h" opt; do
  case $opt in
    "o" ) OUTPUT_FILE="${OPTARG}" ;;
    "h" ) usage ;;
    \? )
      echo "Invalid option: -${OPTARG}" >&2
      usage
      ;;
    : )
      echo "Option -${OPTARG} requires an argument." >&2
      usage
      ;;
  esac
done

shift $((OPTIND - 1))

if [[ $# -ne 1 ]]; then
  echo "Error: VM YAML file is required" >&2
  usage
fi

VM_FILE="${1}"

if [ ! -f "${VM_FILE}" ] && [ "${VM_FILE}" != "/dev/stdin" ]; then
  echo "Error: File ${VM_FILE} does not exist" >&2
  exit 1
fi

# Check for required commands
for cmd in yq jq date; do
  if ! command -v "${cmd}" &> /dev/null; then
    echo "Error: ${cmd} is required but not installed" >&2
    exit 1
  fi
done

generate_timeline "${VM_FILE}" "${OUTPUT_FILE}"
