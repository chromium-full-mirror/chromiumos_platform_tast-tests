// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/printer/uitools"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/printpreview"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/printing/printer"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintFinishingFeatures,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that finishing features are available in the UI and are correctly reeceived by a printer",
		Contacts:     []string{"chromeos-commercial-printing@google.com", "project-bolton@google.com", "nedol@google.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:paper-io",
			"paper-io_printing",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"chrome", "cros_internal", "cups"},
		Data:         []string{"ipp_conf_finishings.txt", "get-jobs-finishings-info.test"},
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: "chromeLoggedIn",
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           "lacros",
			},
		},
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Set up a printer with advanced features (COM_FOUND_CUJ17_TASK1_WF1).
				Value: "screenplay-487ce3a6-0cb5-4a33-9951-1a8780162b6b",
			},
			{
				Key: "feature_id",
				// Initiate a print job from a printer with advanced features (COM_FOUND_CUJ17_TASK2_WF1).
				Value: "screenplay-8ca42bd6-e9d2-4ce4-a493-1e2c57b9aaca",
			},
		},
	})
}

func startIPPEverywherePrinter(ctx context.Context, name string, arg ...string) (*testexec.Cmd, int, error) {
	ippeveprinter := testexec.CommandContext(ctx, name, arg...)
	stderr, err := ippeveprinter.StderrPipe()
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to fetch stderr")
	}

	if err := ippeveprinter.Start(); err != nil {
		return nil, 0, errors.Wrap(err, "failed to start ippeveprinter")
	}

	var printerPort int
	r := regexp.MustCompile(`Listening on port (\d+)\.`)
	rd := bufio.NewReader(stderr)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return nil, 0, errors.Wrap(err, "failed to read output from pipe")
		}
		matches := r.FindStringSubmatch(line)
		if matches != nil {
			if printerPort, err = strconv.Atoi(matches[1]); err != nil {
				return nil, 0, errors.Wrap(err, "failed to parse printer port")
			}
			break
		}
	}

	return ippeveprinter, printerPort, nil
}

func stopIPPEverywherePrinter(ctx context.Context, ippeveprinter *testexec.Cmd) error {
	testing.ContextLogf(ctx, "Terminating ippeveprinter with PID %d", ippeveprinter.Cmd.Process.Pid)
	if err := ippeveprinter.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to send SIGTERM to ippeveprinter")
	}
	if err := ippeveprinter.Wait(); err != nil {
		// We're expecting the exit status to be non-zero if the process was killed by SIGTERM.
		// Anything else indicates a problem.
		if ws, ok := testexec.GetWaitStatus(err); !ok || !ws.Signaled() || ws.Signal() != unix.SIGTERM {
			return errors.Wrap(err, "failed to wait for ippeveprinter termination")
		}
	}
	return nil
}

func PrintFinishingFeatures(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	bt := s.Param().(browser.Type)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	s.Log("Installing printer")
	if err := printer.ResetCups(ctx); err != nil {
		s.Fatal("Failed to reset cupsd: ", err)
	}

	// `ipp_conf_finishings.txt` is a modified ipp attributes file fetched from
	// ippeveprinter with default parameters so that finishing printing features are supported
	ippeveprinter, printerPort, err := startIPPEverywherePrinter(ctx, "stdbuf", "-o0", "ippeveprinter", "ippeveprinter_test", "-a", s.DataPath("ipp_conf_finishings.txt"))
	if err != nil {
		s.Fatal("Failed to start IPPEverywherePrinter: ", err)
	}
	defer func(ctx context.Context) {
		if err := stopIPPEverywherePrinter(ctx, ippeveprinter); err != nil {
			s.Fatal("Failed to stop IPPEverywherePrinter: ", err)
		}
	}(ctx)

	// Open OS Settings and navigate to the Printing page.
	ui := uiauto.New(tconn)
	entryFinder := uitools.PrintersFinder.Ancestor(ossettings.WindowFinder)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, uitools.SettingsPageName, ui.Exists(entryFinder)); err != nil {
		s.Fatal("Failed to launch Settings page: ", err)
	}

	const printerName = "IPP Everywhere Printer"
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close()
	// Hide all notifications to prevent them from covering the printer entry.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close all notifications: ", err)
	}

	if err := uiauto.Combine("open the add printer window",
		ui.LeftClick(entryFinder),
		ui.LeftClick(nodewith.ClassName("action-button").Name("Add printer")),
	)(ctx); err != nil {
		s.Fatal("Failed to open Add Printer window: ", err)
	}

	nameFinder := nodewith.Role("textField").Name("Name")
	addressFinder := nodewith.Role("textField").Name("Address")
	if err := uiauto.Combine("set printer details",
		ui.LeftClick(nameFinder),
		ui.EnsureFocused(nameFinder),
		kb.TypeAction(printerName),
		ui.LeftClick(addressFinder),
		ui.EnsureFocused(addressFinder),
		kb.TypeAction(fmt.Sprintf("localhost:%d", printerPort)),
		ui.LeftClick(nodewith.Role("button").Name("Add")),
	)(ctx); err != nil {
		s.Fatal("Failed to set printer details: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	conn, err := br.NewConn(ctx, "chrome://version/")
	if err != nil {
		s.Fatal("Failed to connect to browser: ", err)
	}
	defer conn.Close()

	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	// Select printer and click Print button.
	s.Log("Selecting printer")
	if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}

	if err := printpreview.WaitForPrintPreview(tconn)(ctx); err != nil {
		s.Fatal("Failed to wait for Print Preview: ", err)
	}

	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to expand more settings: ", err)
	}

	if err := printpreview.OpenAdvancedSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open Advanced Settings: ", err)
	}

	for _, advancedSetting := range []string{"Double-gate fold", "Dual punch left", "Staple top right"} {
		if err := printpreview.SetAdvancedSetting(ctx, tconn, advancedSetting, true); err != nil {
			s.Fatal("Failed to set the value of checkbox ", advancedSetting, ": ", err)
		}
	}

	if err := printpreview.CloseAdvancedSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to close Advanced Settings: ", err)
	}

	if err := printpreview.Print(ctx, tconn); err != nil {
		s.Fatal("Failed to print: ", err)
	}

	cmd := testexec.CommandContext(ctx, "ipptool", "-tv", fmt.Sprintf("ipp://localhost:%d/ipp/print", printerPort), s.DataPath("get-jobs-finishings-info.test"))
	stdout, _, err := cmd.SeparatedOutput()
	// ippeveprinter cleans up print jobs after 60 seconds, so we should be able to see information about the job sent in this test
	if !strings.Contains(string(stdout), "finishings (1setOf enum) = fold-double-gate,punch-dual-left,staple-top-right") {
		s.Fatal("Job description message doesn't contain info about finishing options: ", string(stdout))
	}
}
