// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// aimEligibilityURL is the omnibox debug page that reports AI Mode
	// eligibility. It needs internal debugging pages to be enabled, and
	// doesn't exist on M156.
	aimEligibilityURL = "chrome://omnibox/aim-eligibility"

	// aimEligibilityHistogram is the fallback eligibility signal for
	// images where aimEligibilityURL doesn't exist.
	aimEligibilityHistogram = "Omnibox.AimEligibility.EligibilityResponse.is_eligible"
)

// recordAimEligibility reads AI Mode eligibility and stores it, to be
// reported as Omnibox.AimEligible. Eligibility comes from a server
// response and differs between accounts, so it is recorded on every run.
// Failures are logged and don't fail the test.
func (e *omniboxEnv) recordAimEligibility(ctx context.Context) {
	eligible, err := e.aimEligibility(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to determine AI Mode eligibility: ", err)
		return
	}
	e.aimEligible = &eligible
}

// aimEligibility reads AI Mode eligibility from the omnibox debug page,
// falling back to the eligibility histogram where the page isn't
// available (e.g. M156).
func (e *omniboxEnv) aimEligibility(ctx context.Context) (bool, error) {
	eligible, details, err := e.readAimEligibilityPage(ctx)
	if err == nil {
		testing.ContextLogf(ctx, "AI Mode eligibility (%s): eligible=%t; %s", aimEligibilityURL, eligible, details)
		return eligible, nil
	}
	testing.ContextLog(ctx, "Falling back to the eligibility histogram: ", err)

	h, err := metrics.GetHistogram(ctx, e.tconn, aimEligibilityHistogram)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get %s", aimEligibilityHistogram)
	}
	// The histogram is a boolean, so its mean is the fraction of true
	// samples. Histograms are aggregated, so the last sample isn't
	// available; use the most frequent value, and log the full histogram.
	mean, err := h.Mean()
	if err != nil {
		return false, errors.Wrapf(err, "failed to get the mean of %s", aimEligibilityHistogram)
	}
	eligible = mean > 0.5
	testing.ContextLogf(ctx, "AI Mode eligibility (histogram): eligible=%t; %v", eligible, h)
	return eligible, nil
}

// deepTextJS returns the text of all text nodes in the document, including
// those in shadow roots, one per line. <style> and <script> are skipped.
const deepTextJS = `(() => {
  const out = [];
  const walk = (root) => {
    for (const el of root.querySelectorAll('*')) {
      if (el.shadowRoot) walk(el.shadowRoot);
      const tag = el.tagName.toLowerCase();
      if (tag === 'style' || tag === 'script') continue;
      for (const n of el.childNodes) {
        if (n.nodeType === Node.TEXT_NODE && n.textContent.trim()) {
          out.push(n.textContent.trim());
        }
      }
    }
  };
  walk(document);
  return out.join('\n');
})()`

// clickButtonJS clicks the first button (including in shadow roots) whose
// text is the given text, and returns true, or returns false if there is no
// such button.
const clickButtonJS = `(text) => {
  const walk = (root) => {
    for (const el of root.querySelectorAll('*')) {
      const tag = el.tagName.toLowerCase();
      if (tag === 'button' || tag === 'cr-button') {
        if ((el.innerText || el.textContent || '').trim() === text) {
          el.click();
          return true;
        }
      }
      if (el.shadowRoot && walk(el.shadowRoot)) return true;
    }
    return false;
  };
  return walk(document);
}`

// Button texts on chrome://chrome-urls.
const (
	enableDebugPagesButton  = "Enable internal debugging pages"
	disableDebugPagesButton = "Disable internal debugging pages"
)

// setInternalDebugPages enables or disables internal debugging pages on
// chrome://chrome-urls. It returns whether it changed the setting.
func (e *omniboxEnv) setInternalDebugPages(ctx context.Context, enable bool) (bool, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, omniboxCleanupTime)
	defer cancel()

	conn, err := e.cr.NewConn(ctx, "chrome://chrome-urls/")
	if err != nil {
		return false, errors.Wrap(err, "failed to open chrome://chrome-urls")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	// click is the button that changes the setting, and done is the
	// button shown once the setting is in the wanted state.
	click, done := enableDebugPagesButton, disableDebugPagesButton
	if !enable {
		click, done = disableDebugPagesButton, enableDebugPagesButton
	}
	// The click is handled asynchronously, so wait until the page shows
	// the wanted state before returning; closing the page earlier could
	// race with it. Clicking again while waiting is harmless, since the
	// button only enables or only disables.
	var changed bool
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var clicked bool
		if err := conn.Call(ctx, &clicked, clickButtonJS, click); err != nil {
			return testing.PollBreak(err)
		}
		if clicked {
			changed = true
		}
		var text string
		if err := conn.Eval(ctx, deepTextJS, &text); err != nil {
			return testing.PollBreak(err)
		}
		if !strings.Contains(text, done) {
			return errors.Errorf("%q button not shown yet", done)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return false, err
	}
	return changed, nil
}

// enableInternalDebugPages enables internal debugging pages, which the AI
// Mode eligibility page needs, and returns a function that disables them
// again if they were disabled before.
func (e *omniboxEnv) enableInternalDebugPages(ctx context.Context) (action.Action, error) {
	changed, err := e.setInternalDebugPages(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable internal debugging pages")
	}
	if !changed {
		return func(context.Context) error { return nil }, nil
	}
	return func(ctx context.Context) error {
		if _, err := e.setInternalDebugPages(ctx, false); err != nil {
			return errors.Wrap(err, "failed to disable internal debugging pages")
		}
		return nil
	}, nil
}

// readAimEligibilityPage reads the AI Mode eligibility page and returns the
// overall eligibility and a summary of its breakdown.
func (e *omniboxEnv) readAimEligibilityPage(ctx context.Context) (bool, string, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, omniboxCleanupTime)
	defer cancel()

	disable, err := e.enableInternalDebugPages(ctx)
	if err != nil {
		return false, "", err
	}
	defer func(ctx context.Context) {
		if err := disable(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to disable internal debugging pages: ", err)
		}
	}(cleanupCtx)

	conn, err := e.cr.NewConn(ctx, aimEligibilityURL)
	if err != nil {
		return false, "", errors.Wrap(err, "failed to open the eligibility page")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	var eligible bool
	var details string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var text string
		if err := conn.Eval(ctx, deepTextJS, &text); err != nil {
			return testing.PollBreak(err)
		}
		var ok bool
		eligible, details, ok = parseAimEligibility(strings.Split(text, "\n"))
		if !ok {
			return errors.New("eligibility not shown yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return false, "", errors.Wrap(err, "failed to read the eligibility page")
	}
	return eligible, details, nil
}

// recordAimEligibilityResult saves Omnibox.AimEligible (1 if the account
// is eligible for AI Mode, 0 if not), if eligibility could be determined.
func (e *omniboxEnv) recordAimEligibilityResult(pv *perf.Values) {
	if e.aimEligible == nil {
		return
	}
	v := 0.0
	if *e.aimEligible {
		v = 1
	}
	pv.Set(perf.Metric{
		Name:      "Omnibox.AimEligible",
		Unit:      "bool",
		Direction: perf.BiggerIsBetter,
	}, v)
}
