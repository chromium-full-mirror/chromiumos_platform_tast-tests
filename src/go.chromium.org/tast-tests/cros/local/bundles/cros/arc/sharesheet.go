// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	arcCommon "go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Sharesheet,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Install ARC app and share to app via Sharesheet",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
		},
		BugComponent: "b:1203766",
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      7 * time.Minute,
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		VarDeps: []string{
			arcCommon.SharesheetPoolVarName,
		},
	})
}

func Sharesheet(ctx context.Context, s *testing.State) {
	const (
		appShareLabel        = "ARC Chrome Sharesheet Test"
		expectedFileName     = "test.txt"
		expectedFileContents = "test file contents"
		fileContentsID       = "org.chromium.arc.testapp.chromesharesheet:id/file_content"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup Chrome.
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(arcCommon.SharesheetPoolVarName)),
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}

	// Setup the test file.
	testFileLocation := filepath.Join(downloadsPath, expectedFileName)
	if err := ioutil.WriteFile(testFileLocation, []byte(expectedFileContents), 0644); err != nil {
		s.Fatalf("Failed to create file %q: %s", testFileLocation, err)
	}
	defer os.Remove(testFileLocation)

	// Setup Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Setup ARC device and UI Automator.
	arcDevice, uiAutomator, err := setUpARC(ctx, cr, s.OutDir())
	if err != nil {
		s.Fatal("Failed to setup ARC: ", err)
	}
	defer arcDevice.Close(cleanupCtx)
	defer uiAutomator.Close(cleanupCtx)

	if err := arcDevice.Install(ctx, arc.APKPath("ArcChromeSharesheetTest.apk")); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	// Open chrome://app-service-internals to and download the installed apps.
	// This is called for both succeeded cases and failed cases to compare.
	// The whole process should not fail the test case.
	// TODO (crbug/1416707): to remove this part once the bug crbug/1416707 is resolved.
	defer func() {
		if err := apps.DownloadAppServiceInternals(ctx, cr, s.OutDir()); err != nil {
			s.Log("Failed to download app service internal: ", err)
		}
	}()

	// Open the Files App.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}

	// The Sharesheet appears to not properly update the accessibility tree with
	// the coordinates whilst animating. The total time to animate is currently 150ms
	// so setting to 1s to ensure low-end devices are given enough time.
	crosui := uiauto.New(tconn).WithInterval(time.Second)
	sharesheetTargetButton := nodewith.Role(role.Button).NameContaining(appShareLabel).ClassName("SharesheetTargetButton")

	if err := uiauto.Combine("open Downloads and click sharesheet",
		files.OpenDownloads(),
		files.ClickContextMenuItem(expectedFileName, filesapp.Share),
		crosui.LeftClick(sharesheetTargetButton),
	)(ctx); err != nil {
		s.Log("It fails to find the test app, try to search it from launcher")
		kb, errkb := input.Keyboard(ctx)
		if errkb != nil {
			s.Log("Failed to initial keyboard: ", errkb)
		}
		if errApp := uiauto.Retry(3, uiauto.Combine("query",
			launcher.Open(tconn),
			launcher.Search(tconn, kb, appShareLabel),
			crosui.WaitUntilExists(launcher.CreateAppSearchFinder(ctx, tconn, appShareLabel))),
		)(ctx); errApp != nil {
			s.Log("Failed to find the test app: ", errApp)
		}
		s.Fatal("Failed to open downloads and click share button: ", err)
	}

	// Wait for the file contents to show in the Android test app.
	fileContentField := uiAutomator.Object(ui.ID(fileContentsID), ui.Text(expectedFileContents))
	if err := fileContentField.WaitForExists(ctx, 15*time.Second); err != nil {
		s.Fatalf("Failed to wait for file contents %q to appear in ARC window: %v", expectedFileContents, err)
	}
}

// setUpARC starts an ARC device and starts UI automator.
func setUpARC(ctx context.Context, cr *chrome.Chrome, outDir string) (*arc.ARC, *ui.Device, error) {
	// Setup ARC device.
	arcDevice, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start ARC")
	}

	// Start up UI automator.
	uiAutomator, err := arcDevice.NewUIDevice(ctx)
	if err != nil {
		if err := arcDevice.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close UI automator: ", err)
		}
		return nil, nil, errors.Wrap(err, "failed to initialize UI automator")
	}

	if err := arcDevice.WaitIntentHelper(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for intent helper")
	}

	return arcDevice, uiAutomator, nil
}
