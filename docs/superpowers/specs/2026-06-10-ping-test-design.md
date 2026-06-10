# Ping / test server (TCP-dial latency) — Design

Date: 2026-06-10
Status: approved
Phase: 5 (#3 of 3 — autostart ✓ → ping/test → traffic stats)

## Problem

There is no way to check whether a configured server is reachable or how fast it
responds. Users add a server link and can only find out by connecting.

## Goal

A per-server "Пинг" button in the Settings server list that measures TCP connect
latency to the server's `host:port` and shows the result inline (e.g. `32 мс`,
`таймаут`, `недоступен`). No xray spin-up; works whether or not connected. This
is the "ping" of Hiddify/v2rayN — it measures reachability + latency of the
endpoint, not that VLESS/Reality or internet egress works (that is the future
proxy-test).

## Architecture

New package `internal/netping`:

```go
// Ping dials addr ("host:port") over TCP and returns the connect round-trip
// time. The dial is bounded by timeout. A successful dial is closed immediately.
func Ping(addr string, timeout time.Duration) (time.Duration, error)
```

Implementation: record start, `net.DialTimeout("tcp", addr, timeout)`, on success
`conn.Close()` and return `time.Since(start)`. Pure and testable against a local
listener.

## Service layer

`PingResultDTO` (in `internal/app/types.go`):

```go
// PingResultDTO is the outcome of a server reachability test.
type PingResultDTO struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latencyMs"`
	Error     string `json:"error"`
}
```

`Ping` (new file `internal/app/ping.go`):

```go
const pingTimeout = 5 * time.Second

// Ping measures TCP connect latency to the server at the given index. The dial
// runs outside the lock so concurrent pings don't block other operations.
func (s *Service) Ping(index int) PingResultDTO {
	s.mu.Lock()
	if index < 0 || index >= len(s.state.Servers) {
		s.mu.Unlock()
		return PingResultDTO{Error: "неверный сервер"}
	}
	srv := s.state.Servers[index]
	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	s.mu.Unlock()

	d, err := netping.Ping(addr, pingTimeout)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return PingResultDTO{Error: "таймаут"}
		}
		return PingResultDTO{Error: "недоступен"}
	}
	return PingResultDTO{OK: true, LatencyMs: int(d.Milliseconds())}
}
```

**Concurrency:** Wails dispatches each bound method on its own goroutine; the dial
runs outside `s.mu`; `Ping` only reads state (under the lock) and mutates nothing,
so concurrent pings are safe.

## gui_app

A bound method delegating to the service (mirrors `CheckUpdate` returning a
struct):

```go
// Ping measures TCP connect latency to the server at index. Bound to the frontend.
func (a *App) Ping(index int) app.PingResultDTO {
	return a.svc.Ping(index)
}
```

## Frontend

- `frontend/wailsjs/go/models.ts`: generated `PingResultDTO` class.
- `frontend/src/main.ts`:
  - import `Ping`.
  - a module-scoped `pingResults: Record<string, string> = {}` keyed by
    `${host}:${port}` (stable across index shifts when a server is removed).
  - in `render()`, each server `<li>` gets a "Пинг" button and a result
    `<span>` whose text is `pingResults[key] ?? ""`.
  - the button handler sets the span to `…`, calls `Ping(i)`, then writes
    `${latencyMs} мс` on `ok`, otherwise the `error` string, into `pingResults`
    and updates the span (re-render or direct text set).

## Testing

- **`internal/netping`** (`netping_test.go`): success against a `net.Listen`
  ephemeral listener returns a latency `> 0` and `< timeout`; a dial to a closed
  port returns an error; a malformed address returns an error.
- **`internal/app`** (`ping_test.go`): `Ping` with an out-of-range index →
  `OK=false`, `Error="неверный сервер"`; `Ping` with a server whose
  `Host=127.0.0.1`, `Port=<live local listener>` (injected directly into
  `svc.state.Servers`) → `OK=true`; a closed port → `OK=false`.
- **Frontend:** manual QA.

## Verification

`go test ./...`, `wails build -tags "wails webkit2_41"`,
`GOOS=windows GOARCH=amd64 go build -tags wails ./...`,
`cd frontend && npm run build`.

## Out of scope

- Full proxy test (xray spin-up + HTTP 204) — future.
- "Ping all" at once — per-server button only (YAGNI).
- Pinging the active server from the Главная tab — Settings list only.
- Persisting results — ephemeral, frontend-only.
