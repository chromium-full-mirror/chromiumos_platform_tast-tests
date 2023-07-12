// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast/core/errors"
)

// CopyRemoteFile saves the remote DUT file to the folder on the local host machine.
func CopyRemoteFile(ctx context.Context, fs *dutfs.Client, remoteFilePath, localFilePath string) (string, error) {
	readData, err := fs.ReadFile(ctx, remoteFilePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read files on server")
	}

	savedFilePath := filepath.Join(localFilePath, filepath.Base(remoteFilePath))
	err = ioutil.WriteFile(savedFilePath, readData, 0777)
	if err != nil {
		return "", errors.Wrap(err, "failed to save record file")
	}
	return savedFilePath, nil
}

// ValidateImgColor validate the local host image matches the specified color.
func ValidateImgColor(ctx context.Context, localFilePath, color string) error {
	expectedColor, err := GetColor(localFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to get image color")
	}
	if expectedColor != color {
		return errors.Wrap(err, "image did not have matching dominant color")
	}
	return nil
}

// ValidateVideoColor divide the local host video into individual frames per second and verify if they match the specified color.
func ValidateVideoColor(ctx context.Context, localVideoPath, localDir string) error {
	localScreenVideoDir := filepath.Join(filepath.Dir(localVideoPath), "frame_of_video")

	if err := os.Mkdir(localScreenVideoDir, 0750); err != nil {
		return errors.Wrap(err, "failed to create the directory")
	}

	localScreenVideoPath := filepath.Join(localScreenVideoDir, "out-%03d.jpg")
	if err := testexec.CommandContext(ctx, "ffmpeg", "-i", localVideoPath, "-vf", "fps=1", localScreenVideoPath).Run(); err != nil {
		return errors.Wrap(err, "failed to take screenshot on video every second")
	}

	files, err := os.ReadDir(localScreenVideoDir)
	if err != nil {
		return errors.Wrap(err, "failed to read the camera directory")
	}

	for _, file := range files {
		localFilePath := filepath.Join(localScreenVideoDir, file.Name())
		if err := ValidateImgColor(ctx, localFilePath, "red"); err != nil {
			return err
		}
	}

	return nil
}

// GenerateImage generates solid color within the given width and height image.
func GenerateImage(width, height int, pixelColor color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, pixelColor)
		}
	}
	return img
}

// WriteImageOnDUT writes image data to the named file on th DUT.
func WriteImageOnDUT(ctx context.Context, fs *dutfs.Client, img image.Image, imgFile string) error {
	buff := new(bytes.Buffer)
	if err := png.Encode(buff, img); err != nil {
		return errors.Wrap(err, "failed to encode PNG file")
	}

	if err := fs.WriteFile(ctx, imgFile, buff.Bytes(), 0644); err != nil {
		return errors.Wrap(err, "failed to write image to file")
	}

	exists, err := fs.Exists(ctx, imgFile)
	if err != nil {
		return errors.Wrapf(err, "error checking that file exists: %q", imgFile)
	}
	if !exists {
		return errors.Errorf("file does not exist: %q", imgFile)
	}
	return nil
}
