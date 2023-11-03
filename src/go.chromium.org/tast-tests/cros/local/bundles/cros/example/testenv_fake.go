// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TestenvFake,
		Desc:         "Example that set up the fake test environments",
		Contacts:     []string{"cros-ufo-testing@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{},  // manual
	})
}

func TestenvFake(ctx context.Context, s *testing.State) {
	env, err := testenv.NewFakeEnv(ctx)
	if err != nil {
		s.Fatal("Failed to set up the fake env: ", err)
	}
	if err := env.SetUp(ctx); err != nil {
		s.Fatal("Failed to turn up the fake env: ", err)
	}
	defer env.TearDown(ctx)
}
