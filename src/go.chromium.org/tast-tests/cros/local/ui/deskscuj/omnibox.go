// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// omniboxNumPages is the number of on-DUT pages served by the
	// omnibox variants. See omniboxPages* below for how they are used.
	omniboxNumPages = 20

	// omniboxSuggestPref is the pref that controls remote search
	// suggestions.
	omniboxSuggestPref = "search.suggest_enabled"

	// omniboxPageHTMLTemplate is the HTML template of the on-DUT pages.
	// Both %s are the page title.
	omniboxPageHTMLTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:sans-serif;margin:2em;}</style></head>
<body><h1>%s</h1><p>Static page for the DesksCUJ omnibox variants.</p></body></html>
`

	// omniboxCleanupTime is the time reserved for cleaning up after the
	// omnibox setup steps.
	omniboxCleanupTime = 10 * time.Second
)

// Page allocation. Each window gets a distinct page where it matters, so
// that waiting on a window title is unambiguous. The numbers only need to
// be unique (see TestOmniboxPageAllocation).
var (
	omniboxPagesDesk1    = []int{0, 1, 2, 3}
	omniboxPagesDesk2    = []int{4, 5, 6, 7}
	omniboxPagesDesk3    = []int{8, 9}
	omniboxPagesDesk4    = []int{10}
	omniboxPagesRAMTabs  = []int{11, 12, 13}
	omniboxPagesNav      = []int{14, 15}
	omniboxPagesRecorded = []int{16, 17, 18, 19}
)

// omniboxPageWords gives each page a distinct word in its title.
var omniboxPageWords = []string{
	"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel",
	"india", "juliet", "kilo", "lima", "mike", "november", "oscar", "papa",
	"quebec", "romeo", "sierra", "tango",
}

// omniboxEnv holds the state shared by the omnibox variants.
type omniboxEnv struct {
	cr     *chrome.Chrome
	tconn  *chrome.TestConn
	kw     *input.KeyboardEventWriter
	ui     *uiauto.Context
	server *httptest.Server
	param  TestParam

	// hostPort is the host:port of the page server, e.g. 127.0.0.1:12345.
	hostPort string

	// Rotating indices so that every run visits windows and types strings
	// in the same order.
	corpusIdx int

	// visits counts the desk visits with omnibox input, and skippedVisits
	// the ones where the omnibox couldn't be focused, so no omnibox input
	// was done.
	visits        int
	skippedVisits int

	// mem collects the memory snapshots taken as windows are opened, and
	// reports them as the Omnibox.Mem* metrics (see memTracker).
	mem *memTracker

	// restoreSuggest restores the remote suggest pref, if it was changed.
	restoreSuggest action.Action
}

// newOmniboxEnv starts the on-DUT page server, seeds history and turns off
// remote search suggestions, so that the popup contents only come from
// history. Call Close to clean up.
func newOmniboxEnv(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, kw *input.KeyboardEventWriter, param TestParam) (env *omniboxEnv, retErr error) {
	e := &omniboxEnv{
		cr:    cr,
		tconn: tconn,
		kw:    kw,
		ui:    uiauto.New(tconn),
		param: param,
		mem:   newMemTracker("Omnibox"),
	}

	// Clean up whatever was set up if a later step fails.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, omniboxCleanupTime)
	defer cancel()
	defer func(ctx context.Context) {
		if retErr != nil {
			e.Close(ctx)
		}
	}(closeCtx)

	e.server = httptest.NewServer(http.HandlerFunc(serveOmniboxPage))
	u := strings.TrimPrefix(e.server.URL, "http://")
	if _, _, err := net.SplitHostPort(u); err != nil {
		return nil, errors.Wrapf(err, "unexpected server URL %q", e.server.URL)
	}
	e.hostPort = u

	if err := e.seedHistory(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to seed history")
	}

	restore, err := setSuggestPref(ctx, tconn, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to turn off remote suggest")
	}
	e.restoreSuggest = restore
	return e, nil
}

// Close stops the page server and restores the prefs changed by
// newOmniboxEnv.
func (e *omniboxEnv) Close(ctx context.Context) {
	if e.restoreSuggest != nil {
		if err := e.restoreSuggest(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to restore the remote suggest pref: ", err)
		}
	}
	if e.server != nil {
		e.server.Close()
	}
}

// serveOmniboxPage serves tiny static pages at /pNN, with no external
// resources.
func serveOmniboxPage(w http.ResponseWriter, r *http.Request) {
	var n int
	if _, err := fmt.Sscanf(r.URL.Path, "/p%02d", &n); err != nil || n < 0 || n >= omniboxNumPages {
		// Includes /favicon.ico. Respond without a body to keep the
		// request cheap.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	title := html.EscapeString(omniboxPageTitle(n))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, omniboxPageHTMLTemplate, title, title)
}

// omniboxPageTitle returns the title of page n, e.g. "Omnitest 03 delta".
func omniboxPageTitle(n int) string {
	return fmt.Sprintf("Omnitest %02d %s", n, omniboxPageWords[n%len(omniboxPageWords)])
}

// pageURL returns the full URL of page n.
func (e *omniboxEnv) pageURL(n int) string {
	return fmt.Sprintf("%s/p%02d", e.server.URL, n)
}

// pageURLs returns the full URLs of the given pages.
func (e *omniboxEnv) pageURLs(pages []int) []string {
	var urls []string
	for _, n := range pages {
		urls = append(urls, e.pageURL(n))
	}
	return urls
}

// typedPageURL returns the string typed into the omnibox to navigate to
// page n.
func (e *omniboxEnv) typedPageURL(n int) string {
	return fmt.Sprintf("%s/p%02d", e.hostPort, n)
}

// seedHistory visits every page once, in a temporary tab that is closed
// afterwards, so that typed strings match deterministic history entries.
func (e *omniboxEnv) seedHistory(ctx context.Context) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, omniboxCleanupTime)
	defer cancel()

	conn, err := e.cr.NewConn(ctx, e.pageURL(0))
	if err != nil {
		return errors.Wrap(err, "failed to open the history seeding tab")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	for n := 1; n < omniboxNumPages; n++ {
		if err := conn.Navigate(ctx, e.pageURL(n)); err != nil {
			return errors.Wrapf(err, "failed to navigate to page %d", n)
		}
	}
	return nil
}

// setSuggestPref sets the remote suggest pref and returns a function that
// restores its previous value.
func setSuggestPref(ctx context.Context, tconn *chrome.TestConn, enabled bool) (action.Action, error) {
	var pref struct {
		Value bool `json:"value"`
	}
	if err := tconn.Call(ctx, &pref, `tast.promisify(chrome.settingsPrivate.getPref)`, omniboxSuggestPref); err != nil {
		return nil, errors.Wrapf(err, "failed to get %s", omniboxSuggestPref)
	}
	if err := tconn.Call(ctx, nil, `tast.promisify(chrome.settingsPrivate.setPref)`, omniboxSuggestPref, enabled); err != nil {
		return nil, errors.Wrapf(err, "failed to set %s", omniboxSuggestPref)
	}
	prev := pref.Value
	return func(ctx context.Context) error {
		return tconn.Call(ctx, nil, `tast.promisify(chrome.settingsPrivate.setPref)`, omniboxSuggestPref, prev)
	}, nil
}

// omniboxMetricConfigs returns the extra histograms collected by the
// omnibox variants.
func omniboxMetricConfigs() []cujrecorder.MetricConfig {
	var configs []cujrecorder.MetricConfig
	ms := func(name string) {
		configs = append(configs, cujrecorder.NewDistributionMetricConfig(name, "ms", perf.SmallerIsBetter))
	}
	// The popup variants log under Omnibox.Popup.<variant>: WebUI is the
	// classic WebUI popup, FullWebUI the full WebUI popup and Aim the AI
	// Mode popup.
	for _, variant := range []string{"WebUI", "FullWebUI", "Aim"} {
		p := "Omnibox.Popup." + variant
		ms(p + ".ShowToPaint.Duration")
		ms(p + ".ShowToPaint.Duration.FirstShow")
		ms(p + ".ContentReady.Duration")
		ms(p + ".ContentReady.Duration.FirstShow")
		// Informational only: on R157 this overflows for popups that
		// are constructed long before they are first shown.
		ms(p + ".ConstructionToFirstShownDuration")
		configs = append(configs,
			cujrecorder.NewEnumCustomMetricConfig(p+".CrashRecovery", nil, nil),
			cujrecorder.NewEnumCustomMetricConfig(p+".RendererProcessGoneStatus", nil, nil),
		)
	}
	// Optional until crrev/c/8528499 is in the image. Histograms without
	// samples are not reported.
	ms("Ash.Window.InitToFirstPresentation.Browser")
	ms("Omnibox.CharTypedToRepaintLatency.ToPaint")
	ms("Omnibox.WebUI.CharTypedToRepaintLatency.ToPaint")
	ms("Omnibox.Popup.WebUI.ResultChangedToRepaintLatency.ToPaint")
	// Baseline arm with the Views popup.
	ms("Omnibox.Views.PopupFirstPaint")
	return configs
}

// recordResults saves the omnibox-specific perf values.
func (e *omniboxEnv) recordResults(ctx context.Context, pv *perf.Values) {
	pv.Set(perf.Metric{
		Name:      "Omnibox.SkippedVisits",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(e.skippedVisits))
	if e.skippedVisits > 0 {
		testing.ContextLogf(ctx, "Skipped the omnibox input on %d visits", e.skippedVisits)
	}
	e.mem.record(ctx, pv)
}
