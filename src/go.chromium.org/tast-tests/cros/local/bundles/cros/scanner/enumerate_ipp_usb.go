// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanner

import (
	"context"
	"regexp"

	lpb "chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast-tests/cros/local/scanner/lorgnette"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type scannerInfo struct {
	name            string
	options         []usbprinter.Option
	platenImage     string
	shouldEnumerate bool
}

var ippUsbFormat = regexp.MustCompile("^ippusb:escl:.*:(....)_(....)/.*")

type enumParams struct {
	AsyncDiscovery bool // Use an asynchronous discovery session instead of ListScanners.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnumerateIPPUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that IPP-USB devices are correctly found",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_scanning",
		},
		SoftwareDeps: []string{"cups", "chrome"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Params: []testing.Param{{
			Val: &enumParams{
				AsyncDiscovery: false,
			},
		}, {
			Name: "async",
			Val: &enumParams{
				AsyncDiscovery: true,
			},
		}},
	})
}

// isMatchingScanner returns true iff scanner refers to an ippusb device with the same VID and PID as devInfo.
func isMatchingScanner(scanner *lpb.ScannerInfo, devInfo usbprinter.DevInfo) bool {
	if match := ippUsbFormat.FindStringSubmatch(scanner.Name); match != nil {
		return devInfo.VID == match[1] && devInfo.PID == match[2]
	}

	return false
}

func getScannersSync(ctx context.Context, l *lorgnette.Lorgnette) ([]*lpb.ScannerInfo, error) {
	scanners, err := l.ListScanners(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call ListScanners")
	}

	return scanners, nil
}

func getScannersAsync(ctx context.Context, l *lorgnette.Lorgnette) ([]*lpb.ScannerInfo, error) {
	startDiscoveryRequest := &lpb.StartScannerDiscoveryRequest{
		ClientId:       "EnumerateIPPUSB",
		DownloadPolicy: lpb.BackendDownloadPolicy_DOWNLOAD_NEVER,
		LocalOnly:      true,
		PreferredOnly:  false,
	}
	startDiscoveryResponse, err := l.StartScannerDiscovery(ctx, startDiscoveryRequest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send StartScannerDiscovery request")
	}
	if !startDiscoveryResponse.Started {
		return nil, errors.New("scanner discovery did not start")
	}
	scanners, err := l.AccumulateDiscoveredScanners(ctx, startDiscoveryResponse.SessionId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ScannerListChanged events")
	}
	stopDiscoveryRequest := &lpb.StopScannerDiscoveryRequest{
		SessionId: startDiscoveryResponse.SessionId,
	}
	stopDiscoveryResponse, err := l.StopScannerDiscovery(ctx, stopDiscoveryRequest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send StopScannerDiscovery request")
	}
	if !stopDiscoveryResponse.Stopped {
		return nil, errors.New("scanner discovery did not stop")
	}

	return scanners, nil
}

// runEnumerationTest sets up virtual-usb-printer to emulate the device specified in info,
// calls lorgnette's ListScanners, and checks to see if the device was listed in the response.
func runEnumerationTest(ctx context.Context, s *testing.State, info scannerInfo) {
	s.Logf("Checking if %s is listed", info.name)

	testOpt := s.Param().(*enumParams)

	printer, err := usbprinter.Start(ctx, info.options...)
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(ctx)

	s.Log("Connecting to lorgnette")
	l, err := lorgnette.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to lorgnette: ", err)
	}
	defer func() {
		// Lorgnette was auto started during testing.  Kill it to avoid
		// affecting subsequent tests.
		lorgnette.StopService(ctx)
	}()

	var scanners []*lpb.ScannerInfo
	if testOpt.AsyncDiscovery {
		s.Log("Requesting scanner list with async discovery")
		scanners, err = getScannersAsync(ctx, l)
	} else {
		s.Log("Requesting scanner list with ListScanners")
		scanners, err = getScannersSync(ctx, l)
	}
	if err != nil {
		s.Fatal("Failed to get scanner list: ", err)
	}

	found := false
	for _, scanner := range scanners {
		if isMatchingScanner(scanner, printer.DevInfo) {
			found = true
			break
		}
	}
	if found != info.shouldEnumerate {
		s.Errorf("%s enumerated=%v, want=%v", info.name, found, info.shouldEnumerate)
	}
}

func EnumerateIPPUSB(ctx context.Context, s *testing.State) {
	for _, info := range []scannerInfo{{
		name: "Non-IPP USB printer",
		options: []usbprinter.Option{
			usbprinter.WithDescriptors("usb_printer.json"),
			usbprinter.ExpectUdevEventOnStop(),
		},
		shouldEnumerate: false,
	}, {
		name: "IPP USB printer without eSCL",
		options: []usbprinter.Option{
			usbprinter.WithIPPUSBDescriptors(),
			usbprinter.WithGenericIPPAttributes(),
			usbprinter.WaitUntilConfigured(),
			usbprinter.ExpectUdevEventOnStop(),
		},
		shouldEnumerate: false,
	}, {
		name: "IPP USB printer with eSCL",
		options: []usbprinter.Option{
			usbprinter.WithIPPUSBDescriptors(),
			usbprinter.WithGenericIPPAttributes(),
			usbprinter.WithESCLCapabilities("/usr/local/etc/virtual-usb-printer/escl_capabilities.json"),
			usbprinter.WaitUntilConfigured(),
			usbprinter.ExpectUdevEventOnStop(),
		},
		shouldEnumerate: true,
	}} {
		runEnumerationTest(ctx, s, info)
	}
}
