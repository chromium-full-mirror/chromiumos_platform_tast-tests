// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// testFunc is a function that tests a specific download scenario.
type testFunc func(*downloadArguments, uiauto.Action) uiauto.Action

type downloadParams struct {
	testfunc testFunc
	files    []string
}

// downloadArguments holds resources to perform the holdingspace.Download tests.
type downloadArguments struct {
	cr     *chrome.Chrome
	tconn  *chrome.TestConn
	kb     *input.KeyboardEventWriter
	ui     *uiauto.Context
	outDir string
	files  []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Download,
		Desc:         "Verifies download behavior in holding space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-d1cdf1fd-1bf3-42ee-acef-5858f9ceb074",
		}},
		Params: []testing.Param{{
			Name: "cancel",
			Val: downloadParams{
				testfunc: testDownloadCancel,
				files:    []string{"download.html"},
			},
		}, {
			Name: "cancel_multiple",
			Val: downloadParams{
				testfunc: testDownloadCancel,
				files:    []string{"download1.html", "download2.html"},
			},
		}, {
			Name: "launch",
			Val: downloadParams{
				testfunc: testDownloadLaunch,
				files:    []string{"download.html"},
			},
		}, {
			Name: "launch_multiple",
			Val: downloadParams{
				testfunc: testDownloadLaunch,
				files:    []string{"download1.html", "download2.html"},
			},
		}, {
			Name: "pause_and_resume",
			Val: downloadParams{
				testfunc: testDownloadPauseAndResume,
				files:    []string{"download.html"},
			},
		}, {
			Name: "pause_and_resume_multiple",
			Val: downloadParams{
				testfunc: testDownloadPauseAndResume,
				files:    []string{"download1.html", "download2.html"},
			},
		}, {
			Name: "pin_and_unpin",
			Val: downloadParams{
				testfunc: testDownloadPinAndUnpin,
				files:    []string{"download.html"},
			},
		}, {
			Name: "pin_unpin_multiple",
			Val: downloadParams{
				testfunc: testDownloadPinAndUnpin,
				files:    []string{"download1.html", "download2.html"},
			},
		}, {
			Name: "remove",
			Val: downloadParams{
				testfunc: testDownloadRemove,
				files:    []string{"download.html"},
			},
		}, {
			Name: "remove_multiple",
			Val: downloadParams{
				testfunc: testDownloadRemove,
				files:    []string{"download1.html", "download2.html"},
			},
		}},
	})
}

var (
	menuOption         = holdingspace.FindContextMenuItem()
	cancelOption       = menuOption.Name("Cancel")
	pauseOption        = menuOption.Name("Pause")
	pinOption          = menuOption.Name("Pin")
	removeOption       = menuOption.Name("Remove")
	resumeOption       = menuOption.Name("Resume")
	showInFolderOption = menuOption.Name("Show in folder")
	unpinOption        = menuOption.Name("Unpin")
)

// Download verifies download behavior in holding space. It is expected that
// initiating a download will result in an item being added to holding space
// from which the user can cancel/pause/resume the download. Upon download
// completion, the user should be able to pin the download.
func Download(ctx context.Context, s *testing.State) {
	params := s.Param().(downloadParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Connect to a fresh ash-chrome instance (cr) to ensure holding space first-run state.
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	arg := &downloadArguments{
		cr:     cr,
		tconn:  tconn,
		kb:     kb,
		ui:     uiauto.New(tconn),
		outDir: s.OutDir(),
		files:  params.files,
	}

	// Ensure the tray does not exist prior adding anything to holding space.
	if err = arg.ui.EnsureGoneFor(holdingspace.FindTray(), 5*time.Second)(ctx); err != nil {
		s.Fatal("Tray exists: ", err)
	}

	// Cache the name and location of the download.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	downloadLocations := make([]string, 0, len(params.files))
	for _, fileName := range params.files {
		downloadLocations = append(downloadLocations, filepath.Join(downloadsPath, fileName))
	}
	defer func() {
		for _, location := range downloadLocations {
			if err := os.Remove(location); err != nil {
				s.Log("Failed to remove download: ", err)
			}
		}
	}()

	// Create a local server. If a request indicates `redirect=true`, the response
	// HTML will cause automatic redirection back to the root URL after a short
	// delay. Otherwise, the response will result in a download being started that
	// will block completion until the `unblockDownloadChannel` is signaled.
	unblockDownloadChannel := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Type", "text/html")
			downloadFileName := r.URL.Query().Get("file")
			if redirect := r.URL.Query().Get("redirect"); redirect == "true" {
				fmt.Fprintf(w, "<meta http-equiv='refresh' content='1; url=/?file=%s' />", downloadFileName)
				return
			}
			w.Header().Add("Content-Disposition", "attachment; filename="+downloadFileName)
			fmt.Fprintf(w, "Download started\n")
			f := w.(http.Flusher)
			f.Flush()
			<-unblockDownloadChannel
			fmt.Fprintf(w, "Download finished\n")
		}))
	defer server.Close()

	for _, file := range arg.files {
		// Connect to the local server. Note that this method will block until the
		// browser has finished navigating to the desired URL. Since we actually want
		// to start a download and not navigate the browser we'll use a redirect
		// workaround to satisfy the requirement to navigate.
		conn, err := cr.NewConn(ctx, server.URL+"?redirect=true&file="+file)
		if err != nil {
			s.Fatal("Failed to connect to local server: ", err)
		}
		defer conn.Close()
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := uiauto.Combine("open bubble and confirm initial state",
		// Left click the tray to open the bubble.
		arg.ui.LeftClick(holdingspace.FindTray()),

		// The pinned files section should contain a placeholder when empty.
		arg.ui.WaitUntilExists(holdingspace.FindPinnedFilesSectionPlaceholder()),
	)(ctx); err != nil {
		s.Fatal("Failed to open bubble and confirm initial state: ", err)
	}

	// Perform additional parameterized testing.
	if err := params.testfunc(arg, func(ctx context.Context) error {
		close(unblockDownloadChannel)
		return nil
	})(ctx); err != nil {
		s.Fatal("Fail to perform parameterized testing: ", err)
	}

	// Remove all files in `downloadLocations` which is backing the download. Note that
	// this will result in any associated holding space items being removed.
	for _, location := range downloadLocations {
		if err := os.Remove(location); err != nil && !os.IsNotExist(err) {
			s.Fatalf("Failed to remove file with path %q: %s", location, err)
		}
	}

	// Ensure all holding space chips associated with the underlying download are
	// removed when the backing file is removed.
	if err := waitUntilAllDownloadChipsGone(arg)(ctx); err != nil {
		s.Fatal("Chip exists: ", err)
	}
}

func testDownloadCancel(
	arg *downloadArguments, unblockDownload uiauto.Action) uiauto.Action {
	return uiauto.Combine("test cancel",
		// Select all download chips.
		selectAllDownloadChips(arg),

		// Right click the download chip to show the context menu. Note that the
		// download chip is currently bound to an in-progress download.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Left click the "Cancel" context menu item. Note that this will result in
		// the underlying download being cancelled and the context menu being
		// closed.
		arg.ui.LeftClick(cancelOption),

		// Unblock the download so that the local server can complete the download
		// request. This is necessary even though the download has been cancelled to
		// keep the local server from hanging.
		unblockDownload,

		// Ensure the download chip is removed with its backing file.
		waitUntilAllDownloadChipsGone(arg),
	)
}

func testDownloadLaunch(
	arg *downloadArguments, unblockDownload uiauto.Action) uiauto.Action {
	return uiauto.Combine("test launch file(s)",
		// Unblock the download so that the local server can complete the download
		// request. Until the download is unblocked, the local server will hang.
		unblockDownload,

		// Wait for the download to complete.
		waitUntilAllDownloadCompletedChipsExist(arg),

		// Select all download chips.
		selectAllDownloadChips(arg),

		// Launch file by keyboard event.
		arg.kb.AccelAction("enter"),
		waitUntilAllFilesLaunched(arg),
	)
}

func testDownloadPauseAndResume(
	arg *downloadArguments, unblockDownload uiauto.Action) uiauto.Action {
	return uiauto.Combine("test pause and resume",
		// Select all download chips.
		selectAllDownloadChips(arg),

		// Right click the download chip to show the context menu. Note that the
		// download chip is currently bound to an in-progress download.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Left click the "Pause" context menu item. Note that this will result in
		// the underlying download being paused and the context menu being closed.
		arg.ui.LeftClick(pauseOption),

		// Right click the download chip to show the context menu. Note that the
		// download chip is currently bound to a paused download.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Left click the "Resume" context menu item. Note that this will result in
		// the underlying download being resumed and the context menu being closed.
		arg.ui.LeftClick(resumeOption),

		// Unblock the download so that the local server can complete the download
		// request. Until the download is unblocked, the local server will hang.
		unblockDownload,

		// Wait for the download to complete.
		waitUntilAllDownloadCompletedChipsExist(arg),
	)
}

func testDownloadPinAndUnpin(
	arg *downloadArguments, unblockDownload uiauto.Action) uiauto.Action {
	// assertOptions asserts that the "Show in folder" option in context menu
	// doesn't show when multiple files are selected.
	assertOptions := arg.ui.EnsureGoneFor(showInFolderOption, 5*time.Second)
	if len(arg.files) == 1 {
		// The "Show in folder" option should appear when single file is selected however.
		assertOptions = arg.ui.WaitUntilExists(showInFolderOption)
	}

	return uiauto.Combine("test pin and unpin",
		// Unblock the download so that the local server can complete the download
		// request. Until the download is unblocked, the local server will hang.
		unblockDownload,

		// Wait for the download to complete.
		waitUntilAllDownloadCompletedChipsExist(arg),

		// Select all download chips.
		selectAllDownloadChips(arg),

		// Right click the download chip to show the context menu. Note that this
		// will wait until the underlying download has completed.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Left click the "Pin" context menu item. Note that this will result in
		// a pinned holding space item being created for the underlying download and
		// the context menu being closed.
		arg.ui.LeftClick(pinOption),

		// Ensure the pinned file chip is created.
		waitUntilAllPinnedFileChipsExist(arg),

		// Right click the download chip to show the context menu.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Verify that the context menu has the correct options.
		assertOptions,

		// Left click the "Unpin" context menu item. Note that this will result in
		// the pinned file chip being removed and the context menu being closed.
		arg.ui.LeftClick(unpinOption),

		// Ensure that the pinned file chip is removed.
		waitUntilAllPinnedFileChipsGone(arg),

		// Ensure that the download chip continues to exist despite the pinned
		// holding space item associated with the same download being destroyed.
		waitUntilAllDownloadCompletedChipsExist(arg),
	)
}

func testDownloadRemove(
	arg *downloadArguments, unblockDownload uiauto.Action) uiauto.Action {
	return uiauto.Combine("test remove",
		// Unblock the download so that the local server can complete the download
		// request. Until the download is unblocked, the local server will hang.
		unblockDownload,

		// Wait for the download to complete.
		waitUntilAllDownloadCompletedChipsExist(arg),

		// Select all download chips.
		selectAllDownloadChips(arg),

		// Right click the download chip to show the context menu. Note that this
		// will wait until the underlying download has completed.
		arg.ui.RightClick(holdingspace.FindDownloadChip().First()),

		// Left click the "Remove" context menu item. Note that this will result in
		// the holding space item for the underlying download being removed and the
		// context menu being closed.
		arg.ui.LeftClick(removeOption),

		// Ensure all download chips are removed.
		waitUntilAllDownloadChipsGone(arg),
	)
}

func forEachFile(arg *downloadArguments, f func(file string) uiauto.Action) uiauto.Action {
	return func(ctx context.Context) error {
		for _, file := range arg.files {
			if err := f(file)(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

func selectAllDownloadChips(arg *downloadArguments) uiauto.Action {
	return uiauto.Combine("select all download chips",
		arg.kb.AccelPressAction("shift"),
		forEachFile(arg, func(file string) uiauto.Action {
			return arg.ui.LeftClick(holdingspace.FindDownloadChip().NameContaining(file))
		}),
		arg.kb.AccelReleaseAction("shift"),
	)
}

func waitUntilAllDownloadChipsGone(arg *downloadArguments) uiauto.Action {
	return forEachFile(arg, func(file string) uiauto.Action {
		return arg.ui.WaitUntilGone(holdingspace.FindDownloadChip().NameContaining(file))
	})
}

func waitUntilAllDownloadCompletedChipsExist(arg *downloadArguments) uiauto.Action {
	return forEachFile(arg, func(file string) uiauto.Action {
		// NOTE: Name is only an exact match if the download is completed. Otherwise
		// it is prefaced to indicate in-progress state, e.g. "Downloading file...";
		return arg.ui.WaitUntilExists(holdingspace.FindDownloadChip().Name(file))
	})
}

func waitUntilAllPinnedFileChipsExist(arg *downloadArguments) uiauto.Action {
	return forEachFile(arg, func(file string) uiauto.Action {
		return arg.ui.WaitUntilExists(holdingspace.FindPinnedFileChip().Name(file))
	})
}

func waitUntilAllPinnedFileChipsGone(arg *downloadArguments) uiauto.Action {
	return forEachFile(arg, func(file string) uiauto.Action {
		return arg.ui.WaitUntilGone(holdingspace.FindPinnedFileChip().Name(file))
	})
}

func waitUntilAllFilesLaunched(arg *downloadArguments) uiauto.Action {
	return forEachFile(arg, func(file string) uiauto.Action {
		return func(ctx context.Context) error {
			relativePath := filepath.Join("Downloads/", file)
			relativePathMatcher := func(t *target.Info) bool {
				return strings.HasSuffix(t.URL, relativePath)
			}
			return testing.Poll(ctx, func(ctx context.Context) error {
				success, err := arg.cr.IsTargetAvailable(ctx, relativePathMatcher)
				if !success || err != nil {
					return errors.Wrapf(err, "failed to find target %q", relativePath)
				}
				return nil
			}, &testing.PollOptions{Interval: 10 * time.Millisecond})
		}
	})
}
