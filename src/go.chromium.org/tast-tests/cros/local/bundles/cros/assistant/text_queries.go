// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TextQueries,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests Assistant basic functionality with text queries",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Fixture:      "assistant",
	})
}

func isExpected(text string) bool {
	expectedResponses := []string{
		"No one told me there would be a test",
		"You're coming in loud and clear",
		"debug OK",
		"I can assure you, this thing's on",
		"Is this thing on?"}

	matched := false
	for _, expected := range expectedResponses {
		if strings.Contains(text, expected) {
			matched = true
			break
		}
	}
	return matched
}

func TextQueries(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	s.Log("Sending text query to the Assistant")

	// As the query result will be parsed from the TEXT string.
	queryStatus, err := assistant.SendTextQuery(ctx, tconn, "test")
	if err != nil {
		s.Fatal("Failed to get Assistant text query response: ", err)
	}

	s.Log("Verifying the text query result")
	text := queryStatus.QueryResponse.TEXT
	if text == "" {
		s.Fatal("No TEXT response sent back from Assistant")
	}

	// The TEXT string should contain one of the expected responses of the text query.
	if !isExpected(text) {
		// Writes the TEXT response to logName file for debugging if no matching results found.
		const logName = "text_query_text_response.txt"
		s.Log("No matching results found. Try to log the TEXT response to ", logName)
		if err := ioutil.WriteFile(filepath.Join(s.OutDir(), logName), []byte(text), 0644); err != nil {
			s.Logf("Failed to log response to %s: %v", logName, err)
		}
		s.Fatal("TEXT response doesn't contain the response of the text query")
	}
}
