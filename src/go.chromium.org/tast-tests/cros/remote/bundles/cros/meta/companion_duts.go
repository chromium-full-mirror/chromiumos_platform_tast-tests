// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CompanionDUTs,
		Desc:         "Ensure DUT and companion DUTs are accessible",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:hw_agnostic"},
	})
}

// CompanionDUTs ensures DUT and companion DUTs are accessible in test.
func CompanionDUTs(ctx context.Context, s *testing.State) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	companionDUT := s.CompanionDUT("cd1")
	if companionDUT == nil {
		s.Fatal("Failed to get companion DUT cd1")
	}
	companionCl, err := rpc.Dial(ctx, companionDUT, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the companion DUT: ", err)
	}
	defer companionCl.Close(ctx)
}
