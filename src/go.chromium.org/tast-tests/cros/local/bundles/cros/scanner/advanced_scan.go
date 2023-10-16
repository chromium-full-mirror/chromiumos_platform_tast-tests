// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package scanner

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	lpb "chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/local/printing/ippusbbridge"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast-tests/cros/local/scanner/lorgnette"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type advParams struct {
	Network bool // Set up as a network device instead of local.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdvancedScan,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests eSCL scanning via an ipp-over-usb tunnel using the lorgnette advanced APIs",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_scanning",
			"informational",
		},
		SoftwareDeps: []string{"cups", "chrome"},
		Fixture:      "virtualUsbPrinterModulesLoadedWithChromeLoggedIn",
		Params: []testing.Param{{
			Name: "usb",
			Val: &advParams{
				Network: false,
			},
		}, {
			Name: "network",
			Val: &advParams{
				Network: true,
			},
		}},
	})
}

func checkOption(option *lpb.ScannerOption, expectedType lpb.OptionType, expectedUnit lpb.OptionUnit) error {
	if option.OptionType != expectedType {
		return errors.Errorf("Wrong type.  Got %v, expected %v", lpb.OptionType.String(option.OptionType), lpb.OptionType.String(expectedType))
	}
	if option.Unit != expectedUnit {
		return errors.Errorf("Wrong unit.  Got %v, expected %v", lpb.OptionUnit.String(option.Unit), lpb.OptionUnit.String(expectedUnit))
	}
	return nil
}

func checkImage(scanData []byte, width, height int) error {
	imgInfo, err := png.DecodeConfig(bytes.NewReader(scanData))
	if err != nil {
		return err
	}

	if imgInfo.ColorModel != color.RGBAModel {
		return errors.Errorf("Incorrect color model.  Got %v, expected %v", imgInfo.ColorModel, color.RGBAModel)
	}
	if imgInfo.Width != width {
		return errors.Errorf("Incorrect width.  Got %v, expected %v", imgInfo.Width, width)
	}
	if imgInfo.Height != height {
		return errors.Errorf("Incorrect height.  Got %v, expected %v", imgInfo.Height, height)
	}
	return nil
}

func AdvancedScan(ctx context.Context, s *testing.State) {
	const esclCapabilities = "/usr/local/etc/virtual-usb-printer/escl_capabilities.json"

	testOpt := s.Param().(*advParams)

	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WithESCLCapabilities(esclCapabilities),
		usbprinter.ExpectUdevEventOnStop(),
		usbprinter.WaitUntilConfigured())
	if err != nil {
		s.Fatal("Failed to attach virtual printer: ", err)
	}
	defer func(ctx context.Context) {
		if err := printer.Stop(ctx); err != nil {
			s.Error("Failed to stop printer: ", err)
		}
	}(cleanupCtx)

	scanner, err := ippusbbridge.PrepareScannerConnection(ctx, printer.DevInfo, testOpt.Network)
	if err != nil {
		s.Fatal("Failed to prepare scanner connection: ", err)
	}
	defer scanner.Cleanup(cleanupCtx)

	tmpDir, err := ioutil.TempDir("", "tast.scanner.AdvancedScan.")
	if err != nil {
		s.Fatal("Failed to create temporary directory: ", err)
	}
	defer os.RemoveAll(tmpDir)

	s.Log("Connecting to lorgnette")
	l, err := lorgnette.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to lorgnette: ", err)
	}
	defer func(ctx context.Context) {
		// Lorgnette was auto started during testing.  Kill it to avoid
		// affecting subsequent tests.
		lorgnette.StopService(ctx)
	}(cleanupCtx)

	s.Log("Opening scanner ", scanner.ConnectionString)
	openScannerRequest := &lpb.OpenScannerRequest{
		ScannerId: &lpb.ScannerId{
			ConnectionString: scanner.ConnectionString,
		},
		ClientId: "tast.scanner.AdvancedScan",
	}
	openScannerResponse, err := l.OpenScanner(ctx, openScannerRequest)
	if err != nil {
		s.Fatal("Failed to call OpenScanner: ", err)
	}
	if openScannerResponse.Result != lpb.OperationResult_OPERATION_RESULT_SUCCESS {
		s.Fatalf("Failed to open scanner %v: %v", scanner.ConnectionString, lpb.OperationResult.String(openScannerResponse.Result))
	}
	if openScannerResponse.Config == nil {
		s.Fatal("Successful OpenScanner response is missing Config")
	}
	handle := openScannerResponse.Config.Scanner
	defer func(ctx context.Context) {
		s.Log("Closing scanner ", scanner.ConnectionString)
		closeScannerRequest := &lpb.CloseScannerRequest{
			Scanner: handle,
		}
		closeScannerResponse, err := l.CloseScanner(ctx, closeScannerRequest)
		if err != nil {
			s.Fatal("Failed to call CloseScanner: ", err)
		}
		if closeScannerResponse.Result != lpb.OperationResult_OPERATION_RESULT_SUCCESS {
			s.Fatalf("Failed to close scanner %v: %v", scanner.ConnectionString, lpb.OperationResult.String(closeScannerResponse.Result))
		}
	}(cleanupCtx)

	// Check a few options.  Don't exhausively compare the config because it contains a lot of strings that don't have
	// semantic meaning.
	resolution, found := openScannerResponse.Config.Options["resolution"]
	if !found || resolution == nil {
		s.Fatal("OpenScanner response missing expected option resolution")
	}
	if err := checkOption(resolution, lpb.OptionType_TYPE_INT, lpb.OptionUnit_UNIT_DPI); err != nil {
		s.Error("Mismatch for resolution: ", err)
	}

	mode, found := openScannerResponse.Config.Options["mode"]
	if !found || mode == nil {
		s.Fatal("OpenScanner response missing expected option mode")
	}
	if err := checkOption(mode, lpb.OptionType_TYPE_STRING, lpb.OptionUnit_UNIT_NONE); err != nil {
		s.Error("Mismatch for mode: ", err)
	}

	source, found := openScannerResponse.Config.Options["source"]
	if !found || source == nil {
		s.Fatal("OpenScanner response missing expected option source")
	}
	if err := checkOption(source, lpb.OptionType_TYPE_STRING, lpb.OptionUnit_UNIT_NONE); err != nil {
		s.Error("Mismatch for source: ", err)
	}

	// Set the requested dimensions to color A4 at 150 dpi.
	s.Log("Setting scan options")
	setOptionsRequest := &lpb.SetOptionsRequest{
		Scanner: handle,
		Options: []*lpb.ScannerOption{
			&lpb.ScannerOption{
				// Deliberately include a bad option to verify that it doesn't prevent the rest from being set.
				Name:       "bad-option",
				OptionType: lpb.OptionType_TYPE_BOOL,
				Value:      &lpb.ScannerOption_BoolValue{BoolValue: true},
			},
			&lpb.ScannerOption{
				Name:       "mode",
				OptionType: lpb.OptionType_TYPE_STRING,
				Value:      &lpb.ScannerOption_StringValue{StringValue: "color"},
			},
			&lpb.ScannerOption{
				Name:       "resolution",
				OptionType: lpb.OptionType_TYPE_INT,
				Value: &lpb.ScannerOption_IntValue{
					IntValue: &lpb.ScannerOption_IntValues{
						Value: []int32{150},
					},
				},
			},
			&lpb.ScannerOption{
				Name:       "tl-x",
				OptionType: lpb.OptionType_TYPE_FIXED,
				Value: &lpb.ScannerOption_FixedValue{
					FixedValue: &lpb.ScannerOption_FixedValues{
						Value: []float64{0.0},
					},
				},
			},
			&lpb.ScannerOption{
				Name:       "tl-y",
				OptionType: lpb.OptionType_TYPE_FIXED,
				Value: &lpb.ScannerOption_FixedValue{
					FixedValue: &lpb.ScannerOption_FixedValues{
						Value: []float64{0.0},
					},
				},
			},
			&lpb.ScannerOption{
				Name:       "br-x",
				OptionType: lpb.OptionType_TYPE_FIXED,
				Value: &lpb.ScannerOption_FixedValue{
					FixedValue: &lpb.ScannerOption_FixedValues{
						Value: []float64{210.0},
					},
				},
			},
			&lpb.ScannerOption{
				Name:       "br-y",
				OptionType: lpb.OptionType_TYPE_FIXED,
				Value: &lpb.ScannerOption_FixedValue{
					FixedValue: &lpb.ScannerOption_FixedValues{
						Value: []float64{297.0},
					},
				},
			},
		},
	}
	setOptionsResponse, err := l.SetOptions(ctx, setOptionsRequest)
	if err != nil {
		s.Fatal("Failed to call SetOptions: ", err)
	}
	for optname, result := range setOptionsResponse.Results {
		// bad-option should fail and the rest should succeed.
		if optname == "bad-option" && result != lpb.OperationResult_OPERATION_RESULT_UNSUPPORTED {
			s.Fatal("Wrong SetOptions result for bad-option: ", lpb.OperationResult.String(result))
		}
		if optname != "bad-option" && result != lpb.OperationResult_OPERATION_RESULT_SUCCESS {
			s.Fatalf("Wrong SetOptions result for %v: %v", optname, lpb.OperationResult.String(result))
		}
	}
	if setOptionsResponse.Config == nil {
		s.Fatal("Successful SetOptions response is missing Config")
	}

	s.Log("Starting scan")
	startScanRequest := &lpb.StartPreparedScanRequest{
		Scanner:     handle,
		ImageFormat: "image/png",
	}
	startScanResponse, err := l.StartPreparedScan(ctx, startScanRequest)
	if err != nil {
		s.Fatal("Failed to call StartPreparedScan: ", err)
	}
	if startScanResponse.Result != lpb.OperationResult_OPERATION_RESULT_SUCCESS {
		s.Fatal("Failed to start scan: ", lpb.OperationResult.String(startScanResponse.Result))
	}
	if startScanResponse.JobHandle == nil {
		s.Fatal("Successful StartPreparedScan response is missing job handle")
	}
	jobHandle := startScanResponse.JobHandle
	defer func(ctx context.Context) {
		s.Log("Canceling scan job")
		cancelScanRequest := &lpb.CancelScanRequest{
			JobHandle: jobHandle,
		}
		cancelScanResponse, err := l.CancelScan(ctx, cancelScanRequest)
		if err != nil {
			s.Fatal("Failed to call CancelScan: ", err)
		}
		if cancelScanResponse.Result != lpb.OperationResult_OPERATION_RESULT_SUCCESS {
			s.Fatal("Failed to cancel scan: ", lpb.OperationResult.String(cancelScanResponse.Result))
		}
	}(cleanupCtx)

	s.Log("Reading scan data")
	readScanDataRequest := &lpb.ReadScanDataRequest{
		JobHandle: jobHandle,
	}
	scanData := make([]byte, 0)
	keepReading := true
	for keepReading {
		readResponse, err := l.ReadScanData(ctx, readScanDataRequest)
		if err != nil {
			s.Fatal("Failed to call ReadScanData: ", err)
		}

		switch readResponse.Result {
		case lpb.OperationResult_OPERATION_RESULT_SUCCESS:
			scanData = append(scanData, readResponse.Data...)
			if len(readResponse.Data) == 0 {
				// GoBigSleepLint: Give the scanner time to produce more data.
				testing.Sleep(ctx, 100*time.Millisecond)
			}
		case lpb.OperationResult_OPERATION_RESULT_EOF:
			scanData = append(scanData, readResponse.Data...)
			keepReading = false
		default:
			s.Fatal("Got unexpected result from ReadScanData: ", lpb.OperationResult.String(readResponse.Result))
		}
	}
	s.Logf("Got %d total scanned bytes", len(scanData))

	// Expect a color 210x297mm image at 150 dpi.
	if err := checkImage(scanData, 1240, 1753); err != nil {
		s.Error("Incorrect scanned image: ", err)
		saveScanPath := filepath.Join(s.OutDir(), "scan.png")
		if err := ioutil.WriteFile(saveScanPath, scanData, 0644); err != nil {
			s.Fatal("Failed to save scanned image: ", err)
		}
	}
}
