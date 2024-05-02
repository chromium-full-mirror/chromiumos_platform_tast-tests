// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package documentscanapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	lpb "go.chromium.org/chromiumos/system_api/lorgnette_proto"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/documentscanapi/setup"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/printing/ippusbbridge"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast-tests/cros/local/scanner/lorgnette"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdvancedScan,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that a scan can be performed using the advanced Document Scan API",
		Contacts:     []string{"project-bolton@google.com", "bmgordon@chromium.org"},
		// ChromeOS > Platform > Services > Scanning
		BugComponent: "b:860616",
		Data:         []string{"manifest.json", "background.js", "scan.css", "scan.html", "scan.js", "scan_escl_ipp_source.jpg", "scan_escl_ipp_golden.png"},
		// TODO(b/311454704): Remove cups dependency when USE flag is created.
		SoftwareDeps: []string{"chrome", "cups"},
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_scanning",
			"informational",
		},
		Fixture: "virtualUsbPrinterModulesLoaded",
		Params: []testing.Param{
			{
				Val: browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

func setLorgnetteDebug(ctx context.Context, enabled bool) (bool, error) {
	l, err := lorgnette.New(ctx)
	if err != nil {
		return false, errors.Wrap(err, "unable to connect to lorgnette")
	}

	request := &lpb.SetDebugConfigRequest{
		Enabled: enabled,
	}
	response, err := l.SetDebugConfig(ctx, request)
	if err != nil {
		return false, errors.Wrap(err, "unable to send SetDebugConfigRequest")
	}

	return response.OldEnabled, nil
}

// AdvancedScan tests the advanced chrome.documentScan API.
func AdvancedScan(ctx context.Context, s *testing.State) {
	// Use cleanupCtx for any deferred cleanups in case of timeouts or
	// cancellations on the shortened context.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	extDir, err := ioutil.TempDir("", "tast.documentscanapi.AdvancedScan.")
	if err != nil {
		s.Fatal("Failed to create temp extension dir: ", err)
	}
	defer os.RemoveAll(extDir)

	scanTargetExtID, err := setup.CreateExtension(ctx, s, extDir)
	if err != nil {
		s.Fatal("Failed setup of Document Scan extension: ", err)
	}

	bt := s.Param().(browser.Type)
	opts := []chrome.Option{chrome.EnableFeatures("AsynchronousScannerDiscovery", "AdvancedDocumentScanAPI")}
	if bt == browser.TypeLacros {
		opts = append(opts, chrome.LacrosUnpackedExtension(extDir))
	} else {
		opts = append(opts, chrome.UnpackedExtension(extDir))
	}
	lacrosConfig := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("AsynchronousScannerDiscovery", "AdvancedDocumentScanAPI")))
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosConfig, opts...)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// Open the ash-chrome test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	printer, err := usbprinter.Start(ctx,
		usbprinter.WithIPPUSBDescriptors(),
		usbprinter.WithGenericIPPAttributes(),
		usbprinter.WithESCLCapabilities(setup.EsclCapabilities),
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
	if err = ippusbbridge.WaitForSocket(ctx, printer.DevInfo); err != nil {
		s.Fatal("Failed to wait for ippusb socket: ", err)
	}
	if err = ippusbbridge.ContactPrinterEndpoint(ctx, printer.DevInfo, "/eSCL/ScannerCapabilities"); err != nil {
		s.Fatal("Failed to get scanner status over ippusb_bridge socket: ", err)
	}

	if err = tconn.Call(ctx, nil, "tast.promisify(chrome.autotestPrivate.setAllowedPref)", "document_scan.document_scan_api_trusted_extensions", []string{scanTargetExtID}); err != nil {
		s.Fatal("Failed to add extension to DocumentScanAPITrustedExtensions: ", err)
	}

	// Put lorgnette into debug mode so we get extra logs if the scanner fails.
	oldDebug, err := setLorgnetteDebug(ctx, true)
	if err != nil {
		s.Fatal("Failed to set lorgnette debug level: ", err)
	}
	defer func(ctx context.Context) {
		_, err := setLorgnetteDebug(ctx, oldDebug)
		if err != nil {
			s.Log("Failed to reset lorgnette debug level: ", err)
		}
	}(cleanupCtx)

	extURL := "chrome-extension://" + scanTargetExtID + "/scan.html"
	conn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(extURL))
	if err != nil {
		s.Fatalf("Failed to connect to extension URL at %v: %v", extURL, err)
	}
	defer conn.Close()

	// APIs are not immediately available to extensions: https://crbug.com/789313.
	s.Log("Waiting for chrome.documentScan API to become available")
	if err := conn.WaitForExprFailOnErr(ctx, "chrome.documentScan"); err != nil {
		s.Fatal("chrome.documentScan API unavailable: ", err)
	}

	s.Log("Getting scanner list")
	ui := uiauto.New(tconn)

	extensionWindow := nodewith.Name("Scanner Control").Role(role.RootWebArea)
	getScannersButton := nodewith.Name("Get Scanners").Role(role.Button).Ancestor(extensionWindow)
	// We use First() because there are two nodes that share the dialog Role with the same Name, one is an immediate parent of the next.
	allowDiscoveryPrompt := nodewith.Name("Allow").Role(role.Button).Ancestor(nodewith.Name("Find document scanners").Role(role.Dialog).First())

	if err := uiauto.Combine("Click scanner list button",
		ui.WithTimeout(10*time.Second).WaitUntilExists(getScannersButton),
		ui.DoDefault(getScannersButton),
		ui.WithTimeout(2*time.Second).WaitForRestriction(getScannersButton, restriction.Disabled),
		uiauto.IfSuccessThen(ui.WithTimeout(2*time.Second).WaitUntilExists(allowDiscoveryPrompt), ui.DoDefault(allowDiscoveryPrompt)),
		ui.WithTimeout(30*time.Second).WaitUntilEnabled(getScannersButton),
	)(ctx); err != nil {
		s.Fatal("Failed to get scanner list: ", err)
	}

	openScannerButton := nodewith.Name("Open Scanner").Role(role.Button).Ancestor(extensionWindow)
	closeScannerButton := nodewith.Name("Close Scanner").Role(role.Button).Ancestor(extensionWindow)
	scanButton := nodewith.Name("Scan").Role(role.Button).Ancestor(extensionWindow)
	s.Log("Opening scanner")
	if err := uiauto.Combine("Open virtual scanner",
		ui.DoDefault(openScannerButton),
		ui.WaitUntilEnabled(closeScannerButton),
	)(ctx); err != nil {
		s.Fatal("Failed to open scanner: ", err)
	}

	s.Log("Configuring scan")
	sourceSelect := nodewith.Name("Scan source").Role(role.ComboBoxSelect).Ancestor(extensionWindow)
	adfSource := nodewith.Name("ADF").Role(role.ListBoxOption).Ancestor(sourceSelect)
	if err := uiauto.Combine("Select ADF",
		ui.WaitUntilExists(sourceSelect.Focusable()),
		ui.EnsureFocused(sourceSelect),
		ui.DoDefault(sourceSelect),
		ui.WaitUntilExists(adfSource),
		ui.DoDefault(adfSource),
		ui.DoDefault(sourceSelect),
		ui.WaitUntilGone(adfSource),
		ui.WaitUntilEnabled(scanButton),
	)(ctx); err != nil {
		s.Fatal("Failed to select ADF: ", err)
	}

	letterButton := nodewith.Name("Letter").Role(role.Button).Ancestor(extensionWindow)
	if err := uiauto.Combine("Select Letter Size",
		ui.DoDefault(letterButton),
		ui.WaitUntilEnabled(scanButton),
	)(ctx); err != nil {
		s.Fatal("Failed to select Letter size: ", err)
	}

	var value string
	if err := conn.Eval(ctx, "document.getElementById('scanResolution').value", &value); err != nil {
		s.Fatal("Failed to get resolution: ", err)
	}
	var resolution int
	if resolution, err = strconv.Atoi(value); err != nil {
		s.Fatal("Failed to parse resolution: ", err)
	}

	s.Log("Starting first scan")
	cancelButton := nodewith.Name("Cancel").Role(role.Button).Ancestor(extensionWindow)
	// We use First() because there are two nodes that share the dialog Role with the same Name, one is an immediate parent of the next.
	allowScanPrompt := nodewith.Name("Allow").Role(role.Button).Ancestor(nodewith.Name("Start scan").Role(role.Dialog).First())
	if err := uiauto.Combine("Scan and cancel",
		ui.DoDefault(scanButton),
		ui.WaitForRestriction(scanButton, restriction.Disabled),
		uiauto.IfSuccessThen(ui.WithTimeout(2*time.Second).WaitUntilExists(allowScanPrompt), ui.DoDefault(allowScanPrompt)),
		ui.WaitUntilEnabled(cancelButton),
		ui.DoDefault(cancelButton),
		ui.WaitUntilEnabled(scanButton),
	)(ctx); err != nil {
		s.Fatal("Failed to perform first scan: ", err)
	}

	s.Log("Verifying cancel")
	var numImages int
	if err := conn.Eval(ctx, "document.getElementById('imageContainer').children.length", &numImages); err != nil {
		s.Fatal("Failed to get image source: ", err)
	}
	if numImages != 0 {
		s.Fatalf("Expected 0 images, got %d", numImages)
	}

	s.Log("Starting second scan")
	if err := uiauto.Combine("Scan and wait for completion",
		ui.DoDefault(scanButton),
		ui.WaitForRestriction(scanButton, restriction.Disabled),
		ui.WaitUntilEnabled(scanButton),
	)(ctx); err != nil {
		s.Fatal("Failed to perform second scan: ", err)
	}

	s.Log("Closing scanner")
	if err := uiauto.Combine("Close virtual scanner",
		ui.DoDefault(closeScannerButton),
		ui.WaitForRestriction(closeScannerButton, restriction.Disabled),
	)(ctx); err != nil {
		s.Fatal("Failed to close scanner: ", err)
	}

	s.Log("Verifying scanned images")
	if err := conn.Eval(ctx, "document.getElementById('imageContainer').children.length", &numImages); err != nil {
		s.Fatal("Failed to get image source: ", err)
	}
	if numImages != 1 {
		s.Fatalf("Expected 1 image, got %d", numImages)
	}

	for i := 0; i < numImages; i++ {
		s.Log("Extracting image ", i)
		expr := fmt.Sprintf("getScannedImageBytes(%d)", i)
		var imageData []byte
		if err := conn.Eval(ctx, expr, &imageData); err != nil {
			s.Fatal("Failed to get image data: ", err)
		}

		scanPath := filepath.Join(s.OutDir(), fmt.Sprintf("scanned_%d.png", i))
		scanFile, err := os.Create(scanPath)
		if err != nil {
			s.Fatal("Failed to open scan output file: ", err)
		}

		if _, err := scanFile.Write(imageData); err != nil {
			s.Fatal("Failed to write out image file: ", err)
		}

		var jpg image.Image
		if jpg, _, err = image.Decode(bytes.NewReader(imageData)); err != nil {
			s.Fatal("Failed to decode image: ", err)
		}

		if jpg.Bounds().Max.X != int(float32(resolution)*8.5) {
			s.Fatalf("Wrong width, expected %d, got %d ", resolution, jpg.Bounds().Max.X)
		}
		if jpg.Bounds().Max.Y != resolution*11 {
			s.Fatalf("Wrong height, expected %d, got %d ", resolution, jpg.Bounds().Max.Y)
		}
	}
}
