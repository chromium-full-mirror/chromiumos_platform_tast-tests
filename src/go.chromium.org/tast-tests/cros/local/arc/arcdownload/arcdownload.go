// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcdownload provides set of util functions used to download files
// through ARC apps.
package arcdownload

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DownloadTestFile uses a fake Android app to download a file through ARC. The
// given `sourcePath` in ChromeOS will be downloaded to `targetPath` in Android,
// using a temporary local HTTP server.
func DownloadTestFile(ctx context.Context, cr *chrome.Chrome, a *arc.ARC,
	d *ui.Device, sourcePath, targetPath string) error {
	const localServerPort = 8080

	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Create and start a local HTTP server that serves the test file data.
	mux := createServeMux(sourcePath)

	server := &http.Server{Addr: fmt.Sprintf(":%d", localServerPort),
		Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			testing.ContextLog(ctx, "Failed to start a local server: ", err)
		}
	}()
	defer server.Shutdown(cleanupCtx)

	if err := waitForServerStart(ctx, localServerPort); err != nil {
		return errors.Wrap(err, "failed to wait for the server to start")
	}

	// In ARCVM, Downloads integration depends on MyFiles mount.
	if err := arc.WaitForARCMyFilesVolumeMountIfARCVMEnabled(ctx, a); err != nil {
		return errors.Wrap(err, "failed to wait for MyFiles to be mounted in ARC")
	}

	// Download the test file with an Android app from the local server.
	if err := downloadFileWithApp(
		ctx, cr, a, d, localServerPort, sourcePath, targetPath); err != nil {
		return errors.Wrap(err, "failed to download the test file with Android app")
	}
	return nil
}

// createServeMux returns an HTTP request multiplexer that serves the file in a
// given path. It also responds to a ping with the status code OK, which can be
// used to check whether a server is properly started.
func createServeMux(path string) *http.ServeMux {
	mux := http.NewServeMux()

	// Register a handler that responds to a ping with OK.
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusBadRequest)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Register a handler that serves the test file data.
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusBadRequest)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(binary.Size(content)))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(content); err != nil {
			return
		}
	})

	return mux
}

// waitForServerStart waits for a local HTTP server to start. It assumes that
// the server responds to a ping with the handler registered by createServeMux.
func waitForServerStart(ctx context.Context, localServerPort int) error {
	pingURL := fmt.Sprintf("http://localhost:%d/ping", localServerPort)
	return testing.Poll(ctx, func(ctx context.Context) error {
		r, err := http.Get(pingURL)
		if err != nil {
			return err
		}
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return errors.Errorf("received a non-OK status code: %d", r.StatusCode)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// downloadFileWithApp downloads a file from sourcePath in ChromeOS to
// targetPath in Android with an Android app via a local HTTP server.
// It first sets up reverse port forwarding to connect the local server to an
// Android port, triggers the download from the connected Android port, and then
// returns when the download is completed.
func downloadFileWithApp(ctx context.Context, cr *chrome.Chrome, a *arc.ARC,
	d *androidui.Device, localServerPort int, sourcePath,
	targetPath string) error {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Build the test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	// Set up an Android port to which the host port is forwarded.
	androidPort, cleanupFunc, err := setUpAndroidPort(ctx, a, localServerPort)
	if err != nil {
		return errors.Wrap(err, "failed to set up Android port")
	}
	defer cleanupFunc(cleanupCtx)

	// Start downloading the test file.
	cleanupFunc, err = startDownloadFileWithApp(ctx, a, tconn, androidPort,
		sourcePath, targetPath)
	if err != nil {
		return errors.Wrap(err, "failed to start downloading the test file")
	}
	defer cleanupFunc(cleanupCtx)

	return waitForDownloadComplete(ctx, d)
}

// setUpAndroidPort sets up an Android port to which a specified host port is
// forwarded using reverse port forwarding.
func setUpAndroidPort(ctx context.Context, a *arc.ARC, localServerPort int) (
	int, func(context.Context), error) {
	androidPort, err := a.ReverseTCP(ctx, localServerPort)
	if err != nil {
		return -1, nil, errors.Wrap(err, "failed to start reverse port forwarding")
	}

	cleanupFunc := func(ctx context.Context) {
		if err := a.RemoveReverseTCP(ctx, androidPort); err != nil {
			testing.ContextLog(ctx, "Failed to stop reverse port forwarding: ", err)
		}
	}

	return androidPort, cleanupFunc, nil
}

// startDownloadFileWithApp starts to download a file from sourcePath in Chrome
// OS to targetPath in Android with an Android app via a specified Android port.
// It first installs the app, and then starts its MainActivity with an explicit
// intent to trigger the download.
func startDownloadFileWithApp(ctx context.Context, a *arc.ARC,
	tconn *chrome.TestConn, androidPort int, sourcePath,
	targetPath string) (func(context.Context), error) {
	const (
		apkName       = "ArcDownloadManagerTest.apk"
		packageName   = "org.chromium.arc.testapp.downloadmanager"
		sourceURLKey  = "source_url"
		targetPathKey = "target_path"
	)

	// Install the test app.
	if err :=
		a.Install(ctx, arc.APKPath(apkName), adb.InstallOptionGrantPermissions); err != nil {
		return nil, errors.Wrapf(err, "failed to install %s", apkName)
	}

	// Create the MainActivity of the test app.
	act, err := arc.NewActivity(a, packageName, ".MainActivity")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create the main activity for %s",
			packageName)
	}

	// Start the MainActivity with an intent. When started successfully,
	// the test app automatically starts downloading the test file.
	sourceURL := fmt.Sprintf("http://localhost:%d%s", androidPort, sourcePath)

	if err := act.Start(ctx, tconn,
		arc.WithWaitForLaunch(),
		arc.WithForceStop(),
		arc.WithExtraString(sourceURLKey, sourceURL),
		arc.WithExtraString(targetPathKey, targetPath),
	); err != nil {
		act.Close(ctx)
		return nil, errors.Wrapf(err, "failed to start the main activity for %s",
			packageName)
	}

	cleanupFunc := func(ctx context.Context) {
		if err := act.Stop(ctx, tconn); err != nil {
			testing.ContextLogf(ctx, "Failed to stop the main activity for %s: %v",
				packageName, err)
		}
		act.Close(ctx)
	}

	return cleanupFunc, nil
}

// waitForDownloadComplete waits for a download session triggered by
// downloadFileWithApp to be completed. It checks the app's status field and
// returns when the "Finished" message appears on the field.
func waitForDownloadComplete(ctx context.Context, d *androidui.Device) error {
	const (
		appStatusFieldID  = "org.chromium.arc.testapp.downloadmanager:id/status"
		appStatusFinished = "Finished"
	)

	obj := d.Object(androidui.ID(appStatusFieldID))
	if err := obj.WaitForExists(ctx, 10*time.Second); err != nil {
		return errors.Wrapf(err, "failed to find the label id %s", appStatusFieldID)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		t, err := obj.GetText(ctx)
		if err != nil {
			return err
		}
		if t != appStatusFinished {
			return errors.Errorf("app status: %s", t)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for the app to finish downloading")
	}

	return nil
}
