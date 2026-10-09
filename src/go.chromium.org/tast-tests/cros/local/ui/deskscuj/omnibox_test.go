// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeOmniboxPage(t *testing.T) {
	for _, tc := range []struct {
		path      string
		wantCode  int
		wantTitle string
	}{
		{path: "/p00", wantCode: http.StatusOK, wantTitle: "Omnitest 00 alpha"},
		{path: "/p03", wantCode: http.StatusOK, wantTitle: "Omnitest 03 delta"},
		{path: "/p19", wantCode: http.StatusOK, wantTitle: "Omnitest 19 tango"},
		{path: "/p20", wantCode: http.StatusNoContent},
		{path: "/favicon.ico", wantCode: http.StatusNoContent},
		{path: "/", wantCode: http.StatusNoContent},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			serveOmniboxPage(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("GET %s: status %d; want %d", tc.path, rec.Code, tc.wantCode)
			}
			if tc.wantTitle == "" {
				return
			}
			body := rec.Body.String()
			if want := "<title>" + tc.wantTitle + "</title>"; !strings.Contains(body, want) {
				t.Errorf("GET %s: body %q doesn't contain %q", tc.path, body, want)
			}
			// The pages must not load anything else.
			for _, s := range []string{"src=", "href=", "@import", "url("} {
				if strings.Contains(body, s) {
					t.Errorf("GET %s: body contains %q", tc.path, s)
				}
			}
		})
	}
}

func TestOmniboxPageAllocation(t *testing.T) {
	if len(omniboxPageWords) < omniboxNumPages {
		t.Errorf("Only %d page words for %d pages", len(omniboxPageWords), omniboxNumPages)
	}
	seen := make(map[int]string)
	for name, pages := range map[string][]int{
		"Desk1":    omniboxPagesDesk1,
		"RAMTabs":  omniboxPagesRAMTabs,
		"Desk2":    omniboxPagesDesk2,
		"Desk3":    omniboxPagesDesk3,
		"Desk4":    omniboxPagesDesk4,
		"Nav":      omniboxPagesNav,
		"Recorded": omniboxPagesRecorded,
	} {
		for _, n := range pages {
			if n < 0 || n >= omniboxNumPages {
				t.Errorf("omniboxPages%s has page %d; want [0, %d)", name, n, omniboxNumPages)
			}
			if other, ok := seen[n]; ok {
				t.Errorf("Page %d is used by both omniboxPages%s and omniboxPages%s", n, other, name)
			}
			seen[n] = name
		}
	}

	// The window counts must match the desk layout (5/4/2/1 windows,
	// where desk 1 also has the RAM pressure window).
	for _, tc := range []struct {
		name  string
		pages []int
		want  int
	}{
		{"Desk1", omniboxPagesDesk1, 4},
		{"Desk2", omniboxPagesDesk2, 4},
		{"Desk3", omniboxPagesDesk3, 2},
		{"Desk4", omniboxPagesDesk4, 1},
	} {
		if len(tc.pages) != tc.want {
			t.Errorf("omniboxPages%s has %d pages; want %d", tc.name, len(tc.pages), tc.want)
		}
	}

	titles := make(map[string]bool)
	for n := 0; n < omniboxNumPages; n++ {
		title := omniboxPageTitle(n)
		if titles[title] {
			t.Errorf("Duplicate page title %q", title)
		}
		titles[title] = true
	}
}

func TestOmniboxPageURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(serveOmniboxPage))
	defer server.Close()
	e := &omniboxEnv{server: server, hostPort: strings.TrimPrefix(server.URL, "http://")}

	if got, want := e.pageURL(7), server.URL+"/p07"; got != want {
		t.Errorf("pageURL(7) = %q; want %q", got, want)
	}
	if got, want := e.typedPageURL(7), e.hostPort+"/p07"; got != want {
		t.Errorf("typedPageURL(7) = %q; want %q", got, want)
	}
	if got := e.pageURLs([]int{1, 2}); len(got) != 2 || got[1] != server.URL+"/p02" {
		t.Errorf("pageURLs([1 2]) = %v", got)
	}

	resp, err := http.Get(e.pageURL(5))
	if err != nil {
		t.Fatal("Failed to fetch page 5: ", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET page 5: status %d; want %d", resp.StatusCode, http.StatusOK)
	}
}
