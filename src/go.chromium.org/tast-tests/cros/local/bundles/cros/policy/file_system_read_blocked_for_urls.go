// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	filesystemreadwrite "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/file_system_read_write"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

const readBlockTestHTML = "file_system_read_for_urls_index.html"

func init() {
	testing.AddTest(&testing.Test{
		Func:           FileSystemReadBlockedForUrls,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantExists,
		Desc:           "Checking if file system reads are blocked depending on the value of this policy",
		Contacts: []string{
			// "cros-engprod-muc@google.com",
			// "chromeos-sw-engprod@google.com",
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1263917",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: []string{readBlockTestHTML},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.FileSystemReadBlockedForUrls{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// FileSystemReadBlockedForUrls tests the FileSystemReadBlockedForUrls policy.
func FileSystemReadBlockedForUrls(ctx context.Context, s *testing.State) {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		s.Fatal("Failed to parse url: ", err)
	}
	baseURL.Path = filepath.Join(baseURL.Path, readBlockTestHTML)
	url := baseURL.String()

	for _, param := range []filesystemreadwrite.TestCase{
		{
			// Blocked matching test.
			Name:                 "blocked_matching",
			URL:                  url,
			WantFileSystemAccess: false,
			Method:               filesystemreadwrite.Read,
			Policies: []policy.Policy{
				&policy.FileSystemReadBlockedForUrls{Val: []string{url}}},
		}, {
			// Blocked non matching test.
			Name:                 "blocked_non_matching",
			URL:                  url,
			WantFileSystemAccess: true,
			Method:               filesystemreadwrite.Read,
			Policies: []policy.Policy{
				&policy.FileSystemReadBlockedForUrls{Val: []string{""}}},
		}, {
			// Unset test.
			Name:                 "unset",
			URL:                  url,
			WantFileSystemAccess: true,
			Method:               filesystemreadwrite.Read,
			Policies: []policy.Policy{
				&policy.FileSystemReadBlockedForUrls{Stat: policy.StatusUnset}},
		},
	} {
		filesystemreadwrite.RunTestCases(ctx, s, param)
	}
}
