// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CanaryCloser is a function which closes opened canaries.
type CanaryCloser func(ctx context.Context)

// IsCanaryStillAlive is a function that checks if the canary is still alive. If
// there are any parts of a canary alive, it returns true.
type IsCanaryStillAlive func(ctx context.Context) bool

func openTabCanaries(ctx context.Context, allocMiB int, ratio float32, br *browser.Browser, fs http.FileSystem) (CanaryCloser, IsCanaryStillAlive, error) {
	var bgTab *MemoryStressUnit
	var protTab *MemoryStressUnit
	closer := func(ctx context.Context) {
		if bgTab != nil {
			if err := bgTab.Close(ctx, br); err != nil {
				testing.ContextLog(ctx, "Failure when closing background tab canary: ", err)
			}
		}
		if protTab != nil {
			if err := protTab.Close(ctx, br); err != nil {
				testing.ContextLog(ctx, "Failure when closing protected background tab canary: ", err)
			}
		}
	}
	failed := true
	defer func() {
		if failed {
			closer(ctx)
		}
	}()
	server := NewMemoryStressServer(fs)
	bgTab = server.NewMemoryStressUnit(allocMiB, ratio, 2*time.Second)
	if err := bgTab.Run(ctx, br); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run background tab canary")
	}
	protTab = server.NewMemoryStressUnit(allocMiB, ratio, 2*time.Second)
	if err := protTab.Run(ctx, br); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run protected background tab canary")
	}
	failed = false
	stillAlive := func(ctx context.Context) bool {
		return protTab.StillAlive(ctx, br) || bgTab.StillAlive(ctx, br)
	}
	return closer, stillAlive, nil
}

func openAppCanaries(ctx context.Context, allocMiB int, ratio float32, tconn *chrome.TestConn, a *arc.ARC) (CanaryCloser, IsCanaryStillAlive, error) {
	var cacheApp *ArcLifecycleUnit
	var previousApp *ArcLifecycleUnit
	var percApp *ArcLifecycleUnit
	var fgApp *ArcLifecycleUnit
	closer := func(ctx context.Context) {
		if cacheApp != nil {
			cacheApp.Close(ctx, a)
		}
		if previousApp != nil {
			previousApp.Close(ctx, a)
		}
		if percApp != nil {
			percApp.Close(ctx, a)
		}
		if fgApp != nil {
			fgApp.Close(ctx, a)
		}
	}
	failed := true
	defer func() {
		if failed {
			closer(ctx)
		}
	}()

	if err := InstallArcLifecycleTestApps(ctx, a, 4); err != nil {
		return nil, nil, errors.Wrap(err, "failed to install the test apps")
	}

	cacheApp = NewArcLifecycleUnit(0, int64(allocMiB), float64(ratio), nil, true)
	if err := cacheApp.Run(ctx, a, tconn); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run cached app canary")
	}

	// The most recently minimized App will have a priority of PREVIOUS_APP_ADJ
	// so we need to launch another app after our cached app for it to actually
	// be >= CACHED_APP_MIN_ADJ.
	previousApp = NewArcLifecycleUnit(1, 0, 1.0, nil, true)
	if err := previousApp.Run(ctx, a, tconn); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run previous app")
	}

	percApp = NewArcLifecycleUnit(2, int64(allocMiB), float64(ratio), nil, false)
	if err := percApp.Run(ctx, a, tconn); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run perceptible app canary")
	}

	fgApp = NewArcLifecycleUnit(3, int64(allocMiB), float64(ratio), nil, false)
	if err := fgApp.Run(ctx, a, tconn); err != nil {
		return nil, nil, errors.Wrap(err, "failed to run foreground app canary")
	}
	failed = false
	stillAlive := func(ctx context.Context) bool {
		return fgApp.StillAlive(ctx, a) || percApp.StillAlive(ctx, a) || cacheApp.StillAlive(ctx, a)
	}
	return closer, stillAlive, nil
}

// OpenAppTabCanaries opens app and tab canaries at various priorities.
// ctx       - The context the test is running on.
// allocMib  - The amount of memory allocated to a canary.
// ratio     - How compressible the allocated memory will be.
// br        - Browser to open the tab on.
// fs        - FileSystem to initialize the memory stress server.
// tconn     - Test connection to Chrome.
// a         - ARC test object.
func OpenAppTabCanaries(ctx context.Context, allocMiB int, ratio float32, br *browser.Browser, fs http.FileSystem, tconn *chrome.TestConn, a *arc.ARC) (CanaryCloser, IsCanaryStillAlive, error) {
	var tabCloser CanaryCloser
	var appCloser CanaryCloser
	closer := func(ctx context.Context) {
		if tabCloser != nil {
			tabCloser(ctx)
		}
		if appCloser != nil {
			appCloser(ctx)
		}
	}
	failed := true
	defer func() {
		if failed {
			closer(ctx)
		}
	}()

	var err error
	var tabStillAlive, appStillAlive IsCanaryStillAlive
	tabCloser, tabStillAlive, err = openTabCanaries(ctx, allocMiB, ratio, br, fs)
	if err != nil {
		return nil, nil, err
	}
	appCloser, appStillAlive, err = openAppCanaries(ctx, allocMiB, ratio, tconn, a)
	if err != nil {
		return nil, nil, err
	}

	failed = false
	stillALive := func(ctx context.Context) bool {
		return tabStillAlive(ctx) || appStillAlive(ctx)
	}
	return closer, stillALive, nil
}
