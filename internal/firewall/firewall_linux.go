package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

type linuxFirewall struct{}

// New returns the Linux (nftables) kill switch.
func New() Firewall { return linuxFirewall{} }

// checkNft verifies the nft binary is available before we try to use it, so a
// missing dependency is a clear error rather than a confusing apply failure.
func checkNft() error {
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("firewall: nft not found in PATH: %w", err)
	}
	return nil
}

// On installs the kill-switch table. It is idempotent: any stale table from a
// previous run is removed first. With no CIDRs to protect it is a no-op.
func (linuxFirewall) On(c Config) error {
	if c.Device == "" {
		return fmt.Errorf("firewall: empty device name")
	}
	if err := checkNft(); err != nil {
		return err
	}
	v4, v6 := splitCIDRs(c.CIDRs)
	if len(v4) == 0 && len(v6) == 0 {
		return nil
	}
	// Drop any leftover table so re-applying is clean; ignore "not found".
	_ = exec.Command("nft", "delete", "table", "inet", tableName).Run()

	ruleset := buildRuleset(c.Device, v4, v6)
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("firewall: apply ruleset: %w: %s", err, out)
	}
	return nil
}

// Off removes the kill-switch table. Removing an absent table is success.
func (linuxFirewall) Off() error {
	out, err := exec.Command("nft", "delete", "table", "inet", tableName).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "No such file") {
		return fmt.Errorf("firewall: delete table: %w: %s", err, out)
	}
	return nil
}
