// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mahicuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
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

// GalleryCUJRun opens an example pdf in the gallery app and uses Mahi features (summary, QA) on it.
func GalleryCUJRun(ctx context.Context, cr *chrome.Chrome, proxyScriptPath, pdfFileName, outDir string) (pv *perf.Values, retErr error) {
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

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API connection")
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
	fa, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch files app")
	}

	if err := uiauto.Combine("open filesapp and launch the PDF file in gallery app",
		fa.OpenDownloads(),
		fa.WaitForFile(pdfFileName),
		fa.OpenFile(pdfFileName),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to open pdf file")
	}

	gallery, err := galleryapp.ConnectToApp(ctx, cr, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to gallery app")
	}
	defer gallery.Close(closeCtx)

	if err := uiauto.Combine("setup PDF file",
		gallery.DismissPDFDialog(),
		gallery.MaximizeWindow(),
		gallery.WaitPDFOpened(),
		gallery.WaitForGalleryQuiescence(cr))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to setup pdf file in gallery app")
	}

	window, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the active window")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(closeCtx)

	recorder, err := cujrecorder.NewRecorder(ctx, cr, tconn, nil, cujrecorder.NewPerformanceCUJOptions())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a CUJ recorder")
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to add common metrics to CUJ recorder")
	}

	runGalleryCUJ := func(ctx context.Context) (retErr error) {
		// Do a summary then send a question on the result panel.
		if err := mahiutil.RightClickAndMaybeShowMahiWidget(
			ctx, tconn, window, ui, true /*expectMahiWidget*/); err != nil {
			return errors.Wrap(err, "failed to do a right click")
		}

		if err := mahiutil.DoSummaryForGalleryPDFWithConsentUI(ctx, ui, true /*expectMockResponse*/); err != nil {
			return errors.Wrap(err, "failed to do a mahi summary")
		}

		if err := mahiutil.AskQuestionOnMahiPanel(ctx, ui, kb); err != nil {
			return errors.Wrap(err, "failed to ask a question")
		}

		if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
			return errors.Wrap(err, "failed to clean mahi UI elements after summary")
		}

		// Right click again and send a question on the widget.
		if err := mahiutil.RightClickAndMaybeShowMahiWidget(
			ctx, tconn, window, ui, true /*expectMahiWidget*/); err != nil {
			return errors.Wrap(err, "failed to do a right click")
		}

		if err := mahiutil.AskQuestionOnMahiWidget(ctx, ui, kb); err != nil {
			return errors.Wrap(err, "failed to send a question on the widget")
		}

		if err := mahiutil.CleanUIElement(ctx, ui, kb); err != nil {
			return errors.Wrap(err, "failed to clean mahi UI elements after sending a question")
		}

		return nil
	}

	if err := recorder.Run(ctx, runGalleryCUJ); err != nil {
		return nil, errors.Wrap(err, "failed to conduct the Mahi on Gallery CUJ recorder task")
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
