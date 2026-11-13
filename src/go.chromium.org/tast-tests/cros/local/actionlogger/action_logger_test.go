// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package actionlogger

import (
	"context"
	"fmt"
	"os"
	gotesting "testing"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/testing"
)

func TestMain(m *gotesting.M) {
	if err := testing.InitializeVarsForUnitTest(nil); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize Tast vars: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestShouldRunDisabledByDefault(t *gotesting.T) {
	prevLogger := logger
	defer func() {
		logger = prevLogger
	}()

	if ShouldRun() {
		t.Fatal("ShouldRun() = true; want false")
	}

	if l := newActionLogger(); l.ShouldRun {
		t.Fatal("newActionLogger().ShouldRun = true; want false")
	}

	_, preExistingErr := os.Stat(tempLoggerFile)
	preExisting := preExistingErr == nil

	ctx := context.Background()
	Reset(ctx)
	if logger == nil || logger.ShouldRun {
		t.Fatalf("after Reset(), logger = %+v; want non-nil with ShouldRun=false", logger)
	}
	RecordClickAction(ctx, nil, MouseClick, "target", coords.Rect{}, coords.Point{}, mouse.LeftButton)
	RecordClickAtLocationAction(ctx, nil, MouseClick, "target", coords.Point{}, mouse.LeftButton)
	recordAction(ctx, nil, WaitUIExist, "target", nil, nil)
	if err := Save(ctx); err != nil {
		t.Errorf("Save() = %v; want nil", err)
	}

	if !preExisting {
		if _, err := os.Stat(tempLoggerFile); !os.IsNotExist(err) {
			t.Errorf("%s was created unexpectedly (stat err=%v)", tempLoggerFile, err)
		}
	}
}
