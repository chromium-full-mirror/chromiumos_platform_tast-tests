// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"io/ioutil"
	"net/http"

	ppb "chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Printscanmgr,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performs validity testing of printer-related D-Bus methods",
		Contacts: []string{
			"project-bolton@google.com",
			"pmoy@chromium.org",
		},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		SoftwareDeps: []string{"chrome", "cups", "printscanmgr"},
		Data:         []string{"GenericPostScript.ppd.gz"},
		Fixture:      "chromeLoggedIn",
		Attr: []string{
			"group:mainline",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
	})
}

func pageNotAPrinter(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("It is not a printer!"))
}

func Printscanmgr(ctx context.Context, s *testing.State) {
	// Start a local HTTP server on port 7001.
	http.HandleFunc("/not_a_printer", pageNotAPrinter)
	server := &http.Server{Addr: ":7001"}
	go server.ListenAndServe()
	defer server.Shutdown(ctx)

	ppd, err := ioutil.ReadFile(s.DataPath("GenericPostScript.ppd.gz"))
	if err != nil {
		s.Fatal("Failed to read PPD file: ", err)
	}

	p, err := printscanmgr.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to printscanmgr: ", err)
	}

	if err := printer.ResetCups(ctx, /*usePrintscanmgr=*/true); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	s.Log("Validating that a printer can be installed")
	const printerAddr = "localhost:9101"
	if result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx,
		&ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        "ManualPrinterGood",
			Uri:         "socket://" + printerAddr + "/ipp/fake_printer",
			PpdContents: ppd}); err != nil {
		s.Error("Failed to call CupsAddManuallyConfiguredPrinter: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS {
		s.Error("Could not set up valid printer: ", result.Result)
	}

	s.Log("Verifying that an error is returned for lpadmin failure")
	if result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx,
		&ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        "CUPS rejects names with spaces",
			Uri:         "socket://" + printerAddr + "/ipp/fake_printer",
			PpdContents: ppd}); err != nil {
		s.Error("Failed to call CupsAddManuallyConfiguredPrinter: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_FATAL {
		s.Error("Names with spaces should be rejected by CUPS: ", result.Result)
	}

	s.Log("Validating that malformed PPDs are rejected")
	badPPD := []byte("This is not a valid ppd")
	if result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx,
		&ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        "ManualPrinterBreaks",
			Uri:         "socket://" + printerAddr + "/ipp/fake_printer",
			PpdContents: badPPD}); err != nil {
		s.Error("Failed to call CupsAddManuallyConfiguredPrinter: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_INVALID_PPD {
		s.Error("Incorrect error code received: ", result.Result)
	}

	s.Log("Attempting to add an unreachable autoconfigured printer")
	if result, err := p.CupsAddAutoConfiguredPrinter(
		ctx,
		&ppb.CupsAddAutoConfiguredPrinterRequest{
			Name: "AutoconfPrinter",
			Uri:  "ipp://" + printerAddr + "/ipp/print"}); err != nil {
		s.Error("Failed to call CupsAddAutoConfiguredPrinter: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_PRINTER_UNREACHABLE {
		s.Error("Incorrect error code received: ", result.Result)
	}

	// Make sure that the HTTP server on port 7001 is ready.
	getPage := func(ctx context.Context) error {
		httpReq, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:7001/not_a_printer", nil)
		if err == nil {
			var res *http.Response
			res, err = http.DefaultClient.Do(httpReq)
			if err == nil && res.StatusCode != http.StatusOK {
				err = errors.Errorf("unexpected status of HTTP response: %d", res.StatusCode)
			}
		}
		return err
	}
	if testing.Poll(ctx, getPage, nil) != nil {
		s.Fatal("Cannot start local HTTP server ")
	}

	s.Log("Attempting to add a url that returns HTTP_BAD_REQUEST")
	if result, err := p.CupsAddAutoConfiguredPrinter(
		ctx,
		&ppb.CupsAddAutoConfiguredPrinterRequest{
			Name: "NotAPrinter",
			Uri:  "ipp://localhost:7001/bad_request"}); err != nil {
		s.Error("Calling printer setup crashed: ", err)
	} else if result.Result != ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_PRINTER_WRONG_RESPONSE {
		s.Error("Incorrect error code received: ", result.Result)
	}
}
