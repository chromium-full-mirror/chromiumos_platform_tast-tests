// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	"context"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/touch"
	"go.chromium.org/tast-tests/cros/local/coords"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var serverKeyType = testing.RegisterVarString(
	KeyType,
	"",
	"The key type of ChromeOS UI detection server",
)
var serverKey = testing.RegisterVarString(
	Key,
	"",
	"The key of ChromeOS UI detection server",
)
var serverKeyAddr = testing.RegisterVarString(
	Server,
	"",
	"The address of ChromeOS UI detection server",
)

// ScreenshotStrategy holds the different screenshot strategies that can be used for image-based UI detection.
type ScreenshotStrategy int

// Holds all the screenshot strategies that can be used.
const (
	// Default. Wait until the screen is not changing before taking a screenshot.
	StableScreenshot ScreenshotStrategy = iota
	// Take a screenshot immediately.
	ImmediateScreenshot
)

// ScreenshotResizingStrategy holds all of the image resizing strategies that
// can be used.
//
// Resizing scales the screenshot up by 50% with bicubic scaling
// to prevent common detection issues. Detection coordinates are automatically
// scaled back down.
type ScreenshotResizingStrategy int

// Holds all the resizing strategies that can be used.
const (
	// Do not resize the original image. This minimises detection
	// latency.
	ResizingDisabled ScreenshotResizingStrategy = iota
	// Default. Fall back to the resized image if no elements were detected using the
	// original image.
	ResizeAsFallback
	// Start with the resized image, and fall back to the original image if
	// no elements are detected.
	AlwaysResize
)

const maxPollingGoroutines = 5

// Context provides functionalities for image-based UI automation.
type Context struct {
	tconn                        *chrome.TestConn
	detector                     *uiDetector
	pollOpts                     testing.PollOptions
	options                      *Options
	deviceInfo                   *DeviceInfo
	screenshotStrategy           ScreenshotStrategy
	screenshotResizingStrategy   ScreenshotResizingStrategy
	disableDynamicElementMasking bool
	saveResizedScreenshots       bool
}

// New returns a new UI Detection automation instance.
func New(t *chrome.TestConn, keyType, key, server string) *Context {
	return &Context{
		tconn: t,
		detector: &uiDetector{
			keyType: keyType,
			key:     key,
			server:  server,
		},
		pollOpts: testing.PollOptions{
			Interval: 300 * time.Millisecond,
			Timeout:  60 * time.Second,
		},
		options:                    DefaultOptions(),
		screenshotStrategy:         StableScreenshot,
		screenshotResizingStrategy: ResizeAsFallback,
	}
}

// NewDefault returns a new UI Detection automation instance with default params.
func NewDefault(t *chrome.TestConn) *Context {
	return New(t, serverKeyType.Value(), serverKey.Value(), serverKeyAddr.Value())
}

func (uda *Context) copy() *Context {
	return &Context{
		tconn:                        uda.tconn,
		detector:                     uda.detector,
		pollOpts:                     uda.pollOpts,
		options:                      uda.options,
		deviceInfo:                   uda.deviceInfo,
		screenshotStrategy:           uda.screenshotStrategy,
		screenshotResizingStrategy:   uda.screenshotResizingStrategy,
		disableDynamicElementMasking: uda.disableDynamicElementMasking,
		saveResizedScreenshots:       uda.saveResizedScreenshots,
	}
}

// WithTimeout returns a new Context with the specified timeout.
func (uda *Context) WithTimeout(timeout time.Duration) *Context {
	c := uda.copy()
	c.pollOpts.Timeout = timeout
	return c
}

// WithInterval returns a new Context with the specified polling interval.
func (uda *Context) WithInterval(interval time.Duration) *Context {
	c := uda.copy()
	c.pollOpts.Interval = interval
	return c
}

// WithPollOpts returns a new Context with the specified polling options.
func (uda *Context) WithPollOpts(pollOpts testing.PollOptions) *Context {
	c := uda.copy()
	c.pollOpts = pollOpts
	return c
}

// WithScreenshotStrategy returns a new Context with the specified screenshot strategy.
func (uda *Context) WithScreenshotStrategy(s ScreenshotStrategy) *Context {
	c := uda.copy()
	c.screenshotStrategy = s
	return c
}

// WithScreenshotResizingStrategy returns a new Context with the specified
// screenshot resizing strategy.
func (uda *Context) WithScreenshotResizingStrategy(s ScreenshotResizingStrategy) *Context {
	c := uda.copy()
	c.screenshotResizingStrategy = s
	return c
}

// WithScreenshotResizing is deprecated: use
// WithScreenshotResizingStrategy(uidetection.AlwaysResize) instead.
func (uda *Context) WithScreenshotResizing() *Context {
	return uda.WithScreenshotResizingStrategy(AlwaysResize)
}

// DisableDynamicElementMasking returns a new Context that disables masking dynamic
// elements: date, time, network status and power status.
// ONLY USE THIS when intentionally doing detection in these regions.
func (uda *Context) DisableDynamicElementMasking() *Context {
	c := uda.copy()
	c.disableDynamicElementMasking = true
	return c
}

// SaveResizedScreenshots saves resized screenshots to the Tast output
// directory for debugging. This option has a time and network overhead and is
// intended for local development only.
func (uda *Context) SaveResizedScreenshots() *Context {
	c := uda.copy()
	c.saveResizedScreenshots = true
	return c
}

// WithOptions returns a new Context with the specified detection options.
func (uda *Context) WithOptions(optionList ...Option) *Context {
	c := uda.copy()
	for _, opt := range optionList {
		opt(c.options)
	}
	return c
}

// click attempts to locate and click on an element.
func (uda *Context) click(s *Finder, button mouse.Button) uiauto.Action {
	return func(ctx context.Context) error {
		loc, err := uda.Location(ctx, s)
		if err != nil {
			return errors.Wrapf(err, "failed to find the location of %q", s.desc)
		}

		// Move the mouse over a short duration of time. This fixes the issue where
		// the implementation of `mouse.Click` moves the mouse to the desired location immediately,
		// which prevents applications from registering the move properly. This is especially
		// important in ARC++ applications, which exhibited behavior where two
		// simultaneous clicks needed to be sent since the first would not be
		// registered correctly.
		if err := mouse.Move(uda.tconn, loc.CenterPoint(), 250*time.Millisecond)(ctx); err != nil {
			return errors.Wrap(err, "failed to move the mouse into position")
		}

		return mouse.Click(uda.tconn, loc.CenterPoint(), button)(ctx)
	}
}

// attemptClickUntilSuccess repeatedly attempts to locate and click on an
// element until it successfully performs a click on the element.
func (uda *Context) attemptClickUntilSuccess(s *Finder, button mouse.Button) uiauto.Action {
	return action.Retry(uda.options.Retries, func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			return uda.click(s, button)(ctx)
		}, &uda.pollOpts)
	}, uda.options.RetryInterval)
}

// clickUntil returns a function that uses the specified mouse button on the
// finder until the condition returns no error.
func (uda *Context) clickUntil(finder *Finder, condition func(context.Context) error, button mouse.Button) uiauto.Action {
	return func(ctx context.Context) error {
		if err := uda.click(finder, button)(ctx); err != nil {
			return errors.Wrap(err, "failed to initially click the element")
		}
		// GoBigSleepLint: Wait a little bit before polling `condition`.
		if err := testing.Sleep(ctx, uda.pollOpts.Interval); err != nil {
			return err
		}
		return testing.Poll(ctx, func(ctx context.Context) error {
			if err := condition(ctx); err != nil {
				if err := uda.click(finder, button)(ctx); err != nil {
					return errors.Wrap(err, "failed to click the element")
				}
				return errors.Wrap(err, "click may not have been received yet")
			}
			return nil
		}, &uda.pollOpts)
	}
}

// LeftClick returns an action that left-clicks a finder.
func (uda *Context) LeftClick(s *Finder) uiauto.Action {
	return uda.attemptClickUntilSuccess(s, mouse.LeftButton)
}

// LeftClickUntil returns a function that repeatedly left clicks the finder
// until the condition returns no error. This is useful for situations where
// there is no indication of whether the element is ready to receive clicks.
// It uses the polling options from the Context.
func (uda *Context) LeftClickUntil(finder *Finder, condition func(context.Context) error) uiauto.Action {
	return uda.clickUntil(finder, condition, mouse.LeftButton)
}

// RightClick returns an action that right-clicks a finder.
func (uda *Context) RightClick(s *Finder) uiauto.Action {
	return uda.attemptClickUntilSuccess(s, mouse.RightButton)
}

// RightClickUntil returns a function that repeatedly right clicks the finder
// until the condition returns no error. This is useful for situations where
// there is no indication of whether the element is ready to receive clicks.
// It uses the polling options from the Context.
func (uda *Context) RightClickUntil(finder *Finder, condition func(context.Context) error) uiauto.Action {
	return uda.clickUntil(finder, condition, mouse.RightButton)
}

// DoubleClick returns an action that double-clicks a finder.
func (uda *Context) DoubleClick(s *Finder) uiauto.Action {
	return action.Retry(uda.options.Retries, func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			loc, err := uda.Location(ctx, s)
			if err != nil {
				return errors.Wrapf(err, "failed to find the location of %q", s.desc)
			}

			if err := mouse.Move(uda.tconn, loc.CenterPoint(), 250*time.Millisecond)(ctx); err != nil {
				return errors.Wrap(err, "failed to move the mouse into position")
			}

			return mouse.DoubleClick(uda.tconn, loc.CenterPoint(), 50*time.Millisecond)(ctx)
		}, &uda.pollOpts)
	}, uda.options.RetryInterval)
}

// Tap performs a single touchscreen tap.
func (uda *Context) Tap(s *Finder) uiauto.Action {
	return action.Retry(uda.options.Retries, func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			loc, err := uda.Location(ctx, s)
			if err != nil {
				return errors.Wrapf(err, "failed to find the location of %q", s.desc)
			}

			ts, err := touch.New(ctx, uda.tconn)
			if err != nil {
				return errors.Wrap(err, "failed to create touchscreen")
			}

			return ts.TapAt(loc.CenterPoint())(ctx)
		}, &uda.pollOpts)
	}, uda.options.RetryInterval)
}

// Location finds the location of a finder in the screen.
func (uda *Context) Location(ctx context.Context, s *Finder) (*Location, error) {
	screens, err := display.GetInfo(ctx, uda.tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the display info")
	}

	// Find the ratio to convert coordinates in the screenshot to those in the screen.
	scaleFactor, err := screens[0].GetEffectiveDeviceScaleFactor()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the device scale factor")
	}

	location, err := s.locationPx(ctx, uda, scaleFactor)
	if err != nil {
		return nil, err
	}
	rect := coords.NewRect(location.Left, location.Top, location.Width, location.Height)

	return &Location{
		Text: location.Text,
		Rect: coords.ConvertBoundsFromPXToDP(rect, scaleFactor)}, nil
}

// WaitForLocation returns a function that waits until the UI location is
// stabilized or consistent for two consecutive screenshots.
func (uda *Context) WaitForLocation(s *Finder) uiauto.Action {
	return func(ctx context.Context) error {
		var lastLocation *Location
		var currentLocation *Location
		var err error

		start := time.Now()
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if currentLocation, err = uda.Location(ctx, s); err != nil {
				// Reset lastLocation on error.
				lastLocation = &Location{}
				return err
			}
			if lastLocation == nil || (currentLocation.Rect != lastLocation.Rect) {
				lastLocation = currentLocation
				elapsed := time.Since(start)
				return errors.Errorf("%s has not stopped changing location after %s, perhaps increase timeout", s.desc, elapsed)
			}
			return nil
		}, &uda.pollOpts); err != nil {
			return err
		}
		return nil
	}
}

// Exists returns an action that returns nil if the specified element exists.
func (uda *Context) Exists(s *Finder) uiauto.Action {
	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Looking for an element %q", s.desc)
		if loc, err := uda.Location(ctx, s); err != nil {
			return err
		} else if loc == nil {
			return errors.Errorf("failed to find element: %q", s.desc)
		} else {
			return nil
		}
	}
}

// WaitUntilExists returns an action that waits until the specified element exists.
func (uda *Context) WaitUntilExists(s *Finder) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, uda.Exists(s), &uda.pollOpts)
	}
}

// Gone returns an action that returns nil if the specified element doesn't exist.
func (uda *Context) Gone(s *Finder) uiauto.Action {
	return func(ctx context.Context) error {
		// An not-found error is expected.
		if _, err := uda.Location(ctx, s); err == nil {
			return errors.Errorf("element %q still exists", s.desc)
		} else if !strings.Contains(err.Error(), ErrNotFound) {
			return errors.Errorf("expected error: %q, actual error: %q", ErrNotFound, err)
		}
		return nil
	}
}

// WaitUntilGone returns an action that waits until the specified element doesnt exist.
func (uda *Context) WaitUntilGone(s *Finder) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, uda.Gone(s), &uda.pollOpts)
	}
}

// TimeElementAppears waits for the specified element to exist, then returns the
// time when the first successful poll began. This is an approximation of when
// the element first appeared on the screen.
func (uda *Context) TimeElementAppears(ctx context.Context, s *Finder) (time.Time, error) {
	timeout := uda.pollOpts.Timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		startTime := time.Now()
		err := uda.Exists(s)(ctx)
		if err == nil {
			return startTime, nil
		}

		if ctx.Err() != nil {
			return time.Time{}, errors.Wrapf(err, "%s during a poll with timeout %v; last error follows", ctx.Err(), timeout)
		}
	}
}

// AccurateTimeElementAppears is similar to TimeElementAppears, but provides
// are more accurate timing by increasing the polling rate.
func (uda *Context) AccurateTimeElementAppears(ctx context.Context, s *Finder) (time.Time, error) {
	// Create multiple goroutines to poll for the element.
	times := make(chan time.Time, maxPollingGoroutines)
	errs, ctx := errgroup.WithContext(ctx)
	for i := 0; i < maxPollingGoroutines; i++ {
		errs.Go(func() error {
			successTime, err := uda.TimeElementAppears(ctx, s)
			if err != nil {
				return err
			}
			times <- successTime
			return nil
		})
		// GoBigSleepLint: Stagger requests.
		testing.Sleep(ctx, uda.pollOpts.Interval)
	}

	// Wait until all goroutines complete, since even the last goroutine to
	// complete could have found the element first.
	if err := errs.Wait(); err != nil {
		return time.Time{}, err
	}
	close(times)

	// Find the earliest time the element was detected between the goroutines.
	earliestTime := <-times
	for t := range times {
		if t.Before(earliestTime) {
			earliestTime = t
		}
	}
	return earliestTime, nil
}
