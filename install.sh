#!/usr/bin/env bash
# ============================================================================
#  aiway — transparent AI-service proxy installer
#
#  Based on https://habr.com/ru/articles/982070/ by crims0n
#  Usage: sudo bash install.sh
# ============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/utils.sh"
source "${SCRIPT_DIR}/lib/domains.sh"

# ── Paths ────────────────────────────────────────────────────────────────────
AIWAY_ETC_DIR="/etc/aiway"
AIWAY_RUNTIME_DIR="${AIWAY_ETC_DIR}/runtime"
AIWAY_CUSTOM_DOMAINS_FILE="${AIWAY_ETC_DIR}/custom-domains.txt"
AIWAY_INSTALLER_ENV="${AIWAY_ETC_DIR}/installer.env"
AIWAY_CTL_TARGET="/usr/local/bin/aiwayctl"
ANGIE_CONF="/etc/angie/angie.conf"
ANGIE_STREAM_DIR="/etc/angie/stream.d"
ANGIE_HTTP_DIR="/etc/angie/http.d"
BLOCKY_DIR="/opt/blocky"
BLOCKY_CONFIG="${BLOCKY_DIR}/config.yml"

# ── Globals populated by gather_inputs ───────────────────────────────────────
VPS_IP=""
DOT_DOMAIN=""
ACME_EMAIL=""

append_unique() {
    local value="$1"
    shift
    local existing
    for existing in "$@"; do
        [[ "$existing" == "$value" ]] && return 0
    done
    return 1
}

load_custom_domains() {
    [[ -f "$AIWAY_CUSTOM_DOMAINS_FILE" ]] || return 0

    local domain
    while IFS= read -r domain; do
        domain="${domain%%#*}"
        domain="${domain//[[:space:]]/}"
        [[ -z "$domain" ]] && continue
        append_unique "$domain" "${AI_APEX_DOMAINS[@]}" || AI_APEX_DOMAINS+=("$domain")
    done < "$AIWAY_CUSTOM_DOMAINS_FILE"
}

load_custom_domains

# ── Cleanup trap — rolls back on unexpected failure ───────────────────────────
_INSTALL_SUCCESS=false

cleanup_on_error() {
    [[ "$_INSTALL_SUCCESS" == "true" ]] && return
    echo ""
    echo -e "${RED}${BOLD}  Installation failed — rolling back changes...${RESET}"

    # Stop Blocky if it was started
    docker rm -f blocky 2>/dev/null || true

    # Restore systemd-resolved if we touched it
    local conf="/etc/systemd/resolved.conf"
    if [[ -f "${conf}.aiway.bak" ]]; then
        cp "${conf}.aiway.bak" "$conf"
        systemctl restart systemd-resolved 2>/dev/null || true
        print_info "systemd-resolved restored from backup"
    fi

    echo -e "  ${DIM}Fix the error above and re-run: sudo bash install.sh${RESET}\n"
}
trap cleanup_on_error EXIT

# ── Banner ───────────────────────────────────────────────────────────────────
print_banner() {
    echo -e "${CYAN}${BOLD}"
    cat <<'EOF'
    ___  _
   / _ \(_)_      ____ _ _   _
  / /_\ | \ \ /\ / / _` | | | |
 / /  | | |\ V  V / (_| | |_| |
 \/   |_|_| \_/\_/ \__,_|\__, |
                           |___/

  Transparent AI proxy — VPS edition
EOF
    echo -e "${RESET}"
    echo -e "  ${DIM}Routes AI traffic through your server without a VPN${RESET}"
    echo -e "  ${DIM}Angie (nginx fork) + Blocky DNS + optional DoT/DoH${RESET}\n"
}

# ── Preflight ─────────────────────────────────────────────────────────────────
preflight() {
    print_step "Preflight checks"
    check_root
    detect_os

    echo -e "\n  ${YELLOW}This installer will:${RESET}"
    echo -e "   • Install ${BOLD}Angie${RESET} (nginx fork) as SNI proxy on port 443"
    echo -e "   • Install ${BOLD}Blocky${RESET} (Docker) as DNS server on port 53"
    echo -e "   • Redirect ${BOLD}${#AI_APEX_DOMAINS[@]} AI service domains${RESET} through this server"
    echo -e "   • Modify ${BOLD}/etc/systemd/resolved.conf${RESET} (backed up first)"
    echo -e "   • Stop conflicting DNS services (dnsmasq, bind9) if found\n"

    if [[ "${AIWAY_YES:-0}" == "1" || "${AIWAY_NONINTERACTIVE:-0}" == "1" ]]; then
        print_info "Non-interactive mode enabled: continuing automatically"
        return
    fi

    read -rp "  Continue? [y/N] " confirm
    [[ "${confirm,,}" == "y" ]] || { echo "Aborted."; exit 0; }
}

# ── Gather inputs ─────────────────────────────────────────────────────────────
gather_inputs() {
    print_step "Configuration"

    mkdir -p "$AIWAY_ETC_DIR"

    if [[ -f "$AIWAY_INSTALLER_ENV" ]]; then
        # shellcheck source=/dev/null
        source "$AIWAY_INSTALLER_ENV"
    fi

    # --- VPS public IP (try 3 sources, strip whitespace) ---
    local detected_ip=""
    detected_ip=$(
        curl -sf --max-time 5 https://api.ipify.org      2>/dev/null ||
        curl -sf --max-time 5 https://ifconfig.me        2>/dev/null ||
        curl -sf --max-time 5 https://icanhazip.com      2>/dev/null ||
        true
    )
    detected_ip="${detected_ip//[[:space:]]/}"

    VPS_IP="${AIWAY_VPS_IP:-${VPS_IP:-}}"
    DOT_DOMAIN="${AIWAY_DOT_DOMAIN:-${DOT_DOMAIN:-}}"
    ACME_EMAIL="${AIWAY_ACME_EMAIL:-${ACME_EMAIL:-}}"

    echo ""
    if [[ -z "$VPS_IP" ]]; then
        if [[ -n "$detected_ip" ]]; then
            echo -e "  Detected public IP: ${BOLD}${detected_ip}${RESET}"
            if [[ "${AIWAY_NONINTERACTIVE:-0}" == "1" ]]; then
                VPS_IP="$detected_ip"
            else
                read -rp "  VPS public IP [${detected_ip}]: " VPS_IP
                VPS_IP="${VPS_IP:-$detected_ip}"
            fi
        else
            if [[ "${AIWAY_NONINTERACTIVE:-0}" == "1" ]]; then
                print_error "AIWAY_VPS_IP is required in non-interactive mode"
                exit 1
            fi
            read -rp "  VPS public IP: " VPS_IP
        fi
    else
        print_info "Using saved/configured VPS IP: ${VPS_IP}"
    fi

    # Validate — loop until valid
    while ! [[ "$VPS_IP" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; do
        print_error "Invalid IP: ${VPS_IP:-<empty>}"
        read -rp "  VPS public IP: " VPS_IP
    done
    print_ok "VPS IP: ${VPS_IP}"

    # --- Optional domain for DoT / DoH ---
    echo ""
    echo -e "  ${DIM}Optional: a domain pointing to this server enables DoT (port 853) and DoH.${RESET}"
    echo -e "  ${DIM}Without a domain, plain DNS on port 53 still works on all devices.${RESET}\n"

    if [[ -z "$DOT_DOMAIN" && "${AIWAY_NONINTERACTIVE:-0}" != "1" ]]; then
        read -rp "  Domain for DoT/DoH (blank to skip): " DOT_DOMAIN
    fi
    DOT_DOMAIN="${DOT_DOMAIN:-}"
    DOT_DOMAIN="${DOT_DOMAIN#https://}"; DOT_DOMAIN="${DOT_DOMAIN#http://}"; DOT_DOMAIN="${DOT_DOMAIN%/}"

    if [[ -n "$DOT_DOMAIN" ]]; then
        print_ok "DoT/DoH domain: ${DOT_DOMAIN}"
        if [[ -z "$ACME_EMAIL" && "${AIWAY_NONINTERACTIVE:-0}" != "1" ]]; then
            read -rp "  Email for Let's Encrypt: " ACME_EMAIL
        fi
        while [[ -z "$ACME_EMAIL" ]]; do
            [[ "${AIWAY_NONINTERACTIVE:-0}" == "1" ]] && { print_error "AIWAY_ACME_EMAIL is required when AIWAY_DOT_DOMAIN is set"; exit 1; }
            print_error "Email required for ACME certificate."
            read -rp "  Email for Let's Encrypt: " ACME_EMAIL
        done
        print_ok "ACME email: ${ACME_EMAIL}"
    else
        print_warn "No domain — skipping DoT/DoH."
        ACME_EMAIL=""
    fi
}

install_runtime_assets() {
    print_step "Installing aiway runtime assets"

    mkdir -p "$AIWAY_RUNTIME_DIR/lib"

    install -m 755 "$SCRIPT_DIR/install.sh" "$AIWAY_RUNTIME_DIR/install.sh"
    install -m 755 "$SCRIPT_DIR/uninstall.sh" "$AIWAY_RUNTIME_DIR/uninstall.sh"
    install -m 644 "$SCRIPT_DIR/lib/utils.sh" "$AIWAY_RUNTIME_DIR/lib/utils.sh"
    install -m 644 "$SCRIPT_DIR/lib/domains.sh" "$AIWAY_RUNTIME_DIR/lib/domains.sh"
    install -m 755 "$SCRIPT_DIR/server/aiwayctl.sh" "$AIWAY_CTL_TARGET"

    cat > "$AIWAY_INSTALLER_ENV" <<EOF
AIWAY_VPS_IP="${VPS_IP}"
AIWAY_DOT_DOMAIN="${DOT_DOMAIN}"
AIWAY_ACME_EMAIL="${ACME_EMAIL}"
EOF

    touch "$AIWAY_CUSTOM_DOMAINS_FILE"
    print_ok "Runtime assets installed in ${AIWAY_RUNTIME_DIR}"
    print_ok "CLI installed at ${AIWAY_CTL_TARGET}"
}

# ── Free up port 53 ───────────────────────────────────────────────────────────
free_port_53() {
    print_step "Checking port 53"

    # Stop dnsmasq
    if systemctl is-active --quiet dnsmasq 2>/dev/null; then
        print_warn "dnsmasq is running on port 53 — stopping it"
        run_quietly "Stopping dnsmasq"           systemctl stop    dnsmasq
        run_quietly "Disabling dnsmasq autostart" systemctl disable dnsmasq
    fi

    # Stop bind9 / named
    for svc in bind9 named; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            print_warn "${svc} is running on port 53 — stopping it"
            run_quietly "Stopping ${svc}"            systemctl stop    "$svc"
            run_quietly "Disabling ${svc} autostart"  systemctl disable "$svc"
        fi
    done

    # systemd-resolved stub will be fixed by fix_resolved() below
    print_ok "Port 53 conflict checks done"
}

# ── Docker ───────────────────────────────────────────────────────────────────
ensure_docker() {
    print_step "Docker"

    # Check both binary presence AND daemon health
    if has_cmd docker && docker info &>/dev/null; then
        print_ok "Docker is installed and running ($(docker --version | cut -d' ' -f3 | tr -d ','))"
        return
    fi

    run_quietly "Downloading Docker install script" \
        curl -fsSL https://get.docker.com -o /tmp/get-docker.sh

    run_quietly "Installing Docker" bash /tmp/get-docker.sh
    rm -f /tmp/get-docker.sh

    run_quietly "Enabling Docker service" systemctl enable --now docker

    # Poll until daemon is up (up to 20 s)
    local i=0
    while ! docker info &>/dev/null && (( i < 20 )); do sleep 1; (( i++ )); done
    if ! docker info &>/dev/null; then
        print_error "Docker daemon failed to start. Check: systemctl status docker"
        exit 1
    fi

    print_ok "Docker installed and running"
}

# ── Angie ────────────────────────────────────────────────────────────────────
install_angie() {
    print_step "Angie (nginx fork with built-in ACME)"

    if has_cmd angie; then
        print_ok "Angie already installed ($(angie -v 2>&1 | head -1))"
        return
    fi

    # Install prerequisites (including dnsutils for 'dig' self-test)
    run_quietly "Installing prerequisites" \
        apt-get install -y -q curl gnupg2 ca-certificates lsb-release apt-transport-https dnsutils

    run_quietly "Adding Angie GPG key" bash -c \
        'curl -fsSL https://angie.software/angie/signing.asc | gpg --dearmor -o /usr/share/keyrings/angie.gpg'

    local codename
    codename=$(lsb_release -sc 2>/dev/null || echo "${OS_CODENAME:-}")
    [[ -z "$codename" ]] && { print_error "Cannot determine OS codename."; exit 1; }

    run_quietly "Adding Angie apt repository" bash -c \
        "echo 'deb [signed-by=/usr/share/keyrings/angie.gpg] https://deb.angie.software/angie/${OS_ID} ${codename} main' \
         > /etc/apt/sources.list.d/angie.list"

    # apt-get update runs once here; main() does NOT call it again before install_angie
    run_quietly "Updating apt cache" apt-get update -q

    run_quietly "Installing Angie" apt-get install -y -q angie

    run_quietly "Enabling Angie service" systemctl enable angie

    print_ok "Angie installed"
}

# ── systemd-resolved ──────────────────────────────────────────────────────────
fix_resolved() {
    print_step "systemd-resolved (DNSStubListener)"

    if ! systemctl is-active --quiet systemd-resolved 2>/dev/null; then
        print_info "systemd-resolved not running — skipping"
        return
    fi

    local resolved_conf="/etc/systemd/resolved.conf"
    [[ ! -f "$resolved_conf" ]] && { print_warn "$resolved_conf not found — skipping"; return; }

    # Backup once (never overwrite an existing backup — it's the original)
    [[ ! -f "${resolved_conf}.aiway.bak" ]] && cp "$resolved_conf" "${resolved_conf}.aiway.bak"
    print_info "Backup at ${resolved_conf}.aiway.bak"

    # Remove any existing DNSStubListener line (commented or not) then set to no
    sed -i '/^#\?DNSStubListener=/d' "$resolved_conf"
    echo "DNSStubListener=no" >> "$resolved_conf"

    run_quietly "Restarting systemd-resolved" systemctl restart systemd-resolved

    # Verify port 53 is now actually free (up to 5 s)
    if wait_for_port_free 53 5; then
        print_ok "Port 53 is free"
    else
        print_warn "Something is still on port 53 — check: ss -tlunp | grep :53"
    fi
}

# ── Generate Angie configuration ──────────────────────────────────────────────
#
# Architecture when DOT_DOMAIN is set (solves the stream/http port-443 conflict):
#
#   client ──443──► Angie stream (ssl_preread)
#                       │
#                       ├── SNI = DOT_DOMAIN ──► 127.0.0.1:8443 (http block, DoH)
#                       └── SNI = anything else ──► $ssl_preread_server_name:443
#
#   client ──853──► Angie stream (ssl_preread, terminates TLS) ──► 127.0.0.1:53 (Blocky)
#
# The http block NEVER binds 443 publicly — it only listens on 127.0.0.1:8443.
# The stream block handles ALL public port 443 traffic via SNI routing.
#
generate_angie_conf() {
    print_step "Generating Angie configuration"

    mkdir -p "$ANGIE_STREAM_DIR" "$ANGIE_HTTP_DIR" /var/log/angie /var/lib/angie/acme

    # ── main angie.conf ────────────────────────────────────────────────────
    cat > "$ANGIE_CONF" <<ANGIEEOF
# /etc/angie/angie.conf — generated by aiway $(date -u '+%Y-%m-%d %H:%M UTC')
# https://github.com/yourname/aiway

user www-data;
worker_processes auto;
pid /run/angie.pid;
include /etc/angie/modules-enabled/*.conf;

events {
    worker_connections 4096;
    multi_accept on;
    use epoll;
}

http {
    include      /etc/angie/mime.types;
    default_type application/octet-stream;

    access_log /var/log/angie/access.log;
    error_log  /var/log/angie/error.log warn;

    sendfile    on;
    tcp_nopush  on;
    tcp_nodelay on;
    keepalive_timeout 65;
    server_tokens off;

    include ${ANGIE_HTTP_DIR}/*.conf;
}

stream {
    log_format proxy '\$remote_addr [\$time_local] \$protocol \$status '
                     '\$bytes_sent \$bytes_received \$session_time '
                     '"\$ssl_preread_server_name"';

    access_log /var/log/angie/stream.log proxy;

    include ${ANGIE_STREAM_DIR}/*.conf;
}
ANGIEEOF

    # ── Append ACME block if domain provided ──────────────────────────────
    if [[ -n "$DOT_DOMAIN" ]]; then
        cat >> "$ANGIE_CONF" <<ACMEEOF

# Let's Encrypt via Angie built-in ACME client
acme {
    client letsencrypt {
        directory https://acme-v02.api.letsencrypt.org/directory;
        email     ${ACME_EMAIL};
    }

    certificate ${DOT_DOMAIN} {
        client  letsencrypt;
        domains ${DOT_DOMAIN};
        webroot /var/lib/angie/acme;
    }
}
ACMEEOF
    fi

    print_ok "Written: ${ANGIE_CONF}"

    # ── Stream block ───────────────────────────────────────────────────────
    if [[ -n "$DOT_DOMAIN" ]]; then
        # SNI map routes DOT_DOMAIN to local HTTPS (127.0.0.1:8443).
        # Everything else passes through to the real server by SNI name.
        # This is the ONLY correct way to have both a local HTTPS service
        # and a pass-through SNI proxy on the same port 443.
        cat > "${ANGIE_STREAM_DIR}/ai-proxy.conf" <<STREAMEOF
# SNI router: ${DOT_DOMAIN} → local HTTPS; all other TLS → passthrough
map \$ssl_preread_server_name \$stream_upstream {
    "${DOT_DOMAIN}"  "127.0.0.1:8443";
    default          "\$ssl_preread_server_name:443";
}

server {
    listen      443;
    ssl_preread on;

    proxy_pass            \$stream_upstream;
    proxy_connect_timeout 10s;
    proxy_timeout         600s;
    proxy_buffer_size     16k;
}

# DNS-over-TLS (853): terminates TLS, proxies plain DNS to Blocky on 53
server {
    listen     853 ssl;
    ssl_certificate     /etc/angie/acme/${DOT_DOMAIN}/fullchain.cer;
    ssl_certificate_key /etc/angie/acme/${DOT_DOMAIN}/${DOT_DOMAIN}.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
    ssl_session_cache   shared:DoT:4m;
    ssl_session_timeout 5m;

    proxy_pass            127.0.0.1:53;
    proxy_connect_timeout 5s;
    proxy_timeout         30s;
}
STREAMEOF
    else
        # No DOT_DOMAIN — pure blind passthrough, simplest possible config
        cat > "${ANGIE_STREAM_DIR}/ai-proxy.conf" <<STREAMEOF
# Pure SNI passthrough: all TLS forwarded unmodified to the real server
server {
    listen      443;
    ssl_preread on;

    proxy_pass            \$ssl_preread_server_name:443;
    proxy_connect_timeout 10s;
    proxy_timeout         600s;
    proxy_buffer_size     16k;
}
STREAMEOF
    fi
    print_ok "Written: ${ANGIE_STREAM_DIR}/ai-proxy.conf"

    # ── HTTP block ─────────────────────────────────────────────────────────
    if [[ -n "$DOT_DOMAIN" ]]; then
        cat > "${ANGIE_HTTP_DIR}/local-services.conf" <<HTTPEOF
# Port 80: redirect to HTTPS + ACME challenge
server {
    listen 80;
    server_name ${DOT_DOMAIN};

    location /.well-known/acme-challenge/ {
        root /var/lib/angie/acme;
        try_files \$uri =404;
    }
    location / { return 301 https://\$host\$request_uri; }
}

# Internal HTTPS on 127.0.0.1:8443 — reached via stream SNI map on port 443
# (never exposed directly to the internet)
server {
    listen 127.0.0.1:8443 ssl;
    server_name ${DOT_DOMAIN};

    ssl_certificate     /etc/angie/acme/${DOT_DOMAIN}/fullchain.cer;
    ssl_certificate_key /etc/angie/acme/${DOT_DOMAIN}/${DOT_DOMAIN}.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache   shared:DoH:4m;
    ssl_session_timeout 5m;

    add_header Strict-Transport-Security "max-age=31536000" always;

    # DNS-over-HTTPS: forward to Blocky's HTTP port
    location /dns-query {
        proxy_pass         http://127.0.0.1:4000/dns-query;
        proxy_http_version 1.1;
        proxy_set_header   Host \$host;
        proxy_set_header   X-Real-IP \$remote_addr;
        proxy_read_timeout 10s;
    }

    location / {
        return 200 'aiway is running\n';
        add_header Content-Type text/plain;
    }
}
HTTPEOF
        print_ok "Written: ${ANGIE_HTTP_DIR}/local-services.conf (with DoH on port 443 via SNI)"
    else
        cat > "${ANGIE_HTTP_DIR}/local-services.conf" <<HTTPEOF
# Minimal placeholder — no domain configured
server {
    listen 80 default_server;
    server_name _;
    return 444;
}
HTTPEOF
        print_ok "Written: ${ANGIE_HTTP_DIR}/local-services.conf (minimal)"
    fi
}

# ── Blocky configuration ──────────────────────────────────────────────────────
generate_blocky_config() {
    print_step "Generating Blocky DNS configuration"

    mkdir -p "$BLOCKY_DIR"

    local dns_entries=""
    for domain in "${AI_APEX_DOMAINS[@]}"; do
        dns_entries+="    ${domain}: ${VPS_IP}"$'\n'
    done

    cat > "$BLOCKY_CONFIG" <<BLOCKYEOF
# /opt/blocky/config.yml — generated by aiway
# https://0xerr0r.github.io/blocky/

upstreams:
  groups:
    default:
      - tcp+udp:8.8.8.8
      - tcp+udp:8.8.4.4
      - tcp+udp:1.1.1.1
      - tcp+udp:1.0.0.1

bootstrapDns:
  - 8.8.8.8
  - 1.1.1.1

# AI domains → this VPS IP; all other domains resolve normally
customDNS:
  mapping:
${dns_entries}
ports:
  dns:  53
  http: 4000   # DoH endpoint (proxied via Angie at /dns-query)

log:
  level:  warn
  format: text

caching:
  minTime:          5m
  maxTime:          30m
  prefetching:      true
  prefetchExpires:  2h
  prefetchThreshold: 5

queryLog:
  type: none
BLOCKYEOF

    print_ok "Written: ${BLOCKY_CONFIG}"
}

# ── Start Blocky container ────────────────────────────────────────────────────
start_blocky() {
    print_step "Starting Blocky DNS container"

    # Remove stale container if present
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q "^blocky$"; then
        run_quietly "Removing stale blocky container" docker rm -f blocky
    fi

    run_quietly "Pulling spx01/blocky image" docker pull spx01/blocky:latest

    # --network=host avoids Docker NAT for UDP 53 (more reliable than port mapping)
    docker run -d \
        --name  blocky \
        --restart=always \
        --network=host \
        -v "${BLOCKY_CONFIG}:/app/config.yml:ro" \
        spx01/blocky:latest >/dev/null

    print_ok "Blocky container started"

    # Poll for readiness (up to 30 s) — healthcheck or successful DNS query
    local deadline=$(( $(date +%s) + 30 ))
    local ready=false
    echo -ne "       ${DIM}Waiting for Blocky to accept queries...${RESET}"
    while (( $(date +%s) < deadline )); do
        if docker exec blocky blocky healthcheck &>/dev/null 2>&1; then
            ready=true; break
        fi
        if has_cmd dig && dig +short +time=1 openai.com @127.0.0.1 &>/dev/null 2>&1; then
            ready=true; break
        fi
        echo -ne "."
        sleep 1
    done
    echo ""

    if [[ "$ready" == "false" ]]; then
        if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^blocky$"; then
            print_error "Blocky container exited! Logs:"
            docker logs blocky 2>&1 | tail -20
            exit 1
        fi
        print_warn "Blocky startup timed out — container is running, may need another moment"
    fi

    # DNS self-test
    if has_cmd dig; then
        local result
        result=$(dig +short +time=5 +tries=2 openai.com @127.0.0.1 2>/dev/null | head -1 || true)
        if [[ "$result" == "$VPS_IP" ]]; then
            print_ok "DNS self-test passed: openai.com → ${VPS_IP}"
        elif [[ -n "$result" ]]; then
            print_warn "DNS self-test: openai.com → ${result} (expected ${VPS_IP}) — check ${BLOCKY_CONFIG}"
        else
            print_warn "DNS self-test inconclusive (empty response) — Blocky may still be initializing"
        fi
    fi
}

# ── Start Angie ───────────────────────────────────────────────────────────────
start_angie() {
    print_step "Starting Angie"

    # Config test first — shows exact error line if broken
    if ! angie -t 2>/tmp/angie-test.log; then
        print_error "Angie config test failed:"
        cat /tmp/angie-test.log; rm -f /tmp/angie-test.log
        exit 1
    fi
    rm -f /tmp/angie-test.log
    print_ok "Config syntax OK"

    run_quietly "Starting Angie" systemctl restart angie

    sleep 1
    if systemctl is-active --quiet angie; then
        print_ok "Angie is running"
    else
        print_error "Angie failed to start. Last 15 log lines:"
        journalctl -u angie -n 15 --no-pager
        exit 1
    fi
}

# ── Firewall ──────────────────────────────────────────────────────────────────
configure_firewall() {
    print_step "Firewall"

    local extra_ports=""
    [[ -n "$DOT_DOMAIN" ]] && extra_ports=", 853, 80"

    if has_cmd ufw && ufw status 2>/dev/null | head -1 | grep -q "active"; then
        # Older ufw versions don't support inline 'comment' — use plain form
        ufw allow 443/tcp >/dev/null 2>&1 || true
        ufw allow 53/udp  >/dev/null 2>&1 || true
        ufw allow 53/tcp  >/dev/null 2>&1 || true
        if [[ -n "$DOT_DOMAIN" ]]; then
            ufw allow 853/tcp >/dev/null 2>&1 || true
            ufw allow 80/tcp  >/dev/null 2>&1 || true
        fi
        print_ok "ufw: opened ports 443, 53${extra_ports}"

    elif has_cmd firewall-cmd; then
        firewall-cmd --permanent --add-port=443/tcp >/dev/null
        firewall-cmd --permanent --add-port=53/udp  >/dev/null
        firewall-cmd --permanent --add-port=53/tcp  >/dev/null
        if [[ -n "$DOT_DOMAIN" ]]; then
            firewall-cmd --permanent --add-port=853/tcp >/dev/null
            firewall-cmd --permanent --add-port=80/tcp  >/dev/null
        fi
        firewall-cmd --reload >/dev/null
        print_ok "firewalld: opened ports 443, 53${extra_ports}"

    else
        print_warn "No managed firewall detected."
        echo -e "  ${YELLOW}Open these ports in your VPS control panel / iptables:${RESET}"
        echo -e "  ${BOLD}  TCP 443  UDP/TCP 53${extra_ports:+  TCP 853  TCP 80}${RESET}"
    fi
}

# ── Final summary ─────────────────────────────────────────────────────────────
print_summary() {
    echo ""
    local w=62
    echo -e "${GREEN}${BOLD}"
    printf "  ╔%s╗\n" "$(printf '═%.0s' $(seq 1 $w))"
    printf "  ║  %-${w}s║\n" "✓  aiway installed successfully!"
    printf "  ╚%s╝\n" "$(printf '═%.0s' $(seq 1 $w))"
    echo -e "${RESET}"

    echo -e "  ${BOLD}Set DNS to this IP on your devices:${RESET}"
    echo -e "    ${CYAN}${BOLD}${VPS_IP}${RESET}   (plain DNS, UDP/TCP port 53)\n"

    if [[ -n "$DOT_DOMAIN" ]]; then
        echo -e "  ${BOLD}DNS-over-TLS:${RESET}    ${CYAN}${DOT_DOMAIN}${RESET}  (port 853)"
        echo -e "  ${BOLD}DNS-over-HTTPS:${RESET}  ${CYAN}https://${DOT_DOMAIN}/dns-query${RESET}\n"
    fi

    echo -e "  ${BOLD}Device setup:${RESET}"
    if [[ -n "$DOT_DOMAIN" ]]; then
        echo -e "  ${YELLOW}Android / iOS${RESET}   Private DNS → ${BOLD}${DOT_DOMAIN}${RESET}"
    else
        echo -e "  ${YELLOW}Android / iOS${RESET}   Wi-Fi DNS → ${BOLD}${VPS_IP}${RESET}"
    fi
    echo -e "  ${YELLOW}macOS${RESET}           System Preferences → Network → DNS → ${BOLD}${VPS_IP}${RESET}"
    echo -e "  ${YELLOW}Windows${RESET}         Settings → Network → DNS server → ${BOLD}${VPS_IP}${RESET}"
    echo -e "  ${YELLOW}Router${RESET}          DHCP primary DNS → ${BOLD}${VPS_IP}${RESET}  (covers all devices)\n"

    echo -e "  ${BOLD}Add more domains later:${RESET}"
    echo -e "  ${DIM}  Edit ${SCRIPT_DIR}/lib/domains.sh, then re-run: sudo bash install.sh${RESET}\n"

    echo -e "  ${BOLD}Useful commands:${RESET}"
    echo -e "  ${DIM}  docker logs -f blocky              # live DNS logs${RESET}"
    echo -e "  ${DIM}  systemctl status angie             # proxy status${RESET}"
    echo -e "  ${DIM}  dig openai.com @${VPS_IP}          # test DNS resolution${RESET}"
    echo -e "  ${DIM}  aiwayctl status                    # machine-readable status${RESET}"
    echo -e "  ${DIM}  aiwayctl doctor                    # connectivity + service checks${RESET}"
    echo -e "  ${DIM}  aiwayctl add-domain example.com    # add extra proxied service${RESET}"
    echo -e "  ${DIM}  sudo bash ${SCRIPT_DIR}/uninstall.sh   # remove aiway${RESET}\n"
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
    [[ "${AIWAY_NO_CLEAR:-0}" == "1" ]] || clear
    print_banner
    preflight
    gather_inputs

    print_step "Starting installation"

    # Single apt-get update here — install_angie does NOT call it again
    run_quietly "Updating apt package index" apt-get update -q

    ensure_docker
    install_angie
    free_port_53
    fix_resolved
    generate_angie_conf
    generate_blocky_config
    start_blocky
    start_angie
    configure_firewall
    install_runtime_assets

    _INSTALL_SUCCESS=true   # disarm cleanup trap
    print_summary
}

main "$@"
