// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func getBits(value uint32, startBit, endBit int) uint32 {
	mask := (uint32(1) << (endBit - startBit + 1)) - 1 // Create a mask
	return (value >> uint32(startBit)) & mask          // Shift and apply the mask
}

func getMEStatus(ctx context.Context, s *testing.FixtState, board string) (uint32, error) {

	if board != "Hatch" && board != "Octopus" {
		return 0, errors.New("unsupported board")
	}

	var re *regexp.Regexp
	if board == "Hatch" {
		// ME: HFSTS1                  : 0x90000245
		re = regexp.MustCompile(`ME: HFSTS1\s+:\s+(0x[0-9a-fA-F]+)`)
	}
	if board == "Octopus" {
		// CSE FWSTS1: 0x80000045
		re = regexp.MustCompile(`CSE FWSTS1:\s+(0x[0-9a-fA-F]+)`)
	}

	b, err := s.DUT().Conn().CommandContext(ctx, "/usr/bin/cbmem", "-1").Output()
	if err != nil {
		return 0, errors.Wrap(err, "cbmem -1 failed")
	}
	match := re.FindStringSubmatch(string(b))
	if match == nil {
		return 0, errors.New("failed to parse ME status from cbmem -1")
	}
	hexValue := match[1]
	MEStatus, err := strconv.ParseUint(hexValue, 0, 32)
	s.Logf("ME status: 0x%X", MEStatus)

	return uint32(MEStatus), nil
}

func hasValidMEStatus(ctx context.Context, s *testing.FixtState) bool {

	// 'cros_config /identity platform-name' works for Hatch and Octopus, but may return 'Generic' for boards like Brya.
	boardInByte, err := s.DUT().Conn().CommandContext(ctx, "cros_config", "/identity", "platform-name").Output()
	if err != nil {
		// Assume it's valid to reduce the test failure rate.
		s.Log("cros_config  platform-name failed: ", err)
		return true
	}
	board := string(boardInByte)

	s.Logf("board: %s", board)

	// b/320175503: GLK/CML devices have un-fixable ME issues.
	if board != "Hatch" && board != "Octopus" {
		return true
	}

	MEStatus, err := getMEStatus(ctx, s, board)

	if err != nil {
		// Assume it's valid until cbmem parsing failure confirms an audio DSP issue.
		s.Log("Get MEStatus failed: ", err)
		return true
	}

	if getBits(MEStatus, 0, 3) != 5 || getBits(MEStatus, 12, 15) != 0 {
		s.Logf("ME is either not in normal working state: 0x%X or has hit some error: 0x%X", getBits(MEStatus, 0, 3), getBits(MEStatus, 12, 15))
		return false
	}

	return true
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "rebootForAudioDSPFixture",
		Desc: "Reboots the DUT if the Audio DSP failed; Audio test Only; DO NOT USE for other tests",
		Contacts: []string{
			"judyhsiao@google.com",
			"chromeos-audio-sw@google.com",
		},
		SetUpTimeout: 5 * time.Minute,
		Impl:         &rebootForAudioDSPFixtureImpl{},
	})
}

type rebootForAudioDSPFixtureImpl struct{}

func (f *rebootForAudioDSPFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	for reboot := 0; reboot < 2; reboot++ {
		if hasValidMEStatus(ctx, s) {
			return nil
		}
		s.Log("Performing cold reset due to invalid ME status")
		if err := s.DUT().Conn().CommandContext(ctx, "ectool", "reboot_ec", "cold", "at-shutdown").Run(); err != nil {
			s.Fatal("Failed to execute ectool reboot_ec cmd: ", err)
		}

		if err := s.DUT().Conn().CommandContext(ctx, "shutdown", "-h", "now").Run(); err != nil {
			s.Fatal("Failed to execute shutdown command: ", err)
		}
		if err := s.DUT().WaitConnect(ctx); err != nil {
			s.Fatal("Failed to wake up DUT: ", err)
		}
	}
	return nil
}

func (f *rebootForAudioDSPFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *rebootForAudioDSPFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootForAudioDSPFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *rebootForAudioDSPFixtureImpl) Reset(ctx context.Context) error {
	return nil
}
