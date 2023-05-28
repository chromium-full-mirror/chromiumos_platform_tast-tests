// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// OpenAppAndGetStartTime launches a new activity, starts it and records start time.
// The caller must close the returned activity when the test is done.
func OpenAppAndGetStartTime(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC,
	pkgName, appName, startActivity string) (time.Duration, *arc.Activity, error) {
	act, err := arc.NewActivity(a, pkgName, startActivity)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "failed to create new activity for %s", startActivity)
	}
	startTime := time.Now()
	// Start() will invoke "am start" and waits for the app to be visible on the Chrome side.
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return 0, nil, errors.Wrapf(err, "failed to start %s", appName)
	}

	return time.Since(startTime), act, nil
}

// WaitForExists returns an action function which waits for a view matching the selector to appear.
func WaitForExists(obj *ui.Object, timeout time.Duration) action.Action {
	return func(ctx context.Context) error {
		if err := obj.WaitForExists(ctx, timeout); err != nil {
			return errors.Wrap(err, "failed to wait for the target object")
		}
		return nil
	}
}

// FindAnyExists returns the first found node, otherwise error if none of them are found.
func FindAnyExists(ctx context.Context, timeout time.Duration, finders ...*ui.Object) (*ui.Object, error) {
	var targetFinder *ui.Object
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		for _, finder := range finders {
			if err := finder.Exists(ctx); err != nil {
				continue
			}
			targetFinder = finder
			return nil
		}
		return errors.New("failed to find objects")
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return nil, err
	}
	return targetFinder, nil
}

// WaitUntilGone returns an action function which waits for a view matching the selector to disappear.
func WaitUntilGone(obj *ui.Object, timeout time.Duration) action.Action {
	return func(ctx context.Context) error {
		if err := obj.WaitUntilGone(ctx, timeout); err != nil {
			return errors.Wrap(err, "failed to wait for the target object disappear")
		}
		return nil
	}
}

// FindAndClick returns an action function which finds and clicks Android ui object.
func FindAndClick(obj *ui.Object, timeout time.Duration) action.Action {
	return func(ctx context.Context) error {
		if err := obj.WaitForExists(ctx, timeout); err != nil {
			return errors.Wrap(err, "failed to find the target object")
		}
		if err := obj.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click the target object")
		}
		return nil
	}
}

// ClickIfExist returns an action function which clicks the UI object if it exists.
func ClickIfExist(obj *ui.Object, timeout time.Duration) action.Action {
	return func(ctx context.Context) error {
		if err := obj.WaitForExists(ctx, timeout); err != nil {
			if ui.IsTimeout(err) {
				return nil
			}
			return errors.Wrap(err, "failed to wait for the target object")
		}
		return obj.Click(ctx)
	}
}
