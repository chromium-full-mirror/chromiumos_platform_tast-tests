// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/tastrun"
	"go.chromium.org/tast/core/testing"

	// Register the fixtures to remote bundle.
	_ "go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/fixture"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunTestsRemoteFixture,
		Desc:         "Verifies that Tast can run remote fixtures",
		Contacts:     []string{"tast-core@google.com", "oka@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		// TODO: b/316638447 -- Reenable after this test is more stable.
		Attr: []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

func RunTestsRemoteFixture(ctx context.Context, s *testing.State) {
	const (
		setUpError    = "meta.metaRemote.SetUpError"
		tearDownError = "meta.metaRemote.tearDownError"
	)

	for _, tc := range []struct {
		name     string
		vars     map[string]string
		wantLogs map[string]*regexp.Regexp
	}{
		{
			name: "success",
			wantLogs: map[string]*regexp.Regexp{
				"fixtures/metaRemote/log.txt": regexp.MustCompile(`(?s)SetUp metaRemote\n.*TearDown metaRemote\n`),
				"full.txt":                    regexp.MustCompile(`(?s)SetUp metaRemote\n.*Hello test\n.*TearDown metaRemote\n`),
			},
		},
		{
			name: "setup_failure",
			vars: map[string]string{
				setUpError: "Whoa",
			},
			wantLogs: map[string]*regexp.Regexp{
				"fixtures/metaRemote/log.txt":           regexp.MustCompile(`Whoa\n`),
				"tests/meta.LocalRemoteFixture/log.txt": regexp.MustCompile(`\[Fixture failure\] metaRemote: Whoa\n`),
			},
		},
		// TODO(oka): test TearDown failures after fixutre failures become
		// reported.
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			subtestsDir := fmt.Sprintf("subtest_results_%s", tc.name)
			resultsDir := filepath.Join(s.OutDir(), subtestsDir)
			flags := []string{
				"-resultsdir=" + resultsDir,
			}
			for k, v := range tc.vars {
				flags = append(flags, "-var", fmt.Sprintf("%s=%s", k, v))
			}
			_, _, err := tastrun.Exec(ctx, s, "run", flags, []string{"meta.LocalRemoteFixture"})
			if err != nil {
				s.Fatal("Failed to run tast: ", err)
			}

			for k, re := range tc.wantLogs {
				if b, err := ioutil.ReadFile(filepath.Join(resultsDir, k)); err != nil {
					s.Errorf("Log %s: %v", k, err)
				} else if !re.Match(b) {
					s.Errorf("Log %s didn't match with %s", k, re)
				}
			}
		})
	}
}
