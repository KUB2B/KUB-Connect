# Anthropic preset + Telegram moved into toggle grid — дизайн

Дата: 2026-07-02

## Контекст

Whitelist-routing UI (`internal/routing/profile.go`, `frontend/index.html`, `frontend/src/main.ts`) уже предлагает 8 service-пресетов (YouTube, Instagram, Facebook, Twitter/X, Discord, Netflix, Spotify, ChatGPT/OpenAI) как чекбоксы в сетке `#preset-list`, плюс отдельный чекбокс Telegram над этой сеткой со своим хинтом. Запрос: добавить пресет Anthropic (Claude) в сетку и визуально перенести Telegram внутрь той же сетки, сохранив «включён по умолчанию».

## 1. Новый пресет Anthropic

**Категория подтверждена офлайн:** `strings data/geosite.dat | grep -i anthropic` даёт `claude.ai`, `anthropic.com`, `claude.com`, `statsig.anthropic.com` и др. — категория `anthropic` реально присутствует в текущем embedded `geosite.dat`, значит `geosite:anthropic` резолвится в реальные домены при билде xray-конфига.

**Изменение:** `internal/routing/profile.go`, в срез `Presets` добавляется:
```go
{Key: "anthropic", Title: "Claude (Anthropic)", Domains: []string{"geosite:anthropic"}},
```
Порядок — в конец среза (после `openai`), без переупорядочивания существующих записей (`ProxyPresets` хранит ключи, порядок в срезе на персистентность не влияет).

**Frontend:** `frontend/src/main.ts`, в массив `PRESETS` (комментарий "Keep in sync with routing.Presets" уже требует держать в паре) добавляется:
```ts
{ key: "anthropic", title: "Claude (Anthropic)" },
```
в конец массива, тем же порядком, что и в Go.

Никаких изменений схемы `Profile` не требуется — пресет живёт целиком в существующем механизме `ProxyPresets []string` + `PresetByKey`/`presetDomains()`.

## 2. Telegram — визуальный перенос в сетку пресетов

**Текущее состояние** (`frontend/index.html`, внутри `#whitelist-only`):
```html
<label><input type="checkbox" id="tg-toggle" /> Telegram → VPN</label>
<span class="hint">Заворачивать Telegram в туннель.</span>
<div id="preset-list" class="preset-list"></div>
<span class="hint">Отмеченные сервисы идут через VPN. Если сервер пропускает только Telegram — оставьте всё выключенным.</span>
```

**Меняется на:**
```html
<div id="preset-list" class="preset-list">
  <label><input type="checkbox" id="tg-toggle" class="toggle-switch" /> Telegram → VPN</label>
</div>
<span class="hint">Отмеченные сервисы идут через VPN. Telegram включён по умолчанию — если сервер пропускает только его, оставьте остальное выключенным.</span>
```

- `#tg-toggle` становится первым статическим ребёнком `#preset-list`; `PRESETS.forEach(...)` в `wire()` (`frontend/src/main.ts`) вызывает `presetBox.append(label)` для каждого пресета — это ДОБАВЛЕНИЕ в конец существующих детей, так что Telegram гарантированно остаётся первым в сетке без изменений в JS-цикле рендера пресетов.
- `class="toggle-switch"` — переиспользуется существующий CSS-класс (толгл-свитч, введён для пресетов), чтобы Telegram визуально не отличался от остальных карточек сетки. Новый CSS не нужен.
- Отдельная строка-хинт "Заворачивать Telegram в туннель." удаляется, текст общего хинта под сеткой правится (см. выше) — объясняет и назначение сетки, и default-on статус Telegram.
- `Profile.Telegram bool` и его routing-логика (`internal/routing/profile.go:141-144`, baked-in `TelegramCIDRs` + `geosite:telegram`) НЕ меняются. `Default()` уже возвращает `Telegram: true` — поведение "включён по умолчанию" уже есть на бэкенде для новых профилей, эта задача только про то, ГДЕ чекбокс визуально стоит.

## 3. Обязательный сопутствующий фикс: изоляция pushProfile-селектора

**Проблема:** обработчик изменения пресет-чекбоксов в `wire()`:
```ts
cb.addEventListener("change", () => {
  current.profile.proxyPresets = Array.from(
    document.querySelectorAll<HTMLInputElement>("#preset-list input:checked"),
  ).map((c) => c.value);
  pushProfile();
});
```
использует `document.querySelectorAll("#preset-list input:checked")` — глобальный по контейнеру селектор. После переноса `#tg-toggle` внутрь `#preset-list` этот селектор при СМЕНЕ ЛЮБОГО пресета (не только Telegram) будет ЗАХВАТЫВАТЬ и `#tg-toggle`, если он в этот момент отмечен (а он отмечен по умолчанию). У чекбокса без явного атрибута `value` браузер отдаёт `.value === "on"` — значит `profile.proxyPresets` будет засоряться строкой `"on"` при каждом изменении любого пресета, пока Telegram включён.

Бэкенд не упадёт (`presetDomains()` молча пропускает неизвестные ключи через `PresetByKey`), но в сохранённый `profile.json` будет попадать мусорное значение — грязное состояние, которое не должно тут появляться.

**Фикс:** в `wire()`, в блоке `PRESETS.forEach((p) => {...})`, добавить маркер на каждый ПРЕСЕТНЫЙ чекбокс:
```ts
cb.dataset.preset = "1";
```
и заменить селектор в обработчике на:
```ts
document.querySelectorAll<HTMLInputElement>('#preset-list input[data-preset]:checked')
```
`#tg-toggle` не получает `data-preset`, поэтому в эту выборку не попадает; его состояние по-прежнему пишется отдельным, уже существующим обработчиком `$("tg-toggle").addEventListener("change", ...)`, который явно устанавливает `current.profile.telegram`.

## Не входит в эту задачу

- Изменение `Profile`-схемы (Telegram остаётся отдельным bool-полем, не мигрирует в `ProxyPresets`).
- Иконки для пресетов — как и раньше, чисто текстовые toggle-чекбоксы.
- Default-on для новых пресетов (Anthropic по умолчанию выключен, как и остальные 8 — только Telegram имеет default-on, и это уже существующее поведение).

## Риски / открытые вопросы

Нет открытых вопросов — категория `geosite:anthropic` подтверждена офлайн-грепом эмбеддед `data/geosite.dat`, схема данных не меняется, единственный найденный побочный эффект (селектор `pushProfile`) зафиксирован как обязательный шаг реализации, а не риск.
