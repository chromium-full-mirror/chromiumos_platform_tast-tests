// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetTimer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests Assistant to set timer",
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Contacts:     []string{"assistive-eng@google.com"},
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      chrome.GAIALoginTimeout + time.Minute,
		Fixture:      "assistantWithGaia",
	})
}

func isExpectedSetTimerResponse(text string) bool {
	expectedResponses := []string{
		"Starting… now.",
		"Starting now.",
		"And we're starting… now.",
		"And that's starting… now.",
	}

	matched := false
	for _, expected := range expectedResponses {
		if strings.Contains(text, expected) {
			matched = true
			break
		}
	}
	return matched
}

func SetTimer(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	const duration = 10
	queryString := fmt.Sprintf("Set a timer for %v seconds", duration)

	s.Log(queryString)
	queryStatus, err := assistant.SendTextQuery(ctx, tconn, queryString)
	if err != nil {
		s.Fatal("Failed to set a timer: ", err)
	}
	s.Log("Verifying the set a timer response")
	text := queryStatus.QueryResponse.TEXT
	if !isExpectedSetTimerResponse(text) {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	s.Log("Pause the timer")
	queryStatus, err = assistant.SendTextQuery(ctx, tconn, "Pause the timer")
	if err != nil {
		s.Fatal("Failed to pause a timer: ", err)
	}
	s.Log("Verifying the pause a timer response")
	text = queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "paused") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	s.Log("Resume the timer")
	queryStatus, err = assistant.SendTextQuery(ctx, tconn, "Resume the timer")
	if err != nil {
		s.Fatal("Failed to resume the timer: ", err)
	}
	s.Log("Verifying the resume a timer response")
	text = queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "Resuming now") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	s.Log("Cancel the timer")
	queryStatus, err = assistant.SendTextQuery(ctx, tconn, "Cancel the timer")
	if err != nil {
		s.Fatal("Failed to cancel a timer: ", err)
	}
	s.Log("Verifying the cancel a timer response")
	text = queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "canceled") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}

	s.Log("Show my timers")
	queryStatus, err = assistant.SendTextQuery(ctx, tconn, "Show my timers")
	if err != nil {
		s.Fatal("Failed to show timers: ", err)
	}
	s.Log("Verifying the show timers response")
	text = queryStatus.QueryResponse.TEXT
	if !strings.Contains(text, "It looks like you don't have any timers set at the moment.") {
		s.Fatal("TEXT response doesn't contain the expected response of the text query: ", text)
	}
}
