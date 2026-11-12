// Command headless is a debugging aid: it starts xray for one vless:// link
// and points the system SOCKS proxy at it, without the GUI or the service
// layer. Proxy mode only — TUN capture lives in the GUI (internal/app), which
// carries the interface binding, IPv6 and route handling it needs.
//
// The default routing profile uses geoip:/geosite: rules, so xray must find the
// databases: run from the repo with XRAY_LOCATION_ASSET=data.
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
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tunnel"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

// version is overridden at release time via -ldflags "-X main.version=v0.1.0".
var version = "dev"

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

func main() {
	link := flag.String("link", "", "vless:// share link")
	port := flag.Int("port", 10808, "local SOCKS inbound port")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *link == "" {
		log.Fatal("usage: headless -link 'vless://...' [-port 10808]")
	}

	srv, err := vless.Parse(*link)
	if err != nil {
		log.Fatalf("parse link: %v", err)
	}
	fmt.Printf("server: %s (%s:%d)\n", srv.Name, srv.Host, srv.Port)

	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: *port})
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	tn := tunnel.New(tunnel.Config{
		XrayJSON:  cfgJSON,
		SocksHost: "127.0.0.1",
		SocksPort: *port,
		Mode:      store.ModeProxy,
	}, tunnel.Deps{Core: coreAdapter{}, Proxy: sysproxy.New()})
	if err := tn.Start(); err != nil {
		log.Fatalf("start tunnel: %v", err)
	}
	fmt.Printf("system SOCKS proxy -> 127.0.0.1:%d. Ctrl-C to stop.\n", *port)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := tn.Stop(); err != nil {
		log.Printf("stop: %v", err)
	}
	fmt.Println("stopped.")
}
