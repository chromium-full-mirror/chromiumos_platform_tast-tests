// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pdfocr provides constants that are used in tests interacting with the
// PDF OCR feature on ChromeOS.
package pdfocr

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/timing"
)

// Strings used in go.chromium.org/tast-tests/cros/local/bundles/cros/a11y/pdfocr*.go
const (
	// Status node message when PDF OCR finished converting image to text
	StatusReadyMessage = "Image converted to text"
	// Subpage url in the Settings
	SettingsSubPageURL = "textToSpeech"
	// Toggle menu name in the Settings
	SettingsToggleName = "Convert PDF images to text"
	// Testing PDF's filename
	TestPDFName = "inaccessible-text.pdf"
	// Inaccessible text embedded in an image in the testing PDF file
	TextInPDFImage = "Hello, world!"
)

// DlcFailureSetUpData contains necessary objects for PDF OCR tests with dlc
// failure and is returned by SetUpDlcFailure. TDown contains defer functions
// that clean up the testing environment. EnsureDlc contains a defer function
// that checks whether the dlcservice state is restored during cleanup.
type DlcFailureSetUpData struct {
	CTX        context.Context
	CleanupCTX context.Context
	TDown      *a11y.TearDownHelper
}

// SetUpDlcFailure executes common setup code that simulates the screen-ai DLC
// failure and returns a DlcFailureSetUpData. See the documentation for
// DlcFailureSetUpData for more information.
func SetUpDlcFailure(ctx context.Context) (data DlcFailureSetUpData, e error) {
	setupData := DlcFailureSetUpData{ctx, nil, &a11y.TearDownHelper{}}

	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		return setupData, errors.Wrapf(err, "failed to stop %q", dlc.JobName)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	setupData.TDown.Append(func() error {
		cancel()
		return nil
	})
	setupData.CTX = ctx
	setupData.CleanupCTX = cleanupCtx
	// Ensure the test restores the dlcservice state.
	setupData.TDown.Append(func() error {
		ctx, st := timing.Start(ctx, "cleanUp")
		defer st.End()
		return upstart.EnsureJobRunning(ctx, dlc.JobName)
	})

	// Force a DLC Install failure by moving the PRELOAD directory to another place.
	extDirBase, err := ioutil.TempDir("", "")
	if err != nil {
		return setupData, errors.Wrap(err, "failed to create a temp dir")
	}
	screenAiDlcID := "screen-ai"
	preloadPath := filepath.Join(dlc.PreloadDir, screenAiDlcID)
	tempDlcPath := filepath.Join(extDirBase, screenAiDlcID)

	err = fsutil.CopyDir(preloadPath, tempDlcPath)
	if err != nil {
		return setupData, errors.Wrap(err, "failed to move the screen-ai dlc to a temp directory")
	}
	os.RemoveAll(preloadPath)
	setupData.TDown.Append(func() error {
		return fsutil.CopyDir(tempDlcPath, preloadPath)
	})

	if err := upstart.StartJob(ctx, dlc.JobName); err != nil {
		return setupData, errors.Wrap(err, "failed to start dlcservice")
	}

	return setupData, nil
}
