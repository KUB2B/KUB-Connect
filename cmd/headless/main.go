package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zki/vless-client/internal/core"
	"github.com/zki/vless-client/internal/netcfg"
	"github.com/zki/vless-client/internal/privilege"
	"github.com/zki/vless-client/internal/routing"
	"github.com/zki/vless-client/internal/store"
	"github.com/zki/vless-client/internal/sysproxy"
	"github.com/zki/vless-client/internal/tun"
	"github.com/zki/vless-client/internal/tunnel"
	"github.com/zki/vless-client/internal/vless"
	"github.com/zki/vless-client/internal/xrayconf"
)

// coreAdapter adapts core.Start to the tunnel.Core interface.
type coreAdapter struct{}

func (coreAdapter) Start(jsonConfig []byte) (tunnel.Stopper, error) {
	return core.Start(jsonConfig)
}

// tunAdapter adapts the package-level tun funcs to the tunnel.Tun interface.
type tunAdapter struct{}

func (tunAdapter) Start(device, socksURL string) error { return tun.Start(device, socksURL) }
func (tunAdapter) Stop() error                         { return tun.Stop() }

func main() {
	link := flag.String("link", "", "vless:// share link")
	port := flag.Int("port", 10808, "local SOCKS inbound port")
	mode := flag.String("mode", "proxy", "capture mode: proxy | tun")
	device := flag.String("device", "tun0", "TUN device name (tun mode)")
	flag.Parse()

	if *link == "" {
		log.Fatal("usage: headless -link 'vless://...' [-port 10808] [-mode proxy|tun]")
	}

	m := store.Mode(*mode)
	if m != store.ModeProxy && m != store.ModeTUN {
		log.Fatalf("invalid -mode %q (want proxy or tun)", *mode)
	}
	if m == store.ModeTUN {
		if err := privilege.RequireElevated("TUN mode"); err != nil {
			log.Fatal(err)
		}
	}

	srv, err := vless.Parse(*link)
	if err != nil {
		log.Fatalf("parse link: %v", err)
	}
	fmt.Printf("server: %s (%s:%d) mode=%s\n", srv.Name, srv.Host, srv.Port, m)

	cfgJSON, err := xrayconf.Build(srv, routing.Default(), xrayconf.Options{SocksPort: *port})
	if err != nil {
		log.Fatalf("build config: %v", err)
	}

	tn := tunnel.New(tunnel.Config{
		XrayJSON:   cfgJSON,
		SocksHost:  "127.0.0.1",
		SocksPort:  *port,
		Mode:       m,
		Device:     *device,
		TunIP:      "198.18.0.1",
		TunPrefix:  15,
		RouteCIDRs: routing.TelegramCIDRs,
	}, tunnel.Deps{
		Core:   coreAdapter{},
		Proxy:  sysproxy.New(),
		Tun:    tunAdapter{},
		Router: netcfg.New(),
	})

	if err := tn.Start(); err != nil {
		log.Fatalf("start tunnel: %v", err)
	}
	fmt.Printf("tunnel up (mode=%s). Ctrl-C to stop.\n", m)
	if m == store.ModeProxy {
		fmt.Printf("system SOCKS proxy -> 127.0.0.1:%d\n", *port)
	} else {
		fmt.Printf("routing Telegram CIDRs into %s\n", *device)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := tn.Stop(); err != nil {
		log.Printf("stop: %v", err)
	}
	fmt.Println("stopped.")
}
