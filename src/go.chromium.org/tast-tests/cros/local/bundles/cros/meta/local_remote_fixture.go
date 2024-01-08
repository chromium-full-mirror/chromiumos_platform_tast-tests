// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalRemoteFixture,
		Desc:         "Tests local tests can depend on remote fixtures",
		Contacts:     []string{"tast-core@google.com", "oka@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Fixture:      "metaRemote",
		Attr:         []string{"group:hw_agnostic"},
	})
}

func LocalRemoteFixture(ctx context.Context, s *testing.State) {
	s.Log("Hello test")
}
