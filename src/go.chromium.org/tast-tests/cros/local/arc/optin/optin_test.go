// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package optin

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.chromium.org/tast/core/errors"
)

type fakeJSEvaluator struct {
	waitForExpr func(ctx context.Context, expr string) error
	eval        func(ctx context.Context, expr string, out any) error
}

func (f *fakeJSEvaluator) WaitForExpr(ctx context.Context, expr string) error {
	return f.waitForExpr(ctx, expr)
}

func (f *fakeJSEvaluator) Eval(ctx context.Context, expr string, out any) error {
	return f.eval(ctx, expr, out)
}

func overrideTermsTimeouts(t *testing.T, loadTimeout, netTimeout, retryDelay time.Duration) {
	t.Helper()
	origLoadTimeout := termsLoadTimeout
	origNetTimeout := termsNetworkWaitTimeout
	origRetryDelay := termsRetryDelay
	termsLoadTimeout = loadTimeout
	termsNetworkWaitTimeout = netTimeout
	termsRetryDelay = retryDelay
	t.Cleanup(func() {
		termsLoadTimeout = origLoadTimeout
		termsNetworkWaitTimeout = origNetTimeout
		termsRetryDelay = origRetryDelay
	})
}

func TestWaitForTermsBoundsAttemptTimeoutAndRecovers(t *testing.T) {
	overrideTermsTimeouts(t, 30*time.Millisecond, 10*time.Millisecond, 5*time.Millisecond)

	attempts := 0
	reloaded := false
	fake := &fakeJSEvaluator{
		waitForExpr: func(ctx context.Context, expr string) error {
			if strings.Contains(expr, "LoadState.LOADED") {
				attempts++
				// Simulate a hung webview load on the first attempt that blocks
				// until the per-attempt timeout expires.
				if !reloaded {
					<-ctx.Done()
					return errors.Wrap(ctx.Err(), `"termsPage.state_ == LoadState.LOADED" is false`)
				}
			}
			return nil
		},
		eval: func(_ context.Context, expr string, out any) error {
			if strings.Contains(expr, "startTermsViewLoading_") {
				reloaded = true
			}
			if s, ok := out.(*string); ok {
				*s = ""
			}
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := withRetry("load terms of service", waitForTerms, defaultTermsLoadAttempts, fake)(ctx); err != nil {
		t.Fatalf("withRetry(waitForTerms) = %v, want nil", err)
	}
	if attempts != 2 {
		t.Errorf("load attempts = %d, want 2", attempts)
	}
	if !reloaded {
		t.Error("reloaded = false, want true (startTermsViewLoading_ not invoked)")
	}
	if ctx.Err() != nil {
		t.Errorf("ctx.Err() = %v, want nil", ctx.Err())
	}
}

func TestWithRetryRecoversAcrossOfflineAndDelayedAbort(t *testing.T) {
	overrideTermsTimeouts(t, 30*time.Millisecond, 10*time.Millisecond, 2*time.Millisecond)

	attempts := 0
	onlineChecks := 0
	var evals []string
	fake := &fakeJSEvaluator{
		waitForExpr: func(ctx context.Context, expr string) error {
			if expr == "navigator.onLine" {
				onlineChecks++
				// Simulate a temporary offline window on the first recovery check.
				if onlineChecks == 1 {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			}
			if strings.Contains(expr, "LoadState.LOADED") {
				attempts++
			}
			return nil
		},
		eval: func(_ context.Context, expr string, out any) error {
			if strings.Contains(expr, "button-retry") {
				evals = append(evals, expr)
				return nil
			}
			if s, ok := out.(*string); ok {
				// Fail attempts 1 (timeout/offline) and 2 (delayed ERR_ABORTED),
				// then succeed on attempt 3.
				if attempts < 3 {
					*s = "Couldn't connect with the server."
				} else {
					*s = ""
				}
			}
			return nil
		},
	}

	if err := withRetry("load terms of service", waitForTerms, defaultTermsLoadAttempts, fake)(context.Background()); err != nil {
		t.Fatalf("withRetry(waitForTerms) = %v, want nil", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if onlineChecks != 2 {
		t.Errorf("onlineChecks = %d, want 2", onlineChecks)
	}
	if len(evals) != 2 {
		t.Fatalf("len(evals) = %d, want 2", len(evals))
	}
	for _, snippet := range []string{
		"termsPage.state_ === LoadState.LOADING",
		"termsPage.termsView_.stop()",
		"termsPage.state_ = LoadState.ABORTED",
		"errorSection.hidden = true",
		"getElementById('button-retry').click()",
		"termsPage.startTermsViewLoading_()",
	} {
		if !strings.Contains(evals[0], snippet) {
			t.Errorf("recovery script missing %q:\n%s", snippet, evals[0])
		}
	}
}

func TestWithRetryDefaultTermsLoadAttemptsBoundary(t *testing.T) {
	overrideTermsTimeouts(t, 30*time.Millisecond, 5*time.Millisecond, 2*time.Millisecond)

	// Verify succeeding on the 4th attempt (defaultTermsLoadAttempts = 4).
	attempts := 0
	fake := &fakeJSEvaluator{
		waitForExpr: func(_ context.Context, expr string) error {
			if strings.Contains(expr, "LoadState.LOADED") {
				attempts++
			}
			return nil
		},
		eval: func(_ context.Context, expr string, out any) error {
			if strings.Contains(expr, "button-retry") {
				return nil
			}
			if s, ok := out.(*string); ok {
				if attempts < defaultTermsLoadAttempts {
					*s = "Couldn't connect with the server."
				} else {
					*s = ""
				}
			}
			return nil
		},
	}

	if err := withRetry("load terms of service", waitForTerms, defaultTermsLoadAttempts, fake)(context.Background()); err != nil {
		t.Fatalf("withRetry(waitForTerms) on 4th attempt = %v, want nil", err)
	}
	if attempts != defaultTermsLoadAttempts {
		t.Errorf("attempts = %d, want %d", attempts, defaultTermsLoadAttempts)
	}

	// Verify exhausting all 4 attempts returns the error message.
	attempts = 0
	alwaysFailFake := &fakeJSEvaluator{
		waitForExpr: func(_ context.Context, expr string) error {
			if strings.Contains(expr, "LoadState.LOADED") {
				attempts++
			}
			return nil
		},
		eval: func(_ context.Context, expr string, out any) error {
			if strings.Contains(expr, "button-retry") {
				return nil
			}
			if s, ok := out.(*string); ok {
				*s = "Couldn't connect with the server."
			}
			return nil
		},
	}

	err := withRetry("load terms of service", waitForTerms, defaultTermsLoadAttempts, alwaysFailFake)(context.Background())
	if err == nil {
		t.Fatal("withRetry(waitForTerms) = nil, want non-nil error after 4 failed attempts")
	}
	if attempts != defaultTermsLoadAttempts {
		t.Errorf("attempts = %d, want %d", attempts, defaultTermsLoadAttempts)
	}
	if want := "Couldn't connect with the server."; !strings.Contains(err.Error(), want) {
		t.Errorf("withRetry(waitForTerms) = %v, want error containing %q", err, want)
	}
}

func TestWithRetryPreservesActionErrorWhenRecoveryInterrupted(t *testing.T) {
	overrideTermsTimeouts(t, 45*time.Second, 20*time.Second, 200*time.Millisecond)

	fake := &fakeJSEvaluator{
		waitForExpr: func(_ context.Context, _ string) error { return nil },
		eval:        func(_ context.Context, _ string, _ any) error { return nil },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	attempts := 0
	err := withRetry("load terms of service", func(_ context.Context, _ jsEvaluator) error {
		attempts++
		return errors.New("termsPage.state_ stuck at LOADING")
	}, 2, fake)(ctx)
	if err == nil {
		t.Fatal("withRetry(...) = nil, want non-nil error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if want := "termsPage.state_ stuck at LOADING"; !strings.Contains(err.Error(), want) {
		t.Fatalf("withRetry(...) = %v, want error containing %q", err, want)
	}
	if want := "retry recovery failed"; !strings.Contains(err.Error(), want) {
		t.Fatalf("withRetry(...) = %v, want error containing %q", err, want)
	}
}

func TestWithRetryPreservesActionErrorWhenParentExpiresDuringNetworkWait(t *testing.T) {
	overrideTermsTimeouts(t, 45*time.Second, 200*time.Millisecond, 2*time.Millisecond)

	evalCalled := false
	fake := &fakeJSEvaluator{
		waitForExpr: func(ctx context.Context, expr string) error {
			if expr == "navigator.onLine" {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		},
		eval: func(_ context.Context, expr string, _ any) error {
			if strings.Contains(expr, "button-retry") {
				evalCalled = true
			}
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	attempts := 0
	err := withRetry("load terms of service", func(_ context.Context, _ jsEvaluator) error {
		attempts++
		return errors.New("initial ToS load failure")
	}, 2, fake)(ctx)
	if err == nil {
		t.Fatal("withRetry(...) = nil, want non-nil error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if evalCalled {
		t.Error("evalCalled = true, want false when parent ctx expires during navigator.onLine wait")
	}
	if want := "initial ToS load failure"; !strings.Contains(err.Error(), want) {
		t.Errorf("withRetry(...) = %v, want error containing %q", err, want)
	}
	if want := "context expired while waiting for network before retry"; !strings.Contains(err.Error(), want) {
		t.Errorf("withRetry(...) = %v, want error containing %q", err, want)
	}
}
