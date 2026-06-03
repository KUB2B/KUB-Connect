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
