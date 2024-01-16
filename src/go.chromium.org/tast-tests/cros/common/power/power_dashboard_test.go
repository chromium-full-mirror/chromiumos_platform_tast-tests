// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
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

	tsStartMs = 5100
	tsEndMs   = 7240
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

func initializeValues() *perf.Values {
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
	return p
}

func TestCheckpointTags(t *testing.T) {
	p := initializeValues()

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

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}
	example := [][]string{{"none"}, {"work1"}, {"work1", "work2"}, {"work1", "work3"}, {"work3"}}

	compareTags(t, tags, example)
}

func TestCheckpointTagsOverlap(t *testing.T) {
	p := initializeValues()

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

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}
	example := [][]string{{"none"}, {"work1"}, {"work1"}, {"work1"}, {"work1"}}

	compareTags(t, tags, example)
}

func TestCheckpointTagsEmpty(t *testing.T) {
	p := initializeValues()

	c := perf.NewCheckpoints()

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}

	if tags != nil {
		t.Fatal("Checkpoints should generate nil but instead generated tags: ", tags)
	}
}

func TestCheckpointTagsNil(t *testing.T) {
	p := initializeValues()

	tags, err := tagTimelineWithCheckpoints(p, nil)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}

	if tags != nil {
		t.Fatal("Checkpoints should generate nil but instead generated tags: ", tags)
	}
}

func TestCheckpointTagsFrontBack(t *testing.T) {
	p := initializeValues()

	c := perf.NewCheckpoints()
	idle1 := perf.NewSection(time.UnixMilli(idle1StartMs))
	idle1.SetEnd(time.UnixMilli(idle1EndMs))
	c.AddSectionForTesting("idle1", idle1)
	idle2 := perf.NewSection(time.UnixMilli(idle2StartMs))
	idle2.SetEnd(time.UnixMilli(idle2EndMs))
	c.AddSectionForTesting("idle2", idle2)

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}

	if tags != nil {
		t.Fatal("Checkpoints should generate nil but instead generated tags: ", tags)
	}
}

// TestCheckpointTagsOnTimestamp tests the tagging behavior when the start and
// end of a Checkpoint Section each fall on a different timestamp.
// Example:
// Data timestamp: t1 t2 t3 t4 t5
// Checkpoint Section: [t2, t4]
// Then t2 & t3 count in this Section, while t4 is not. Only the left end is
// included in tagging.
func TestCheckpointTagsOnTimestamp(t *testing.T) {
	p := initializeValues()

	c := perf.NewCheckpoints()
	s := perf.NewSection(time.UnixMilli(tsStartMs))
	s.SetEnd(time.UnixMilli(tsEndMs))
	c.AddSectionForTesting("work", s)

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}
	example := [][]string{{"none"}, {"work"}, {"work"}, {"none"}, {"none"}}

	compareTags(t, tags, example)
}

// TestCheckpointTagsOnSameTimestamp tests the tagging behavior when the start and
// end of a Checkpoint Section both fall on the same timestamp.
// Example:
// Data timestamp: t1 t2 t3 t4 t5
// Checkpoint Section: [t2, t2]
// Then no data point get tagged in this Section.
func TestCheckpointTagsOnSameTimestamp(t *testing.T) {
	p := initializeValues()

	c := perf.NewCheckpoints()
	s := perf.NewSection(time.UnixMilli(tsStartMs))
	c.AddSectionForTesting("work", s)

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}

	if tags != nil {
		t.Fatal("Checkpoints should generate nil but instead generated tags: ", tags)
	}
}

func TestValuesWithNoData(t *testing.T) {
	// p has metrics but no data.
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
	p.Append(data)
	p.Append(ts)

	c := perf.NewCheckpoints()
	work1 := perf.NewSection(time.UnixMilli(work1StartMs))
	work1.SetEnd(time.UnixMilli(work1EndMs))

	tags, err := tagTimelineWithCheckpoints(p, c)
	if err != nil {
		t.Fatal("Fail to tag Timeline Values with Checkpoints: ", err)
	}

	if tags != nil {
		t.Fatal("Checkpoints should generate nil but instead generated tags: ", tags)
	}
}
