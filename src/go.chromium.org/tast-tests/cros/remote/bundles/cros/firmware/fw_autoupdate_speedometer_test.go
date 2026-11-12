// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"google.golang.org/grpc"
)

type fakeSpeedometerService struct {
	fwpb.UtilsServiceClient

	newChromeErr     error
	newChromeFn      func(context.Context) error
	closeChromeErr   error
	closeCalls       int
	closeCtxErr      error
	closeHadDeadline bool
	performResult    string
	performFn        func(context.Context) (*fwpb.SpeedometerResponse, error)
	performCalls     int
}

func (f *fakeSpeedometerService) NewChrome(ctx context.Context, _ *empty.Empty, _ ...grpc.CallOption) (*empty.Empty, error) {
	if f.newChromeFn != nil {
		if err := f.newChromeFn(ctx); err != nil {
			return nil, err
		}
	}
	if f.newChromeErr != nil {
		return nil, f.newChromeErr
	}
	return &empty.Empty{}, nil
}

func (f *fakeSpeedometerService) CloseChrome(ctx context.Context, _ *empty.Empty, _ ...grpc.CallOption) (*empty.Empty, error) {
	f.closeCalls++
	f.closeCtxErr = ctx.Err()
	_, f.closeHadDeadline = ctx.Deadline()
	if f.closeChromeErr != nil {
		return nil, f.closeChromeErr
	}
	return &empty.Empty{}, nil
}

func (f *fakeSpeedometerService) PerformSpeedometerTest(ctx context.Context, _ *empty.Empty, _ ...grpc.CallOption) (*fwpb.SpeedometerResponse, error) {
	f.performCalls++
	if f.performFn != nil {
		return f.performFn(ctx)
	}
	return &fwpb.SpeedometerResponse{Result: f.performResult}, nil
}

func TestCheckSpeedometerDegradation(t *testing.T) {
	errRPC := errors.New("rpc timeout")
	for _, tc := range []struct {
		name                         string
		baseline, latest             float64
		baselineErr, latestErr       error
		wantComparable, wantDegraded bool
	}{
		{"baseline_failed", 0, 88.7, errRPC, nil, false, false},
		{"latest_failed_no_phantom_degradation", 163, 0, nil, errRPC, false, false},
		{"both_failed", 0, 0, errRPC, errRPC, false, false},
		{"zero_baseline", 0, 100, nil, nil, false, false},
		{"zero_latest", 100, 0, nil, nil, false, false},
		{"nan_latest", 100, math.NaN(), nil, nil, false, false},
		{"inf_baseline", math.Inf(1), 100, nil, nil, false, false},
		{"exact_threshold", 100, 95, nil, nil, true, false},
		{"within_variance", 369, 365, nil, nil, true, false},
		{"below_threshold", 100, 94.9, nil, nil, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comparable, degraded := checkSpeedometerDegradation(tc.baseline, tc.baselineErr, tc.latest, tc.latestErr, speedometerMinRatio)
			if comparable != tc.wantComparable || degraded != tc.wantDegraded {
				t.Errorf("checkSpeedometerDegradation(%v, %v, %v, %v, %v) = (%v, %v), want (%v, %v)",
					tc.baseline, tc.baselineErr, tc.latest, tc.latestErr, speedometerMinRatio,
					comparable, degraded, tc.wantComparable, tc.wantDegraded)
			}
		})
	}
}

func TestRunSpeedTestIsolatesBenchmarkTimeout(t *testing.T) {
	var sleepCalls int
	fake := &fakeSpeedometerService{
		newChromeFn: func(ctx context.Context) error {
			select {
			case <-time.After(120 * time.Millisecond):
				return ctx.Err()
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		performFn: func(ctx context.Context) (*fwpb.SpeedometerResponse, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, errors.New("expected benchmark deadline on context")
			}
			// Setup (120ms) + cooldown (120ms) exceeds benchmark (200ms), yet benchmark must keep >=120ms.
			if rem := time.Until(deadline); rem < 120*time.Millisecond {
				return nil, errors.Errorf("benchmark deadline shrunk to %v; setup/cooldown leaked into benchmark budget", rem)
			}
			return &fwpb.SpeedometerResponse{Result: "140.5"}, nil
		},
	}
	cfg := speedTestConfig{
		chromeSetup: 500 * time.Millisecond,
		chromeClose: 500 * time.Millisecond,
		cooldown:    120 * time.Millisecond,
		benchmark:   200 * time.Millisecond,
		sleep: func(ctx context.Context, d time.Duration) error {
			sleepCalls++
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	got, err := runSpeedTestWithConfig(t.Context(), fake, cfg)
	if err != nil {
		t.Fatalf("runSpeedTestWithConfig() unexpected error: %v", err)
	}
	if math.Abs(got-140.5) > 1e-9 {
		t.Errorf("runSpeedTestWithConfig() score = %v, want 140.5", got)
	}
	if sleepCalls != 1 || fake.closeCalls != 1 || fake.closeCtxErr != nil || !fake.closeHadDeadline {
		t.Errorf("runSpeedTestWithConfig() post-state = {sleep:%d close:%d closeErr:%v hasDeadline:%v}, want {1 1 nil true}",
			sleepCalls, fake.closeCalls, fake.closeCtxErr, fake.closeHadDeadline)
	}
}

func TestRunSpeedTestClosesChromeOnBenchmarkTimeout(t *testing.T) {
	fake := &fakeSpeedometerService{
		performFn: func(ctx context.Context) (*fwpb.SpeedometerResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	cfg := speedTestConfig{
		chromeSetup: time.Second,
		chromeClose: time.Second,
		cooldown:    0,
		benchmark:   20 * time.Millisecond,
	}

	if _, err := runSpeedTestWithConfig(t.Context(), fake, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runSpeedTestWithConfig() err = %v, want context.DeadlineExceeded", err)
	}
	if fake.closeCalls != 1 || fake.closeCtxErr != nil || !fake.closeHadDeadline {
		t.Errorf("runSpeedTestWithConfig() CloseChrome state = {calls:%d err:%v hasDeadline:%v}, want {1 nil true}",
			fake.closeCalls, fake.closeCtxErr, fake.closeHadDeadline)
	}
}

func TestRunSpeedTestCloseChromeErrorDoesNotClobberScore(t *testing.T) {
	fake := &fakeSpeedometerService{
		performResult:  "163.0",
		closeChromeErr: errors.New("close rpc failed"),
	}
	cfg := speedTestConfig{
		chromeSetup: time.Second,
		chromeClose: time.Second,
		cooldown:    time.Millisecond,
		benchmark:   time.Second,
	}

	got, err := runSpeedTestWithConfig(t.Context(), fake, cfg)
	if err != nil || math.Abs(got-163.0) > 1e-9 || fake.closeCalls != 1 {
		t.Fatalf("runSpeedTestWithConfig() = (%v, %v), closeCalls=%d; want (163.0, nil), 1", got, err, fake.closeCalls)
	}
}

func TestRunSpeedTestCooldownCancel(t *testing.T) {
	fake := &fakeSpeedometerService{performResult: "100.0"}
	cfg := speedTestConfig{
		chromeSetup: time.Second,
		chromeClose: time.Second,
		cooldown:    time.Second,
		benchmark:   time.Second,
		sleep:       func(context.Context, time.Duration) error { return context.Canceled },
	}

	if _, err := runSpeedTestWithConfig(t.Context(), fake, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("runSpeedTestWithConfig() err = %v, want context.Canceled", err)
	}
	if fake.performCalls != 0 || fake.closeCalls != 1 {
		t.Errorf("runSpeedTestWithConfig() calls = {perform:%d close:%d}, want {0 1}", fake.performCalls, fake.closeCalls)
	}
}

func TestRunSpeedTestErrors(t *testing.T) {
	var sleepCalls int
	errSetup := errors.New("cdp connect failed")
	fakeFailSetup := &fakeSpeedometerService{newChromeErr: errSetup}
	cfg := speedTestConfig{
		chromeSetup: time.Second,
		chromeClose: time.Second,
		cooldown:    time.Second,
		benchmark:   time.Second,
		sleep:       func(context.Context, time.Duration) error { sleepCalls++; return nil },
	}
	if _, err := runSpeedTestWithConfig(t.Context(), fakeFailSetup, cfg); !errors.Is(err, errSetup) {
		t.Errorf("runSpeedTestWithConfig() setup err = %v, want %v", err, errSetup)
	}
	if sleepCalls != 0 || fakeFailSetup.performCalls != 0 || fakeFailSetup.closeCalls != 0 {
		t.Errorf("runSpeedTestWithConfig() unexpected calls after NewChrome failure: sleep=%d perform=%d close=%d",
			sleepCalls, fakeFailSetup.performCalls, fakeFailSetup.closeCalls)
	}

	fakeBadParse := &fakeSpeedometerService{performResult: "not-a-float"}
	cfg.cooldown = 0
	if _, err := runSpeedTestWithConfig(t.Context(), fakeBadParse, cfg); err == nil {
		t.Error("runSpeedTestWithConfig() err = nil for non-numeric score, want error")
	}
	if fakeBadParse.closeCalls != 1 {
		t.Errorf("runSpeedTestWithConfig() closeCalls = %d, want 1", fakeBadParse.closeCalls)
	}
}
