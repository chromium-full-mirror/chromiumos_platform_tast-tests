// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

// WebSourceType defines the type of web source.
// Contains "external", "google", "local". Usually the default is "google".
type WebSourceType string

const (
	// ExternalWebSource is the external website option of web source.
	ExternalWebSource WebSourceType = "external"
	// GoogleWebSource is the google website option of web source.
	GoogleWebSource WebSourceType = "google"
	// LocalWebSource is the local website option of web source.
	LocalWebSource WebSourceType = "local"
)
