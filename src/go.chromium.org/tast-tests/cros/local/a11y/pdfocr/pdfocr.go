// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pdfocr provides constants that are used in tests interacting with the
// PDF OCR feature on ChromeOS.
package pdfocr

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/upstart"
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
	// Menu entry name in the context menu
	ContextMenuName = "Convert image to text"
	// Testing PDF's filename
	TestPDFName = "inaccessible-text.pdf"
	// Inaccessible text embedded in an image in the testing PDF file
	TextInPDFImage = "Hello, world!"
	// Eight-page document PDF including inaccessible text
	MultiPagePDFName = "building_chromium.pdf"
	// JSON file that contains expected texts from `RealPDFName`
	MultiPagePDFExpectedTextJSONName = "building_chromium_expected.json"
)

// ContextMenuOption is a string value that indicates a context menu option for PDF OCR.
type ContextMenuOption string

// List of context menu options for PDF OCR.
const (
	ContextMenuAlways ContextMenuOption = "Always"
	ContextMenuOnce                     = "Just once"
)

// DlcFailureSetUpData contains necessary objects for PDF OCR tests with dlc
// failure and is returned by SetUpDlcFailure. TDown contains defer functions
// that clean up the testing environment. EnsureDlc contains a defer function
// that checks whether the dlcservice state is restored during cleanup.
type DlcFailureSetUpData struct {
	TempDlcPath string
	TDown       *a11y.TearDownHelper
}

// SetUpData contains necessary objects for PDF OCR tests and is returned by
// SetUp. TDown contains defer functions that clean up the testing environment.
type SetUpData struct {
	CR     *chrome.Chrome
	Server *httptest.Server
	TConn  *chrome.TestConn
	TDown  *a11y.TearDownHelper
}

// Pages represents a top dictionary object in a JSON file used in
// `PDFOCRMultiPage`.
type Pages struct {
	Pages []Page `json:"pages"`
}

// Page represents a dictionary object within the Pages object in a JSON file
// used in `PDFOCRMultiPage`.
type Page struct {
	Number   int      `json:"number"`
	Expected []string `json:"expected"`
}

// TestStep represents a test step of testing PDF OCR outputs using ChromeVox.
// It consists of a string representing a key command for ChromeVox and a
// tts.SpeechExpectation representing utterances from ChromeVox.
// used in `PDFOCRMultiPage`.
type TestStep struct {
	KeyCommands  []string
	Expectations []tts.SpeechExpectation
}

// SetUpDlcFailure executes common setup code that simulates the screen-ai DLC
// failure and returns a DlcFailureSetUpData. See the documentation for
// DlcFailureSetUpData for more information.
func SetUpDlcFailure(ctx context.Context) (DlcFailureSetUpData, error) {
	setupData := DlcFailureSetUpData{"", &a11y.TearDownHelper{}}

	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		return DlcFailureSetUpData{}, errors.Wrapf(err, "failed to stop %q", dlc.JobName)
	}

	// Ensure the test restores the dlcservice state.
	setupData.TDown.Append(func() error {
		ctx, st := timing.Start(ctx, "cleanUp")
		defer st.End()
		return upstart.EnsureJobRunning(ctx, dlc.JobName)
	})

	// Force a DLC Install failure by moving the PRELOAD directory to another place.
	extDirBase, err := ioutil.TempDir("", "")
	if err != nil {
		return DlcFailureSetUpData{}, errors.Wrap(err, "failed to create a temp dir")
	}
	screenAiDlcID := "screen-ai"
	preloadPath := filepath.Join(dlc.PreloadDir, screenAiDlcID)
	tempDlcPath := filepath.Join(extDirBase, screenAiDlcID)

	err = fsutil.CopyDir(preloadPath, tempDlcPath)
	if err != nil {
		return DlcFailureSetUpData{}, errors.Wrap(err, "failed to move the screen-ai dlc to a temp directory")
	}
	os.RemoveAll(preloadPath)
	setupData.TempDlcPath = tempDlcPath
	setupData.TDown.Append(func() error {
		return fsutil.CopyDir(tempDlcPath, preloadPath)
	})

	if err := upstart.StartJob(ctx, dlc.JobName); err != nil {
		return DlcFailureSetUpData{}, errors.Wrap(err, "failed to start dlcservice")
	}

	return setupData, nil
}

// RefreshDlc refreshes DLC by moving the screen-ai DLC to the correct directory.
func RefreshDlc(ctx context.Context, tempDlcPath string) error {
	if err := upstart.StopJob(ctx, dlc.JobName); err != nil {
		return errors.Wrapf(err, "failed to stop %q", dlc.JobName)
	}

	// Move the screen-ai dlc from the temp directory to the PRELOAD directory.
	screenAiDlcID := "screen-ai"
	preloadPath := filepath.Join(dlc.PreloadDir, screenAiDlcID)

	if _, err := os.Stat(tempDlcPath); os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to find %s", tempDlcPath)
	}

	if err := fsutil.CopyDir(tempDlcPath, preloadPath); err != nil {
		return errors.Wrap(err, "failed to move the screen-ai dlc to a temp directory")
	}

	if err := upstart.StartJob(ctx, dlc.JobName); err != nil {
		return errors.Wrap(err, "failed to start dlcservice")
	}

	return nil
}

// SetUp executes common setup code and returns a SetUpData. See the documentation
// for SetUpData for more information.
func SetUp(ctx, cleanupCtx context.Context, dataFS http.FileSystem, bt browser.Type) (SetUpData, error) {
	setupData := SetUpData{nil, nil, nil, &a11y.TearDownHelper{}}

	// Setup test HTTP server.
	server := httptest.NewServer(http.FileServer(dataFS))
	setupData.Server = server
	setupData.TDown.Append(func() error {
		server.Close()
		return nil
	})

	// Launch browser with the PDF OCR feature flag for both Ash and Lacros Chrome.
	opts := []chrome.Option{chrome.EnableFeatures("PdfOcr")}
	lacrosConfig := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("PdfOcr")))
	cr, err := browserfixt.NewChrome(ctx, bt, lacrosConfig, opts...)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to start Chrome")
	}
	setupData.CR = cr
	setupData.TDown.Append(func() error {
		cr.Close(cleanupCtx)
		return nil
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to create Test API connection")
	}
	setupData.TConn = tconn

	// TODO(b:291094454): Add a variant of `chromevox.SetUp()` that takes a url as parameter.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		return SetUpData{}, errors.Wrap(err, "failed to enable ChromeVox")
	}
	setupData.TDown.Append(func() error {
		a11y.ClearFeature(cleanupCtx, tconn, a11y.SpokenFeedback)
		return nil
	})

	return setupData, nil
}

// TurnOnFromContextMenu turns on PDF OCR from the Context menu in PDF Viewer.
// `menuOption` must to be either `ContextMenuAlways` or `ContextMenuOnce`.
func TurnOnFromContextMenu(ctx context.Context, ui *uiauto.Context, pdfRoot *nodewith.Finder, menuOption ContextMenuOption) error {
	pdfOCRMenuEntry := nodewith.Name(ContextMenuName).Role(role.MenuItem)
	pdfOCROption := nodewith.Name(string(menuOption)).Role(role.MenuItem)
	if err := uiauto.Combine("Turn on PDF OCR from the Context Menu",
		ui.WithTimeout(5*time.Second).RightClick(pdfRoot),
		ui.WithTimeout(5*time.Second).LeftClick(pdfOCRMenuEntry),
		ui.WithTimeout(5*time.Second).LeftClick(pdfOCROption),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to turn on PDF OCR %s from the Context Menu: ", menuOption)
	}

	return nil
}

// ExpectDownloadFailureUtterance waits to receive the download failure utterance.
func ExpectDownloadFailureUtterance(ctx context.Context, sm *tts.SpeechMonitor) error {
	if err := sm.Consume(ctx, []tts.SpeechExpectation{
		tts.NewStringExpectation("Downloading text recognition files"),
		tts.NewRegexExpectation("Can't download text recognition files*"),
	}); err != nil {
		errors.Wrap(err, "failed to check the ChromeVox announcement for PDF OCR dlc failure")
	}

	return nil
}
