// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/ui/mahicuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	responseMockPy = "mahi_response_mock.py"
	localHTMLZip   = "mahi_html.zip"
	localPDFZip    = "mahi_example_pdf.zip"
	localTextZip   = "mahi_text.zip"
)

type mahiParameters struct {
	urlCount     int
	localZipFile string
	doSimplify   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MahiBrowserCUJ,
		Desc: "CUJ of Help Me Read feature",
		Contacts: []string{
			"alanlxl@google.com",
			"thanhdng@google.com",
		},
		BugComponent: "b:1116342",
		Timeout:      20 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)), // Only enable on CBX devices.
		Data: []string{
			localHTMLZip,
			localPDFZip,
			localTextZip,
			responseMockPy,
		},
		Attr: []string{"group:cuj", "cuj_experimental"},
		Params: []testing.Param{
			{
				Name:    "webpage",
				Fixture: "loggedInToCUJUserWithMahiEnabled",
				Val: mahiParameters{
					urlCount:     5,
					localZipFile: localHTMLZip,
					doSimplify:   false,
				},
			},
			{
				Name:    "webpage_simplify",
				Fixture: "loggedInToCUJUserWithMahiEnabled",
				Val: mahiParameters{
					urlCount:     5,
					localZipFile: localTextZip,
					doSimplify:   true,
				},
			},
			{
				Name:    "webpdf",
				Fixture: "loggedInToCUJUserWithMahiEnabled",
				Val: mahiParameters{
					urlCount:     1,
					localZipFile: localPDFZip,
					doSimplify:   false,
				},
			},
		},
	})
}

func MahiBrowserCUJ(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	params := s.Param().(mahiParameters)

	if _, err := mahicuj.BrowserCUJRun(
		ctx, cr, s.DataPath(responseMockPy), s.DataPath(params.localZipFile),
		s.OutDir(), params.urlCount, params.doSimplify); err != nil {
		s.Fatal("Failed to run MahiBrowserCUJ: ", err)
	}

}
