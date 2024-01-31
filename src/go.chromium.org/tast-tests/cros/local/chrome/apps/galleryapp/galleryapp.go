// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package galleryapp provides a library for controlling galleryapp.
package galleryapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// RootFinder is the main window of Gallery app.
	RootFinder = nodewith.NameStartingWith(apps.Gallery.Name).HasClass("BrowserFrame").Role(role.Window).First()

	// DialogFinder is the finder of popup dialog in Gallery app.
	DialogFinder = nodewith.Role(role.AlertDialog).Ancestor(RootFinder)

	// openImageButtonFinder is the finder of 'Open image' button on zero state page.
	openImageButtonFinder = nodewith.Role(role.Button).Name("Open image").Ancestor(RootFinder)

	// infoButton is the finder of 'Info' button in Gallery app.
	infoButton = nodewith.Name("Info").Role(role.ToggleButton).Ancestor(RootFinder)

	// dateModifiedText is the finder of 'Date modified' text in Gallery app.
	dateModifiedText = nodewith.Name("Date modified").Role(role.StaticText).Ancestor(RootFinder)

	// longUITimeout is the timeout for large file loading UI.
	longUITimeout = 2 * time.Minute
)

const (
	mediaSelector    string = `document.querySelector("video,audio")`
	fastJumpDuration        = 10.0
)

// Gallery represents an instance of the Gallery App.
type Gallery struct {
	apps.App

	cr    *chrome.Chrome
	conn  *chrome.Conn
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// Launch launches the Gallery app and returns an instance of the Gallery app.
func Launch(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*Gallery, error) {
	// Launch the Gallery App.
	if err := apps.Launch(ctx, tconn, apps.Gallery.ID); err != nil {
		return nil, errors.Wrap(err, "failed to launch Gallery")
	}

	testing.ContextLog(ctx, "Wait for Gallery shown in shelf")
	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to check Gallery in shelf")
	}

	return ConnectToApp(ctx, cr, tconn)
}

// ConnectToApp connects to the Gallery app and returns an instance of the Gallery app.
// The app needs to be opened before calling this func, an error will be returned if the app isn't opened.
func ConnectToApp(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) (*Gallery, error) {
	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for app appear in the shelf")
	}

	if _, err := ash.WaitForAppWindow(ctx, tconn, apps.Gallery.ID); err != nil {
		return nil, errors.Wrap(err, "failed to wait for app to be visible")
	}

	// Connect to the Gallery app HTML page,
	// where JavaScript can be executed to simulate interactions with the UI.
	galleryAppConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL("chrome-untrusted://media-app/app.html"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get connection to Gallery app")
	}
	if err := galleryAppConn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		return nil, errors.Wrap(err, "failed to wait for Gallery app to finish loading")
	}

	return &Gallery{
		App:   apps.Gallery,
		cr:    cr,
		tconn: tconn,
		conn:  galleryAppConn,
		ui:    uiauto.New(tconn),
	}, nil
}

// DeleteAndConfirm returns an action clicking 'Delete' button and then 'Confirm' to remove current opened media file.
// It assumes a valid media file is opened.
func (g *Gallery) DeleteAndConfirm() uiauto.Action {
	deleteButtonFinder := nodewith.Role(role.Button).Name("Delete").Ancestor(RootFinder)
	confirmButtonFinder := nodewith.Role(role.Button).Name("Delete").Ancestor(DialogFinder)
	return uiauto.Combine("remove current opened media file",
		g.ui.WithTimeout(30*time.Second).WithInterval(1*time.Second).LeftClickUntil(
			deleteButtonFinder, g.ui.WithTimeout(3*time.Second).WaitUntilExists(confirmButtonFinder)),
		g.ui.LeftClick(confirmButtonFinder),
	)
}

// AssertZeroState asserts that Gallery is on 'open image' page.
func (g *Gallery) AssertZeroState() uiauto.Action {
	return g.ui.WaitUntilExists(openImageButtonFinder)
}

// EvalJSWithShadowPiercer executes javascript in Gallery app web page.
func (g *Gallery) EvalJSWithShadowPiercer(ctx context.Context, expr string, out interface{}) error {
	return webutil.EvalWithShadowPiercer(ctx, g.conn, expr, out)
}

// DecodeFailed verifies that the gallery app failed to decode media.
func (g *Gallery) DecodeFailed(ctx context.Context) error {
	return g.ui.WaitUntilExists(nodewith.Name("Decoding failed").HasClass("display6-typescale").Role(role.Heading).FinalAncestor(RootFinder))(ctx)
}

// Close closes the Gallery app.
func (g *Gallery) Close(ctx context.Context) error {
	if err := apps.Close(ctx, g.tconn, g.ID); err != nil {
		return errors.Wrapf(err, "failed to close %s", g.Name)
	}
	if err := ash.WaitForAppClosed(ctx, g.tconn, g.ID); err != nil {
		return errors.Wrapf(err, "failed waiting for %s to be closed", g.Name)
	}
	if err := g.conn.Close(); err != nil {
		return errors.Wrap(err, "failed to close connection")
	}
	return nil
}

// Play plays audio/video from gallery app.
func (g *Gallery) Play(ctx context.Context) error {
	if err := g.conn.Eval(ctx, fmt.Sprintf("%s.play()", mediaSelector), nil); err != nil {
		return errors.Wrap(err, "failed to play the media")
	}

	if playing, err := g.IsPlaying(ctx); err != nil {
		return err
	} else if playing != true {
		return errors.New("the media is not playing")
	}
	return nil
}

// Pause pauses audio/video from gallery app.
func (g *Gallery) Pause(ctx context.Context) error {
	if err := g.conn.Eval(ctx, fmt.Sprintf("%s.pause()", mediaSelector), nil); err != nil {
		return errors.Wrap(err, "failed to pause the media")
	}

	if paused, err := g.IsPaused(ctx); err != nil {
		return err
	} else if paused != true {
		return errors.New("the media is not paused")
	}
	return nil
}

// IsPlaying returns true if video is playing.
func (g *Gallery) IsPlaying(ctx context.Context) (bool, error) {
	paused, err := g.IsPaused(ctx)
	if err != nil {
		return false, err
	}
	return !paused, nil
}

// IsPaused returns true if video is paused.
func (g *Gallery) IsPaused(ctx context.Context) (paused bool, _ error) {
	if err := g.conn.Eval(ctx, fmt.Sprintf("%s.paused", mediaSelector), &paused); err != nil {
		return false, errors.Wrap(err, "failed to get media status")
	}

	return paused, nil
}

// WaitUntilPlaying waits gallery media status is playing.
func (g *Gallery) WaitUntilPlaying(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if isPlaying, err := g.IsPlaying(ctx); err != nil {
			return err
		} else if isPlaying != true {
			return errors.New("gallery is not playing")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond})
}

// WaitUntilPaused waits gallery media status is paused.
func (g *Gallery) WaitUntilPaused(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if isPaused, err := g.IsPaused(ctx); err != nil {
			return err
		} else if isPaused != true {
			return errors.New("gallery is not paused")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond})
}

// MediaDuration returns the media total duration.
func (g *Gallery) MediaDuration(ctx context.Context) (duration float32, _ error) {
	if err := g.conn.Eval(ctx, fmt.Sprintf("%s.duration", mediaSelector), &duration); err != nil {
		return 0, err
	}
	return duration, nil
}

// Forward seeks the media forward and verifies it.
func (g *Gallery) Forward(ctx context.Context) error {
	script := fmt.Sprintf(`() => {
		let media = %s;
		let before = media.currentTime;
		media.currentTime += %f;
		return before;
	}`, mediaSelector, fastJumpDuration)

	var beforeTime float64
	if err := g.conn.Call(ctx, &beforeTime, script); err != nil {
		return errors.Wrap(err, "failed to forward")
	}

	expr := fmt.Sprintf(`%[1]s.currentTime >= %[2]f+%[3]f && %[1]s.currentTime < %[2]f+(%[3]f+2)`, mediaSelector, beforeTime, fastJumpDuration)
	return g.conn.WaitForExprFailOnErrWithTimeout(ctx, expr, 2*time.Second)
}

// Backward seeks video backward and verifies it.
func (g *Gallery) Backward(ctx context.Context) error {
	script := fmt.Sprintf(`() => {
		let media = %s;
		let before = media.currentTime;
		media.currentTime -= %f;
		return before;
	}`, mediaSelector, fastJumpDuration)

	var beforeTime float64
	if err := g.conn.Call(ctx, &beforeTime, script); err != nil {
		return errors.Wrap(err, "failed to backward")
	}

	expr := fmt.Sprintf(`%[1]s.currentTime >= %[2]f-%[3]f && %[1]s.currentTime < %[2]f-(%[3]f-2)`, mediaSelector, beforeTime, fastJumpDuration)
	return g.conn.WaitForExprFailOnErrWithTimeout(ctx, expr, 2*time.Second)
}

// RandomSeek randomly moves videoElement.currentTime to see if onseeked event works properly.
func (g *Gallery) RandomSeek(ctx context.Context) error {
	script := fmt.Sprintf(`() => {
		let media = %s;
		let randomTime = Math.random() * 0.8 * media.duration;
		media.currentTime = randomTime;
		return randomTime;
	}`, mediaSelector)

	var expectTime float64
	if err := g.conn.Call(ctx, &expectTime, script); err != nil {
		return errors.Wrap(err, "failed to random seek")
	}

	// Wait until video play to around expect time to verify random seek is complete.
	expr := fmt.Sprintf(`%[1]s.currentTime > %[2]f+2 && %[1]s.currentTime < %[2]f+5`, mediaSelector, expectTime)
	return g.conn.WaitForExprFailOnErrWithTimeout(ctx, expr, 5*time.Second)
}

// OpenInfo returns a function that opens data info for image, audio and video.
func (g *Gallery) OpenInfo() uiauto.Action {
	ui := g.ui
	return uiauto.IfSuccessThen(ui.Gone(dateModifiedText),
		uiauto.NamedAction("open info",
			ui.LeftClickUntil(infoButton,
				ui.WithTimeout(3*time.Second).WaitUntilExists(dateModifiedText)),
		))
}

// CloseInfo returns a function that closes data info for image, audio and video.
func (g *Gallery) CloseInfo() uiauto.Action {
	ui := g.ui
	return uiauto.IfSuccessThen(ui.Exists(dateModifiedText),
		uiauto.NamedAction("close info",
			ui.LeftClickUntil(infoButton,
				ui.WithTimeout(3*time.Second).WaitUntilGone(dateModifiedText)),
		))
}

// MaximizeWindow returns a function that maximizes Gallery window.
func (g *Gallery) MaximizeWindow() uiauto.Action {
	return func(ctx context.Context) error {
		galleryWindow, err := ash.WaitForAnyWindowWithTitle(ctx, g.tconn, apps.Gallery.Name)
		if err != nil {
			return errors.Wrap(err, "failed to find the Gallery window")
		}
		if err := galleryWindow.ActivateWindow(ctx, g.tconn); err != nil {
			return errors.Wrap(err, "failed to activate the Gallery window")
		}
		if err := ash.SetWindowStateAndWait(ctx, g.tconn, galleryWindow.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximized the Gallery window")
		}
		return nil
	}
}

// Save returns a function that clicks 'Save' button and waits for the
// file saved.
func (g *Gallery) Save() uiauto.Action {
	ui := g.ui
	saveButton := nodewith.Name("Save").Role(role.Button).Ancestor(RootFinder).Focusable()
	savingText := nodewith.Name("Saving…").Role(role.StaticText).Ancestor(RootFinder).First()
	savedText := nodewith.Name("Saved").Role(role.StaticText).Ancestor(RootFinder).First()
	return uiauto.NamedCombine("save file",
		ui.LeftClick(saveButton),
		ui.WaitUntilAnyExists(savingText, savedText),
		ui.WithTimeout(longUITimeout).WaitUntilGone(savingText),
		ui.WaitUntilExists(savedText),
	)
}

// WaitUntilSpinnerGone returns a function that waits until the spinner
// gone.
func (g *Gallery) WaitUntilSpinnerGone() uiauto.Action {
	spinner := nodewith.HasClass("mdc-circular-progress__spinner-layer").Ancestor(RootFinder)
	return uiauto.IfSuccessThen(
		g.ui.WithTimeout(3*time.Second).WaitUntilExists(spinner),
		g.ui.WithTimeout(longUITimeout).WaitUntilGone(spinner),
	)
}

// Done returns a function that clicks 'Done' button and waits until the
// done button gone.
func (g *Gallery) Done() uiauto.Action {
	ui := g.ui
	doneButton := nodewith.Name("Done").Role(role.Button).Ancestor(RootFinder).Focusable()
	return uiauto.NamedCombine("click done button",
		ui.LeftClick(doneButton),
		ui.WaitUntilGone(doneButton),
	)
}

// WaitForGalleryQuiescence returns a function that waits for the Gallery to be quiesce.
func (g *Gallery) WaitForGalleryQuiescence(cr *chrome.Chrome) uiauto.Action {
	return func(ctx context.Context) error {
		// TODO(b/204528998): Change back to chrome://media-app once the
		// underlying issue is solved so we won't hit the race condition.
		const url = "chrome-untrusted://media-app"
		matcher := func(t *target.Info) bool {
			return strings.Contains(t.URL, url)
		}
		connCtx, cancel := ctxutil.Shorten(ctx, longUITimeout)
		defer cancel()

		conn, err := cr.NewConnForTarget(connCtx, matcher)
		if err != nil {
			return errors.Wrapf(err, "failed to find URL %s", url)
		}

		if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
			return errors.Wrap(err, "failed to wait for Gallery to quiesce")
		}
		return nil
	}
}

// WaitNameChanged waits for file name changed and returns the file name of
// the image or audio or video.
func (g *Gallery) WaitNameChanged(ctx context.Context, currentName string) (fileName string, err error) {
	nameFinder := nodewith.Role(role.StaticText).Ancestor(RootFinder)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if g.ui.Exists(imageFinder)(ctx) == nil {
			nameFinder = imageFinder
		}
		nodes, err := g.ui.NodesInfo(ctx, nameFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find name node")
		}
		fileName = nodes[0].Name
		if currentName != fileName {
			return nil
		}
		return errors.Wrap(err, "file name hasn't changed")
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
		return fileName, err
	}
	return fileName, nil
}

// draw returns a function that draws a pattern through points.
func (g *Gallery) draw(canvas *nodewith.Finder, points []coords.Point) uiauto.Action {
	return func(ctx context.Context) error {
		tconn := g.tconn
		canvasBounds, err := g.ui.ImmediateLocation(ctx, canvas)
		if err != nil {
			return errors.Wrap(err, "failed to get the canvas location")
		}
		isPressed := false
		for _, point := range points {
			location := coords.NewPoint(
				canvasBounds.CenterX()+point.X,
				canvasBounds.CenterY()+point.Y,
			)
			if err := mouse.Move(tconn, location, 200*time.Millisecond)(ctx); err != nil {
				return errors.Wrap(err, "failed to move mouse")
			}
			if !isPressed {
				isPressed = true
				if err := mouse.Press(tconn, mouse.LeftButton)(ctx); err != nil {
					return errors.Wrap(err, "failed to press mouse")
				}
			}
		}
		if err := mouse.Release(tconn, mouse.LeftButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to release mouse")
		}
		return nil
	}
}
