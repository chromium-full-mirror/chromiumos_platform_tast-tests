// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/media/imgcmp"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SaveCropScreenshot saves cropped screenshot with given bounds.
func SaveCropScreenshot(cr *chrome.Chrome, bounds coords.Rect, dir, fileName string) action.Action {
	return func(ctx context.Context) error {
		img, err := screenshot.GrabAndCropScreenshot(ctx, cr, bounds)
		if err != nil {
			return errors.Wrap(err, "failed to grab screenshot")
		}
		testing.ContextLogf(ctx, "Save crop screenshot to %s", filepath.Join(dir, fileName))
		if err := screenshot.SaveImageToFile(img, dir, fileName)(ctx); err != nil {
			return errors.Wrapf(err, "can't save file: %s", fileName)
		}
		return nil
	}
}

// VerifyTwoImagesSimilarity verifies two images are the same or not.
// If expectedSame is true, it will return error if the images are not the same.
// If expectedSame is false, it will return error if the images are the same.
// TODO(b/366075152): Return diff from VerifyTwoImagesSimilarity.
func VerifyTwoImagesSimilarity(dir, fileNameA, fileNameB string, expectedSame bool) action.Action {
	return func(ctx context.Context) error {
		if expectedSame {
			testing.ContextLogf(ctx, "Start to verify %s and %s are the same", fileNameA, fileNameB)
		} else {
			testing.ContextLogf(ctx, "Start to verify %s and %s are not the same", fileNameA, fileNameB)
		}
		pathA := filepath.Join(dir, fileNameA)
		pathB := filepath.Join(dir, fileNameB)

		imgA, err := decodePNGFile(pathA)
		if err != nil {
			return errors.Wrap(err, "failed to decode the png file")
		}

		imgB, err := decodePNGFile(pathB)
		if err != nil {
			return errors.Wrap(err, "failed to decode the png file")
		}

		const roundingErrorThreshold = 1
		diff, err := imgcmp.CountDiffPixels(imgA, imgB, roundingErrorThreshold)
		if err != nil {
			return errors.Wrap(err, "failed to count diff pixels")
		}
		testing.ContextLogf(ctx, "Diff pixels between %s and %s: %d", fileNameA, fileNameB, diff)

		const similarityThreshold = 30
		// Expect the images are the same.
		if expectedSame && diff > similarityThreshold {
			return errors.Wrap(err, "the images are not the same")
		}
		// Expect the images are not the same.
		if !expectedSame && diff == 0 {
			return errors.New("the images are the same")
		}
		return nil
	}
}

func decodePNGFile(filePath string) (image.Image, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the png file: "+filePath)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode the png file: "+filePath)
	}
	return img, nil
}

// WaitUntilIconExists waits until the icon exists.
func WaitUntilIconExists(ud *uidetection.Context, dir, fileName string) action.Action {
	iconPath := filepath.Join(dir, fileName)
	expectedIcon := uidetection.CustomIcon(iconPath)
	return ud.WaitUntilExists(expectedIcon)
}
