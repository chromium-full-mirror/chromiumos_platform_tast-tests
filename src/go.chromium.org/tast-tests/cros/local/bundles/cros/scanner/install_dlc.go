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
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     InstallDLC,
		Desc:     "E2E verification that SANE backend DLCs are installed for specific detected scanners",
		Contacts: []string{"project-bolton@google.com", "aaronmassey@google.com"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"group:paper-io",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"chrome", "cros_internal", "cups", "dlc"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
	})
}

func InstallDLC(ctx context.Context, s *testing.State) {
	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func() {
		lorgnette.StopService(cleanupCtx)
	}()

	// We test the installation of both PFU and Canon backends.
	scannerDlcIds := []string{"sane-backends-pfu", "sane-backends-canon"}

	// Map to associate the DLC ID with its stub configuration
	dlcDescriptors := map[string]string{
		"sane-backends-pfu":   "stub_usb_fujitsu_scanner.json",
		"sane-backends-canon": "stub_usb_canon_scanner.json",
	}

	// Setup Test API connection once before looping
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Loop to test Chrome scanner installation notification for one DLC at a time.
	for _, dlcID := range scannerDlcIds {
		s.Logf("=== Starting test for DLC: %s ===", dlcID)

		// Purge the DLC to ensure a clean state
		if err := dlc.Purge(ctx, dlcID); err != nil {
			s.Logf("Note: Failed to purge %s: %v", dlcID, err)
		}

		// Restart lorgnette to clear cached USB topology and discovery state
		if err := upstart.RestartJob(ctx, "lorgnette"); err != nil {
			s.Fatalf("Failed to restart lorgnette for %s: %v", dlcID, err)
		}

		// MUST re-establish the D-Bus connection AFTER restarting the daemon
		l, err := lorgnette.New(ctx)
		if err != nil {
			s.Fatalf("Failed to connect to lorgnette after restart for %s: %v", dlcID, err)
		}

		printer, err := usbprinter.Start(ctx,
			usbprinter.WithDescriptors(dlcDescriptors[dlcID]))
		if err != nil {
			s.Fatalf("Failed to start stub scanner for %s: %v", dlcID, err)
		}

		// GoBigSleepLint: Allow time for the kernel/udev to enumerate the virtual USB device.
		if err := testing.Sleep(ctx, 3*time.Second); err != nil {
			s.Log("Sleep interrupted: ", err)
		}

		notificationChannel := make(chan error, 1)
		go func() {
			myPredicate := ash.WaitTitleContains("Scanner software installed")
			_, err := ash.WaitForNotification(ctx, tconn, 30*time.Second, myPredicate)
			notificationChannel <- err
		}()

		s.Logf("Requesting scanner list to trigger DLC install for %s", dlcID)
		startDiscoveryRequest := &lpb.StartScannerDiscoveryRequest{
			ClientId:       "ScannerDLCTest_" + dlcID,
			DownloadPolicy: lpb.BackendDownloadPolicy_DOWNLOAD_IF_NEEDED,
			LocalOnly:      true,
			PreferredOnly:  false,
		}

		// Do not ignore the error. This catches D-Bus disconnections immediately.
		if _, err := l.StartScannerDiscovery(ctx, startDiscoveryRequest); err != nil {
			s.Fatalf("Failed to start scanner discovery for %s: %v", dlcID, err)
		}

		s.Log("Waiting for Chrome notification of installed scanner DLCs")
		if err = <-notificationChannel; err != nil {
			s.Errorf("Did not see DLC installed notification for %s: %v", dlcID, err)
		}

		dlcIsInstalled, err := dlc.Installed(ctx, dlcID)
		if err != nil {
			s.Fatalf("Failed to check if scanner DLCs are installed for %s: %v", dlcID, err)
		}
		if !dlcIsInstalled {
			s.Errorf("Failed to install scanner DLC: %s", dlcID)
		}

		// Clear the notification so it does not interfere with the next loop iteration
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			s.Log("Failed to clear notifications: ", err)
		}

		if err := printer.Stop(ctx); err != nil {
			s.Errorf("Failed to stop stub scanner: %v", err)
		}
	}
}
