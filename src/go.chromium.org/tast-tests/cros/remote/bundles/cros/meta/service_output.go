// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/services/cros/meta"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServiceOutput,
		Desc:         "Ensure OutDir works for gRPC services",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		ServiceDeps:  []string{"tast.cros.meta.FileOutputService"},
	})
}

func ServiceOutput(ctx context.Context, s *testing.State) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	want := map[string]string{
		"a.txt":     "foo",
		"dir/b.txt": "bar",
	}

	oc := meta.NewFileOutputServiceClient(cl.Conn)
	if _, err := oc.SaveOutputFiles(ctx, &meta.SaveOutputFilesRequest{Files: want}); err != nil {
		s.Fatal("SaveOutputs RPC failed: ", err)
	}

	got, err := testutil.ReadFiles(s.OutDir())
	if err != nil {
		s.Fatal("Failed to read OutDir: ", err)
	}

	if diff := cmp.Diff(got, want); diff != "" {
		s.Error("OutDir content mismatch: ", diff)
	}
}
