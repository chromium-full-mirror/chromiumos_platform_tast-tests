// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:            DDDTestTwo,
		Desc:            "Example for 3D expression. Always passes",
		Contacts:        []string{"chromeos-test-platform-team@google.com", "dbeckett@google.com"},
		BugComponent:    "b:296602715", // ChromeOS > Infra > Test Scheduling
		Attr:            []string{"group:ddd_test_group"},
		VariantCategory: `{"name": "HWID:touchpad_field_vendor_id:distinct_values"}`,
	})
}

func DDDTestTwo(ctx context.Context, s *testing.State) {
	// No errors means the test passed.
}
