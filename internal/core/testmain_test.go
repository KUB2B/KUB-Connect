package core

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	router "github.com/xtls/xray-core/app/router"
	"google.golang.org/protobuf/proto"
)

// TestMain sets up minimal geosite.dat and geoip.dat stub files so that
// LoadJSONConfig can resolve geosite:/geoip: rules without the real data.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "xray-geodata-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// Build minimal geosite.dat with stub domain entries.
	// Each entry must have at least one domain so that routing rules built
	// from these lookups are non-empty (xray-core rejects rules with no conditions).
	geosite := &router.GeoSiteList{
		Entry: []*router.GeoSite{
			{
				CountryCode: "CATEGORY-RU",
				Domain: []*router.Domain{
					{Type: router.Domain_Domain, Value: "ru"},
				},
			},
			{
				CountryCode: "TELEGRAM",
				Domain: []*router.Domain{
					{Type: router.Domain_Domain, Value: "telegram.org"},
				},
			},
		},
	}
	geositeBytes, err := proto.Marshal(geosite)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geosite.dat"), geositeBytes, 0o644); err != nil {
		panic(err)
	}

	// Build minimal geoip.dat with stub CIDR entries.
	// Each entry must have at least one CIDR so routing rules are non-empty.
	geoip := &router.GeoIPList{
		Entry: []*router.GeoIP{
			{
				CountryCode: "RU",
				Cidr:        []*router.CIDR{ipv4CIDR("1.2.3.0", 24)},
			},
			{
				CountryCode: "PRIVATE",
				Cidr: []*router.CIDR{
					ipv4CIDR("127.0.0.0", 8),
					ipv4CIDR("10.0.0.0", 8),
					ipv4CIDR("192.168.0.0", 16),
				},
			},
			{
				CountryCode: "TELEGRAM",
				Cidr:        []*router.CIDR{ipv4CIDR("149.154.160.0", 20)},
			},
		},
	}
	geoipBytes, err := proto.Marshal(geoip)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geoip.dat"), geoipBytes, 0o644); err != nil {
		panic(err)
	}

	// Tell xray-core where to find asset files.
	os.Setenv("XRAY_LOCATION_ASSET", dir)

	os.Exit(m.Run())
}

func ipv4CIDR(ip string, prefix uint32) *router.CIDR {
	return &router.CIDR{Ip: net.ParseIP(ip).To4(), Prefix: prefix}
}
