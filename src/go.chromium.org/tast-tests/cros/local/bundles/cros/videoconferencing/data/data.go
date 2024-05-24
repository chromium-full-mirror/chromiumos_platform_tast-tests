// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package data provides file names for test data.
package data

const (
	// SpeechInputFile is a WAVE file containing 2.090667 seconds of speech of the
	// English word "hello".
	SpeechInputFile = "vc_tester/voice_en_hello.wav"

	// BackgroundImageDirname is the directory to put the background images.
	BackgroundImageDirname = "custom-camera-backgrounds/original"
	// BackgroundImageJpg is the background image filename.
	BackgroundImageJpg = "3162101071.jpg"
	// BackgroundMetadata is the metadata file name of the background image.
	BackgroundMetadata = "3162101071.jpg.metadata"

	// VcAppHTML is the html file for VcTester.
	VcAppHTML = "vc_tester/popup.html"
	// VcAppJs used inside VcAppHTML
	VcAppJs = "vc_tester/popup.js"
	// VcAppIcon is the icon for VcAppHTML
	VcAppIcon = "vc_tester/camera.png"
	// VcAppManifest is need to treat VcTester as a pwa.
	VcAppManifest = "vc_tester/manifest.json"
)
