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

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast/core/testutil"
)

const (
	idle1StartMs = 100
	idle1EndMs   = 10200

	work1StartMs = 50300
	work2StartMs = 70315
	work2EndMs   = 80435
	work3StartMs = 90000
	work1EndMs   = 100400
	work3EndMs   = 110000

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

	clock.Advance(time.UnixMilli(work1StartMs).Sub(time.UnixMilli(idle1EndMs)))
	work := c.NewSection("work")
	clock.Advance(time.UnixMilli(work1EndMs).Sub(time.UnixMilli(work1StartMs)))
	c.EndSection(work)

	clock.Advance(time.UnixMilli(idle2StartMs).Sub(time.UnixMilli(work1EndMs)))
	idle2 := c.NewSection("idle")
	clock.Advance(time.UnixMilli(idle2EndMs).Sub(time.UnixMilli(idle2StartMs)))
	c.EndSection(idle2)

	saveCheckpointsAndCompare(t, c, "testdata/TestCheckpoints.json")
}

func TestCheckpointsNoEnd(t *testing.T) {
	clock := NewFakeClock()
	c := NewCheckpoints(SetClock(clock))

	clock.Advance(time.UnixMilli(work1StartMs).Sub(time.UnixMilli(idle1EndMs)))
	c.NewSection("work")

	saveCheckpointsAndCompare(t, c, "testdata/TestCheckpointsNoEnd.json")
}

func TestFlatten(t *testing.T) {
	clock := NewFakeClock()
	c := NewCheckpoints(SetClock(clock))

	clock.Advance(time.UnixMilli(work1StartMs).Sub(time.UnixMilli(0)))
	work1 := c.NewSection("work1")

	clock.Advance(time.UnixMilli(work2StartMs).Sub(time.UnixMilli(work1StartMs)))
	work2 := c.NewSection("work2")

	clock.Advance(time.UnixMilli(work2EndMs).Sub(time.UnixMilli(work2StartMs)))
	c.EndSection(work2)

	clock.Advance(time.UnixMilli(work3StartMs).Sub(time.UnixMilli(work2EndMs)))
	work3 := c.NewSection("work3")

	clock.Advance(time.UnixMilli(work1EndMs).Sub(time.UnixMilli(work3StartMs)))
	c.EndSection(work1)

	clock.Advance(time.UnixMilli(work3EndMs).Sub(time.UnixMilli(work1EndMs)))
	c.EndSection(work3)

	example := []CheckpointEvent{
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1StartMs),
			IsStart:        true,
		},
		CheckpointEvent{
			CheckpointName: "work2",
			Ts:             time.UnixMilli(work2StartMs),
			IsStart:        true,
		},
		CheckpointEvent{
			CheckpointName: "work2",
			Ts:             time.UnixMilli(work2EndMs),
			IsStart:        false,
		},
		CheckpointEvent{
			CheckpointName: "work3",
			Ts:             time.UnixMilli(work3StartMs),
			IsStart:        true,
		},
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1EndMs),
			IsStart:        false,
		},
		CheckpointEvent{
			CheckpointName: "work3",
			Ts:             time.UnixMilli(work3EndMs),
			IsStart:        false,
		},
	}

	if !cmp.Equal(c.Flatten(), example) {
		t.Fatal("Checkpoints flattened in a wrong way: ", c.Flatten())
	}
}

func TestFlattenStartEndSameTs(t *testing.T) {
	clock := NewFakeClock()
	c := NewCheckpoints(SetClock(clock))

	clock.Advance(time.UnixMilli(work1StartMs).Sub(time.UnixMilli(0)))
	c.NewSection("work1")
	c.NewSection("work1")

	example := []CheckpointEvent{
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1StartMs),
			IsStart:        true,
		},
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1StartMs),
			IsStart:        true,
		},
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1StartMs),
			IsStart:        false,
		},
		CheckpointEvent{
			CheckpointName: "work1",
			Ts:             time.UnixMilli(work1StartMs),
			IsStart:        false,
		},
	}

	if !cmp.Equal(c.Flatten(), example) {
		t.Fatal("Checkpoints flattened in a wrong way: ", c.Flatten())
	}
}
