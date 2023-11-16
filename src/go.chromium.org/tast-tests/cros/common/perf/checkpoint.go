// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"time"
)

// CheckpointsFileName is the .json file printed for debugging / testing.
const CheckpointsFileName = "checkpoint_log.json"

// Section records the start and end timestamp.
// Every Checkpoint can have multiple sections.
type Section struct {
	startTs time.Time
	endTs   time.Time
}

// SetEnd sets the end timestamp of a Section.
func (s *Section) SetEnd(ts time.Time) {
	s.endTs = ts
}

// MarshalJSON formats Section for printing in json.
func (s Section) MarshalJSON() ([]byte, error) {
	sectionJSON := []float64{
		float64(s.startTs.UnixMicro()) / 1000000.0,
		float64(s.endTs.UnixMicro()) / 1000000.0,
	}
	return json.Marshal(sectionJSON)
}

// NewSection initializes a new Section with both start and end timestamp set to
// the given time.
func NewSection(startTs time.Time) *Section {
	return &Section{
		startTs: startTs,
		endTs:   startTs,
	}
}

// Checkpoints is a map of Checkpoint(string) to a list of Sections.
// The timestamps used in Sections are supplied by Clock.
type Checkpoints struct {
	checkpoints map[string][]*Section
	clock       Clock
}

// NewSection initialize a new Section with the current timestamp.
func (c *Checkpoints) NewSection(checkpointName string) *Section {
	section := NewSection(c.clock.Now())
	if _, ok := c.checkpoints[checkpointName]; ok {
		c.checkpoints[checkpointName] = append(c.checkpoints[checkpointName], section)
	} else {
		c.checkpoints[checkpointName] = []*Section{section}
	}
	return section
}

// EndSection ends section with the current timestamp, and adds it to
// Checkpoints under the given checkpointName.
func (c *Checkpoints) EndSection(section *Section) {
	section.SetEnd(c.clock.Now())
}

// AddSectionForTesting adds a new Section under checkpointName. Used in unit
// tests only.
func (c *Checkpoints) AddSectionForTesting(checkpointName string, section *Section) {
	c.checkpoints[checkpointName] = append(c.checkpoints[checkpointName], section)
}

// Save saves checkpoints as a json file for debugging. outDir should be the
// output directory path obtained from testing.State.
func (c *Checkpoints) Save(outDir string) error {
	fileName := CheckpointsFileName
	json, err := json.MarshalIndent(*c, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(outDir, fileName), json, 0644)
}

// MarshalJSON formats Checkpoints for printing in json.
func (c Checkpoints) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.checkpoints)
}

// NewCheckpoints returns a new empty Checkpoints.
func NewCheckpoints(setters ...CheckpointsSetter) *Checkpoints {
	c := &Checkpoints{
		checkpoints: make(map[string][]*Section),
		clock:       &defaultClock{},
	}
	for _, setter := range setters {
		setter(c)
	}
	return c
}

// CheckpointsSetter sets options for Checkpoints.
type CheckpointsSetter func(*Checkpoints)

// SetClock sets a Clock implementation.
func SetClock(clock Clock) CheckpointsSetter {
	return func(c *Checkpoints) {
		c.clock = clock
	}
}

// CheckpointEvent is a step in the middle to process Checkpoints for
// power_log.json. It represents either the start or the end of a Checkpoint
// Section.
type CheckpointEvent struct {
	CheckpointName string
	Ts             time.Time
	IsStart        bool
}

// Flatten takes all the Checkpoints and flattens them into a sequence of
// starts / ends of all the Checkpoint Sections, sorted by timestamp.
func (c *Checkpoints) Flatten() []CheckpointEvent {
	var events []CheckpointEvent
	for name, sections := range c.checkpoints {
		for _, section := range sections {
			start := CheckpointEvent{
				CheckpointName: name,
				Ts:             section.startTs,
				IsStart:        true,
			}
			end := CheckpointEvent{
				CheckpointName: name,
				Ts:             section.endTs,
				IsStart:        false,
			}
			events = append(events, start)
			events = append(events, end)
		}
	}

	sort.Slice(events, func(i, j int) bool {
		// Sort events by timestamps.
		byTs := events[i].Ts.Before(events[j].Ts)
		// If two events share the same checkpoint name and have the same
		// timestamps, the start event should be sorted before the end event.
		startBeforeEnd := events[i].CheckpointName == events[j].CheckpointName &&
			events[i].Ts.Equal(events[j].Ts) &&
			events[i].IsStart && !events[j].IsStart
		return byTs || startBeforeEnd
	})
	return events
}
