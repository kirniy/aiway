# AIWAY Manager для Keenetic

`aiway` больше не заканчивается на этапе установки VPS. В репозитории теперь есть отдельный подпроект `router/`: это роутерный дашборд `AIWAY Manager`, вдохновлённый AWG Manager и рассчитанный на Keenetic + Entware.

Важно: панель **не обязательна**. Базовый путь проекта по-прежнему простой:

1. поставить `aiway` на VPS через `install.sh`
2. прописать DNS на роутере / телефоне / ноутбуке

Если хочется GUI на самом роутере, health-check, DNS toggle, SSH-управление VPS и LAN-friendly CLI/API - тогда подключается `AIWAY Manager`.

## Что умеет панель

- жить на самом роутере и открываться по адресу вроде `http://192.168.1.1:2233/routing`
- работать в двух режимах:
  - `DNS-only`: просто использовать уже существующий aiway endpoint (`IP + SNI`)
  - `Managed VPS`: дополнительно управлять сервером по SSH
- ставить `aiway` на новый VPS через SSH
- работать и с `username + password`, и с `SSH key`
- принимать приватный SSH-ключ прямо через веб-интерфейс, без ручной раскладки файла на роутере
- делать `install`, `sync`, `reset`, `uninstall` без ручного захода на сервер
- держать список нескольких VPS-профилей
- включать/выключать aiway DNS-режим на уровне панели
- выполнять health-check и включать fail-safe при серии ошибок
- добавлять кастомные домены в проксирование через `aiwayctl add-domain` в режиме `Managed VPS`
- отдавать LAN-friendly API и CLI для агентов и людей

## Установка на роутер одной командой

Зайди в Entware shell (`ssh admin@192.168.1.1` → `exec sh`) и выполни:

```sh
wget -qO- https://raw.githubusercontent.com/kirniy/aiway/main/router/scripts/install.sh | sh
```

Если на роутере нет `wget`, можно использовать:

```sh
curl -fsSL https://raw.githubusercontent.com/kirniy/aiway/main/router/scripts/install.sh | sh
```

Скрипт сам:

- определит архитектуру Keenetic / Entware
- найдет последний релиз `AIWAY Manager`
- скачает правильный `.ipk`
- установит пакет через `opkg`
- выведет локальный URL панели

## Структура

- `router/cmd/aiway-manager`: Go daemon + CLI
- `router/web`: AWG-style React UI
- `router/webui/dist`: встроенная веб-сборка для embedded serving
- `router/package`: init-скрипт и lifecycle-скрипты для Entware-пакета
- `router/scripts/install.sh`: установщик пакета по аналогии с AWG Manager

## CLI

После установки на роутер:

```bash
aiway-manager status --endpoint http://192.168.1.1:2233
aiway-manager check --endpoint http://192.168.1.1:2233
aiway-manager dns on --endpoint http://192.168.1.1:2233
aiway-manager domains add perplexity.ai --endpoint http://192.168.1.1:2233
aiway-manager profiles install --profile primary-vps --endpoint http://192.168.1.1:2233
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

На практике это покрывает разные Keenetic-модели, а не только тот роутер, на котором мы сейчас отлаживаемся.

## Поддержка роутеров

### Что уже есть сейчас

- Keenetic + Entware
- `mips-3.4_kn`
- `mipsel-3.4_kn`
- `aarch64-3.10_kn`
- проверка реального DNS-состояния на роутере, а не только желаемого состояния в UI
- поддержка `legacy` VPS-профилей: панель умеет читать статус старой установки и точечно править legacy-конфиги доменов без полной переустановки

Если у роутера есть Entware и системный `ndmc`, то архитектурно панель уже рассчитана не на один конкретный Keenetic-модельный номер, а на семейство Keenetic.

### Что пока не сделано

- OpenWrt / AsusWRT / MikroTik / FreshTomato / прочие роутеры

Для других роутеров сама идея переносима, но потребуется отдельный слой интеграции с системными настройками DNS/маршрутов. Сейчас продуктовая и кодовая опора сделана именно под Keenetic.

## Legacy VPS mode

Если на сервере уже живёт старая ручная установка `aiway`, панель может работать в бережном режиме:

- читать реальный статус Angie / Blocky / DNS
- использовать SSH для проверок
- показывать уже существующие домены из legacy-конфигов
- добавлять и удалять домены точечно без полной пересборки всей VPS-конфигурации

При этом destructive-операции вроде `install / sync / reset / uninstall` остаются скрытыми, пока сервер не переведён в полностью управляемый режим.

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
