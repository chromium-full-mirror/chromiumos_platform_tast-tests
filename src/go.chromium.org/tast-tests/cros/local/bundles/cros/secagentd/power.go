// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package secagentd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/upstart"
)

type enableXDR struct {
	enable bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Power,
		Desc:         "Navigate to different web pages and print them to pdf",
		BugComponent: "b:1208373",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      setup.PowerAsh,
		Timeout:      15*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{{
			Name:      "baseline",
			Val:       enableXDR{enable: false},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
		}, {
			Name:      "xdr_enabled",
			Val:       enableXDR{enable: true},
			ExtraAttr: []string{"group:crosbolt", "crosbolt_nightly"},
		}},
	},
	)
}

func Power(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		secagentdupstart.RestartSecagentd(ctx, false)
		cancel()
	}(cleanupCtx)

	enableXDR := s.Param().(enableXDR).enable
	const batchIntervalS = 60
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	if enableXDR {
		s.Log("Restarting secagentd")
		agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
			upstart.WithArg("SECAGENTD_LOG_LEVEL", "-1"),
			upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
			upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
			upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))
		if err != nil {
			s.Fatal("Failed to restart secagentd: ", err)
		}
		s.Log("Waiting for secagentd to install BPFs")
		if err := secagentdprocfsscraper.WaitForBpfMaps(ctx, agentPid); err != nil {
			s.Fatal("Failed to verify secagentd is ready to test: ", err)
		}
	}
	fv := s.FixtValue().(setup.PowerUIFixtureData)
	discharge := fv.Discharge
	bt := fv.Bt
	cr := fv.Cr // Chrome

	powerMeasureInterval := 5 * time.Second
	totalTestTime := 8 * time.Minute

	const blankPageContents = `data:text/html,
		<html>
			<body>
				<style media='(prefers-color-scheme: dark)'>
					body { background: black;}
				</style>
				<h1> Hello world </h1>
				<p> This is a page meant to be printed to PDF. </p>
			</body>
		</html>`

	// Render an entirely white or black blank page in the browser in accordance with the OS theme.
	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, bt, blankPageContents)
	if err != nil {
		s.Fatal("Failed to open a blank new tab: ", err)
	}
	defer cleanup(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get ash tconn: ", err)
	}
	w, err := ash.WaitForAnyWindow(ctx, tconn, ash.BrowserTypeMatch())
	if err != nil {
		s.Fatal("Failed to open a browser window: ", err)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize the browser window: ", err)
	}

	r := power.NewRecorder(ctx, powerMeasureInterval, s.OutDir(), s.TestName(), power.DischargeWatchdogOption(discharge))
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Log("Cooldown failed, proceeding with test anyways")
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}
	// Wait 10 seconds between saving PDFs.
	if err := savePdf(ctx, cr, totalTestTime, 10*time.Second); err != nil {
		s.Fatal("Failure while saving PDF: ", err)
	}
	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
	if err := power.SaveScreenshot(ctx, cr); err != nil {
		s.Error("Failed to take screenshot: ", err)
	}
}

func savePdf(ctx context.Context, cr *chrome.Chrome, timeout, interval time.Duration) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "Unable to establish Chrome Test API connection")
	}

	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "Unable to acquire keyboard")

	}
	// print a pdf
	var fileName string
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}
	startTime := time.Now()
	var filesToDelete []string
	iteration := 0
	defer func() {
		for _, file := range filesToDelete {
			os.Remove(file)
		}
	}()

	for {
		if err := uiauto.Combine("Save file as PDF",
			// Open print preview using the Ctrl+P shortcut.
			kb.AccelAction("Ctrl+P"),
			printpreview.WaitForPrintPreview(tconn),

			// Select "Save as PDF" as the printer option.
			func(ctx context.Context) error {
				return printpreview.SelectPrinter(ctx, tconn, "Save as PDF")
			},
			printpreview.WaitForPrintPreview(tconn),

			// Click the "Save" button.
			ui.LeftClick(nodewith.Name("Save").Role(role.Button)),

			// Download file window will popup.
			// Wait for the input field to load with a default file name.
			ui.RetrySilently(8, func(ctx context.Context) error {
				nodeInfo, err := ui.Info(ctx, nodewith.Name("File name").Role(role.TextField))
				if err != nil {
					return err
				}
				if !(filepath.Ext(nodeInfo.Value) == ".pdf") {
					return errors.Errorf("file name %q should contain .pdf", nodeInfo.Value)
				}
				fileName = nodeInfo.Value
				return nil
			}),

			ui.LeftClick(nodewith.Name("Save").
				Role(role.Button).
				Ancestor(nodewith.Name("Save file as").Role(role.Window))),
			func(ctx context.Context) error {
				if fileName != "" {
					newLocation := filepath.Join(downloadsPath, "file_"+strconv.Itoa(iteration))

					downloadLocation := filepath.Join(downloadsPath, fileName)
					os.Rename(downloadLocation, newLocation)
					filesToDelete = append(filesToDelete, newLocation)
				}
				iteration++
				return nil
			},
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to save and verify PDF")
		}
		// GoBigSleepLint: sleep for a bit for a more realistic test.
		if err := testing.Sleep(ctx, interval); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		if time.Since(startTime) >= timeout {
			return nil
		}
	}
}
