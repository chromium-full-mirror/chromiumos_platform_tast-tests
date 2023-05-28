// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/local/audio/
const (
	CrasStopped = "crasStopped"

	// Configure the ALSA loopback device for CRAS.
	AloopLoaded = "aloopLoaded"
	// Configure the ALSA loopback device as a stereo device for CRAS.
	StereoAloopLoaded = "stereoAloopLoaded"
	// Configure the ALSA loopback device for CRAS and stop UI.
	AloopLoadedWithoutUI = "aloopLoadedWithoutUI"
	// Configure the ALSA loopback device as a stereo device for CRAS and stop UI.
	StereoAloopLoadedWithoutUI = "stereoAloopLoadedWithoutUI"

	// For the testbed with Chameleon
	ChameleonAudioTestbed = "chameleonAudioTestbed"
)
