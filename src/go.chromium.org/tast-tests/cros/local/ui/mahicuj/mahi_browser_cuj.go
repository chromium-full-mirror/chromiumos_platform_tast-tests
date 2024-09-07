// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mahicuj provides Mahi related CUJ test.
package mahicuj

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/input"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/mahicuj/mahiutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// BrowserCUJRun opens webpage or pdf served by a local HTTP server and uses Mahi features (summary, QA) on it.
func BrowserCUJRun(ctx context.Context, cr *chrome.Chrome, proxyScriptPath, localZipFilePath, outDir string, urlCount int) (pv *perf.Values, retErr error) {
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		return nil, errors.Wrap(err, "failed to capture device snapshot")
	}

	// Setup proxy.
	mp, err := proxy.NewMitmProxy(ctx, proxy.ScriptPath(proxyScriptPath))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start proxy")
	}

	defer mp.Close(closeCtx)

	if err := mp.Connect(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "failed to configure chrome for proxy")
	}

	// Set up an about:blank page
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, chrome.BlankURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup Chrome")
	}
	defer closeBrowser(closeCtx)
	defer conn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to browser test API connection")
	}

	// Force install screen-ai dlc.
	if err := dlc.Install(ctx, "screen-ai", ""); err != nil {
		return nil, errors.Wrap(err, "failed to install screen-ai dlc")
	}

	// Ensure screen2x is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 10 * time.Second,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to verify screen-ai dlc")
	}

	ui := uiauto.New(tconn)
	window, _ := ash.GetActiveWindow(ctx, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(closeCtx)

	// Unzip the data file and setup test HTTP server
	localFilePath, localFiles, localServer, err := mahiutil.PrepareLocalServer(ctx, localZipFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare local server")
	}
	defer os.RemoveAll(localFilePath)
	defer localServer.Close()

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.NewPerformanceCUJOptions())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a CUJ recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to CUJ recorder")
	}

	if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
		return nil, errors.Wrap(err, "failed to clean mahi UI elements before the CUJ starts")
	}
	if err := mahiutil.MaybePassConsentFlow(
		ctx, conn, tconn, window, ui, kb, localServer.URL+"/"+localFiles[0].Name()); err != nil {
		return nil, errors.Wrap(err, "failed to pass the mahi consent flow")
	}

	runBrowserCUJ := func(ctx context.Context) (retErr error) {
		index := 0
		totalCount := len(localFiles)

		for index < urlCount {
			fileName := localFiles[index%totalCount].Name()
			index++

			if err := mahiutil.NavigateToURL(ctx, conn, localServer.URL+"/"+fileName); err != nil {
				return errors.Wrapf(err, "failed to open local file %s", fileName)
			}

			// Do a summary then send a question on the result panel.
			if err := mahiutil.RightClickAndMaybeShowMahiWidget(
				ctx, tconn, window, ui, true /*expectMahiWidget*/); err != nil {
				return errors.Wrap(err, "failed to do a right click")
			}

			if err := mahiutil.DoSummary(ctx, ui, true /*expectMockResponse*/); err != nil {
				return errors.Wrap(err, "failed to do a mahi summary")
			}

			if err := mahiutil.AskQuestionOnMahiPanel(ctx, ui, kb); err != nil {
				return errors.Wrap(err, "failed to ask a question")
			}

			if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
				return errors.Wrap(err, "failed to clean mahi UI elements after summary")
			}

			// Right click again and if normal mahi widget (instead of compact summary
			// button) shows up, send a question on the widget.
			if err := mahiutil.RightClickAndMaybeShowMahiWidget(
				ctx, tconn, window, ui, true /*expectMahiWidget*/); err != nil {
				return errors.Wrap(err, "failed to do a right click")
			}

			if err := ui.Exists(mahiutil.SummarizeButton)(ctx); err != nil {
				if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
					return errors.Wrap(err, "failed clean the compact widget")
				}
				continue
			}

			if err := mahiutil.AskQuestionOnMahiWidget(ctx, ui, kb); err != nil {
				return errors.Wrap(err, "failed to send a question on the widget")
			}

			if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
				return errors.Wrap(err, "failed to clean mahi UI elements after sending a question")
			}
		}

		return nil
	}

	if err := recorder.Run(ctx, runBrowserCUJ); err != nil {
		return nil, errors.Wrap(err, "failed to conduct the CUJ recorder task")
	}

	if err := recorder.Record(ctx, pv); err != nil {
		return nil, errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return nil, errors.Wrap(err, "failed to save histogram raw data")
	}
	if err := pv.Save(outDir); err != nil {
		return nil, errors.Wrap(err, "failed to store values")
	}
	return pv, nil
}
