// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/perf"
)

const (
	idle1StartMs = 500
	idle1EndMs   = 1000
	work1StartMs = 5000
	work2StartMs = 6000
	work2EndMs   = 7000
	work3StartMs = 7100
	work1EndMs   = 8000
	work3EndMs   = 8500
	idle2StartMs = 9000
	idle2EndMs   = 10000
)

func compareTags(t *testing.T, tags, example [][]string) {
	if len(tags) != len(example) {
		t.Fatal("Checkpoints generated wrong tags: ", tags)
	}

	for i, tag := range tags {
		tagMap := map[string]int{}
		for _, checkpointName := range tag {
			tagMap[checkpointName]++
		}
		exampleMap := map[string]int{}
		for _, checkpointName := range example[i] {
			exampleMap[checkpointName]++
		}
		if !cmp.Equal(tagMap, exampleMap) {
			t.Fatal("Checkpoints generated wrong tags: ", tags)
		}
	}
}

func TestCheckpointTags(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := perf.Metric{
		Name:     "data",
		Unit:     "count",
		Multiple: true,
		Interval: "t",
	}
	ts := perf.Metric{
		Name:       "t",
		Unit:       "s",
		Multiple:   true,
		HasStartTs: true,
		StartTs:    time.Unix(3, 0),
	}
	p := perf.NewValues()
	p.Append(data, 1, 2, 3, 4, 5)
	p.Append(ts, 1.05, 2.1, 3.17, 4.24, 5.31)

	c := perf.NewCheckpoints()
	work1 := perf.NewSection(time.UnixMilli(work1StartMs))
	work1.SetEnd(time.UnixMilli(work1EndMs))
	c.AddSectionForTesting("work1", work1)
	work2 := perf.NewSection(time.UnixMilli(work2StartMs))
	work2.SetEnd(time.UnixMilli(work2EndMs))
	c.AddSectionForTesting("work2", work2)
	work3 := perf.NewSection(time.UnixMilli(work3StartMs))
	work3.SetEnd(time.UnixMilli(work3EndMs))
	c.AddSectionForTesting("work3", work3)

	tags := tagTimelineWithCheckpoints(ctx, p, c)
	example := [][]string{{}, {"work1"}, {"work1", "work2"}, {"work1", "work3"}, {"work3"}}

	compareTags(t, tags, example)
}

func TestCheckpointTagsOverlap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := perf.Metric{
		Name:     "data",
		Unit:     "count",
		Multiple: true,
		Interval: "t",
	}
	ts := perf.Metric{
		Name:       "t",
		Unit:       "s",
		Multiple:   true,
		HasStartTs: true,
		StartTs:    time.Unix(3, 0),
	}
	p := perf.NewValues()
	p.Append(data, 1, 2, 3, 4, 5)
	p.Append(ts, 1.05, 2.1, 3.17, 4.24, 5.31)

	c := perf.NewCheckpoints()
	work1 := perf.NewSection(time.UnixMilli(work1StartMs))
	work1.SetEnd(time.UnixMilli(work1EndMs))
	c.AddSectionForTesting("work1", work1)
	work2 := perf.NewSection(time.UnixMilli(work2StartMs))
	work2.SetEnd(time.UnixMilli(work2EndMs))
	c.AddSectionForTesting("work1", work2)
	work3 := perf.NewSection(time.UnixMilli(work3StartMs))
	work3.SetEnd(time.UnixMilli(work3EndMs))
	c.AddSectionForTesting("work1", work3)

	tags := tagTimelineWithCheckpoints(ctx, p, c)
	example := [][]string{{}, {"work1"}, {"work1"}, {"work1"}, {"work1"}}

	compareTags(t, tags, example)
}

func TestCheckpointTagsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := perf.Metric{
		Name:     "data",
		Unit:     "count",
		Multiple: true,
		Interval: "t",
	}
	ts := perf.Metric{
		Name:       "t",
		Unit:       "s",
		Multiple:   true,
		HasStartTs: true,
		StartTs:    time.Unix(3, 0),
	}
	p := perf.NewValues()
	p.Append(data, 1, 2, 3, 4, 5)
	p.Append(ts, 1.05, 2.1, 3.17, 4.24, 5.31)

	c := perf.NewCheckpoints()

	tags := tagTimelineWithCheckpoints(ctx, p, c)
	example := [][]string{{}, {}, {}, {}, {}}

	compareTags(t, tags, example)
}

func TestCheckpointTagsFrontBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data := perf.Metric{
		Name:     "data",
		Unit:     "count",
		Multiple: true,
		Interval: "t",
	}
	ts := perf.Metric{
		Name:       "t",
		Unit:       "s",
		Multiple:   true,
		HasStartTs: true,
		StartTs:    time.Unix(3, 0),
	}
	p := perf.NewValues()
	p.Append(data, 1, 2, 3, 4, 5)
	p.Append(ts, 1.05, 2.1, 3.17, 4.24, 5.31)

	c := perf.NewCheckpoints()
	idle1 := perf.NewSection(time.UnixMilli(idle1StartMs))
	idle1.SetEnd(time.UnixMilli(idle1EndMs))
	c.AddSectionForTesting("idle1", idle1)
	idle2 := perf.NewSection(time.UnixMilli(idle2StartMs))
	idle2.SetEnd(time.UnixMilli(idle2EndMs))
	c.AddSectionForTesting("idle2", idle2)

	tags := tagTimelineWithCheckpoints(ctx, p, c)
	example := [][]string{{}, {}, {}, {}, {}}

	compareTags(t, tags, example)
}
