// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCFixture,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Demonstrates ARC fixture",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_p"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
	})
}

func ARCFixture(ctx context.Context, s *testing.State) {
	pd := s.FixtValue().(*arc.PreData)
	a := pd.ARC

	// Ensures package manager service is running by checking the existence of the "android" package.
	pkgs, err := a.InstalledPackages(ctx)
	if err != nil {
		s.Fatal("Getting installed packages failed: ", err)
	}
	const want = "android"
	if _, ok := pkgs[want]; !ok {
		s.Fatalf("Package %q not found: %q", want, pkgs)
	}
}
