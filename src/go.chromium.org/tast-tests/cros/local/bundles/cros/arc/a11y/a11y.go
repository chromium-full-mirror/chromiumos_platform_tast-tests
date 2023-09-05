// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility settings
// in ARC accessibility tests.
package a11y

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/sts"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ArcAccessibilityHelperService is the full class name of ArcAccessibilityHelperService.
	ArcAccessibilityHelperService = "org.chromium.arc.accessibilityhelper/org.chromium.arc.accessibilityhelper.ArcAccessibilityHelperService"

	// ApkName is the name of apk which is used in ARC++ accessibility tests.
	ApkName = "ArcAccessibilityTest.apk"
	// PackageName is the Android package name which is used in ARC++ accessibility tests.
	PackageName = "org.chromium.arc.testapp.accessibilitytest"

	// CheckBox class name.
	CheckBox = "android.widget.CheckBox"
	// Button class name.
	Button = "android.widget.Button"
	// EditText class name.
	EditText = "android.widget.EditText"
	// SeekBar class name.
	SeekBar = "android.widget.SeekBar"
	// TextView class name.
	TextView = "android.widget.TextView"
	// ToggleButton class name.
	ToggleButton = "android.widget.ToggleButton"
)

// TestActivity represents an activity that will be used as a test case.
type TestActivity struct {
	Name  string
	Title string
}

// MainActivity is the struct for the main activity used in test cases.
var MainActivity = TestActivity{".MainActivity", "Main Activity"}

// EditTextActivity is the struct for the edit text activity used in test cases.
var EditTextActivity = TestActivity{".EditTextActivity", "Edit Text Activity"}

// LiveRegionActivity is the struct for the live region text activity used in test cases.
var LiveRegionActivity = TestActivity{".LiveRegionActivity", "Live Region Activity"}

// ActionActivity is the struct for the action activity used in test cases.
var ActionActivity = TestActivity{".ActionActivity", "Action Activity"}

// tearDownHelper is a helper struct to accumulate clean up functions.
// TODO(hirokisato): consider to use a11y.TearDownHelper
type tearDownHelper struct {
	funcs []func(context.Context) error
}

func (h *tearDownHelper) tearDown(ctx context.Context) {
	for i := len(h.funcs) - 1; i >= 0; i-- {
		if err := h.funcs[i](ctx); err != nil {
			testing.ContextLog(ctx, "Error during tear down: ", err)
		}
	}
	h.funcs = nil
}

func (h *tearDownHelper) append(f func(context.Context) error) {
	h.funcs = append(h.funcs, f)
}

// IsEnabledAndroid checks if accessibility is enabled in Android.
func IsEnabledAndroid(ctx context.Context, a *arc.ARC) (bool, error) {
	res, err := a.Command(ctx, "settings", "--user", "0", "get", "secure", "accessibility_enabled").Output(testexec.DumpLogOnError)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(res)) == "1", nil
}

// EnabledAndroidAccessibilityServices returns enabled AccessibilityService in Android.
func EnabledAndroidAccessibilityServices(ctx context.Context, a *arc.ARC) ([]string, error) {
	res, err := a.Command(ctx, "settings", "--user", "0", "get", "secure", "enabled_accessibility_services").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(res)), ":"), nil
}

// waitAndroidAccessibilityReady waits for android accessibility settings is enabled.
func waitAndroidAccessibilityReady(ctx context.Context, a *arc.ARC) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if res, err := IsEnabledAndroid(ctx, a); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check whether accessibility is enabled in Android"))
		} else if !res {
			return errors.New("accessibility not enabled")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// prepareFeature prepares to run ARC tests with the specified accessibility feature enabled.
func prepareFeature(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, tconn *chrome.TestConn, feature a11y.Feature) (cleanup func(context.Context), e error) {
	tdh := tearDownHelper{}
	defer func(ctx context.Context) {
		if e != nil {
			tdh.tearDown(ctx)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := crastestclient.Mute(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to mute device")
	}
	tdh.append(func(ctx context.Context) error {
		return crastestclient.Unmute(ctx)
	})

	if err := a.WaitIntentHelper(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for ArcIntentHelper")
	}

	if err := a11y.SetFeatureEnabled(ctx, tconn, feature, true); err != nil {
		return nil, errors.Wrap(err, "failed to enable spoken feedback")
	}
	tdh.append(func(ctx context.Context) error {
		return a11y.ClearFeature(ctx, tconn, feature)
	})

	if err := waitAndroidAccessibilityReady(ctx, a); err != nil {
		return nil, errors.Wrap(err, "failed to enable spoken feedback")
	}

	s.Log("Installing and starting test app")
	if err := a.Install(ctx, arc.APKPath(ApkName)); err != nil {
		return nil, errors.Wrap(err, "failed to install the APK")
	}

	// To speedup test run.
	if err := tts.SetRate(ctx, tconn, 5.0); err != nil {
		return nil, errors.Wrap(err, "failed to change TTS rate")
	}
	tdh.append(func(ctx context.Context) error {
		return tts.SetRate(ctx, tconn, 1.0)
	})

	return tdh.tearDown, nil
}

// SetUpChromeVox runs preparations to run ARC tests with ChromeVox enabled.
// Caller is responsible to run cleanup function after tests finish.
func SetUpChromeVox(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, tconn *chrome.TestConn) (_ *chromevox.Conn, cleanup func(context.Context), e error) {
	tdh := tearDownHelper{}
	defer func(ctx context.Context) {
		if e != nil {
			tdh.tearDown(ctx)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cleanupFeature, err := prepareFeature(ctx, s, cr, a, tconn, a11y.SpokenFeedback)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to enable SpokenFeedback")
	}
	tdh.append(func(ctx context.Context) error {
		cleanupFeature(ctx)
		return nil
	})

	cvconn, err := chromevox.NewConn(ctx, cr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create connection to ChromeVox extension")
	}
	tdh.append(func(ctx context.Context) error {
		cvconn.Close()
		return nil
	})

	return cvconn, tdh.tearDown, nil
}

// SetUpSelectToSpeak runs preparations to run ARC tests with SelectToSpeak enabled.
// Caller is responsible to run cleanup function after tests finish.
func SetUpSelectToSpeak(ctx context.Context, s *testing.State, cr *chrome.Chrome, a *arc.ARC, tconn *chrome.TestConn) (_ *sts.Conn, cleanup func(context.Context), e error) {
	tdh := tearDownHelper{}
	defer func(ctx context.Context) {
		if e != nil {
			tdh.tearDown(ctx)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cleanupFeature, err := prepareFeature(ctx, s, cr, a, tconn, a11y.SelectToSpeak)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to enable SelectToSpeak")
	}
	tdh.append(func(ctx context.Context) error {
		cleanupFeature(ctx)
		return nil
	})

	stsconn, err := sts.NewConn(ctx, cr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create connection to SelectToSpeak extension")
	}
	tdh.append(func(ctx context.Context) error {
		stsconn.Close()
		return nil
	})

	return stsconn, tdh.tearDown, nil
}

// AttachFaillog sets an error handler to the given testing.State struct.
// In the handler, UI dump and screenshot is saved on error with a given prefix filename.
func AttachFaillog(ctx context.Context, s *testing.State, tconn *chrome.TestConn, prefix string) {
	handler := func(msg string) {
		faillog.DumpUITreeWithScreenshotWithTestAPIOnError(ctx, s.OutDir(), s.HasError, tconn, prefix)
	}
	s.AttachErrorHandlers(handler, handler)
}

// StartActivityWithChromeVox launches the activity and wait for ready to run tests with ChromeVox.
// Caller is responsible to run cleanup function after tests finish.
func StartActivityWithChromeVox(ctx context.Context, s *testing.State, a *arc.ARC, tconn *chrome.TestConn, cvconn *chromevox.Conn, activity TestActivity) (cleanup func(context.Context), e error) {
	tdh := tearDownHelper{}
	defer func(ctx context.Context) {
		if e != nil {
			tdh.tearDown(ctx)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// It takes some time for ArcServiceManager to be ready, so make the timeout longer.
	// TODO(b/150734712): Move this out of each subtest once bug has been addressed.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := tconn.Call(ctx, nil, "tast.promisify(chrome.autotestPrivate.setArcTouchMode)", true); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "timed out waiting for touch mode")
	}

	act, err := arc.NewActivity(a, PackageName, activity.Name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new activity")
	}
	tdh.append(func(ctx context.Context) error {
		act.Close(ctx)
		return nil
	})

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to start activity")
	}
	tdh.append(func(ctx context.Context) error {
		return act.Stop(ctx, tconn)
	})

	appRoot := nodewith.Name(activity.Title).Role(role.Application)
	if err = cvconn.WaitForFocusedNode(ctx, tconn, appRoot); err != nil {
		return nil, errors.Wrap(err, "failed to wait for initial ChromeVox focus")
	}

	return tdh.tearDown, nil
}
