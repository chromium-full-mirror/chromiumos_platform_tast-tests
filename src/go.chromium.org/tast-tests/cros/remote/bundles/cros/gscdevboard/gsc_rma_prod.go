// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

// GSC only generates rma_auth challenges once every 10 seconds.
const rmaAuthDelay = 10 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCRMAProd,
		Desc:    "Verify the RMA Auth challenge uses the prod key",
		Timeout: time.Minute,
		Contacts: []string{
			"gsc-sheriff@google.com", // CrOS GSC Developers
			"mruthven@chromium.org",  // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_h1_shield", "gsc_dt_ab", "gsc_dt_shield", "gsc_he",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
	})
}

func GSCRMAProd(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s)
	defer i.Close(ctx)

	s.Log("(Re)starting GSC")
	b.ResetAndTpmStartup(ctx, i, ti50.CcdSuzyQ, ti50.FfClamshell)

	if err := i.WaitUntilBooted(ctx); err != nil {
		s.Fatal("Failed to boot GSC: ", err)
	}

	// Wait for the GSC rma_auth rate limit to pass.
	// GoBigSleepLint sleeping for known required period of time.
	if err := testing.Sleep(ctx, rmaAuthDelay); err != nil {
		s.Fatal("Failed to wait for rma_auth: ", err)
	}

	challenge, err := i.GetRmaAuth(ctx)
	if err != nil {
		s.Fatal("Error communicating with GSC: ", err)
	}
	s.Logf("challenge after 10s: %s", challenge)
	if strings.HasPrefix(challenge, "A") {
		s.Log("challenge used prod key")
	} else if strings.HasPrefix(challenge, "E") {
		s.Fatalf("Dev key challenge: %s", challenge)
	} else {
		s.Fatalf("Unknown challenge. Did not start with A or E: %s", challenge)
	}

	// GSC should rate limit the challenges.
	challenge, err = i.GetRmaAuth(ctx)
	if err != nil {
		s.Fatal("Error communicating with GSC: ", err)
	}
	s.Logf("challenge too soon: %s", challenge)
	if challenge != "" {
		s.Fatalf("Got a challenge after gsc reset %s", challenge)
	}

	// Wait for the GSC rma_auth rate limit to pass.
	// GoBigSleepLint sleeping for known required period of time.
	if err = testing.Sleep(ctx, rmaAuthDelay); err != nil {
		s.Fatal("Failed to wait for rma_auth: ", err)
	}

	// GSC should give another challenge after 10 seconds.
	challenge, err = i.GetRmaAuth(ctx)
	if err != nil {
		s.Fatal("Error communicating with GSC: ", err)
	}
	s.Logf("challenge after 10s: %s", challenge)
	if challenge == "" {
		s.Fatal("Did not get a challenge after waiting 10s")
	}
}
