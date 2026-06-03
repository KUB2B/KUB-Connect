# VLESS Client — Phase 1: Core (headless) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the headless core of the VLESS+Xray client — parse `vless://`, build an xray JSON config with whitelist routing (Telegram on, category-ru forced direct, custom rules), persist settings, and start/stop an embedded xray-core instance that actually proxies traffic.

**Architecture:** Pure-logic Go packages under `internal/`, each with one responsibility and table/golden tests. xray-core is embedded as a library: we build the standard xray JSON config and load it via `serial.LoadJSONConfig`, then run it with `core.New` + `Instance.Start`. A small `cmd/headless` wires everything for manual end-to-end verification (real connect through a local SOCKS inbound).

**Tech Stack:** Go 1.22+, `github.com/xtls/xray-core` (core + infra/conf/serial), standard library only for parsing/storage. No Wails yet (Phase 3).

**Phasing note:** This is Phase 1 of 4. Phases 2–4 (TUN/sysproxy, Wails GUI, extras) get their own plans. This phase must stand alone as working, testable software.

---

### Task 0: Project scaffold

**Files:**
- Create: `go.mod`
- Create: `internal/.gitkeep`
- Create: `cmd/headless/.gitkeep`

- [ ] **Step 1: Init module**

Run:
```bash
cd /home/zki/projects/vless-client
go mod init github.com/zki/vless-client
mkdir -p internal cmd/headless
touch internal/.gitkeep cmd/headless/.gitkeep
go version
```
Expected: `go mod init` prints `go: creating new go.mod`; `go version` prints `go1.22` or newer.

- [ ] **Step 2: Verify build of empty module**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Commit**

```bash
git add go.mod internal/.gitkeep cmd/headless/.gitkeep
git commit -m "chore: scaffold go module and dirs"
```

---

### Task 1: vless link parser — reality/tcp (the 3x-ui default)

**Files:**
- Create: `internal/vless/config.go`
- Create: `internal/vless/parse.go`
- Test: `internal/vless/parse_test.go`

3x-ui v3.2.6 reality share link looks like:
`vless://UUID@HOST:PORT?type=tcp&security=reality&pbk=PBK&fp=chrome&sni=SNI&sid=SID&spx=%2F&flow=xtls-rprx-vision#my%20server`

- [ ] **Step 1: Write the failing test**

`internal/vless/parse_test.go`:
```go
package vless

import "testing"

func TestParseRealityTCP(t *testing.T) {
	link := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@example.com:443" +
		"?type=tcp&security=reality&pbk=ABCpublicKey&fp=chrome&sni=www.microsoft.com" +
		"&sid=0123abcd&spx=%2F&flow=xtls-rprx-vision#my%20server"

	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Name", cfg.Name, "my server"},
		{"Host", cfg.Host, "example.com"},
		{"UUID", cfg.UUID, "b831381d-6324-4d53-ad4f-8cda48b30811"},
		{"Flow", cfg.Flow, "xtls-rprx-vision"},
		{"Security", string(cfg.Security), "reality"},
		{"Network", string(cfg.Network), "tcp"},
		{"SNI", cfg.SNI, "www.microsoft.com"},
		{"Fingerprint", cfg.Fingerprint, "chrome"},
		{"PublicKey", cfg.PublicKey, "ABCpublicKey"},
		{"ShortID", cfg.ShortID, "0123abcd"},
		{"SpiderX", cfg.SpiderX, "/"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if cfg.Port != 443 {
		t.Errorf("Port = %d, want 443", cfg.Port)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vless/ -run TestParseRealityTCP -v`
Expected: FAIL — `undefined: Parse` / package does not compile.

- [ ] **Step 3: Write the config type**

`internal/vless/config.go`:
```go
package vless

// Security is the streamSettings security type.
type Security string

const (
	SecurityNone    Security = "none"
	SecurityTLS     Security = "tls"
	SecurityReality Security = "reality"
)

// Network is the transport type.
type Network string

const (
	NetworkTCP  Network = "tcp"
	NetworkWS   Network = "ws"
	NetworkGRPC Network = "grpc"
)

// ServerConfig is a parsed vless:// link.
type ServerConfig struct {
	Name     string
	Host     string
	Port     int
	UUID     string
	Flow     string
	Security Security
	Network  Network

	// TLS / Reality
	SNI         string
	Fingerprint string
	ALPN        []string

	// Reality only
	PublicKey string
	ShortID   string
	SpiderX   string

	// ws
	Path   string
	WsHost string

	// grpc
	ServiceName string
}
```

- [ ] **Step 4: Write the parser**

`internal/vless/parse.go`:
```go
package vless

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Parse decodes a vless:// share link into a ServerConfig.
func Parse(link string) (*ServerConfig, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "vless" {
		return nil, fmt.Errorf("not a vless link: scheme %q", u.Scheme)
	}
	if u.User == nil || u.User.Username() == "" {
		return nil, errors.New("missing uuid in link")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %q", u.Port())
	}

	q := u.Query()
	cfg := &ServerConfig{
		Name:        u.Fragment, // url.Parse already decodes the fragment
		Host:        u.Hostname(),
		Port:        port,
		UUID:        u.User.Username(),
		Flow:        q.Get("flow"),
		Security:    Security(orDefault(q.Get("security"), string(SecurityNone))),
		Network:     Network(orDefault(q.Get("type"), string(NetworkTCP))),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		PublicKey:   q.Get("pbk"),
		ShortID:     q.Get("sid"),
		SpiderX:     q.Get("spx"),
		Path:        q.Get("path"),
		WsHost:      q.Get("host"),
		ServiceName: q.Get("serviceName"),
	}
	if alpn := q.Get("alpn"); alpn != "" {
		cfg.ALPN = strings.Split(alpn, ",")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *ServerConfig) validate() error {
	if c.Host == "" {
		return errors.New("missing host")
	}
	switch c.Security {
	case SecurityNone, SecurityTLS, SecurityReality:
	default:
		return fmt.Errorf("unknown security %q", c.Security)
	}
	switch c.Network {
	case NetworkTCP, NetworkWS, NetworkGRPC:
	default:
		return fmt.Errorf("unknown network %q", c.Network)
	}
	if c.Security == SecurityReality && c.PublicKey == "" {
		return errors.New("reality requires pbk (public key)")
	}
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/vless/ -run TestParseRealityTCP -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/vless/
git commit -m "feat(vless): parse reality/tcp share links"
```

---

### Task 2: parser — ws/tls and grpc/tls variants + error cases

**Files:**
- Test: `internal/vless/parse_test.go` (add tests)

No production code changes expected — the parser already reads these query params. This task proves coverage and locks behavior.

- [ ] **Step 1: Add failing tests**

Append to `internal/vless/parse_test.go`:
```go
func TestParseWSTLS(t *testing.T) {
	link := "vless://uuid-1@host.net:8443" +
		"?type=ws&security=tls&sni=host.net&path=%2Fwspath&host=cdn.host.net&alpn=h2,http%2F1.1#ws"
	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Network != NetworkWS {
		t.Errorf("Network = %q, want ws", cfg.Network)
	}
	if cfg.Security != SecurityTLS {
		t.Errorf("Security = %q, want tls", cfg.Security)
	}
	if cfg.Path != "/wspath" {
		t.Errorf("Path = %q, want /wspath", cfg.Path)
	}
	if cfg.WsHost != "cdn.host.net" {
		t.Errorf("WsHost = %q, want cdn.host.net", cfg.WsHost)
	}
	if len(cfg.ALPN) != 2 || cfg.ALPN[0] != "h2" || cfg.ALPN[1] != "http/1.1" {
		t.Errorf("ALPN = %v, want [h2 http/1.1]", cfg.ALPN)
	}
}

func TestParseGRPC(t *testing.T) {
	link := "vless://uuid-2@host.net:443?type=grpc&security=tls&sni=host.net&serviceName=mygrpc#g"
	cfg, err := Parse(link)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Network != NetworkGRPC {
		t.Errorf("Network = %q, want grpc", cfg.Network)
	}
	if cfg.ServiceName != "mygrpc" {
		t.Errorf("ServiceName = %q, want mygrpc", cfg.ServiceName)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"wrong scheme":   "vmess://uuid@host:443",
		"missing uuid":   "vless://host.net:443?type=tcp",
		"bad port":       "vless://uuid@host.net:0?type=tcp",
		"reality no pbk": "vless://uuid@host.net:443?type=tcp&security=reality",
		"unknown net":    "vless://uuid@host.net:443?type=kcp&security=tls&sni=x",
	}
	for name, link := range cases {
		if _, err := Parse(link); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/vless/ -v`
Expected: all PASS. If `TestParseErrors/"bad port"` fails because port `0` slips through, confirm the `port <= 0` guard in `parse.go` — it should already reject it.

- [ ] **Step 3: Commit**

```bash
git add internal/vless/parse_test.go
git commit -m "test(vless): cover ws/grpc variants and error cases"
```

---

### Task 3: routing profile + rule generation

**Files:**
- Create: `internal/routing/profile.go`
- Test: `internal/routing/profile_test.go`

This produces an ordered, outbound-tagged rule list. The xrayconf builder (Task 4) turns it into JSON. Keeping rule generation separate keeps the whitelist ordering logic unit-testable without xray.

- [ ] **Step 1: Write the failing test**

`internal/routing/profile_test.go`:
```go
package routing

import "testing"

func TestRulesWhitelistDefault(t *testing.T) {
	p := Profile{
		Telegram:        true,
		ForceRUDirect:   true,
		CustomProxyDomains: []string{"openai.com"},
		CustomProxyIPs:     []string{"1.2.3.4/32"},
	}
	rules := p.Rules()

	// 1st rule must be the forced-direct RU rule (highest priority).
	if rules[0].Outbound != OutboundDirect {
		t.Fatalf("rule[0].Outbound = %q, want direct", rules[0].Outbound)
	}
	if !contains(rules[0].Domains, "geosite:category-ru") {
		t.Errorf("rule[0] domains = %v, want geosite:category-ru", rules[0].Domains)
	}

	// Last rule is the catch-all direct (whitelist default).
	last := rules[len(rules)-1]
	if last.Outbound != OutboundDirect || last.Network != "tcp,udp" {
		t.Errorf("last rule = %+v, want catch-all direct tcp,udp", last)
	}

	// Telegram must route to proxy.
	if !hasProxyDomain(rules, "geosite:telegram") {
		t.Error("expected geosite:telegram -> proxy")
	}
	// Custom domain must route to proxy.
	if !hasProxyDomain(rules, "openai.com") {
		t.Error("expected openai.com -> proxy")
	}
}

func TestRulesTelegramOff(t *testing.T) {
	p := Profile{Telegram: false, ForceRUDirect: true}
	if hasProxyDomain(p.Rules(), "geosite:telegram") {
		t.Error("telegram off: must not route telegram to proxy")
	}
}

func TestRulesRUOff(t *testing.T) {
	p := Profile{Telegram: true, ForceRUDirect: false}
	rules := p.Rules()
	for _, r := range rules {
		if contains(r.Domains, "geosite:category-ru") {
			t.Error("ForceRUDirect off: must not emit category-ru rule")
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func hasProxyDomain(rules []Rule, domain string) bool {
	for _, r := range rules {
		if r.Outbound == OutboundProxy && contains(r.Domains, domain) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/routing/ -v`
Expected: FAIL — package does not compile (`undefined: Profile`).

- [ ] **Step 3: Write the implementation**

`internal/routing/profile.go`:
```go
package routing

// Outbound tag names. Must match the tags emitted by the xrayconf builder.
const (
	OutboundProxy  = "proxy"
	OutboundDirect = "direct"
	OutboundBlock  = "block"
)

// Profile is the user's whitelist routing choices.
type Profile struct {
	Telegram           bool
	ForceRUDirect      bool
	CustomProxyDomains []string
	CustomProxyIPs     []string
}

// Rule is one xray routing rule, outbound-tagged.
type Rule struct {
	Outbound string
	Domains  []string
	IPs      []string
	Network  string // e.g. "tcp,udp"; empty when matching by domain/ip
}

// Rules returns the ordered rule list. Order encodes priority:
// forced-direct RU first, then private, then proxy matches, then a
// catch-all direct (whitelist: anything unmatched goes direct).
func (p Profile) Rules() []Rule {
	var rules []Rule

	if p.ForceRUDirect {
		rules = append(rules,
			Rule{Outbound: OutboundDirect, Domains: []string{"geosite:category-ru"}},
			Rule{Outbound: OutboundDirect, IPs: []string{"geoip:ru"}},
		)
	}
	// Local/private networks always direct.
	rules = append(rules, Rule{Outbound: OutboundDirect, IPs: []string{"geoip:private"}})

	if p.Telegram {
		rules = append(rules,
			Rule{Outbound: OutboundProxy, Domains: []string{"geosite:telegram"}},
			Rule{Outbound: OutboundProxy, IPs: []string{"geoip:telegram"}},
		)
	}
	if len(p.CustomProxyDomains) > 0 {
		rules = append(rules, Rule{Outbound: OutboundProxy, Domains: p.CustomProxyDomains})
	}
	if len(p.CustomProxyIPs) > 0 {
		rules = append(rules, Rule{Outbound: OutboundProxy, IPs: p.CustomProxyIPs})
	}

	// Whitelist catch-all: everything else direct.
	rules = append(rules, Rule{Outbound: OutboundDirect, Network: "tcp,udp"})
	return rules
}

// Default returns the shipped default profile: Telegram on, RU forced direct.
func Default() Profile {
	return Profile{Telegram: true, ForceRUDirect: true}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/routing/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/routing/
git commit -m "feat(routing): whitelist rule generation with forced RU direct"
```

---

### Task 4: xray config builder (golden test)

**Files:**
- Create: `internal/xrayconf/build.go`
- Test: `internal/xrayconf/build_test.go`
- Create: `internal/xrayconf/testdata/reality_default.json` (golden, written in Step 4)

Builds the full xray JSON config from a `vless.ServerConfig` + `routing.Profile`. Modeled as Go structs marshalled with `encoding/json` — struct field order makes output deterministic for golden comparison.

- [ ] **Step 1: Write the failing test**

`internal/xrayconf/build_test.go`:
```go
package xrayconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

func sampleServer() *vless.ServerConfig {
	return &vless.ServerConfig{
		Name: "srv", Host: "example.com", Port: 443,
		UUID:     "b831381d-6324-4d53-ad4f-8cda48b30811",
		Flow:     "xtls-rprx-vision",
		Security: vless.SecurityReality,
		Network:  vless.NetworkTCP,
		SNI:      "www.microsoft.com", Fingerprint: "chrome",
		PublicKey: "ABCpublicKey", ShortID: "0123abcd", SpiderX: "/",
	}
}

func TestBuildGolden(t *testing.T) {
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Pretty-print for stable, diffable golden output.
	var pretty []byte
	pretty, err = json.MarshalIndent(json.RawMessage(got), "", "  ")
	if err != nil {
		t.Fatalf("indent: %v", err)
	}
	pretty = append(pretty, '\n')

	goldenPath := filepath.Join("testdata", "reality_default.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, pretty, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 first): %v", err)
	}
	if string(pretty) != string(want) {
		t.Errorf("config mismatch.\n--- got ---\n%s\n--- want ---\n%s", pretty, want)
	}
}

func TestBuildLoadsIntoXray(t *testing.T) {
	// Guards against producing JSON that xray itself rejects. Imported here
	// (not just core package) so a malformed schema fails at build time.
	got, err := Build(sampleServer(), routing.Default(), Options{SocksPort: 10808})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !json.Valid(got) {
		t.Fatal("Build produced invalid JSON")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/xrayconf/ -v`
Expected: FAIL — `undefined: Build` / package does not compile.

- [ ] **Step 3: Write the builder**

`internal/xrayconf/build.go`:
```go
package xrayconf

import (
	"encoding/json"
	"fmt"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

// Options holds runtime knobs for the generated config.
type Options struct {
	SocksPort int // local SOCKS inbound port (e.g. 10808)
}

// --- xray JSON schema (subset we emit) ---

type xrayConfig struct {
	Log       logConf      `json:"log"`
	DNS       dnsConf      `json:"dns"`
	Inbounds  []inbound    `json:"inbounds"`
	Outbounds []outbound   `json:"outbounds"`
	Routing   routingConf  `json:"routing"`
}

type logConf struct {
	LogLevel string `json:"loglevel"`
}

type dnsConf struct {
	Servers []any `json:"servers"`
}

type inbound struct {
	Tag      string        `json:"tag"`
	Listen   string        `json:"listen"`
	Port     int           `json:"port"`
	Protocol string        `json:"protocol"`
	Settings socksSettings `json:"settings"`
	Sniffing sniffing      `json:"sniffing"`
}

type socksSettings struct {
	Auth string `json:"auth"`
	UDP  bool   `json:"udp"`
}

type sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

type outbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	StreamSettings *streamSettings `json:"streamSettings,omitempty"`
}

type streamSettings struct {
	Network         string           `json:"network"`
	Security        string           `json:"security"`
	RealitySettings *realitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *tlsSettings     `json:"tlsSettings,omitempty"`
	WSSettings      *wsSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *grpcSettings    `json:"grpcSettings,omitempty"`
}

type realitySettings struct {
	ServerName  string `json:"serverName"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId"`
	SpiderX     string `json:"spiderX"`
}

type tlsSettings struct {
	ServerName  string   `json:"serverName"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	ALPN        []string `json:"alpn,omitempty"`
}

type wsSettings struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

type grpcSettings struct {
	ServiceName string `json:"serviceName"`
}

type routingConf struct {
	DomainStrategy string     `json:"domainStrategy"`
	Rules          []ruleJSON `json:"rules"`
}

type ruleJSON struct {
	Type        string   `json:"type"`
	OutboundTag string   `json:"outboundTag"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Network     string   `json:"network,omitempty"`
}

// Build produces the xray JSON config bytes.
func Build(s *vless.ServerConfig, p routing.Profile, opts Options) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil server config")
	}
	if opts.SocksPort <= 0 {
		opts.SocksPort = 10808
	}

	proxyOut, err := buildProxyOutbound(s)
	if err != nil {
		return nil, err
	}

	cfg := xrayConfig{
		Log: logConf{LogLevel: "warning"},
		DNS: dnsConf{Servers: []any{"1.1.1.1", "localhost"}},
		Inbounds: []inbound{{
			Tag:      "socks-in",
			Listen:   "127.0.0.1",
			Port:     opts.SocksPort,
			Protocol: "socks",
			Settings: socksSettings{Auth: "noauth", UDP: true},
			Sniffing: sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
		}},
		Outbounds: []outbound{
			proxyOut,
			{Tag: "direct", Protocol: "freedom"},
			{Tag: "block", Protocol: "blackhole"},
		},
		Routing: routingConf{
			DomainStrategy: "IPIfNonMatch",
			Rules:          toRuleJSON(p.Rules()),
		},
	}
	return json.Marshal(cfg)
}

func buildProxyOutbound(s *vless.ServerConfig) (outbound, error) {
	settings := map[string]any{
		"vnext": []any{map[string]any{
			"address": s.Host,
			"port":    s.Port,
			"users": []any{map[string]any{
				"id":         s.UUID,
				"encryption": "none",
				"flow":       s.Flow,
			}},
		}},
	}
	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return outbound{}, err
	}

	stream := &streamSettings{
		Network:  string(s.Network),
		Security: string(s.Security),
	}
	switch s.Security {
	case vless.SecurityReality:
		stream.RealitySettings = &realitySettings{
			ServerName:  s.SNI,
			Fingerprint: orDefault(s.Fingerprint, "chrome"),
			PublicKey:   s.PublicKey,
			ShortID:     s.ShortID,
			SpiderX:     s.SpiderX,
		}
	case vless.SecurityTLS:
		stream.TLSSettings = &tlsSettings{
			ServerName:  s.SNI,
			Fingerprint: s.Fingerprint,
			ALPN:        s.ALPN,
		}
	}
	switch s.Network {
	case vless.NetworkWS:
		ws := &wsSettings{Path: orDefault(s.Path, "/")}
		if s.WsHost != "" {
			ws.Headers = map[string]string{"Host": s.WsHost}
		}
		stream.WSSettings = ws
	case vless.NetworkGRPC:
		stream.GRPCSettings = &grpcSettings{ServiceName: s.ServiceName}
	}

	return outbound{
		Tag:            "proxy",
		Protocol:       "vless",
		Settings:       rawSettings,
		StreamSettings: stream,
	}, nil
}

func toRuleJSON(rules []routing.Rule) []ruleJSON {
	out := make([]ruleJSON, 0, len(rules))
	for _, r := range rules {
		out = append(out, ruleJSON{
			Type:        "field",
			OutboundTag: r.Outbound,
			Domain:      r.Domains,
			IP:          r.IPs,
			Network:     r.Network,
		})
	}
	return out
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```

- [ ] **Step 4: Generate the golden file, then run the test**

Run:
```bash
UPDATE_GOLDEN=1 go test ./internal/xrayconf/ -run TestBuildGolden -v
go test ./internal/xrayconf/ -v
```
Expected: first command writes `internal/xrayconf/testdata/reality_default.json` and PASSES; second PASSES against it.

After generating, open `internal/xrayconf/testdata/reality_default.json` and eyeball it: confirm rule order is RU-direct → private-direct → telegram-proxy → catch-all direct, and the reality outbound has `publicKey: "ABCpublicKey"`. If wrong, fix the builder and regenerate.

- [ ] **Step 5: Commit**

```bash
git add internal/xrayconf/
git commit -m "feat(xrayconf): build xray json config from server+profile"
```

---

### Task 5: settings store (JSON persistence)

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

Persists servers, active server index, routing profile, and app settings to a JSON file in the user config dir.

- [ ] **Step 1: Write the failing test**

`internal/store/store_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	in := &State{
		Servers: []*vless.ServerConfig{
			{Name: "a", Host: "h1", Port: 443, UUID: "u1", Security: vless.SecurityReality, Network: vless.NetworkTCP, PublicKey: "k"},
		},
		ActiveServer: 0,
		Profile:      routing.Default(),
		Settings:     Settings{Mode: ModeTUN, AutoStart: true, AutoConnect: false, KillSwitch: true},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.Servers) != 1 || out.Servers[0].Host != "h1" {
		t.Errorf("servers round-trip failed: %+v", out.Servers)
	}
	if out.Profile.Telegram != true {
		t.Errorf("profile round-trip failed: %+v", out.Profile)
	}
	if out.Settings.Mode != ModeTUN || !out.Settings.KillSwitch {
		t.Errorf("settings round-trip failed: %+v", out.Settings)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing should not error, got %v", err)
	}
	if out.Settings.Mode != ModeTUN {
		t.Errorf("default Mode = %q, want tun", out.Settings.Mode)
	}
	if !out.Profile.Telegram {
		t.Errorf("default profile should have Telegram on")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Write the implementation**

`internal/store/store.go`:
```go
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
)

// Mode is the traffic capture mode.
type Mode string

const (
	ModeTUN   Mode = "tun"
	ModeProxy Mode = "proxy"
)

// Settings holds app-level toggles.
type Settings struct {
	Mode        Mode `json:"mode"`
	AutoStart   bool `json:"autoStart"`
	AutoConnect bool `json:"autoConnect"`
	KillSwitch  bool `json:"killSwitch"`
}

// State is the full persisted application state.
type State struct {
	Servers      []*vless.ServerConfig `json:"servers"`
	ActiveServer int                   `json:"activeServer"`
	Profile      routing.Profile       `json:"profile"`
	Settings     Settings              `json:"settings"`
}

// DefaultState returns the initial state for a fresh install.
func DefaultState() *State {
	return &State{
		Servers:      nil,
		ActiveServer: -1,
		Profile:      routing.Default(),
		Settings:     Settings{Mode: ModeTUN},
	}
}

// Save writes state to path as indented JSON, creating parent dirs.
func Save(path string, s *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads state from path. A missing file yields DefaultState (no error).
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultState(), nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// DefaultPath returns the OS-appropriate state file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vless-client", "state.json"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat(store): json persistence for servers, profile, settings"
```

---

### Task 6: xray-core wrapper (embed + start/stop)

**Files:**
- Create: `internal/core/core.go`
- Test: `internal/core/core_test.go`
- Modify: `go.mod` (adds xray-core dependency via `go get`)

Wraps the embedded xray-core lifecycle. `serial.LoadJSONConfig` parses our JSON into `*core.Config`; `core.New` + `Instance.Start` runs it; `Instance.Close` stops it.

- [ ] **Step 1: Add the xray-core dependency**

Run:
```bash
go get github.com/xtls/xray-core@latest
go mod tidy
```
Expected: `go.mod`/`go.sum` updated with xray-core and its transitive deps. This pulls many modules; that is expected.

- [ ] **Step 2: Write the failing test**

`internal/core/core_test.go`:
```go
package core

import (
	"runtime"
	"testing"
	"time"

	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

func TestStartStopSmoke(t *testing.T) {
	srv := &vless.ServerConfig{
		Name: "t", Host: "127.0.0.1", Port: 1, // unreachable is fine; we only test lifecycle
		UUID: "b831381d-6324-4d53-ad4f-8cda48b30811", Flow: "xtls-rprx-vision",
		Security: vless.SecurityReality, Network: vless.NetworkTCP,
		SNI: "www.microsoft.com", PublicKey: "ABCpublicKey", ShortID: "00", SpiderX: "/",
	}
	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: 0}) // port 0 = OS-assigned
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	before := runtime.NumGoroutine()

	inst, err := Start(cfgJSON)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the runtime a moment to wind down xray goroutines.
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+5 {
		t.Errorf("possible goroutine leak: before=%d after=%d", before, after)
	}
}
```

Note: `SocksPort: 0` lets the OS assign a free port so concurrent test runs don't collide. Confirm `xrayconf.Build` passes `0` through (it sets a default only when `<= 0`; adjust the test to a fixed high port like `38080` if you prefer a fixed listen and accept the default-substitution).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestStartStopSmoke -v`
Expected: FAIL — `undefined: Start`.

- [ ] **Step 4: Write the implementation**

`internal/core/core.go`:
```go
package core

import (
	"bytes"
	"fmt"

	xcore "github.com/xtls/xray-core/core"
	_ "github.com/xtls/xray-core/main/distro/all" // registers all protocols/transports
	"github.com/xtls/xray-core/infra/conf/serial"
)

// Instance is a running xray-core instance.
type Instance struct {
	inner *xcore.Instance
}

// Start loads the JSON config and starts an xray-core instance.
func Start(jsonConfig []byte) (*Instance, error) {
	pbConfig, err := serial.LoadJSONConfig(bytes.NewReader(jsonConfig))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inst, err := xcore.New(pbConfig)
	if err != nil {
		return nil, fmt.Errorf("new instance: %w", err)
	}
	if err := inst.Start(); err != nil {
		return nil, fmt.Errorf("start instance: %w", err)
	}
	return &Instance{inner: inst}, nil
}

// Stop closes the instance and releases its resources.
func (i *Instance) Stop() error {
	if i == nil || i.inner == nil {
		return nil
	}
	return i.inner.Close()
}
```

Note on import: the blank import `main/distro/all` registers every protocol/transport codec so `LoadJSONConfig` can resolve `vless`, `reality`, `ws`, `grpc`, etc. Without it, loading fails with "unknown protocol". If the import path differs in the pulled xray-core version, run `go doc github.com/xtls/xray-core/main/distro/all` to confirm, or locate the equivalent `features`/`all` registration package.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestStartStopSmoke -v`
Expected: PASS. If it fails with an "address already in use" error, switch the test's `SocksPort` to a different fixed high port.

- [ ] **Step 6: Commit**

```bash
git add internal/core/ go.mod go.sum
git commit -m "feat(core): embed and run xray-core instances"
```

---

### Task 7: headless wiring + manual end-to-end verification

**Files:**
- Create: `cmd/headless/main.go`
- Delete: `cmd/headless/.gitkeep`

A minimal CLI that ties the packages together: parse a link, build config, start xray with a local SOCKS inbound, and block until Ctrl-C. This is the Phase 1 deliverable you can actually use and test against the real server.

- [ ] **Step 1: Write main.go**

`cmd/headless/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

func main() {
	link := flag.String("link", "", "vless:// share link")
	port := flag.Int("port", 10808, "local SOCKS inbound port")
	flag.Parse()

	if *link == "" {
		log.Fatal("usage: headless -link 'vless://...' [-port 10808]")
	}

	srv, err := vless.Parse(*link)
	if err != nil {
		log.Fatalf("parse link: %v", err)
	}
	fmt.Printf("server: %s (%s:%d) security=%s net=%s\n",
		srv.Name, srv.Host, srv.Port, srv.Security, srv.Network)

	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: *port})
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	inst, err := core.Start(cfgJSON)
	if err != nil {
		log.Fatalf("start xray: %v", err)
	}
	fmt.Printf("xray running. SOCKS proxy on 127.0.0.1:%d (Ctrl-C to stop)\n", *port)
	fmt.Println("whitelist: Telegram -> proxy, category-ru -> direct, rest -> direct")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := inst.Stop(); err != nil {
		log.Printf("stop: %v", err)
	}
	fmt.Println("stopped.")
}
```

- [ ] **Step 2: Remove placeholder and build**

Run:
```bash
rm -f cmd/headless/.gitkeep
go build ./...
go vet ./...
```
Expected: builds clean, vet reports nothing.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 4: Manual end-to-end check**

Download geo assets so geosite/geoip rules resolve, then run against your real 3x-ui link:
```bash
mkdir -p ~/.config/xray-assets
# fetch geoip.dat + geosite.dat (one-time), e.g. from the xray-core / v2fly release assets:
curl -L -o ~/.config/xray-assets/geoip.dat   https://github.com/v2fly/geoip/releases/latest/download/geoip.dat
curl -L -o ~/.config/xray-assets/geosite.dat https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat
export XRAY_LOCATION_ASSET=~/.config/xray-assets

go run ./cmd/headless -link 'vless://YOUR-REAL-LINK' -port 10808
```
In a second terminal, verify routing:
```bash
# Telegram domain should egress via the proxy (server's country):
curl -x socks5h://127.0.0.1:10808 -s https://api.telegram.org/ -o /dev/null -w "telegram via proxy: %{http_code}\n"
# A direct (non-whitelisted) site should NOT change your IP — check egress IP:
curl -x socks5h://127.0.0.1:10808 -s https://api.ipify.org -w "\n(egress for ipify — should be your real IP since not whitelisted)\n"
```
Expected: Telegram reachable through the tunnel; non-whitelisted traffic egresses directly (whitelist behavior). Note `geosite.dat` from `dlc.dat` provides `geosite:category-ru` and `geosite:telegram`.

- [ ] **Step 5: Commit**

```bash
git add cmd/headless/main.go
git rm --cached cmd/headless/.gitkeep 2>/dev/null || true
git commit -m "feat(cmd): headless wiring for end-to-end vless connect"
```

---

## Phase 1 Done — Definition of Done

- `go test ./...` green.
- `go run ./cmd/headless -link '<real>'` connects; Telegram routes via proxy, non-whitelisted traffic stays direct, category-ru stays direct.
- Committed in small steps per task.

## What Phase 1 deliberately leaves out (next phases)

- TUN adapter + system proxy + kill switch (Phase 2).
- Wails GUI, state machine, live logs (Phase 3).
- Autostart, ping/latency test, traffic stats via xray StatsService (Phase 4).
- Bundling geo assets into the app (manual download in Phase 1).
