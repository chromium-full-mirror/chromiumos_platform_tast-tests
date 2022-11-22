// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package signedamdfw contains functionality shared by tests that
// deal with one or both SIGNED_AMDFW sections in FMAP.
package signedamdfw

import (
	"context"

	"chromiumos/tast/remote/firmware"
	pb "chromiumos/tast/services/cros/firmware"
	"chromiumos/tast/testing"
)

// CheckForSignedAMDFWSection checks whether the FMAP in the DUT contains SIGNED_AMDFW_A/B sections
func CheckForSignedAMDFWSection(ctx context.Context, s *testing.State, h *firmware.Helper) bool {
	parsedFmap, err := h.BiosServiceClient.ParseFMAP(ctx, &pb.FMAP{Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatal("Failed to save current fw image: ", err)
	}

	for _, v := range parsedFmap.Fmap {
		if v.Section == pb.ImageSection_SignedAMDFWAImageSection || v.Section == pb.ImageSection_SignedAMDFWBImageSection {
			return true
		}
	}
	return false
}
