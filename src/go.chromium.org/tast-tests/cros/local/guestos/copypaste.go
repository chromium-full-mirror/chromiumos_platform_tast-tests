// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package guestos

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	utf8Data = "Some data that gets copied 🔥 ❄"

	// CopyApplet is the data dependency needed to run a copy operation.
	CopyApplet      = "copy_applet.py"
	copyAppletTitle = "gtk3_copy_demo"

	// PasteApplet is the data dependency needed to run a paste operation.
	PasteApplet      = "paste_applet.py"
	pasteAppletTitle = "gtk3_paste_demo"
)

// CopyConfig holds the configuration for the copy half of the test.
type CopyConfig struct {
	gdkBackend string
	cmdArgs    []string
}

// WaylandCopyConfig is the configuration needed to test copying from
// a wayland application.
var WaylandCopyConfig = &CopyConfig{
	gdkBackend: "wayland",
	cmdArgs:    []string{"env", "GDK_BACKEND=wayland", "python3", CopyApplet},
}

// X11CopyConfig is the configuration needed to test copying from
// an X11 application.
var X11CopyConfig = &CopyConfig{
	gdkBackend: "x11",
	cmdArgs:    []string{"env", "GDK_BACKEND=x11", "python3", CopyApplet},
}

// PasteConfig holds the configuration for the paste half of the test.
type PasteConfig struct {
	gdkBackend string
	cmdArgs    []string
}

// WaylandPasteConfig is the configuration needed to test pasting into
// a wayland application.
var WaylandPasteConfig = &PasteConfig{
	gdkBackend: "wayland",
	cmdArgs:    []string{"env", "GDK_BACKEND=wayland", "python3", PasteApplet},
}

// X11PasteConfig is the configuration needed to test pasting into
// a x11 application.
var X11PasteConfig = &PasteConfig{
	gdkBackend: "x11",
	cmdArgs:    []string{"env", "GDK_BACKEND=x11", "python3", PasteApplet},
}

// CopyPasteConfig contains all the data needed to run a single test iteration.
type CopyPasteConfig struct {
	Copy  *CopyConfig
	Paste *PasteConfig
}

// CopyPaste runs an app to copy and paste data to and from the clipboard.
func CopyPaste(ctx context.Context, param CopyPasteConfig, dataPath func(string) string, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, guest vm.Guest) error {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	testing.ContextLog(ctx, "Copying testing applets to container")
	if err := guest.PushFile(ctx, dataPath(CopyApplet), CopyApplet); err != nil {
		return errors.Wrap(err, "failed to push copy applet to container")
	}
	if err := guest.PushFile(ctx, dataPath(PasteApplet), PasteApplet); err != nil {
		return errors.Wrap(err, "failed to push paste applet to container")
	}

	// Add the names of the backends used by each part of the test to differentiate the data used by each test run.
	copiedData := fmt.Sprintf("%v to %v %s", param.Copy.gdkBackend, param.Paste.gdkBackend, utf8Data)

	// The copy event happens at some indeterminate time after the
	// copy applet receives a key press. To be sure we get that event
	// we have to start listening for it before that point.
	// Here, wrapping the promise by a closure in order not to be
	// awaited at this moment.
	var waiting chrome.JSObject
	if err := tconn.Eval(ctx, `(p => () => p)(new Promise((resolve) => {
		  const listener = (e) => {
		    chrome.autotestPrivate.onClipboardDataChanged.removeListener(listener);
		    resolve();
		  };
		  chrome.autotestPrivate.onClipboardDataChanged.addListener(listener);
		}))`, &waiting); err != nil {
		return errors.Wrap(err, "failed to set listener for 'copy' event")
	}
	defer waiting.Release(cleanupCtx)
	if _, err := RunWindowedApp(ctx, tconn, guest, keyboard, 120*time.Second, func(ctx context.Context) error {
		// Unwrap the promise to wait its settled state.
		return tconn.Call(ctx, nil, `p => p()`, &waiting)
	}, true, copyAppletTitle, append(param.Copy.cmdArgs, copiedData)); err != nil {
		return errors.Wrap(err, "failed to run copy applet")
	}

	output, err := RunWindowedApp(ctx, tconn, guest, keyboard, 60*time.Second, nil, false, pasteAppletTitle, param.Paste.cmdArgs)
	if err != nil {
		return errors.Wrap(err, "failed to run paste application")
	}

	if output != copiedData {
		return errors.Errorf("unexpected paste output: got %q, want %q", output, copiedData)
	}

	return nil
}
