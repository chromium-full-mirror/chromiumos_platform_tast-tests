// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.chromium.org/tast/core/testutil"
)

const (
	idle1StartMs = 100
	idle1EndMs   = 10200

	workStartMs = 50300
	workEndMs   = 100400

	idle2StartMs = 200500
	idle2EndMs   = 600600
)

func saveCheckpointsAndCompare(t *testing.T, c *Checkpoints, goldenPath string) {
	t.Helper()

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := c.Save(td); err != nil {
		t.Fatal("Failed saving JSON: ", err)
	}

	path := filepath.Join(td, "checkpoint_log.json")
	if err := jsonEquals(path, goldenPath); err != nil {
		data, _ := ioutil.ReadFile(path)
		t.Fatalf("%v; output:\n%s", err, string(data))
	}
}

func TestCheckpoints(t *testing.T) {
	clock := NewFakeClock()
	c := NewCheckpoints(SetClock(clock))

	// Fake workload: idle -> work -> idle.
	clock.Advance(time.UnixMilli(idle1StartMs).Sub(time.UnixMilli(0)))
	idle1 := c.NewSection("idle")
	clock.Advance(time.UnixMilli(idle1EndMs).Sub(time.UnixMilli(idle1StartMs)))
	c.EndSection(idle1)

	clock.Advance(time.UnixMilli(workStartMs).Sub(time.UnixMilli(idle1EndMs)))
	work := c.NewSection("work")
	clock.Advance(time.UnixMilli(workEndMs).Sub(time.UnixMilli(workStartMs)))
	c.EndSection(work)

	clock.Advance(time.UnixMilli(idle2StartMs).Sub(time.UnixMilli(workEndMs)))
	idle2 := c.NewSection("idle")
	clock.Advance(time.UnixMilli(idle2EndMs).Sub(time.UnixMilli(idle2StartMs)))
	c.EndSection(idle2)

	saveCheckpointsAndCompare(t, c, "testdata/TestCheckpoints.json")
}
