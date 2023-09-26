// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uidetection

import (
	"context"
	"image"
	"math"
	"strings"

	pb "go.chromium.org/tast-tests/cros/local/uidetection/api"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ErrNotFound is the error when there is no matching elements found.
	ErrNotFound = "no matching elements found"
	// ErrMultipleMatch is the error when there are multiple matching elements found.
	ErrMultipleMatch = "there are multiple matches"
	// ErrNthNotFound is the error when the Nth element doesn't exist.
	ErrNthNotFound = "Nth element not found"
	// ErrEmptyBoundingBox is the error when the relative matchers create a
	// screenshot of zero size.
	ErrEmptyBoundingBox = "The element you're trying to screenshot has zero size"
)

// maxScreenSizePx is the maximum width / height of the screen, in pixels.
// Don't use math.maxInt, because it can cause integer overflow.
const maxScreenSizePx = 100000

// Location represents the location of a matching UI element.
type Location struct {
	// Rectangle of the location.
	coords.Rect
	// Text associated with the element, if any.
	Text string
}

// withScale returns a new location with both the coordinate position
// and area scaled by the given amount.
func (l Location) withScale(scale float64) Location {
	scaledRec := coords.NewRect(
		int(math.Round(float64(l.Left)*scale)),
		int(math.Round(float64(l.Top)*scale)),
		int(math.Round(float64(l.Width)*scale)),
		int(math.Round(float64(l.Height)*scale)),
	)
	return Location{scaledRec, l.Text}
}

// Finder represents a data structure that consists of arguments to find
// a UI element.
type Finder struct {
	// The request used to construct the finder.
	request *pb.DetectionRequest
	// Descriptor for the finder.
	desc        string
	nth         int
	exactMatch  bool
	constraints []func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error)
}

func newFinder() *Finder {
	return &Finder{
		nth:        -1,
		exactMatch: false,
	}
}

func newFromRequest(r *pb.DetectionRequest, d string) *Finder {
	return &Finder{
		request:    r,
		desc:       d,
		nth:        -1,
		exactMatch: false,
	}
}

// copy returns a copy of the input Finder.
// It copies all of the keys/values in attributes and state individually.
func (f *Finder) copy() *Finder {
	c := newFinder()
	c.request = f.request
	c.desc = f.desc
	c.nth = f.nth
	c.exactMatch = f.exactMatch
	c.constraints = f.constraints
	return c
}

// First enables the finder to choose the first match of a UI element
// if there are multiple matches.
func (f *Finder) First() *Finder {
	c := f.copy()
	c.nth = 0
	return c
}

// Nth enables the finder to choose the n-th match of a UI element.
// if there are multiple matches.
func (f *Finder) Nth(nth int) *Finder {
	c := f.copy()
	c.nth = nth
	return c
}

// ExactMatch turns off the approximate match for the word finder.
// The results will be filtered by exact string matching.
// USE WITH CAUTION. Due to the performance of the OCR (optical character
// recognition) model, approximate match is the default behavior for
// error-tolerance.
// An example use case is when the matching word is short with two or three
// letters.
// TODO(b/211937254): Allow exact matches with max_edit_distance in new proto.
func (f *Finder) ExactMatch() *Finder {
	c := f.copy()
	c.exactMatch = true
	return c
}

func (f *Finder) newConstraint(constraint func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error)) *Finder {
	c := f.copy()
	c.constraints = append(c.constraints, constraint)
	return c
}

func (f *Finder) pxConstraintBuilder(rect *coords.Rect) *Finder {
	return f.newConstraint(func(context.Context, *Context, float64) (*coords.Rect, error) { return rect, nil })
}

// WithinPx ensures that the element returned must be within the rectangle on the screen, measured in pixels.
func (f *Finder) WithinPx(r coords.Rect) *Finder {
	return f.pxConstraintBuilder(&r)
}

// WithinDp ensures that the element returned must be within the rectangle on the screen, measured in dp.
func (f *Finder) WithinDp(r coords.Rect) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		px := coords.ConvertBoundsFromDPToPX(r, scaleFactor)
		return &px, nil
	})
}

// Within ensures that the element returned must be within the element returned by the finder on the screen.
func (f *Finder) Within(other *Finder) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		loc, err := other.locationPx(ctx, uda, scaleFactor)
		if err != nil {
			return nil, err
		}
		return &coords.Rect{Left: loc.Left, Top: loc.Top, Width: loc.Width, Height: loc.Height}, nil
	})
}

// WithinA11yNode ensures that the element returned must be within the element returned by the a11y node finder on the screen.
//
// Does not wait for the a11y element to appear. If it may not exist yet, use uiauto's WaitForLocation to verify the element exists and is stable.
func (f *Finder) WithinA11yNode(other *nodewith.Finder) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		loc, err := uiauto.New(uda.tconn).ImmediateLocation(ctx, other)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find enclosing a11y node")
		}
		px := coords.ConvertBoundsFromDPToPX(*loc, scaleFactor)
		return &px, nil
	})
}

func above(px int) *coords.Rect {
	return &coords.Rect{Left: 0, Top: 0, Width: maxScreenSizePx, Height: px}
}

func below(px int) *coords.Rect {
	return &coords.Rect{Left: 0, Top: px, Width: maxScreenSizePx, Height: maxScreenSizePx}
}

func leftOf(px int) *coords.Rect {
	return &coords.Rect{Left: 0, Top: 0, Width: px, Height: maxScreenSizePx}
}

func rightOf(px int) *coords.Rect {
	return &coords.Rect{Left: px, Top: 0, Width: maxScreenSizePx, Height: maxScreenSizePx}
}

type directionFn = func(px int) *coords.Rect

func (f *Finder) dpConstraintBuilder(dp int, direction directionFn) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		return direction(int(float64(dp) / scaleFactor)), nil
	})
}

func (f *Finder) constraintBuilder(other *Finder, direction directionFn, side func(Location) int) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		loc, err := other.locationPx(ctx, uda, scaleFactor)
		if err != nil {
			return nil, err
		}
		return direction(side(*loc)), nil
	})
}

func (f *Finder) a11yNodeConstraintBuilder(other *nodewith.Finder, direction directionFn, side func(coords.Rect) int) *Finder {
	return f.newConstraint(func(ctx context.Context, uda *Context, scaleFactor float64) (*coords.Rect, error) {
		loc, err := uiauto.New(uda.tconn).WithPollOpts(uda.pollOpts).Location(ctx, other)
		if err != nil {
			return nil, err
		}
		return direction(side(coords.ConvertBoundsFromDPToPX(*loc, scaleFactor))), nil
	})
}

// AbovePx ensures that the element returned must be above px pixels on the screen.
func (f *Finder) AbovePx(px int) *Finder {
	return f.pxConstraintBuilder(above(px))
}

// AboveDp ensures that the element returned must be above dp display pixels on the screen.
func (f *Finder) AboveDp(dp int) *Finder {
	return f.dpConstraintBuilder(dp, above)
}

// Above ensures that the element returned must be above the element returned by the finder on the screen.
func (f *Finder) Above(other *Finder) *Finder {
	return f.constraintBuilder(other, above, func(l Location) int { return l.Top })
}

// AboveA11yNode ensures that the element returned must be above the element returned by the a11y node finder on the screen.
func (f *Finder) AboveA11yNode(other *nodewith.Finder) *Finder {
	return f.a11yNodeConstraintBuilder(other, above, func(r coords.Rect) int { return r.Top })
}

// BelowPx ensures that the element returned must be below px pixels on the screen.
func (f *Finder) BelowPx(px int) *Finder {
	return f.pxConstraintBuilder(below(px))
}

// BelowDp ensures that the element returned must be below dp display pixels on the screen.
func (f *Finder) BelowDp(dp int) *Finder {
	return f.dpConstraintBuilder(dp, below)
}

// Below ensures that the element returned must be below the element returned by the finder on the screen.
func (f *Finder) Below(other *Finder) *Finder {
	return f.constraintBuilder(other, below, func(l Location) int { return l.Bottom() })
}

// BelowA11yNode ensures that the element returned must be below the element returned by the a11y node finder on the screen.
func (f *Finder) BelowA11yNode(other *nodewith.Finder) *Finder {
	return f.a11yNodeConstraintBuilder(other, below, func(r coords.Rect) int { return r.Bottom() })
}

// LeftOfPx ensures that the element returned must be left of px pixels on the screen.
func (f *Finder) LeftOfPx(px int) *Finder {
	return f.pxConstraintBuilder(leftOf(px))
}

// LeftOfDp ensures that the element returned must be left of dp display pixels on the screen.
func (f *Finder) LeftOfDp(dp int) *Finder {
	return f.dpConstraintBuilder(dp, leftOf)
}

// LeftOf ensures that the element returned must be left of the element returned by the finder on the screen.
func (f *Finder) LeftOf(other *Finder) *Finder {
	return f.constraintBuilder(other, leftOf, func(l Location) int { return l.Left })
}

// LeftOfA11yNode ensures that the element returned must be left of the element returned by the a11y node finder on the screen.
func (f *Finder) LeftOfA11yNode(other *nodewith.Finder) *Finder {
	return f.a11yNodeConstraintBuilder(other, leftOf, func(r coords.Rect) int { return r.Left })
}

// RightOfPx ensures that the element returned must be right of px pixels on the screen.
func (f *Finder) RightOfPx(px int) *Finder {
	return f.pxConstraintBuilder(rightOf(px))
}

// RightOfDp ensures that the element returned must be right of dp display pixels on the screen.
func (f *Finder) RightOfDp(dp int) *Finder {
	return f.dpConstraintBuilder(dp, rightOf)
}

// RightOf ensures that the element returned must be right of the element returned by the finder on the screen.
func (f *Finder) RightOf(other *Finder) *Finder {
	return f.constraintBuilder(other, rightOf, func(l Location) int { return l.Right() })
}

// RightOfA11yNode ensures that the element returned must be right of the element returned by the a11y node finder on the screen.
func (f *Finder) RightOfA11yNode(other *nodewith.Finder) *Finder {
	return f.a11yNodeConstraintBuilder(other, rightOf, func(r coords.Rect) int { return r.Right() })
}

// locationPx resolves the UI detection request and stores the bounding boxes of the matching element in pixels.
func (f *Finder) locationPx(ctx context.Context, uda *Context, scaleFactor float64) (*Location, error) {
	// Take the screenshot depending on the provided strategy.
	var screenshot image.Image
	var err error
	boundingBox := coords.Rect{Left: 0, Top: 0, Width: maxScreenSizePx, Height: maxScreenSizePx}

	for _, constraint := range f.constraints {
		rect, err := constraint(ctx, uda, scaleFactor)
		if err != nil {
			return nil, errors.Wrap(err, "failed to find sub-element bounding box")
		}
		boundingBox = boundingBox.Intersection(*rect)
	}

	switch uda.screenshotStrategy {
	case StableScreenshot:
		screenshot, err = takeStableScreenshot(ctx, uda.tconn, uda.pollOpts, boundingBox, scaleFactor, uda.disableDynamicElementMasking)
		if err != nil {
			return nil, errors.Wrap(err, "failed to take stable screenshot")
		}
	case ImmediateScreenshot:
		screenshot, err = takeScreenshot(ctx, uda.tconn, boundingBox, scaleFactor, uda.disableDynamicElementMasking)
		if err != nil {
			return nil, errors.Wrap(err, "failed to take screenshot")
		}
	default:
		return nil, errors.New("invalid screenshot strategy")
	}

	screenshotPng, err := encodePNG(screenshot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to encode screenshot as a png")
	}

	failure := func(err error) (*Location, error) {
		// Save the screenshot if the test fails to find an element.
		if err := saveBytesImageToOutput(ctx, screenshotPng, screenshotFile); err != nil {
			testing.ContextLogf(ctx, "INFO: couldn't save the screenshot to %s for the failed UI detection: %s", screenshotFile, err)
		}
		return nil, err
	}

	response, err := uda.detector.sendDetectionRequest(ctx, screenshotPng, f.request, uda.screenshotResizingStrategy, testMetadata(ctx, uda), uda.saveResizedScreenshots)
	if err != nil {
		return failure(errors.Wrap(err, "failed to resolve the UI detection request"))
	}

	var locations []Location
	for _, location := range response.BoundingBoxes {
		if f.exactMatch && !strings.EqualFold(location.GetText(), f.desc) {
			continue
		}
		locations = append(
			locations,
			Location{
				Rect: coords.NewRectLTRB(
					boundingBox.Left+int(location.GetLeft()),
					boundingBox.Top+int(location.GetTop()),
					boundingBox.Left+int(location.GetRight()),
					boundingBox.Top+int(location.GetBottom())),
				Text: location.GetText(),
			})
	}

	saveDebugImages(ctx, screenshot, locations, f.desc)
	handleResizedScreenshots(ctx, uda, response.ResizingScaleFactor, response.TransformedImagePng, locations, f.desc)

	numMatches := len(locations)
	switch {
	case numMatches == 0:
		return failure(errors.New(ErrNotFound))
	case numMatches == 1:
		if f.nth > 0 {
			return failure(errors.Errorf("%s: found only one element, but want the %d-th one", ErrNthNotFound, f.nth))
		}
		return &locations[0], nil
	default: // case numMatches > 1.
		if f.nth < 0 {
			return failure(errors.Errorf("%s: found %d elements. If it is expected, consider using First() or Nth()", ErrMultipleMatch, numMatches))
		}
		if f.nth > numMatches-1 {
			return failure(errors.Errorf("%s: found %d elements, but want the %d-th one", ErrNthNotFound, numMatches, f.nth))
		}
		return &locations[f.nth], nil
	}
}

func handleResizedScreenshots(ctx context.Context, uda *Context, resizingScaleFactor float32, transformedImagePng []byte, locations []Location, desc string) {
	if resizingScaleFactor == 1.0 {
		// Resizing not used.
		return
	}

	if uda.screenshotResizingStrategy == ResizeAsFallback {
		testing.ContextLogf(ctx,
			"INFO: resized screenshot by %.1fx looking for element %q, as it was not detected on the original screenshot",
			resizingScaleFactor,
			desc,
		)
	}

	if uda.saveResizedScreenshots && len(transformedImagePng) == 0 {
		testing.ContextLog(ctx, "INFO: a resized debug image was requested, but was not returned. Image size may have been too large")
		return
	} else if !uda.saveResizedScreenshots || len(transformedImagePng) == 0 {
		// No resized screenshot to save.
		return
	}

	transformedImage, err := decodePNG(transformedImagePng)
	if err != nil {
		testing.ContextLogf(ctx, "INFO: couldn't save debug screenshots. Failed to decode transformed image: %s", err)
		return
	}

	// Scale the bounding boxes to the resized image.
	var scaledLocations []Location
	for _, loc := range locations {
		scaledLocations = append(scaledLocations, loc.withScale(resizingScale))
	}
	locations = scaledLocations

	saveDebugImages(ctx, transformedImage, locations, desc+"-resized")
}
