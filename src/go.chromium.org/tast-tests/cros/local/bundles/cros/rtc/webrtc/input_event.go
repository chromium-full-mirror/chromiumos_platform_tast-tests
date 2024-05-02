// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var graphicsHistograms = []string{
	// This event needs UI animation change. We collect this metric only mouse test cases.
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
}

var mouseHistograms = []string{
	"EventLatency.MouseDragged.TotalLatency",
	// MouseMoved.TotalLatency is not recorded probably due to blink issue.
	// Collect the event once the root cause is fixed.
	// "EventLatency.MouseMoved.TotalLatency",
	"EventLatency.MousePressed.TotalLatency",
	"EventLatency.GestureScrollUpdate.TotalLatency2",
}

var keyInputHistograms = []string{
	"EventLatency.KeyPressed.TotalLatency",
}

// startMouseEvent starts mouse operations (dragging, moving, clicking and scrolling) and
// returns histograms about the mouse events and the callback of stopping the mouse operations.
func startMouseEvent(ctx context.Context, tconn, bTconn *chrome.TestConn) ([]string, func(), error) {
	mouseCtx := ctx
	mouseOpCtx, stopMouseOperating := ctxutil.Shorten(mouseCtx, 1*time.Second)

	err := startMouseOperating(mouseCtx, mouseOpCtx, tconn)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed starting operating mouse")
	}
	return append(graphicsHistograms, mouseHistograms...), stopMouseOperating, nil
}

// startKeyInputEvent starts key typing and returns histograms about the key input events and the callback of stopping the key typing.
func startKeyInputEvent(ctx context.Context, tconn, bTconn *chrome.TestConn) ([]string, func(), error) {
	// typeCtx is shorter than kbdCtx because a keyboard needs to be closed
	// after typeCtx is expired.
	kbdCtx := ctx
	typeCtx, stopTyping := ctxutil.Shorten(kbdCtx, 1*time.Second)
	err := startTyping(kbdCtx, typeCtx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed starting text input")
	}
	return keyInputHistograms, stopTyping, nil
}

// startTyping continues typing until the typeCtx is cancelled or expired.
func startTyping(kbdCtx, typeCtx context.Context) error {
	kb, err := input.Keyboard(kbdCtx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}

	go func() {
		i := 0
		var typingStrs = []string{
			"1234567890 ", "1234567891 ", "1234567892 ", "1234567893 ", "1234567894 \n",
			"1234567895 ", "1234567896 ", "1234567897 ", "1234567898 ", "1234567899 \n",
		}
		for {
			select {
			case <-typeCtx.Done():
				kb.Close(kbdCtx)
				return
			default:
				kb.Type(typeCtx, typingStrs[i%len(typingStrs)])
			}

			// GoBigSleepLint: Sleep between typing text to not input too many texts.
			if err := testing.Sleep(typeCtx, typingInterval); err != nil {
				testing.ContextLog(kbdCtx, "Failed in sleep")
				break
			}

			i++
		}
	}()

	return nil
}

// startMouseOperating continues mouse operations (dragging, moving, clicking and scrolling)
// until the mouseOpCtx is cancelled or expired.
func startMouseOperating(mouseCtx, mouseOpCtx context.Context, tconn *chrome.TestConn) error {
	type mouseOpType int
	const (
		drag mouseOpType = iota
		move
		click
		scroll
		mouseOpTypeMax
	)

	mouseWin, err := ash.WaitForAnyWindowWithTitle(mouseCtx, tconn, mouseTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the mouse page window")
	}
	centerWinCoord := mouseWin.BoundsInRoot.CenterPoint()
	// The operations except scrolling are executed with uiaout.mouse that invokes
	// a mouse event by autotest private API.
	// First move to the center of "mouse test" window.
	if err := mouse.Move(tconn, centerWinCoord, 0*time.Second)(mouseOpCtx); err != nil {
		return errors.Wrap(err, "failed to move a mouse to center of mouse page")
	}
	// Creates input.Mouse for scrolling operations.
	mw, err := input.Mouse(mouseCtx)
	if err != nil {
		return errors.Wrap(err, "failed to get a virtual mouse")
	}

	go func() {
		i := 0
		for {
			select {
			case <-mouseOpCtx.Done():
				mw.Close(mouseCtx)
				return
			default:
				switch mouseOpType(i % int(mouseOpTypeMax)) {
				case drag:
					const pixelsX, pixelsY = 100, 100
					if err := mouse.Drag(tconn, centerWinCoord, centerWinCoord.Add(coords.NewPoint(pixelsX, pixelsY)), 1*time.Second)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to drag: ", err)
						return
					}
				case move:
					// First move to the center of "mouse test" window.
					if err := mouse.Move(tconn, centerWinCoord, 1*time.Second)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to move: ", err)
						return
					}
				case click:
					if err := mouse.Press(tconn, mouse.LeftButton)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to press: ", err)
						return
					}
					// GoBigSleepLint: Sleep between mouse press and release.
					if err := testing.Sleep(mouseOpCtx, 50*time.Millisecond); err != nil {
						testing.ContextLog(mouseCtx, "Failed in sleep")
						return
					}
					if err := mouse.Release(tconn, mouse.LeftButton)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to press: ", err)
						return
					}
				case scroll:
					for _, scroll := range [](func() error){
						mw.ScrollDown,
						mw.ScrollUp,
					} {
						for i := 0; i < 20; i++ {
							if err := scroll(); err != nil {
								testing.ContextLog(mouseCtx, "Failed to scroll: ", err)
								return
							}
							// GoBigSleepLint: Sleep during scrolling operation
							if err := testing.Sleep(mouseOpCtx, 50*time.Millisecond); err != nil {
								testing.ContextLog(mouseCtx, "Failed in sleep")
								return
							}
						}
					}
				}
			}

			// GoBigSleepLint: Sleep between operating mouse to not operate mouse so much
			if err := testing.Sleep(mouseOpCtx, mouseOperatingInterval); err != nil {
				testing.ContextLog(mouseCtx, "Failed in sleep")
				return
			}

			i++
		}
	}()

	return nil
}
