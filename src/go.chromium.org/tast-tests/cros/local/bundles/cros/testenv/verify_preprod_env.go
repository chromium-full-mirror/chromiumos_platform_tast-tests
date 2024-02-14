// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyPreprodEnv,
		Desc:         "Example that set up the preprod test environments",
		Contacts:     []string{"cros-ufo-testing@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{},  // manual
	})
}

func VerifyPreprodEnv(ctx context.Context, s *testing.State) {
	env, err := testenv.NewPreprodEnv(ctx)
	if err != nil {
		s.Fatal("Failed to init the preprod env: ", err)
	}
	if err := env.Start(ctx); err != nil {
		s.Fatal("Failed to set up the preprod env: ", err)
	}
	defer env.Close(ctx)
}
