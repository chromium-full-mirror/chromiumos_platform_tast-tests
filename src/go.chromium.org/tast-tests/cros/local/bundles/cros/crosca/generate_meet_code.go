// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package crosca contains a collections of performance tests
package crosca

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// Default runtime should be about 5 mins although manual runtime is expected
// to be about 30 mins controlled by variable
const (
	generateMeetCodeTestTimeout = time.Hour
)

type generateMeetCodeTest struct {
	bots        []int
	botsOptions []bond.AddBotsOption // Customizes the meeting participant bots.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         GenerateMeetCode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Host a Google Meet video conference and type in the chat window",
		Contacts: []string{
			"chromeos-competitive-analysis@google.com",
			"williskung@google.com",
		},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > crosca > Automation
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"crosca.GenerateMeetCode.bond_credentials",
		},
		Params: []testing.Param{
			{
				Name:    "2p",
				Timeout: generateMeetCodeTestTimeout,
				Val: generateMeetCodeTest{
					bots:        []int{1},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
			{
				Name:    "16p",
				Timeout: generateMeetCodeTestTimeout,
				Val: generateMeetCodeTest{
					bots:        []int{15},
					botsOptions: []bond.AddBotsOption{bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv")},
				},
			},
		},
	})
}

// GenerateMeetCode measures the performance of critical user journeys for Google Meet.
// This tast test is structured to create similar workload compared to the corresponding
// Windows python test.
func GenerateMeetCode(ctx context.Context, s *testing.State) {
	const botDurationMinutes = 45

	meetingCode := ""

	// Wait a maximum of 100s for all the bots to be added
	const addBotTimeout = 100 * time.Second
	meet := s.Param().(generateMeetCodeTest)
	if len(meet.bots) == 0 {
		s.Fatal("Must have at least 1 bot count")
	}
	if meet.bots[0] == 0 {
		s.Fatal("First bot count must have at least 1 bot, to add the spotlight bot")
	}
	creds := s.RequiredVar("crosca.GenerateMeetCode.bond_credentials")
	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		s.Fatal("Failed to create a bond client: ", err)
	}
	defer bc.Close()
	meetingCode, err = bc.CreateConference(ctx)
	if err != nil {
		s.Fatal("Failed to create a conference room: ", err)
	}
	s.Log("Created a room with the code ", meetingCode)
	// Variable addBotTimeout(100s) would allow 3 bond.longerSendTimeout(30s) attempts
	// to request the bond server to add bots.
	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()
	// Create a bot with spotlight layout to request HD video.
	typingTimeout := 45 * time.Minute
	spotlightBotList, _, err := bc.AddBots(sctx, meetingCode, 1, typingTimeout+botDurationMinutes*time.Minute, append(meet.botsOptions, bond.WithLayout("SPOTLIGHT"))...)
	if err != nil {
		s.Fatal("Failed to create bot with spotlight layout: ", err)
	}
	if len(spotlightBotList) != 1 {
		s.Fatalf("Unexpected number of bots with spotlight layout successfully started; got %d, expected 1", len(spotlightBotList))
	}
	// Keep track of how many bots are already in the call, so we can add the
	// right number of bots later in the test.
	botsInCall := 1
	addBots := func(ctx context.Context, numBots int) error {
		sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
		defer cancel()
		testing.ContextLogf(ctx, "Adding %d bots to the call", numBots)
		if numBots == 0 {
			return nil
		}
		wait := 100 * time.Millisecond
		for i := 0; i < 3; i++ {
			// GoBigSleepLint: A short sleep before next call to Bond API.
			if err := testing.Sleep(ctx, wait); err != nil {
				s.Errorf("Failed to sleep for %v: %v", wait, err)
			}
			// Exponential backoff. The wait time is 0.1s, 1s and 10s before each retry.
			wait *= 10
			// Add 45 (botDurationMinutes) minutes to the bot duration, to ensure that the bots stay long
			// enough for the test to get info from chrome://webrtc-internals.
			botList, numFailures, err := bc.AddBots(sctx, meetingCode, numBots, typingTimeout+botDurationMinutes*time.Minute, append(meet.botsOptions, bond.WithDefaultVideo("jamboard_three_close_video_hd.1280_720.yuv"))...)
			if err != nil {
				s.Fatalf("Failed to create %d bots: %v", numBots, err)
			}
			s.Logf("%d bots started, %d bots failed", len(botList), numFailures)
			if numFailures == 0 {
				break
			}
			numBots -= len(botList)
		}
		return nil
	}
	numBotsToAdd := meet.bots[0] - botsInCall
	if err := addBots(ctx, numBotsToAdd); err != nil {
		s.Fatalf("Failed to initially add %d bots: %v", numBotsToAdd, err)
	}
	botsInCall += numBotsToAdd
	s.Log("MeetingCode: ", meetingCode, " Number of bots: ", botsInCall)

	// Shorten context to allow for cleanup. Reserve one minute in case of power test.
	ctx, cancel = ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
}
