// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanner

import (
	"context"
	"time"

	lpb "go.chromium.org/chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast-tests/cros/local/scanner/lorgnette"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         InstallDLC,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "E2E verification that SANE backend DLCs are installed for specific detected scanners",
		Contacts:     []string{"project-bolton@google.com", "aaronmassey@google.com"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"group:paper-io",
			"informational",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"cups", "chrome", "dlc"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

func InstallDLC(ctx context.Context, s *testing.State) {
	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	s.Log("Connecting to lorgnette")
	l, err := lorgnette.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to lorgnette: ", err)
	}
	defer func() {
		lorgnette.StopService(cleanupCtx)
	}()

	// Currently we only test the installation of the PFU backend.
	scannerDlcIds := []string{"sane-backends-pfu"}

	// We want to test if we actually install the DLC, so we ought
	// to uninstall it first.
	for _, dlcID := range scannerDlcIds {
		dlcIsInstalled, err := dlc.Installed(ctx, dlcID)
		if err != nil {
			s.Fatal("Failed to check if scanner DLCs are installed: ", err)
		}
		if dlcIsInstalled {
			if err := dlc.Uninstall(ctx, dlcID); err != nil {
				s.Fatalf("Failed to uninstall scanner DLC: %s", dlcID)
			}
		}

	}

	// We need to trick lorgnette into thinking there's a PFU DLC
	// driver supporting scanner attached.
	// NOTE: Create separate stubs to install other backend DLCs.
	_, err = usbprinter.Start(ctx,
		usbprinter.WithDescriptors("stub_usb_fujitsu_scanner.json"))
	if err != nil {
		s.Fatal("Failed to start stub scanner: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// As an E2E test, we do check for an installed notification.
	notificationChannel := make(chan error)
	go func() {
		myPredicate := ash.WaitTitleContains("Scanner software installed")
		// Installation and notification here is usually fast.
		_, err := ash.WaitForNotification(ctx, tconn, 10*time.Second, myPredicate)
		notificationChannel <- err
	}()

	s.Log("Requesting scanner list with async discovery to trigger DLC install")
	startDiscoveryRequest := &lpb.StartScannerDiscoveryRequest{
		ClientId:       "ScannerDLCTest",
		DownloadPolicy: lpb.BackendDownloadPolicy_DOWNLOAD_IF_NEEDED,
		LocalOnly:      true,
		PreferredOnly:  false,
	}
	_, err = l.StartScannerDiscovery(ctx, startDiscoveryRequest)

	s.Log("Waiting for Chrome notification of installed scanner DLCs")
	if err = <-notificationChannel; err != nil {
		s.Error("Did not see DLC installed notification")
	}

	// We only validate that lorgnette installs DLCs instead of
	// directly testing discovery because SANE is only probing a
	// stub usb device that has no response to the SANE driver.
	for _, dlcID := range scannerDlcIds {
		dlcIsInstalled, err := dlc.Installed(ctx, dlcID)
		if err != nil {
			s.Fatal("Failed to check if scanner DLCs are installed: ", err)
		}
		if !dlcIsInstalled {
			s.Errorf("Failed to install scanner DLC: %s", dlcID)
		}

	}
}
