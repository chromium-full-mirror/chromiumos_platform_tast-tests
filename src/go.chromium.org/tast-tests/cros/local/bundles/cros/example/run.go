// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Run,
		Desc:         "Subtest example, always fails",
		BugComponent: "b:1034522",
		Contacts:     []string{"tast-core@google.com", "vsavu@google.com"},
		Attr:         []string{"group:hw_agnostic"},
	})
}

func Run(ctx context.Context, s *testing.State) {
	s.Run(ctx, "ok", func(ctx context.Context, s *testing.State) {
		s.Log("ok")
	})

	s.Run(ctx, "error", func(ctx context.Context, s *testing.State) {
		s.Error("Here's an error")
	})

	s.Run(ctx, "fatal", func(ctx context.Context, s *testing.State) {
		s.Fatal("Here's a fatal error")
	})

	s.Run(ctx, "still-ok", func(ctx context.Context, s *testing.State) {
		s.Log("Still ok")
	})

	s.Run(ctx, "l1", func(ctx context.Context, s *testing.State) {
		s.Log("Level 1")

		s.Run(ctx, "l2", func(ctx context.Context, s *testing.State) {
			s.Log("Level 2")
		})
	})
}
