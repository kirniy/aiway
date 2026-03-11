<div align="center">

```
    ___  _
   / _ \(_)_      ____ _ _   _
  / /_\ | \ \ /\ / / _` | | | |
 / /  | | |\ V  V / (_| | |_| |
 \/   |_|_| \_/\_/ \__,_|\__, |
                           |___/
```

**Прозрачный SNI-прокси для AI-сервисов. Без VPN. Одной командой.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Ubuntu%2020.04+%20%7C%20Debian%2011+-blue)](https://github.com/yourname/aiway)
[![Made in Russia](https://img.shields.io/badge/made%20in-Russia%20🇷🇺-red)](https://github.com/yourname/aiway)
[![Based on Habr](https://img.shields.io/badge/based%20on-habr.com%2F982070-65b4f3)](https://habr.com/ru/articles/982070/)

</div>

---

На дворе 2026 год. Если вы разработчик в РФ: утро начинается не с кофе, а с проверки того, что на этот раз отвалилось.

**aiway** решает это раз и навсегда: один скрипт, один VPS, и ChatGPT / Claude / Gemini / Copilot работают на **всех ваших устройствах сразу**: без кнопок, без приложений, без VPN.

## Быстрый старт

```bash
git clone https://github.com/yourname/aiway
cd aiway
sudo bash install.sh
```

Всё. Скрипт сам установит зависимости, всё настроит и в конце выведет IP, который нужно прописать в DNS на роутере/телефоне/ноутбуке.

## Keenetic dashboard

В репозитории теперь есть отдельный роутерный контур: `AIWAY Manager` для Keenetic / Entware.

Важно: это **опциональная** часть проекта. Если вам нужен только простой путь "поставить `aiway` на VPS и прописать DNS" - можно спокойно игнорировать `router/` и пользоваться только `install.sh`.

- веб-панель на самом роутере
- DNS-only режим: можно просто указать уже существующий `aiway` DNS endpoint без SSH-доступа к VPS
- установка `aiway` на новые VPS через SSH прямо из GUI
- `install / sync / reset / uninstall` без ручной возни в админке Keenetic
- health-check, fail-safe, кастомные домены и LAN-friendly CLI/API

Текущая реализация рассчитана на Keenetic + Entware и уже собирается под несколько архитектур (`mips`, `mipsel`, `aarch64`).

Установка на роутер одной командой из Entware shell:

```sh
wget -qO- https://raw.githubusercontent.com/kirniy/aiway/main/router/scripts/install.sh | sh
```

Если на роутере нет `wget`, можно так:

```sh
curl -fsSL https://raw.githubusercontent.com/kirniy/aiway/main/router/scripts/install.sh | sh
```

Подробности: [`docs/keenetic-dashboard.md`](docs/keenetic-dashboard.md) и подпроект [`router/`](router/).

## VPS hardening

Если хотите подтянуть защиту SSH от брутфорса на VPS, в репозитории есть готовый профиль fail2ban:

- `server/fail2ban-aiway-hardening.local`

Он включает более строгий `sshd` jail и `recidive` для повторных нарушителей.

---

## Как это работает

Вся магия держится на двух технологиях:

```
┌─────────────────────────────────────────────────────────────────┐
│                         Ваше устройство                         │
│                                                                 │
│  браузер/приложение  →  DNS-запрос: "где openai.com?"          │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Ваш VPS (aiway)                             │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Blocky DNS (порт 53)                                    │  │
│  │                                                          │  │
│  │  openai.com?   →  отвечает: IP вашего VPS               │  │
│  │  yandex.ru?    →  отвечает: настоящий IP Яндекса        │  │
│  │  вообще всё остальное → 8.8.8.8 как обычно              │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Angie SNI Proxy (порт 443)                              │  │
│  │                                                          │  │
│  │  читает имя домена из TLS ClientHello (открытым текстом) │  │
│  │  и пересылает соединение на настоящий сервер             │  │
│  │                                                          │  │
│  │  ключи НЕ знает  •  трафик НЕ расшифровывает            │  │
│  │  сертификат остаётся оригинальным (от OpenAI)            │  │
│  └──────────────────────────────────────────────────────────┘  │
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  api.openai.com        │
                    │  (настоящий сервер)    │
                    └───────────────────────┘
```

**DNS (Blocky)**: говорит вашим устройствам, что `openai.com` живёт на IP вашего VPS. Для всего остального: работает как обычный DNS-резолвер.

**SNI Proxy (Angie)**: читает имя домена из первого пакета TLS (он идёт открытым текстом: это стандарт протокола) и перекидывает соединение на настоящий сервер. Работает как почтальон, который смотрит только на адрес на конверте и не вскрывает письмо.

> **Почему это лучше VPN?** Потому что `gosuslugi.ru`, `sber.ru` и всё остальное идёт напрямую. Через VPS летит только трафик к AI-сервисам: строго хирургически.

---

## Что проксируется

| Сервис | Домены |
|:--|:--|
| 🤖 ChatGPT / OpenAI | `openai.com`, `chatgpt.com`, `oaiusercontent.com` |
| 🧠 Claude / Anthropic | `claude.ai`, `anthropic.com` |
| ✨ Gemini / Google AI | `gemini.google.com`, `aistudio.google.com`, `generativelanguage.googleapis.com` |
| 👾 GitHub Copilot | `github.com`, `githubcopilot.com`, `copilot.microsoft.com` |
| 🔍 Perplexity | `perplexity.ai` |
| 🎨 Midjourney | `midjourney.com` |
| 🤗 Hugging Face | `huggingface.co` |
| ⚡ xAI / Grok | `x.ai`, `grok.com` |
| 🌊 Mistral | `mistral.ai` |
| 🦙 Meta AI | `meta.ai` |
| 🎵 Udio | `udio.com` |
| 🔄 Replicate | `replicate.com` |
| 🎯 Stability AI | `stability.ai` |
| 💬 Poe, Character.ai, You.com, Pi | по одному домену каждый |
| 🦜 Cohere | `cohere.ai` |

Все субдомены (`api.openai.com`, `files.oaiusercontent.com` и т.д.) резолвятся автоматически.

---

## Требования

| | |
|:--|:--|
| **VPS** | Ubuntu 20.04+ или Debian 11+, ~512 МБ RAM, белый IPv4 |
| **Порты** | 443/tcp, 53/udp+tcp открыты в файрволе |
| **Клиент** | Прописать IP вашего VPS как DNS на устройстве/роутере |

Скрипт сам поставит: `docker`, `angie`, `dnsutils`. Больше ничего устанавливать не нужно.

---

## Установка

### Минимальная (просто DNS, без доменного имени)

```bash
sudo bash install.sh
```

Скрипт спросит IP вашего VPS (определит автоматически) и всё настроит. В конце получите IP для DNS.

### С DNS-over-TLS и DNS-over-HTTPS (рекомендуется)

Если у вас есть домен, указывающий на VPS:

```bash
sudo bash install.sh
# скрипт спросит:
#   Domain for DoT/DoH: dns.example.com
#   Email for Let's Encrypt: you@example.com
```

Тогда дополнительно заработает:
- `dns.example.com` на порту **853**: DNS-over-TLS (Private DNS на Android 9+, iOS 14+)
- `https://dns.example.com/dns-query`: DNS-over-HTTPS

### Удаление

```bash
sudo bash uninstall.sh
```

Остановит Blocky, уберёт конфиги Angie, восстановит systemd-resolved из бэкапа.

---

## Настройка клиентов

Пропишите IP вашего VPS как DNS-сервер:

<table>
<tr>
<td><b>📱 Android</b></td>
<td>

Если есть DoT:
`Настройки → Сеть → Частный DNS → Имя хоста: dns.example.com`

Если только IP:
`Настройки → Wi-Fi → (сеть) → IP-настройки: Статичный → DNS: ВАШ_IP`

</td>
</tr>
<tr>
<td><b>🍎 iOS / iPadOS</b></td>
<td>

`Настройки → Wi-Fi → (i) → Настроить DNS → Вручную → ВАШ_IP`

Или через профиль `.mobileconfig` с DoH (если настроен домен).

</td>
</tr>
<tr>
<td><b>🍏 macOS</b></td>
<td>

`Системные настройки → Сеть → Wi-Fi → Подробнее → DNS → добавить ВАШ_IP`

</td>
</tr>
<tr>
<td><b>🪟 Windows</b></td>
<td>

`Параметры → Сеть и Интернет → Wi-Fi → (сеть) → DNS-сервер → Вручную → ВАШ_IP`

</td>
</tr>
<tr>
<td><b>🐧 Linux</b></td>
<td>

```bash
echo "nameserver ВАШ_IP" | sudo tee /etc/resolv.conf
```

</td>
</tr>
<tr>
<td><b>📡 Роутер (рекомендуется)</b></td>
<td>

В настройках DHCP поставить основной DNS = `ВАШ_IP`.
Тогда все устройства в сети получат его автоматически: телефоны, ноутбуки, телевизоры, всё.

</td>
</tr>
</table>

---

## Добавить новый сервис

Если какой-то новый AI-сервис не работает, нужно найти его домены и добавить в список.

### Шаг 1: узнать домены

Самый простой способ: смотреть DNS-логи Blocky, пока приложение открыто:

```bash
docker logs -f blocky 2>&1 | grep -v "NOERROR\|127.0.0.1"
# открываете приложение и смотрите, какие домены резолвятся
```

Или через `dig` / `nslookup` напрямую:

```bash
nslookup super-ai.io
# смотрите A-запись
```

### Шаг 2: добавить в `lib/domains.sh`

```bash
AI_DOMAINS=(
    ...
    "super-ai.io"
    "*.super-ai.io"       # покрывает api.super-ai.io, cdn.super-ai.io и т.д.
)

AI_APEX_DOMAINS=(
    ...
    "super-ai.io"         # Blocky автоматически матчит все субдомены
)
```

### Шаг 3: применить

```bash
sudo bash install.sh
# конфиги Angie и Blocky перегенерируются, сервисы перезапустятся
```

Проверяем:

```bash
dig super-ai.io @ВАШ_VPS_IP +short   # должен вернуть IP вашего VPS
```

---

## Архитектура (технические детали)

<details>
<summary>Почему stream и http блок Angie не конфликтуют на порту 443</summary>

Это нетривиальный момент. Если у вас настроен домен для DoH, нужно, чтобы:

- Порт 443 обрабатывал **все входящие TLS** через SNI proxy
- Но также существовал HTTPS-сервер для `dns.example.com/dns-query`

Наивное решение: добавить `listen 443 ssl` в `http` блок: **не работает**: nginx/Angie не позволяет одновременно слушать 443 в `stream` и в `http`.

**Правильная архитектура:**

```
stream {
    # SNI map: домен DoH → внутренний порт 8443; всё остальное → passthrough
    map $ssl_preread_server_name $upstream {
        "dns.example.com"  "127.0.0.1:8443";
        default            "$ssl_preread_server_name:443";
    }

    server {
        listen 443;
        ssl_preread on;
        proxy_pass $upstream;
    }
}

http {
    server {
        # Только локальный адрес: снаружи недоступен напрямую
        listen 127.0.0.1:8443 ssl;
        location /dns-query { proxy_pass http://127.0.0.1:4000/dns-query; }
    }
}
```

Stream-блок принимает **всё** на 443. Для домена DoH: перебрасывает на `127.0.0.1:8443` (где сидит http-сервер). Для всего остального: слепой passthrough по имени из SNI.

</details>

<details>
<summary>Почему Blocky запускается с --network=host вместо port mapping</summary>

UDP + Docker NAT = боль. При маппинге `-p 53:53/udp` Docker добавляет слой трансляции адресов, что на некоторых дистрибутивах приводит к потере UDP-пакетов под нагрузкой.

`--network=host` убирает этот слой: Blocky напрямую биндит порты хоста. Единственный минус: порт 4000 (DoH) тоже торчит наружу, но он без аутентификации доступен только для чтения DNS, что абсолютно нормально.

</details>

<details>
<summary>О ECH и QUIC (почему с ними могут быть нюансы)</summary>

**ECH (Encrypted Client Hello)**: шифрует SNI. Если браузер включит ECH, наш прокси не увидит имя домена и не поймёт, куда пересылать. Плюс ТСПУ в РФ активно блокируют ECH-соединения.

Отключить в Chrome: `chrome://flags` → Encrypted Client Hello → Disabled.

**QUIC (HTTP/3, UDP 443)**: SNI-прокси работает только с TCP. Если приложение уходит на QUIC, оно идёт мимо прокси напрямую (и, скорее всего, получает блок). Исправляется блокировкой UDP 443 на VPS: тогда клиент откатывается на TCP:

```bash
iptables -I INPUT -p udp --dport 443 -j DROP
```

</details>

---

## Диагностика

```bash
# Blocky работает?
docker ps | grep blocky
docker logs blocky --tail 20

# DNS резолвится правильно?
dig openai.com @ВАШ_IP +short    # должен вернуть IP вашего VPS

# Angie работает?
systemctl status angie
journalctl -u angie -n 30

# Что биндит порт 53?
ss -tlunp | grep :53

# Порт 443 слушается?
ss -tlnp | grep :443
```

---

## Контакты

Вопросы, баги, предложения:

- 📧 [kirniy@me.com](mailto:kirniy@me.com)
- 💬 [t.me/kirniy](https://t.me/kirniy)
- 🐛 [GitHub Issues](https://github.com/yourname/aiway/issues)

---

## Документация

Полная документация на русском в папке [`docs/`](docs/):

| Файл | Содержание |
|:--|:--|
| [как-это-работает.md](docs/как-это-работает.md) | Технический разбор SNI proxy и DNS, архитектура, ECH, QUIC |
| [keenetic-dashboard.md](docs/keenetic-dashboard.md) | Дашборд на роутере, CLI/API, Entware-пакет, управление VPS через SSH |
| [устройства.md](docs/устройства.md) | Настройка DNS на роутере, Android, iOS, macOS, Windows, Linux |
| [диагностика.md](docs/диагностика.md) | Что делать если не работает, типичные ошибки, полезные команды |
| [faq.md](docs/faq.md) | Ответы на частые вопросы |

---

## Благодарности

Метод целиком основан на статье **[crims0n](https://habr.com/ru/users/crims0n_ru/)** на Хабре:

> [Свой луна-парк с блэкджеком и нейронками: Восстанавливаем доступ к AI-сервисам без VPN. Часть 1](https://habr.com/ru/articles/982070/)

Спасибо за детальный разбор архитектуры, аналогию с конвертом и за то, что объяснил, почему SNI это не страшно.

---

## Лицензия

[MIT](LICENSE)
