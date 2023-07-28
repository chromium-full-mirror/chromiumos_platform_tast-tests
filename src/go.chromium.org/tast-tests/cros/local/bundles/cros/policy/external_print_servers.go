// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExternalPrintServers,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behavior of external print server configuration user policies",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"ust@google.com",
		},
		BugComponent: "b:1111614", // ChromeOS > Software > Commercial (Enterprise) > Printing
		SoftwareDeps: []string{"chrome", "cups"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Params: []testing.Param{
			{
				Val:     browser.TypeAsh,
				Fixture: fixture.ChromePolicyLoggedIn,
			},
			{
				Name:              "lacros",
				Val:               browser.TypeLacros,
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LacrosPolicyLoggedIn,
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ExternalPrintServers{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.ExternalPrintServersAllowlist{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test print server configuration user policies (COM_FOUND_CUJ7_TASK3_WF1).
				Value: "screenplay-87696fca-4b8c-410d-a5f7-b2b5f1391eb3",
			},
		},
	})
}

func startProcess(ctx context.Context, name string, args ...string) (*testexec.Cmd, error) {
	cmd := testexec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start command")
	}
	return cmd, nil
}

func stopProcess(ctx context.Context, cmd *testexec.Cmd) error {
	testing.ContextLogf(ctx, "Terminating command with PID %d", cmd.Cmd.Process.Pid)
	if err := cmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to send SIGTERM to command")
	}
	if err := cmd.Wait(); err != nil {
		return errors.Wrap(err, "failed to wait for command termination")
	}
	return nil
}

func startSocat(ctx context.Context) (*testexec.Cmd, error) {
	return startProcess(ctx, "socat", "TCP4-LISTEN:631,fork", "/run/cups/cups.sock")
}

func waitPort(ctx context.Context, port string, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		netcat := testexec.CommandContext(ctx, "nc", "-z", "localhost", port)
		err := netcat.Run()
		if _, ok := testexec.GetWaitStatus(err); !ok {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})
}

func startPrinter(ctx context.Context) (*testexec.Cmd, string, error) {
	cmd, err := startProcess(ctx, "ippeveprinter", "-p", "4444", "anyname")
	if err != nil {
		return cmd, "", errors.Wrap(err, "failed to start ippeveprinter")
	}

	// Wait until printer is listening on port.
	// It starts very quickly so 5 seconds is plenty of time to wait.
	if err := waitPort(ctx, "4444", time.Second*5); err != nil {
		return cmd, "", errors.Wrap(err, "timed out waiting for ippeveprinter to start")
	}

	return cmd, "ipp://localhost:4444/ipp/print", nil
}

func setupPrinterWithCups(ctx context.Context, printerName, printerURI string) error {
	const lpadminCmdLine = "/usr/sbin/lpadmin"
	const lpadminUser = "lpadmin"
	cmd := testexec.CommandContext(ctx, "sudo", "-u", lpadminUser, lpadminCmdLine, "-m",
		"everywhere", "-p", printerName, "-v", printerURI)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		output := ""
		if outBytes != nil {
			output = string(outBytes)
		}
		return errors.Wrapf(err, "lpadmin invocation failed with output: %s", output)
	}
	return nil
}

// ExternalPrintServers does not test the case when ExternalPrintServersAllowlist policy is empty
// because of a known bug (b/286878873).
// TODO(b/293995962): Add test case for the empty allowlist case.
func ExternalPrintServers(ctx context.Context, s *testing.State) {
	bt := s.Param().(browser.Type)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Internet connectivity needed because ExternalPrintServers is an external data policy.
	if err := ping.VerifyInternetConnectivity(ctx, 10*time.Second); err != nil {
		s.Fatal("Cannot fetch external data policy, no internet connectivity: ", err)
	}

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Run `socat` to listen on localhost:631 and relay traffic to the cupsd socket.
	// This effectively gives us a CUPS print server running on localhost:631.
	socat, err := startSocat(ctx)
	if err != nil {
		s.Fatal("Could not run 'socat' to map cupsd socket to a TCP port: ", err)
	}
	defer stopProcess(cleanupCtx, socat)

	// Run a fake printer.
	printerProcess, printerURI, err := startPrinter(ctx)
	if err != nil {
		s.Fatal("Could not start fake printer: ", err)
	}
	defer stopProcess(cleanupCtx, printerProcess)

	// Set up the fake printer with CUPS.
	// Note: This does not make the printer available to chrome because it's not set up in chrome.
	// It should become available when we set up a print server with ipp://localhost:631 URI.
	const fakePrinterName = "server-printer"
	if err := setupPrinterWithCups(ctx, fakePrinterName, printerURI); err != nil {
		s.Fatal("Could not set up the fake printer with CUPS: ", err)
	}

	// Perform cleanup.
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	// These two policies together configure a print server with a ipp://localhost:631 URL.
	policies := []policy.Policy{
		&policy.ExternalPrintServersAllowlist{Val: []string{"truly-random-and-unique-id"}},
		&policy.ExternalPrintServers{Val: &policy.ExternalPrintServersValue{
			Url:  "https://storage.googleapis.com/chromiumos-test-assets-public/enterprise/printservers.json",
			Hash: "3bbfcf22cd77cf6e48022b32a34e4b21c384a73c4e438cd7a37f28428a3e1fab",
		}},
	}

	// Update policies.
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Create a browser (either ash or lacros, based on browser type).
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	conn, err := br.NewConn(ctx, "chrome://version/")
	if err != nil {
		s.Fatal("Failed to connect to browser: ", err)
	}
	defer conn.Close()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
		printpreview.WaitForPrintPreview(tconn),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	if err := printpreview.SelectPrinter(ctx, tconn, fakePrinterName); err != nil {
		s.Fatal(fmt.Sprintf("Failed to select printer %s: ", fakePrinterName), err)
	}
}
