// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    DrmCounterVc,
		Desc:    "Verify vendor commands for reading and incrementing the DRM counter",
		Timeout: 5 * time.Minute,
		Contacts: []string{
			"vbendeb@google.com",
			"cros-hwsec@google.com",
		},
		BugComponent: "b:452450415",
		Attr: []string{
			"group:gsc",
			"gsc_dt_shield", "gsc_ot_shield",
			"gsc_image_ti50",
			"gsc_nightly",
		},
		Fixture: fixture.GSCOpenCCD,
	})
}

func readWidevineGscCounterSeed(s *testing.State, tpm *utils.TpmHelper) [32]byte {

	seed, err := tpm.ReadWidevineRot(32, 64)

	if err != nil {
		s.Fatal("Failed to read  GSC seed: ", err)
	}

	return [32]byte(seed.Data)
}

func readWideVineGscCounter(s *testing.State, tpm *utils.TpmHelper, challenge, seed [32]byte, inc bool, iter uint) uint32 {
	t := fmt.Sprintf("Iteration %d Drm counter response", iter)
	response, err := tpm.TpmvDrmCounter(challenge, inc)
	if err != nil {
		s.Fatalf("%s attempt to read failed with err: %v", t, err)
	}

	var command uint8
	if inc {
		command = 1
	} else {
		command = 0
	}

	/* The response must be of the following structure (ranges inclusive)
	| offset  |        field      |                 comment                        |
	----------+-------------------+------------------------------------------------|
	|   0     | structure version |                              hardcoded to zero |
	|   1     |           command |                         0 - read, 1 -increment |
	|   2     |        counter_id |                 future enhancement currently 0 |
	|   3     |          reserved |                future enhancement, currently 0 |
	|  4..35  |         challenge |                      copy of the request value |
	| 36..39  |           counter |                       the actual counter value |
	| 40..71  |              hmac | Hmac of all previous bytes by Gsc Counter Seed |

	*/

	if len(response) != 72 {
		s.Fatalf("%s size %d instead of 72", t, len(response))
	}
	if response[0] != 0 {
		s.Fatalf("%s structure version %d instead of 0", t, response[0])

	}
	if response[1] != command {
		s.Fatalf("%s command %d instead of %d", t, response[1], command)
	}
	if response[2] != 0 {
		s.Fatalf("%s counter id %d instead of 0", t, response[2])
	}
	if response[3] != 0 {
		s.Fatalf("%s reserved field %d instead of 0", t, response[3])
	}

	if !bytes.Equal(response[4:36], challenge[:]) {
		s.Fatalf("%s challenge field %x instead of %x", t, response[4:36], challenge)
	}

	h := hmac.New(sha256.New, seed[:])
	h.Write(response[:40])
	hmac := h.Sum(nil)

	if !bytes.Equal(hmac, response[40:]) {
		s.Fatalf("%s hmac mismatch %x instead of %x", t, response[40:], hmac)
	}

	// Return the actual counter value.
	return binary.BigEndian.Uint32(response[36:40])
}

func DrmCounterVc(ctx context.Context, s *testing.State) {

	// 1. Setup Board connection (SPI/I2C/GPIO)
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)

	// Any 32 byte value will do, define something to make sure the test is reproducible.
	challenge := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	counterSeed := readWidevineGscCounterSeed(s, tpm)

	// Read the counter once to have the base value to compare against.
	counter := readWideVineGscCounter(s, tpm, challenge, counterSeed, false, 0) // use distinctive bogus iteration value

	// Now, let's read the counter multiple times, with and without
	// increment, with AP reset and total reset, with different challenge
	// values, total 16 iterations.
	// The resets should not affect the value of the counter, only the
	// reading with increment should.
	s.Log("Starting 16 cycles of reading DRM counter")
	for iter := uint(0); iter < 16; iter++ {
		inc := iter&3 == 1
		newCounter := readWideVineGscCounter(s, tpm, challenge, counterSeed, inc, iter)
		if inc {
			if newCounter != counter+1 {
				s.Fatalf("Counter increment failed on iteration %d", iter)
			}
			counter++
		} else {
			if newCounter != counter {
				s.Fatalf("Counter read mismatch on iteration %d, expected %d, got %d", iter, counter, newCounter)
			}
		}
		challenge[0]++ // Let's make sure the challenge value changes constantly.
		if iter == 8 {
			// Restart the GSC once during the test
			tpm = b.ResetAndTpmStartup(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
			counterSeed = readWidevineGscCounterSeed(s, tpm)
			continue

		}
		if iter&7 == 4 {
			// Toggle AP reset twice during the test.
			b.SimulateApS3(ctx, tpm)
			counterSeed = readWidevineGscCounterSeed(s, tpm)
		}
	}
}
