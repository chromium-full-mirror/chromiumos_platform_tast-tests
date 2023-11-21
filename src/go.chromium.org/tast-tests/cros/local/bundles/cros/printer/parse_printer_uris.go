// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"io/ioutil"

	ppb "chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/local/printing/printer"
	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast/core/testing"
)

// A printerSpec defines a struct carrying an omnibus of printer
// configuration commonly needed in several test functions. Using this
// struct helps simplify some function definitions, shortening the
// parameter list.
type printerSpec struct {
	name           string            // a descriptive printer name pertinent to test subject
	uri            string            // the printer URI
	ppdContents    []byte            // PPD contents if applicable - empty otherwise
	expectedStatus ppb.AddPrinterResult // result we expect from printscanmgr
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ParsePrinterUris,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests printscanmgr's behavior when parsing printer URIs",
		Contacts: []string{
			"project-bolton@google.com",
			"pmoy@google.com",
		},
		// ChromeOS > Platform > baseOS > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"cups"},
		Data:         []string{"GenericPostScript.ppd.gz"},
	})
}

// addPrinterWithExpectedStatus adds a manually configured printer with the
// given |spec|. It expects that the D-Bus call to printscanmgr via |p| succeeds
// and returns spec.expectedStatus, logging an error otherwise.
func addPrinterWithExpectedStatus(
	ctx context.Context,
	s *testing.State,
	p *printscanmgr.Printscanmgr,
	spec printerSpec) {
	result, err := p.CupsAddManuallyConfiguredPrinter(
		ctx, &ppb.CupsAddManuallyConfiguredPrinterRequest{
			Name:        spec.name,
			Uri:         spec.uri,
			PpdContents: spec.ppdContents})
	if err != nil {
		s.Error("Failed to call p.CupsAddManuallyConfiguredPrinter: ", err)
	} else if result.Result != spec.expectedStatus {
		s.Errorf("%s (%s) failed: got %s; want %s",
			spec.name, spec.uri, result.Result, spec.expectedStatus)
	}
}

// ParsePrinterUris exercises printscanmgr's URI validation routine. ATOW this
// involves a sandboxed helper executable; this test helps guard against the
// possibility of seccomp filters going stale.
func ParsePrinterUris(ctx context.Context, s *testing.State) {
	ppd, err := ioutil.ReadFile(s.DataPath("GenericPostScript.ppd.gz"))
	if err != nil {
		s.Fatal("Failed to read PPD file: ", err)
	}

	p, err := printscanmgr.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to printscanmgr: ", err)
	}

	if err := printer.ResetCups(ctx); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	var spec printerSpec

	s.Log("printscanmgr must reject an empty URI")
	spec = printerSpec{
		name:           "EmptyUri",
		uri:            "",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must reject a URI containing unprintable ASCII")
	spec = printerSpec{
		name:           "BadCharacters",
		uri:            "ipps://hello-there\x7F:9001/general-kenobi",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must reject URIs with invalid percent-encodings")
	spec = printerSpec{
		name:           "BadPercents",
		uri:            "http://localhost:9001/your%zz%zzprinter",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must reject URIs with invalid percent-encodings")
	spec = printerSpec{
		name:           "TrailingPercent",
		uri:            "ipp://localhost:9001/your-printer%",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must accept URIs with valid percent-encodings")
	spec = printerSpec{
		name:           "GoodPercents",
		uri:            "ippusb://localhost:9001/%ffyour%20printer%19/",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must reject URIs with invalid port numbers")
	spec = printerSpec{
		name:           "BadPortNumber",
		uri:            "usb://localhost:abcd/hello-there",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must reject URIs with out-of-range port numbers")
	spec = printerSpec{
		name:           "OverlargePortNumber",
		uri:            "socket://localhost:65536/hello-there",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_CUPS_BAD_URI,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must accept URIs with valid port numbers")
	spec = printerSpec{
		name:           "ValidPortNumber",
		uri:            "lpd://localhost:65535/hello-there",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)

	s.Log("printscanmgr must accept IPv6 URIs not violating the above conditions")
	spec = printerSpec{
		name:           "IPv6TestA",
		uri:            "ipp://[2001:4860:4860::8888]:65535/hello%20there",
		ppdContents:    ppd,
		expectedStatus: ppb.AddPrinterResult_ADD_PRINTER_RESULT_SUCCESS,
	}
	addPrinterWithExpectedStatus(ctx, s, p, spec)
}
