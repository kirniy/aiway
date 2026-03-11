# AIWAY Manager для Keenetic

`aiway` больше не заканчивается на этапе установки VPS. В репозитории теперь есть отдельный подпроект `router/`: это роутерный дашборд `AIWAY Manager`, вдохновлённый AWG Manager и рассчитанный на Keenetic + Entware.

## Что умеет панель

- жить на самом роутере и открываться по адресу вроде `http://192.168.1.1:2222/routing`
- ставить `aiway` на новый VPS через SSH
- работать и с `username + password`, и с `SSH key`
- делать `install`, `sync`, `reset`, `uninstall` без ручного захода на сервер
- держать список нескольких VPS-профилей
- включать/выключать aiway DNS-режим на уровне панели
- выполнять health-check и включать fail-safe при серии ошибок
- добавлять кастомные домены в проксирование через `aiwayctl add-domain`
- отдавать LAN-friendly API и CLI для агентов и людей

## Структура

- `router/cmd/aiway-manager`: Go daemon + CLI
- `router/web`: AWG-inspired React UI
- `router/webui/dist`: встроенная веб-сборка для embedded serving
- `router/package`: init-скрипт и lifecycle-скрипты для Entware-пакета
- `router/scripts/install.sh`: установщик пакета по аналогии с AWG Manager

## CLI

После установки на роутер:

```bash
aiway-manager status --endpoint http://192.168.1.1:2222
aiway-manager check --endpoint http://192.168.1.1:2222
aiway-manager dns on --endpoint http://192.168.1.1:2222
aiway-manager domains add perplexity.ai --endpoint http://192.168.1.1:2222
aiway-manager profiles install --profile primary-vps --endpoint http://192.168.1.1:2222
```

Это обычный HTTP API, поэтому им удобно пользоваться из локальной сети, из терминала и из агентных систем.

## Сборка пакетов

```bash
cd router
make package
```

Собираются три Entware-пакета:

- `aarch64-3.10`
- `mips-3.4`
- `mipsel-3.4`

## Что делает VPS-сторона

На VPS теперь ставится `aiwayctl`:

- `aiwayctl status`
- `aiwayctl doctor`
- `aiwayctl list-domains`
- `aiwayctl add-domain example.com`
- `aiwayctl remove-domain example.com`
- `aiwayctl reapply`
- `aiwayctl uninstall`

Именно через этот слой роутерный дашборд управляет установленным `aiway`.
