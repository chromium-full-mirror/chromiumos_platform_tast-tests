// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/facegaze"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Facegaze,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Tests that the FaceGaze feature can be turned on and used",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1546021",
		Timeout:      10 * time.Minute,
		Attr:         []string{"group:mainline", "informational", "group:camera_dependent"},
		Data:         []string{facegaze.FakeCameraVideoFile720p},
		SoftwareDeps: []string{"chrome"},
	})
}

func Facegaze(ctx context.Context, s *testing.State) {
	driver, err := facegaze.SetUp(ctx, s.DataPath)
	if err != nil {
		s.Fatal("Failed to set up FaceGaze: ", err)
	}

	defer func() {
		if err := driver.TearDown(); err != nil {
			s.Fatal("Failed to tear down FaceGaze: ", err)
		}
	}()

	driver.Start()
}
