// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/tast/core/errors"
)

func TestOmniboxCorpus(t *testing.T) {
	e := &omniboxEnv{hostPort: "127.0.0.1:12345"}

	// Every corpus string must prefix-match a seeded page title or typed
	// URL, so that the popup contents come from history.
	var targets []string
	for n := 0; n < omniboxNumPages; n++ {
		targets = append(targets, strings.ToLower(omniboxPageTitle(n)), e.typedPageURL(n))
	}
	for _, s := range e.corpus() {
		matched := false
		for _, target := range targets {
			if strings.HasPrefix(target, strings.ToLower(s)) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("Corpus string %q doesn't prefix-match any page title or URL", s)
		}
	}

	// nextCorpus cycles through the corpus in order.
	c := e.corpus()
	for i := 0; i < 2*len(c); i++ {
		if got, want := e.nextCorpus(), c[i%len(c)]; got != want {
			t.Errorf("nextCorpus() #%d = %q; want %q", i, got, want)
		}
	}
}

func TestCheckSkippedVisits(t *testing.T) {
	for _, tc := range []struct {
		skipped, visits int
		wantErr         bool
	}{
		{0, 0, false},
		{0, 40, false},
		{3, 40, false},
		{20, 40, false},
		{21, 40, true},
		{40, 40, true},
	} {
		e := &omniboxEnv{skippedVisits: tc.skipped, visits: tc.visits}
		if err := e.checkSkippedVisits(); (err != nil) != tc.wantErr {
			t.Errorf("checkSkippedVisits() with %d of %d skipped = %v; want error %t", tc.skipped, tc.visits, err, tc.wantErr)
		}
	}
}

func TestVisitAction(t *testing.T) {
	errOther := errors.New("other error")
	for _, tc := range []struct {
		name        string
		err         error
		wantErr     error
		wantSkipped int
	}{
		{"success", nil, nil, 0},
		{"not focused", errors.Wrap(errOmniboxNotFocused, "no focused omnibox input"), nil, 1},
		{"other error", errOther, errOther, 0},
	} {
		e := &omniboxEnv{}
		a := e.visitAction(func(context.Context) error { return tc.err })
		if err := a(context.Background()); !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: visitAction() = %v; want %v", tc.name, err, tc.wantErr)
		}
		if e.visits != 1 || e.skippedVisits != tc.wantSkipped {
			t.Errorf("%s: visits, skippedVisits = %d, %d; want 1, %d", tc.name, e.visits, e.skippedVisits, tc.wantSkipped)
		}
	}
}
