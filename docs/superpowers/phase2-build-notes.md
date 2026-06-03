# Phase 2 build/verification notes

## Cross-platform build status (verified 2026-06-03)

| Package | linux | darwin | windows |
|---|---|---|---|
| internal/privilege | built + tested | builds | builds |
| internal/sysproxy  | built + tested | builds (test on mac) | builds (test on win) |
| internal/tun       | built (+root smoke skips unprivileged) | builds | builds |
| internal/netcfg    | built + tested | NOT built (Linux-only PoC) | NOT built (Linux-only PoC) |
| internal/tunnel    | built + tested | builds | builds |
| cmd/headless       | built (proxy+tun) | NOT built (needs netcfg) | NOT built (needs netcfg) |

`go test ./...` is green on Linux (root-gated TUN test skips when unprivileged).
`go build ./...`, `go vet ./...`, `gofmt -l .` all clean on Linux.
Library packages cross-build verified: `GOOS=darwin` and `GOOS=windows`
`go build ./internal/{sysproxy,privilege,tun,tunnel}` all succeed.

## Manual verification done

- **proxy mode (Linux, no root): end-to-end PASS.** `headless -mode proxy` starts
  xray-core, sets GNOME system SOCKS proxy (gsettings: mode=manual,
  host=127.0.0.1, port=10808); on Ctrl-C it restores mode=none. Confirmed by
  reading `gsettings get org.gnome.system.proxy.*` before/during/after.

## Pending manual verification (needs privileges this box can't grant)

- **TUN PoC (Linux, root):** `sudo` requires a password in this environment, so
  the root TUN run was not auto-executed. Components are otherwise verified
  (`internal/tun` builds, `/dev/net/tun` present, `netcfg` command builders
  unit-tested). Run manually:
  ```bash
  export XRAY_LOCATION_ASSET=~/.config/xray-assets   # geoip.dat + geosite.dat
  go build -o /tmp/headless ./cmd/headless
  sudo -E /tmp/headless -link 'vless://YOUR-REAL-LINK' -mode tun -device tun0 -port 10808
  # in another shell:
  ip addr show tun0                      # 198.18.0.1/15, UP
  ip route get 149.154.167.51            # should show: dev tun0
  curl -s --max-time 8 https://api.telegram.org/ -o /dev/null -w "telegram=%{http_code}\n"
  curl -s --max-time 8 https://api.ipify.org -w "\nnon-tunneled egress (unchanged)\n"
  ```
  Expected: Telegram IPs route via tun0 (through the tunnel); other traffic is
  unaffected (no routing loop, because only whitelisted CIDRs enter the TUN).
- sysproxy macOS (`networksetup`) and Windows (WinINet registry) exec paths:
  unit-tested builders only; run the package tests on a real mac / Windows box.

## Known issue to address before GUI (Phase 3)

- `tun2socks` `engine.Start()` / `engine.Stop()` call `log.Fatalf` internally on
  failure (they do not return errors). In headless PoC that is acceptable, but a
  GUI must not have the engine kill the whole process. Phase 3/later: run the
  engine in a supervised goroutine, or fork/patch to surface errors, or switch
  to driving the gVisor stack directly.

## Deferred (later phases)

- netcfg for darwin/windows + full default-route TUN with loop avoidance
  (fwmark/policy-routing on Linux, bind-to-interface on macOS, route metrics on
  Windows).
- Kill switch, autostart, ping/latency, traffic stats.
- CI matrix: GitHub Actions ubuntu/macos/windows runners running
  `go build ./...` + `go test ./...` per OS once netcfg covers all platforms.
