#!/usr/bin/env bash
# ============================================================================
#  aiway — uninstaller
#  Removes Blocky container, Angie config files, and restores systemd-resolved
#  Usage: sudo bash uninstall.sh
# ============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/utils.sh"

ANGIE_CONF="/etc/angie/angie.conf"
ANGIE_STREAM_FILE="/etc/angie/stream.d/ai-proxy.conf"
ANGIE_HTTP_FILE="/etc/angie/http.d/local-services.conf"
BLOCKY_DIR="/opt/blocky"
AIWAY_ETC_DIR="/etc/aiway"

print_banner() {
    echo -e "${RED}${BOLD}"
    cat <<'EOF'
    ___  _
   / _ \(_)_      ____ _ _   _
  / /_\ | \ \ /\ / / _` | | | |
 / /  | | |\ V  V / (_| | |_| |
 \/   |_|_| \_/\_/ \__,_|\__, |
                          |___/

  Uninstaller
EOF
    echo -e "${RESET}"
}

main() {
    [[ "${AIWAY_NO_CLEAR:-0}" == "1" ]] || clear
    print_banner
    check_root

    echo -e "  ${YELLOW}This will remove:${RESET}"
    echo -e "   • Blocky Docker container and /opt/blocky/"
    echo -e "   • Angie config files created by aiway"
    echo -e "   • Restore systemd-resolved stub listener\n"

    if [[ "${AIWAY_YES:-0}" != "1" ]]; then
        read -rp "  Are you sure? [y/N] " confirm
        [[ "${confirm,,}" == "y" ]] || { echo "Aborted."; exit 0; }
    fi

    # ── Blocky ───────────────────────────────────────────────────────────
    print_step "Removing Blocky"

    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q "^blocky$"; then
        run_quietly "Stopping and removing blocky container" docker rm -f blocky
        print_ok "Blocky container removed"
    else
        print_warn "No blocky container found — skipping"
    fi

    if [[ -d "$BLOCKY_DIR" ]]; then
        rm -rf "$BLOCKY_DIR"
        print_ok "Removed ${BLOCKY_DIR}"
    fi

    # ── Angie config ──────────────────────────────────────────────────────
    print_step "Removing Angie configuration"

    for f in "$ANGIE_CONF" "$ANGIE_STREAM_FILE" "$ANGIE_HTTP_FILE"; do
        if [[ -f "$f" ]]; then
            rm -f "$f"
            print_ok "Removed: $f"
        else
            print_warn "Not found (already removed?): $f"
        fi
    done

    # Remove ACME blocks appended to angie.conf (already deleted above)
    # Restore a minimal default angie.conf so the service can still start
    if has_cmd angie && [[ ! -f "$ANGIE_CONF" ]]; then
        cat > "$ANGIE_CONF" <<'DEFAULTEOF'
# Minimal angie.conf restored by aiway uninstaller
user www-data;
worker_processes auto;
pid /run/angie.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/angie/mime.types;
    default_type application/octet-stream;
    sendfile on;
    keepalive_timeout 65;
}
DEFAULTEOF
        print_ok "Restored minimal ${ANGIE_CONF}"
        run_quietly "Reloading Angie" systemctl reload angie 2>/dev/null || \
            run_quietly "Restarting Angie" systemctl restart angie
    fi

    # ── systemd-resolved ─────────────────────────────────────────────────
    print_step "Restoring systemd-resolved"

    local conf="/etc/systemd/resolved.conf"
    local bak="${conf}.aiway.bak"

    if [[ -f "$bak" ]]; then
        cp "$bak" "$conf"
        rm -f "$bak"
        run_quietly "Restarting systemd-resolved" systemctl restart systemd-resolved
        print_ok "Restored ${conf} from backup"
    elif [[ -f "$conf" ]]; then
        # Just re-enable the stub listener
        sed -i '/^DNSStubListener=no$/d' "$conf"
        echo "DNSStubListener=yes" >> "$conf"
        run_quietly "Restarting systemd-resolved" systemctl restart systemd-resolved
        print_ok "Re-enabled DNSStubListener in ${conf}"
    else
        print_warn "${conf} not found — nothing to restore"
    fi

    # ── Firewall cleanup ─────────────────────────────────────────────────
    print_step "Firewall cleanup"

    if has_cmd ufw && ufw status 2>/dev/null | grep -q "active"; then
        for rule in "443/tcp" "53/udp" "53/tcp" "853/tcp" "80/tcp"; do
            ufw delete allow "$rule" >/dev/null 2>&1 || true
        done
        print_ok "Removed aiway ufw rules (if they existed)"
    fi

    if [[ -d "$AIWAY_ETC_DIR" ]]; then
        rm -rf "$AIWAY_ETC_DIR"
        print_ok "Removed ${AIWAY_ETC_DIR}"
    fi

    rm -f /usr/local/bin/aiwayctl

    echo ""
    echo -e "${GREEN}${BOLD}  aiway has been removed.${RESET}"
    echo -e "  ${DIM}Angie package itself was not uninstalled — run:${RESET}"
    echo -e "  ${DIM}  apt-get remove --purge angie${RESET}"
    echo -e "  ${DIM}Docker was not uninstalled — run:${RESET}"
    echo -e "  ${DIM}  apt-get remove --purge docker-ce docker-ce-cli containerd.io${RESET}\n"
}

main "$@"
