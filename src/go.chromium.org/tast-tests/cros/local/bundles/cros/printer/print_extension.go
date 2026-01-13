// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/fake"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/printer/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/printing/document"
	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParams struct {
	cancelPrint bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     PrintExtension,
		Desc:     "Tests that printing via the chrome.printing extension API works properly",
		Contacts: []string{"project-bolton@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		Data:         []string{ppdFile, goldenFile},
		SoftwareDeps: []string{"cups", "ghostscript"},
		HardwareDeps: hwdep.D(pre.PrinterSkipUnstableModels),
		Params: []testing.Param{
			{
				Name:              "cancel",
				Val:               testParams{cancelPrint: true},
				ExtraSoftwareDeps: []string{"chrome"},
				Fixture:           "chromeLoggedIn",
			},
			{
				Name:              "complete",
				Val:               testParams{cancelPrint: false},
				ExtraSoftwareDeps: []string{"chrome"},
				Fixture:           "chromeLoggedIn",
			},
		},
	})
}

const ppdFile = "print_usb_ps.ppd.gz"
const goldenFile = "print_extension_golden.ps"

func PrintExtension(ctx context.Context, s *testing.State) {
	const (
		printerID   = "FakePrinterID"
		printerName = "FakePrinterName"
		printerDesc = "FakePrinterDescription"
	)
	ppdFilePath := s.DataPath(ppdFile)
	if _, err := os.Stat(ppdFilePath); err != nil {
		s.Fatal("Failed to read PPD file: ", err)
	}

	expect, err := os.ReadFile(s.DataPath(goldenFile))
	if err != nil {
		s.Fatal("Failed to read golden file: ", err)
	}

	if err := printer.ResetCups(ctx, true /*usePrintscanmgr*/); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	printer, err := fake.NewPrinter(ctx)
	if err != nil {
		s.Fatal("Failed to start fake printer: ", err)
	}
	defer printer.Close()

	params := s.Param().(testParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to ash test API: ", err)
	}

	s.Log("Registering a printer")
	const printerURI = "localhost:9101"
	if err := tconn.Call(ctx, nil, "chrome.autotestPrivate.updatePrinter", map[string]string{"printerName": printerName, "printerId": printerID, "printerDesc": printerDesc, "printerUri": "socket://" + printerURI, "printerPpd": ppdFilePath}); err != nil {
		s.Fatal("autotestPrivate.updatePrinter() failed: ", err)
	}

	defer func() {
		if err := tconn.Call(ctx, nil, "chrome.autotestPrivate.removePrinter", printerID); err != nil {
			s.Fatal("autotestPrivate.removePrinter() failed: ", err)
		}
	}()

	s.Log("Calling chrome.printing.getPrinters")
	var printers []struct {
		Description      string
		ID               string
		IsDefault        bool
		Name             string
		RecentlyUsedRank int
		Source           string
		URI              string
	}

	if err := tconn.Call(ctx, &printers, "tast.promisify(chrome.printing.getPrinters)"); err != nil {
		s.Fatal("Failed to call getPrinters: ", err)
	}

	// Look through our printers and make sure we have one (and exactly one) that
	// matches what we expect.
	count := 0
	for _, printer := range printers {
		if printer.Description == printerDesc &&
			printer.ID == printerID &&
			!printer.IsDefault &&
			printer.Name == printerName &&
			printer.Source == "USER" &&
			printer.URI == "socket://"+printerURI {
			count++
		}
	}
	if count != 1 {
		for index, printer := range printers {
			s.Logf("Printer %d: %v", index, printer)
		}
		s.Fatal("Expected 1 printer, found ", count)
	}

	s.Log("Calling chrome.printing.getPrinterInfo")
	var info struct {
		Capabilities struct {
			Version string
			Printer map[string]interface{}
			Scanner map[string]interface{}
		}
		Status string
	}
	if err := tconn.Call(ctx, &info, "tast.promisify(chrome.printing.getPrinterInfo)", printerID); err != nil {
		s.Fatal("Failed to call getPrinterInfo: ", err)
	}
	if info.Capabilities.Version != "1.0" {
		s.Error("Unexpected version: ", info.Capabilities.Version)
	}
	for _, attr := range []string{"color", "collate", "copies", "dpi", "duplex", "media_size", "page_orientation", "pin", "supported_content_type", "vendor_capability"} {
		if _, ok := info.Capabilities.Printer[attr]; !ok {
			s.Error("Missing printer capability: ", attr)
		}
	}
	if len(info.Capabilities.Scanner) != 0 {
		s.Errorf("Unexpected scanner capabilities: found %d elements", len(info.Capabilities.Scanner))
	}
	if info.Status != "AVAILABLE" {
		s.Error("Unexpected status: ", info.Status)
	}

	s.Log("Registering chrome.printing.onJobStatusChanged listener")
	if err := tconn.Eval(ctx, "var events = []; function printEventCallback(id,status) { events.push({id: id, status: status}); }; chrome.printing.onJobStatusChanged.addListener(printEventCallback)", nil); err != nil {
		s.Fatal("Failed to register onJobStatusChanged observer: ", err)
	}
	defer func() {
		if err := tconn.Eval(ctx, "chrome.printing.onJobStatusChanged.removeListener(printEventCallback)", nil); err != nil {
			s.Fatal("chrome.printing.onJobStatusChanged.removeListener() failed: ", err)
		}
	}()

	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", "printing.printing_api_extensions_whitelist", []string{chrome.TestExtensionID}); err != nil {
		s.Fatal("Failed to set printing.printing_api_extensions_whitelist: ", err)
	}

	s.Log("Calling chrome.printing.submitJob")
	var job struct {
		JobID  string
		Status string
	}
	if err := tconn.Eval(ctx, `tast.promisify(chrome.printing.submitJob)({
	  job: {
	    contentType: "application/pdf",
		document: new Blob([atob("JVBERi0xLjAKMSAwIG9iajw8L1R5cGUgL0NhdGFsb2cgL1BhZ2VzIDIgMCBSPj5lbmRvYmoKMiAw\nIG9iajw8L1R5cGUgL1BhZ2VzIC9LaWRzWzMgMCBSXS9Db3VudCAxPj5lbmRvYmoKMyAwIG9iajw8\nL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUi9NZWRpYUJveFswIDAgNTk1IDg0Ml0+PmVuZG9iagp0\ncmFpbGVyPDwvU2l6ZSAzL1Jvb3QgMSAwIFI+PgolJUVPRgo=")]),
	    printerId: "`+printerID+`",
	    ticket: {
	      version: "1.0",
	      print: {
		color: { type: "STANDARD_COLOR" },
		duplex: { type: "NO_DUPLEX" },
		page_orientation: { type: "PORTRAIT" },
		copies: { copies: 2 },
		margins: {
			top_microns: 12700,
			right_microns: 6350,
			bottom_microns: 12700,
			left_microns: 6350
		},
		dpi: { horizontal_dpi: 600, vertical_dpi: 600 },
		media_size: { width_microns: 210000, height_microns: 297000, vendor_id: "iso_a4_210x297mm" },
		collate: { collate: false }
	      }
	    },
	    title: "title"
	  }
	})`, &job); err != nil {
		s.Fatal("Failed to call submitJob: ", err)
	}
	if job.Status != "OK" {
		s.Fatal("Unexpected status: ", job.Status)
	}
	if len(job.JobID) == 0 {
		s.Fatal("Empty JobId")
	}

	s.Log("Receiving print request")
	recvCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := printer.ReadRequest(recvCtx)
	if err != nil {
		s.Fatal("Fake printer didn't receive a request: ", err)
	}

	if err = document.CompareFileContents(ctx, string(request), string(expect),
		s.OutDir(), "diff.txt", goldenFile); err != nil {
		s.Error("Printer output differs from expected: ", err)
	}

	var events []struct {
		ID     string
		Status string
	}
	if err := tconn.Eval(ctx, "events", &events); err != nil {
		s.Fatal("Failed to get events: ", err)
	}
	if len(events) != 2 {
		s.Fatalf("Unexpected number of events (%d): %s", len(events), events)
	}
	if events[0].ID != job.JobID || events[0].Status != "PENDING" {
		s.Errorf("Unxpected event: %s %s", events[0].ID, events[0].Status)
	}
	if events[1].ID != job.JobID || events[1].Status != "IN_PROGRESS" {
		s.Errorf("Unexpected event: %s %s", events[1].ID, events[1].Status)
	}

	var expectedStatus string
	if params.cancelPrint {
		s.Log("Calling chrome.printing.cancelJob")
		if err := tconn.Call(ctx, nil, "tast.promisify(chrome.printing.cancelJob)", job.JobID); err != nil {
			s.Fatal("Failed to call cancelJob: ", err)
		}
		expectedStatus = "CANCELED"
	} else {
		s.Log("Disconnecting printer")
		printer.Close()
		expectedStatus = "PRINTED"
	}
	if err := tconn.WaitForExprFailOnErrWithTimeout(ctx, "events.length >= 3", 10*time.Second); err != nil {
		s.Error("Failure waiting for events: ", err)
	}
	if err := tconn.Eval(ctx, "events", &events); err != nil {
		s.Fatal("Failed to get events: ", err)
	}
	if len(events) != 3 {
		s.Fatalf("Unexpected number of events (%d): %s", len(events), events)
	}
	if events[2].ID != job.JobID || events[2].Status != expectedStatus {
		s.Errorf("Unexpected event: %s %s", events[2].ID, events[2].Status)
	}
}
