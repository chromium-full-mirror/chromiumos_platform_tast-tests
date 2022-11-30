// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/kioskmode"
	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/syslog"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppsCachedOffline,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Kiosk apps can be cached and launched offline",
		Contacts: []string{
			"yixie@google.com", // Test author
			"chromeos-kiosk-eng+TAST@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "ash",
			Val:  chrome.ExtraArgs(""),
		}, {
			Name:              "lacros",
			Val:               chrome.ExtraArgs("--enable-features=LacrosSupport,ChromeKioskEnableLacros", "--lacros-availability-ignore"),
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		Fixture: fixture.KioskAutoLaunchCleanup,
		Timeout: 5 * time.Minute, // Starting Kiosk twice requires longer timeout.
	})
}

type helper struct {
	Manager            *shill.Manager
	enableEthernetFunc func(ctx context.Context)
	enableWifiFunc     func(ctx context.Context)
	enableCellularFunc func(ctx context.Context)
}

func AppsCachedOffline(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	chromeOptions := s.Param().(chrome.Option)
	cleanUpCtx := ctx
	kiosk, _, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.ExtraChromeOptions(
			chromeOptions,
		),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer kiosk.Close(ctx)

	s.Log("Waiting for Kiosk crx to be cached")
	if err := kioskmode.WaitForCrxInCache(ctx, kioskmode.KioskAppID); err != nil {
		s.Fatal("Kiosk crx is not cached: ", err)
	}

	s.Log("Trying to launch Kiosk app offline")

	restartAndLaunchKiosk(ctx, cleanUpCtx, kiosk, s, fdms)
}

func restartAndLaunchKiosk(ctx, cleanUpCtxs context.Context, kiosk *kioskmode.Kiosk, s *testing.State, fdms *fakedms.FakeDMS) {
	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	chromeOptions := s.Param().(chrome.Option)
	if err != nil {
		s.Fatal("Failed to start log reader: ", err)
	}
	defer reader.Close()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create Manager object: ", err)
	}
	h := helper{Manager: manager}
	defer h.restoreAllNetworkInterfaces(cleanUpCtxs)
	if err := h.disableAllNetworkInterfaces(ctx); err != nil {
		s.Fatal("Failed to disable non cellular interface: ", err)
	}

	// GetEnabledTechnologies returns a list of all enabled shill networking technologies.
	enabledTechnologies, err := manager.GetEnabledTechnologies(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve enabled shill networking technologies: ", err)
	}

	s.Log("List of all enabled shill networking technologies ", enabledTechnologies)

	_, err = kiosk.RestartChromeWithOptions(
		ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.NoLogin(),
		chrome.KeepState(),
		chromeOptions,
	)
	if err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}

	if err := kioskmode.ConfirmKioskStarted(ctx, reader); err != nil {
		s.Fatal("Kiosk is not started after restarting Chrome: ", err)
	}
}

func (h *helper) disableAllNetworkInterfaces(ctx context.Context) error {
	ctx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime*2)
	defer cancel()

	// Disable Ethernet if present and maybe re-enabling.
	ethernetFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet)
	if err != nil {
		return errors.Wrap(err, "unable to disable Ethernet")
	}

	// Disable  Cellular if present and maybe re-enabling.
	cellularFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyCellular)
	if err != nil {
		return errors.Wrap(err, "unable to disable Cellular")
	}

	// Disable Wifi if present and maybe re-enabling.
	wifiFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyWifi)
	if err != nil {
		return errors.Wrap(err, "unable to disable Wifi")
	}

	h.enableEthernetFunc = ethernetFunc
	h.enableWifiFunc = wifiFunc
	h.enableCellularFunc = cellularFunc

	return nil
}

// restoreAllNetworkInterfaces enable previously disabled interfaces.
func (h *helper) restoreAllNetworkInterfaces(ctx context.Context) {
	if h.enableEthernetFunc != nil {
		h.enableEthernetFunc(ctx)
	}

	if h.enableWifiFunc != nil {
		h.enableWifiFunc(ctx)
	}

	if h.enableCellularFunc != nil {
		h.enableWifiFunc(ctx)
	}

	h.enableEthernetFunc = nil
	h.enableWifiFunc = nil
	h.enableCellularFunc = nil
}
