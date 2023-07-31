// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dictation provides functions to assist with interacting with the ChromeOS Dictation feature.
package dictation

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// driver contains useful objects for driving Dictation tests and is
// returned by SetUp. Most notably, TearDown() should be run in a defer
// statement by the calling test to properly clean up Dictation.
type driver struct {
	ctx      context.Context
	ui       *uiauto.Context
	bubble   *nodewith.Finder
	editable *nodewith.Finder
	tdh      *a11y.TearDownHelper
}

func newNoOpDriver(tdh *a11y.TearDownHelper) driver {
	return driver{tdh: tdh}
}

// TearDown is a convenience method that routes directly to TearDownHelper's
// implementation of TearDown.
func (d driver) TearDown() error {
	return d.tdh.TearDown()
}

// waitForBubbleVisible waits until the Dictation bubble UI is visible, which
// indicates that speech recognition has started and Dictation is active.
func (d driver) waitForBubbleVisible() error {
	ctx := d.ctx
	ui := d.ui
	bubble := d.bubble

	if err := ui.EnsureExistsFor(bubble, 2*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the Dictation bubble UI")
	}

	return nil
}

// waitForBubbleGone waits until the Dictation bubble UI is hidden/gone, which
// indicates that speech recognition has stopped and Dictation is inactive.
func (d driver) waitForBubbleGone() error {
	ctx := d.ctx
	ui := d.ui
	bubble := d.bubble

	if err := ui.EnsureGoneFor(bubble, 2*time.Second)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the Dictation bubble UI")
	}

	return nil
}

// waitForEditableValue waits until the editable field has a value of
// expectedValue.
func (d driver) waitForEditableValue(expectedValue string) error {
	ctx := d.ctx
	ui := d.ui
	editableWithValue := d.editable.Attribute("value", expectedValue)

	if err := ui.WaitUntilExists(editableWithValue)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for editable value using uiauto")
	}

	return nil
}

// SetUp executes common Dictation setup code and returns a driver that can be
// used to easily drive Dictation tests.
func SetUp(ctx context.Context, html, className string, bt browser.Type) (d driver, e error) {
	// Tears down Dictation if SetUp encountered an error.
	defer func() {
		if e != nil {
			d.TearDown()
		}
	}()

	tdh := &a11y.TearDownHelper{}

	// Shorten deadline to leave time for cleanup.
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	tdh.Append(func() error {
		cancel()
		return nil
	})

	cr, err := browserfixt.NewChrome(ctx, bt, lacrosfixt.NewConfig(),
		// Enforce on-device speech recognition.
		chrome.EnableFeatures("OnDeviceSpeechRecognition"),
	)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to start chrome")
	}
	tdh.Append(func() error {
		return cr.Close(cleanUpCtx)
	})

	// Ensure the device is unmuted and that the volume is loud enough so that
	// Dictation can recognize speech.
	if err := crastestclient.Unmute(cleanUpCtx); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to unmute the device")
	}

	cras, err := audio.NewCras(ctx)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to create new cras")
	}

	if err := audio.WaitForDevice(ctx, audio.OutputStream); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to wait for output stream")
	}

	node, err := activeCrasNode(ctx, cras)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to get initial active cras node")
	}

	originalVolume := int(node.NodeVolume)
	if err := cras.SetOutputNodeVolume(ctx, *node, 100); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to set volume")
	}

	tdh.Append(func() error {
		return cras.SetOutputNodeVolume(ctx, *node, originalVolume)
	})

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to create Test API connection")
	}

	// Setup a browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to setup browser")
	}
	tdh.Append(func() error {
		return closeBrowser(cleanUpCtx)
	})

	brConn, err := a11y.NewTabWithURL(ctx, br, a11y.URLFromHTML(html))
	if err != nil {
		return newNoOpDriver(tdh), errors.Wrapf(err, "failed to open a new tab with HTML: %q", html)
	}
	tdh.Append(brConn.Close)

	// Close the extra new tab page.
	if err := br.CloseWithURL(ctx, chrome.NewTabURL); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to close new tab page")
	}

	// Enable Dictation.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.Dictation, true); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to enable Dictation")
	}
	tdh.Append(func() error {
		// Note: using a11y.ClearFeature() won't work here, we need to explicitly
		// set the Dictation feature enabled status to false.
		if err := a11y.SetFeatureEnabled(cleanUpCtx, tconn, a11y.Dictation, false); err != nil {
			return errors.Wrap(err, "failed to disable Dictation")
		}

		return nil
	})

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to wait for libsoda dlc to be installed")
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := maybeClosePrivacyDialog(ctx, ui); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to close the Dictation privacy dialog")
	}

	// Focus the editable field.
	editable := nodewith.Editable().HasClass(className).Onscreen()
	if err := uiauto.Combine("Focus editable field",
		ui.WaitUntilExists(editable),
		ui.FocusAndWait(editable),
	)(ctx); err != nil {
		return newNoOpDriver(tdh), errors.Wrap(err, "failed to focus the editable field")
	}

	bubble := nodewith.Role(role.GenericContainer).HasClass("DictationBubbleView").Onscreen().First()
	return driver{ctx, ui, bubble, editable, tdh}, nil
}

// maybeClosePrivacyDialog closes the dialog that is shown when Dictation is first
// enabled, if it appears on the screen. The dialog warns the user that their voice
// is sent to Google. This function accepts the dialog so we can use the feature.
func maybeClosePrivacyDialog(ctx context.Context, ui *uiauto.Context) error {
	dialogText := nodewith.NameContaining("Dictation sends your voice to Google").Onscreen()
	continueButton := nodewith.Name("Continue").ClassName("MdTextButton").Onscreen()

	// Check if the dialog pops up.
	if err := ui.WaitUntilExists(dialogText)(ctx); err != nil {
		// If the Dictation dialog can't be found, then we don't need to do anything.
		return nil
	}

	if err := uiauto.Combine("Close Dictation dialog",
		ui.LeftClick(continueButton),
		ui.WaitUntilGone(dialogText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to close the Dictation dialog")
	}

	return nil
}

// activeCrasNode finds the currently active audio node.
func activeCrasNode(ctx context.Context, cras *audio.Cras) (*audio.CrasNode, error) {
	nodes, err := cras.GetNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes from cras")
	}

	for _, n := range nodes {
		if n.Active && !n.IsInput {
			return &n, nil
		}
	}
	return nil, errors.New("failed to find active node")
}

// ToggleOn uses the keyboard to activate Dictation and waits for the bubble UI
// to be visible.
func (d driver) ToggleOn() error {
	ctx := d.ctx

	ew, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create EventWriter")
	}
	defer ew.Close(d.ctx)

	if err := ew.Accel(ctx, "Search+D"); err != nil {
		return errors.Wrap(err, "failed to press Search + D to toggle Dictation on")
	}

	if err := d.waitForBubbleVisible(); err != nil {
		return errors.Wrap(err, "failed to wait for speech recognition started")
	}

	return nil
}

// ToggleOff uses the keyboard to deactivate Dictation and waits for the bubble
// UI to disappear.
func (d driver) ToggleOff() error {
	ctx := d.ctx

	ew, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create EventWriter")
	}
	defer ew.Close(d.ctx)

	if err := ew.Accel(ctx, "Search+D"); err != nil {
		return errors.Wrap(err, "failed to press Search + D to toggle Dictation off")
	}

	if err := d.waitForBubbleGone(); err != nil {
		return errors.Wrap(err, "failed to wait for speech recognition stopped")
	}

	return nil
}

// DictateAndWaitForEditableValue plays an audio file for Dictation to listen
// to; note, it is assumed that speech recognition is on when calling this
// method. It then verifies that the expected text was entered into the editable
// field by using uiauto.
func (d driver) DictateAndWaitForEditableValue(audioFile, expectedValue string) error {
	ctx := d.ctx

	if err := voice.AudioFromFile(ctx, audioFile); err != nil {
		return errors.Wrap(err, "failed to play audio file")
	}

	if err := d.waitForEditableValue(expectedValue); err != nil {
		return errors.Wrapf(err, "failed to wait for editable value: %s", expectedValue)
	}

	return nil
}
