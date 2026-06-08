# Release Prep — v0.1.0 (Windows + macOS)

**Дата:** 2026-06-08
**Статус:** design approved, ожидает ревью spec

## Цель

Подготовить первый публичный релиз `vless-client` (v0.1.0) для Windows и macOS:
собрать, упаковать и опубликовать в GitHub Releases с user-facing документацией.

Приоритеты релиза: **упаковка/installer** и **документация**. CI/CD и Linux-релиз — вне scope.

## Подход

Выбран **подход A**: локальные build-скрипты + ручная публикация. Использует имеющийся
Mac + Apple Developer аккаунт для подписи/нотаризации macOS; Windows кросс-компилируется
из Linux. Скрипты структурированы под перенос в CI позже (секреты через env-vars).

## Scope (что шипим в v0.1.0)

| Платформа | Proxy | TUN | Kill switch |
|-----------|-------|-----|-------------|
| Windows   | ✅     | ✅   | ❌           |
| macOS     | ✅     | ❌   | ❌           |

- Версия: **v0.1.0** (первый публичный; не 1.0 — функционал не на паритете между ОС).
- Канал: GitHub Releases.
- Формат: installer — Windows NSIS (`*-installer.exe`), macOS `.dmg`.
- Windows-бинарь **не подписан** code-signing сертификатом (бесплатного варианта, снимающего
  SmartScreen warning, нет). macOS-бинарь подписан и нотаризован.

### Вне scope (явно)

- macOS TUN-режим (routing не реализован — `unsupportedRouter` возвращает ошибку).
- Kill switch на любой ОС (`firewall` — stub на Windows/macOS).
- Phase 5: autostart, ping, статистика трафика.
- CI/CD пайплайн.
- Linux-релиз (несмотря на готовность — не целевая платформа этого релиза).
- Windows code-signing.

Эти ограничения документируются в README/INSTALL как «Known limitations».

## Компоненты

### 1. Версия и метаданные

- `build/windows/info.json`: заполнить `ProductName` = `VLESS Client`, `ProductVersion`,
  `CompanyName`, `LegalCopyright`. Сейчас — пустой шаблон.
- Версия в бинаре через ldflags: `-X main.version=v0.1.0`. Источник версии — git-тег.
  - GUI показывает версию; headless поддерживает `--version`.
- macOS `Info.plist` (генерит Wails): bundle ID `pro.qb2b.vless-client`, версия, иконка.
- Git-тег `v0.1.0` — единый источник версии для скриптов.

**Граница:** версия течёт из git-тега → ldflags → бинарь → UI. Один источник правды.

### 2. GUI: гейтинг TUN на macOS

Проблема: `mode-select` показывает все режимы на всех ОС. На macOS выбор TUN приводит к
ошибке из `unsupportedRouter` в рантайме.

- **Backend:** бинд-метод (напр. `Platform()`), возвращающий
  `{os, tunSupported, killSwitchSupported}`. Использует `firewall.Supported()` и аналогичный
  признак для netcfg-роутера (добавить `Supported()` или эквивалент в `netcfg`).
- **Frontend:** на macOS убрать/задизейблить TUN-опцию в `mode-select`. Если в сохранённых
  настройках режим был TUN, а платформа не поддерживает — фолбэк на proxy при загрузке.
- Windows: TUN остаётся доступен; killswitch-контролов в UI нет (подтвердить при имплементации).

**Граница:** фронт не хардкодит список платформ — спрашивает backend через `Platform()`.

**Тестирование:** backend-признаки и фолбэк-логика покрываются юнит-тестами (TDD). Это
единственный кодовый компонент релиза.

### 3. Build & packaging

Оркестратор `scripts/release.sh` + per-OS шаги. Версия из git-тега (`git describe --tags`).

**Windows** (кросс-компиляция из Linux):

```bash
wails build -platform windows/amd64 -tags wails \
  -ldflags "-X main.version=$VER" -nsis
```

→ `vless-client-amd64-installer.exe` (не подписан).

**macOS** (на Mac):

```bash
wails build -platform darwin/universal -tags wails \
  -ldflags "-X main.version=$VER"
```

Затем:
- `codesign --deep --options runtime` (Developer ID Application)
- упаковка в `.dmg`
- `xcrun notarytool submit --wait`
- `xcrun stapler staple`

**Артефакты:** `build/release/`, имена с версией. Секреты подписи (Apple ID, team ID,
app-specific password / API key) — через env-vars, не хардкод.

**Публикация:** `gh release create v0.1.0 build/release/* --title ... --notes-file CHANGELOG`.

### 4. Документация

- **`README.md`** — остаётся dev-ориентированным (сборка, dev-режим), добавить ссылку на
  user-гайд.
- **`docs/INSTALL.md`** (RU, user-facing):
  - Windows: скачать installer → SmartScreen warning («Подробнее → Всё равно выполнить») →
    установка → добавить VLESS-ссылку → выбрать proxy/TUN.
  - macOS: скачать `.dmg` → перетащить в Applications → первый запуск → proxy-режим.
  - Объяснение whitelist-маршрутизации (Telegram + кастом в VPN, остальное direct) простыми
    словами.
  - Секция «Known limitations» (см. Scope).
- **`CHANGELOG.md`** — запись v0.1.0 с содержимым релиза.

### 5. Чистка репо

- Удалить `app.test` (5 МБ тестовый бинарь, закоммичен по ошибке) из git-трекинга.
- `.gitignore`: добавить `build/release/`, `*.dmg`, `*-installer.exe`.
- `build/bin/` уже игнорируется; рабочие артефакты (`vless-client*`, `geo*.dat`) остаются
  untracked — не трекать.
- Проверить отсутствие других мусорных артефактов в трекинге.

## Последовательность работ

1. Чистка репо (компонент 5).
2. Метаданные + ldflags-версия (компонент 1).
3. GUI-гейтинг TUN на macOS (компонент 2) — **через TDD**.
4. Build-скрипты (компонент 3).
5. User-доки + CHANGELOG (компонент 4).
6. Ручное QA: Windows (proxy + TUN), macOS (proxy) на реальных машинах.
7. Тег `v0.1.0` → `gh release create`.

Компонент 2 — код, идёт через TDD. Компоненты 1/3/4/5 — конфиг/скрипты/доки.

## Известные ограничения (для документации)

- Kill switch не реализован ни на одной ОС.
- macOS: только proxy-режим (TUN не реализован).
- Windows-бинарь не подписан → SmartScreen warning при первом запуске.
- Нет авто-обновления.

## Критерии готовности

- `scripts/release.sh` собирает Windows installer и macOS подписанный+нотаризованный `.dmg`.
- macOS `.dmg` открывается без обхода Gatekeeper вручную.
- GUI на macOS не предлагает TUN-режим; фолбэк работает.
- `docs/INSTALL.md` + `CHANGELOG.md` написаны.
- `app.test` и прочий мусор убраны из git.
- Релиз v0.1.0 опубликован в GitHub Releases с обоими артефактами.
