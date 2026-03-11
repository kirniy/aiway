#!/usr/bin/env bash
set -euo pipefail

AIWAY_ETC_DIR="/etc/aiway"
AIWAY_RUNTIME_DIR="${AIWAY_ETC_DIR}/runtime"
AIWAY_CUSTOM_DOMAINS_FILE="${AIWAY_ETC_DIR}/custom-domains.txt"
AIWAY_INSTALLER_ENV="${AIWAY_ETC_DIR}/installer.env"
BLOCKY_CONFIG="/opt/blocky/config.yml"

[[ -f "${AIWAY_INSTALLER_ENV}" ]] && source "${AIWAY_INSTALLER_ENV}"

json_escape() {
    local value="${1//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//$'\n'/\\n}"
    value="${value//$'\r'/}"
    printf '%s' "$value"
}

print_usage() {
    cat <<'EOF'
Usage:
  aiwayctl status [--json]
  aiwayctl doctor [--json]
  aiwayctl list-domains
  aiwayctl add-domain <apex-domain>
  aiwayctl remove-domain <apex-domain>
  aiwayctl reapply
  aiwayctl uninstall
EOF
}

load_domains() {
    source "${AIWAY_RUNTIME_DIR}/lib/domains.sh"
    EXTRA_DOMAINS=()
    if [[ -f "$AIWAY_CUSTOM_DOMAINS_FILE" ]]; then
        while IFS= read -r line; do
            line="${line%%#*}"
            line="${line//[[:space:]]/}"
            [[ -n "$line" ]] && EXTRA_DOMAINS+=("$line")
        done < "$AIWAY_CUSTOM_DOMAINS_FILE"
    fi
}

domain_exists() {
    local needle="$1"
    shift
    local item
    for item in "$@"; do
        [[ "$item" == "$needle" ]] && return 0
    done
    return 1
}

angie_running() {
    systemctl is-active --quiet angie 2>/dev/null
}

blocky_running() {
    docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^blocky$'
}

status_cmd() {
	local as_json="${1:-0}"
    local angie_state="stopped"
    local blocky_state="stopped"
    local dot_domain="${AIWAY_DOT_DOMAIN:-}"
    local vps_ip="${AIWAY_VPS_IP:-}"

    angie_running && angie_state="running"
    blocky_running && blocky_state="running"

    if [[ "$as_json" == "1" ]]; then
        printf '{"angie":"%s","blocky":"%s","vpsIp":"%s","dotDomain":"%s"}\n' \
            "$angie_state" "$blocky_state" "$(json_escape "$vps_ip")" "$(json_escape "$dot_domain")"
        return
    fi

    echo "Angie:  ${angie_state}"
    echo "Blocky: ${blocky_state}"
    echo "VPS IP: ${vps_ip:-unknown}"
    echo "DoT/DoH: ${dot_domain:-disabled}"
}

doctor_cmd() {
	local as_json="${1:-0}"
    local angie_ok="false"
    local blocky_ok="false"
    local dns_ok="false"
    local dns_result=""

    angie_running && angie_ok="true"
    blocky_running && blocky_ok="true"

    if command -v dig >/dev/null 2>&1; then
        dns_result="$(dig +short +time=3 +tries=1 openai.com @127.0.0.1 2>/dev/null | head -1 || true)"
        [[ -n "$dns_result" ]] && dns_ok="true"
    fi

    if [[ "$as_json" == "1" ]]; then
        printf '{"angie":%s,"blocky":%s,"dns":%s,"dnsResult":"%s"}\n' \
            "$angie_ok" "$blocky_ok" "$dns_ok" "$(json_escape "$dns_result")"
        return
    fi

    echo "Angie running:  ${angie_ok}"
    echo "Blocky running: ${blocky_ok}"
    echo "DNS check:      ${dns_ok}${dns_result:+ (${dns_result})}"
}

list_domains_cmd() {
    load_domains
    {
        printf '%s\n' "${AI_APEX_DOMAINS[@]}"
        printf '%s\n' "${EXTRA_DOMAINS[@]}"
    } | awk 'NF' | sort -u
}

rewrite_custom_domains() {
    mkdir -p "$AIWAY_ETC_DIR"
    printf '%s\n' "$@" | awk 'NF' | sort -u > "$AIWAY_CUSTOM_DOMAINS_FILE"
}

reapply_cmd() {
    [[ -x "${AIWAY_RUNTIME_DIR}/install.sh" ]] || {
        echo "aiway runtime not found at ${AIWAY_RUNTIME_DIR}" >&2
        exit 1
    }

    AIWAY_NONINTERACTIVE=1 AIWAY_YES=1 AIWAY_NO_CLEAR=1 \
        bash "${AIWAY_RUNTIME_DIR}/install.sh"
}

add_domain_cmd() {
    local domain="$1"
    [[ "$domain" =~ ^[A-Za-z0-9.-]+$ ]] || {
        echo "Invalid domain: $domain" >&2
        exit 1
    }

    load_domains
    domain_exists "$domain" "${AI_APEX_DOMAINS[@]}" "${EXTRA_DOMAINS[@]}" && {
        echo "Domain already present: $domain"
        return 0
    }

    EXTRA_DOMAINS+=("$domain")
    rewrite_custom_domains "${EXTRA_DOMAINS[@]}"
    reapply_cmd
}

remove_domain_cmd() {
    local domain="$1"
    load_domains
    local kept=()
    local item
    for item in "${EXTRA_DOMAINS[@]}"; do
        [[ "$item" == "$domain" ]] || kept+=("$item")
    done
    rewrite_custom_domains "${kept[@]}"
    reapply_cmd
}

uninstall_cmd() {
    [[ -x "${AIWAY_RUNTIME_DIR}/uninstall.sh" ]] || {
        echo "aiway runtime not found at ${AIWAY_RUNTIME_DIR}" >&2
        exit 1
    }

    AIWAY_YES=1 AIWAY_NO_CLEAR=1 bash "${AIWAY_RUNTIME_DIR}/uninstall.sh"
}

cmd="${1:-}"
arg_json=0
[[ "${2:-}" == "--json" ]] && arg_json=1
case "$cmd" in
    status)
        status_cmd "$arg_json" | cat
        ;;
    doctor)
        doctor_cmd "$arg_json" | cat
        ;;
    list-domains)
        list_domains_cmd
        ;;
    add-domain)
        [[ $# -ge 2 ]] || { print_usage; exit 1; }
        add_domain_cmd "$2"
        ;;
    remove-domain)
        [[ $# -ge 2 ]] || { print_usage; exit 1; }
        remove_domain_cmd "$2"
        ;;
    reapply)
        reapply_cmd
        ;;
    uninstall)
        uninstall_cmd
        ;;
    *)
        print_usage
        exit 1
        ;;
esac
