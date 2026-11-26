package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

// UpgradeResult reports an Upgrade.
type UpgradeResult struct {
	// Upgraded had updates waiting before and none after.
	Upgraded []string `json:"upgraded,omitempty"`
	// Waiting still have updates that unattended-upgrades isn't allowed to
	// install (auto_updates: distro, pinned in trellis.lock, or held).
	Waiting        []string `json:"waiting,omitempty"`
	RebootRequired bool     `json:"reboot_required,omitempty"`
}

// unattendedUpgradeCmd installs now what the nightly run would. Without
// minimal steps it upgrades in one transaction: the timer's run takes one
// package at a time so a shutdown can interrupt it safely, which makes a
// fresh server's first batch take many minutes.
var unattendedUpgradeCmd = host.Cmd{
	Argv: []string{"unattended-upgrade", "--no-minimal-upgrade-steps"},
	Env:  aptEnv,
}

// Upgrade runs unattended-upgrades now, so a server needn't wait for its
// nightly run (a fresh server's first). It installs exactly what
// auto_updates allows, with the same origins, pins and config-file
// handling, and restarts services as the packages ask.
func Upgrade(ctx context.Context, h host.Host, log func(Event)) (*UpgradeResult, error) {
	if log == nil {
		log = func(Event) {}
	}
	prev, err := LoadGeneration(h)
	if err != nil {
		return nil, err
	}
	if prev == nil {
		return nil, errors.New("the server hasn't been converged: run `trellis converge apply` first")
	}
	upgradable := func() ([]string, error) {
		if _, err := h.Run(ctx, host.Cmd{Argv: []string{"apt-get", "update", "-q"}, Env: aptEnv}); err != nil {
			return nil, err
		}
		out, err := h.Run(ctx, upgradableCmd)
		return parseUpgradable(out), err
	}
	before, err := upgradable()
	if err != nil {
		return nil, err
	}
	res := &UpgradeResult{}
	if len(before) > 0 {
		log(Event{Op: "upgrade", Target: fmt.Sprintf("%d packages", len(before)), Message: "unattended-upgrades; services restart as needed"})
		if _, err := h.Run(ctx, unattendedUpgradeCmd); err != nil {
			return nil, err
		}
		out, err := h.Run(ctx, upgradableCmd)
		if err != nil {
			return nil, err
		}
		res.Waiting = parseUpgradable(out)
		for _, p := range before {
			if !slices.Contains(res.Waiting, p) {
				res.Upgraded = append(res.Upgraded, p)
			}
		}
	}
	if fi, err := h.Stat(state.RebootRequiredPath, false); err == nil && fi.Exists {
		res.RebootRequired = true
	}
	return res, nil
}
