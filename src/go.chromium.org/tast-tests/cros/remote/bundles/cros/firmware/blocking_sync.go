// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: BlockingSync,
		Desc: "Verifies that firmware tests can remotely perform a blocking sync on the DUT",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		Attr:         []string{"group:firmware", "firmware_smoke"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func BlockingSync(ctx context.Context, s *testing.State) {
	h := firmware.NewHelper(s.DUT(), s.RPCHint(), "", "", "", "", "", "")
	defer func() {
		if err := h.Close(ctx); err != nil {
			s.Fatal("Closing helper: ", err)
		}
	}()
	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Requiring RPC utils: ", err)
	}

	if _, err := h.RPCUtils.BlockingSync(ctx, &empty.Empty{}); err != nil {
		s.Fatal("During BlockingSync: ", err)
	}
}
