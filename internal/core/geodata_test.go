package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xtls/xray-core/app/router"
	"google.golang.org/protobuf/proto"

	"github.com/zki/vless-client/internal/routing"
)

// TestShippedGeoDataHasRequiredCategories guards the databases embedded into
// the app (data/*.dat). xray refuses to start when a rule names a category the
// file lacks, so a refreshed database that dropped one would break every
// connection using that preset. Set GEO_DIR to check another directory (e.g.
// a fresh download) before replacing data/.
func TestShippedGeoDataHasRequiredCategories(t *testing.T) {
	dir := os.Getenv("GEO_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "data")
	}

	wantSites := map[string]bool{"CATEGORY-RU": true, "TELEGRAM": true}
	for _, p := range routing.Presets {
		for _, d := range p.Domains {
			if name, ok := strings.CutPrefix(d, "geosite:"); ok {
				wantSites[strings.ToUpper(name)] = true
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "geosite.dat"))
	if err != nil {
		t.Skipf("no geosite.dat in %s: %v", dir, err)
	}
	var sites router.GeoSiteList
	if err := proto.Unmarshal(raw, &sites); err != nil {
		t.Fatalf("parse geosite.dat: %v", err)
	}
	for _, e := range sites.Entry {
		if len(e.Domain) > 0 {
			delete(wantSites, strings.ToUpper(e.CountryCode))
		}
	}
	for name := range wantSites {
		t.Errorf("geosite.dat has no (or an empty) category %q", strings.ToLower(name))
	}

	wantIPs := map[string]bool{"RU": true, "PRIVATE": true}
	raw, err = os.ReadFile(filepath.Join(dir, "geoip.dat"))
	if err != nil {
		t.Skipf("no geoip.dat in %s: %v", dir, err)
	}
	var ips router.GeoIPList
	if err := proto.Unmarshal(raw, &ips); err != nil {
		t.Fatalf("parse geoip.dat: %v", err)
	}
	for _, e := range ips.Entry {
		if len(e.Cidr) > 0 {
			delete(wantIPs, strings.ToUpper(e.CountryCode))
		}
	}
	for name := range wantIPs {
		t.Errorf("geoip.dat has no (or an empty) category %q", strings.ToLower(name))
	}
}
