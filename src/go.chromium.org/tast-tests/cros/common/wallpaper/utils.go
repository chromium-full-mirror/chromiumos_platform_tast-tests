// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wallpaper

import "go.chromium.org/tast/core/testing"

// GooglePhotosAccountPoolVarName is the wallpaper googlePhotos account pool name.
const GooglePhotosAccountPoolVarName = "wallpaper.googlePhotosAccountPool"

const googlePhotosDMAAccountPoolVarName = "wallpaper.googlePhotosDMAAccountPool"

var googlePhotosAccountPoolVar = testing.RegisterVarString(
	GooglePhotosAccountPoolVarName,
	"",
	"It contains creds in wallpaper.googlePhotosAccountPool",
)

var googlePhotosDMAAccountPoolVar = testing.RegisterVarString(
	googlePhotosDMAAccountPoolVarName,
	"",
	"It contains creds in wallpaper.googlePhotosDMAAccountPool",
)

// GooglePhotosAccountPoolValue returns credentials from wallpaper.googlePhotosAccountPool.
func GooglePhotosAccountPoolValue() string {
	return googlePhotosAccountPoolVar.Value()
}

// GooglePhotosDMAAccountPoolValue returns credentials from wallpaper.googlePhotosDMAAccountPool.
func GooglePhotosDMAAccountPoolValue() string {
	return googlePhotosDMAAccountPoolVar.Value()
}
