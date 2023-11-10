// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testutil"
)

func loadJSON(path string) (interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening %s", path)
	}
	defer f.Close()

	var v interface{}
	if err := json.NewDecoder(f).Decode(&v); err != nil {
		return nil, errors.Wrapf(err, "failed decoding %s", path)
	}
	return v, nil
}

func jsonEquals(path1, path2 string) error {
	v1, err := loadJSON(path1)
	if err != nil {
		return err
	}
	v2, err := loadJSON(path2)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(v1, v2) {
		return errors.New("JSON files differ")
	}
	return nil
}

func saveAndCompare(t *testing.T, p *Values, goldenPath string) {
	t.Helper()

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	if err := p.Save(td); err != nil {
		t.Fatal("Failed saving JSON: ", err)
	}

	path := filepath.Join(td, "results-chart.json")
	if err := jsonEquals(path, goldenPath); err != nil {
		data, _ := ioutil.ReadFile(path)
		t.Fatalf("%v; output:\n%s", err, string(data))
	}
}

func TestSetSingle(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter}
	p := NewValues()

	p.Set(metric, 1)
	p.Set(metric, 2)
	p.Set(metric, 3)

	saveAndCompare(t, p, "testdata/TestSetSingle.json")
}

func TestSetSinglePanic(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter}
	p := NewValues()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Did not panic")
		}
	}()

	// Set with multiple values panics for single-valued metrics.
	p.Set(metric, 1, 2, 3)
}

func TestSetMultiple(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter, Multiple: true}
	p := NewValues()

	p.Set(metric, 1, 2, 3)
	p.Set(metric, 4, 5, 6)

	saveAndCompare(t, p, "testdata/TestSetMultiple.json")
}

func TestMergeValues(t *testing.T) {
	metric1 := Metric{Name: "metric1", Unit: "unit", Direction: SmallerIsBetter, Multiple: true}
	metric2 := Metric{Name: "metric2", Unit: "unit", Direction: BiggerIsBetter, Multiple: true}
	metric3 := Metric{Name: "metric3", Unit: "unit", Direction: BiggerIsBetter, Multiple: false}
	p1 := NewValues()
	p2 := NewValues()
	p3 := NewValues()

	p1.Set(metric1, 3, 2, 3)
	p2.Set(metric1, 1, 9, 6)
	p2.Set(metric2, 2, 0)
	p3.Set(metric1, 1, 9, 9, 0)
	p3.Set(metric2, 0, 7, 2, 8)
	p3.Set(metric3, 1000)
	p1.Merge(p2, p3)

	saveAndCompare(t, p1, "testdata/TestMergeValues.json")
}

func TestAppendSinglePanic(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter}
	p := NewValues()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Did not panic")
		}
	}()

	// Append panics for single-valued metrics.
	p.Append(metric, 1)
}

func TestAppendMultiple(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter, Multiple: true}
	p := NewValues()

	p.Append(metric, 1)
	p.Append(metric, 2, 3)

	saveAndCompare(t, p, "testdata/TestAppendMultiple.json")
}

func TestSave(t *testing.T) {
	var (
		metric1  = Metric{Name: "metric1", Unit: "unit1", Direction: SmallerIsBetter}
		metric2  = Metric{Name: "metric2", Unit: "unit2", Direction: SmallerIsBetter, Multiple: true}
		metric3a = Metric{Name: "metric3", Variant: "a", Unit: "unit3a", Direction: SmallerIsBetter}
		metric3b = Metric{Name: "metric3", Variant: "b", Unit: "unit3b", Direction: BiggerIsBetter}
	)

	p := NewValues()
	p.Set(metric1, 100)
	p.Set(metric2, 200, 201, 202)
	p.Set(metric3a, 300)
	p.Set(metric3b, 310)

	saveAndCompare(t, p, "testdata/TestSave.json")
}

func TestSave_Zero(t *testing.T) {
	var (
		metric1 = Metric{Name: "metric1", Unit: "unit1", Direction: SmallerIsBetter}
		metric2 = Metric{Name: "metric2", Unit: "unit2", Direction: SmallerIsBetter, Multiple: true}
	)

	p := NewValues()
	p.Set(metric1, 0)
	p.Set(metric2)

	saveAndCompare(t, p, "testdata/TestSave_Zero.json")
}

func TestSetInterval(t *testing.T) {
	metric1 := Metric{Name: "metric1", Unit: "unit", Direction: SmallerIsBetter, Multiple: true, Interval: "5s"}
	metric2 := Metric{Name: "metric2", Unit: "unit", Direction: SmallerIsBetter, Multiple: true, Interval: "TPS.t"}
	metric3 := Metric{Name: "metric3", Unit: "unit", Direction: SmallerIsBetter, Multiple: true, Interval: "t"}

	p := NewValues()

	p.Set(metric1, 1, 2, 3)
	p.Set(metric2, 1, 2, 3)
	p.Set(metric3, 1, 2, 3)

	saveAndCompare(t, p, "testdata/TestSetInterval.json")
}

func TestIntervalForSingleMetricPanic(t *testing.T) {
	metric := Metric{Name: "metric", Unit: "unit", Direction: SmallerIsBetter, Interval: "5s"}
	p := NewValues()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Did not panic")
		}
	}()

	p.Set(metric, 1)
}

func testSetMalformedInterval(t *testing.T, metric Metric) {
	t.Helper()

	p := NewValues()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Did not panic")
		}
	}()

	p.Set(metric, 1)
}

func TestSetMalformedInterval(t *testing.T) {
	// Intervals must end in "s" or ".t", or simply be "t".
	testSetMalformedInterval(t, Metric{Name: "metric", Unit: "unit", Interval: "5ms"})
	testSetMalformedInterval(t, Metric{Name: "metric", Unit: "unit", Interval: "TPSt"})
	testSetMalformedInterval(t, Metric{Name: "metric", Unit: "unit", Interval: "555"})

}

func initializeValues() *Values {
	data := Metric{
		Name:     "data",
		Unit:     "count",
		Multiple: true,
		Interval: "t",
	}
	ts := Metric{
		Name:       "t",
		Unit:       "s",
		Multiple:   true,
		HasStartTs: true,
		StartTs:    time.Unix(3, 0),
	}
	p := NewValues()
	p.Append(data, 1, 2, 3, 4, 5)
	p.Append(ts, 1.05, 2.1, 3.17, 4.24, 5.31)
	return p
}

func checkValuesEmpty(t *testing.T, p *Values) {
	if len(p.GetValues()) != 0 {
		json, _ := p.toCrosbolt()
		t.Fatalf("Values should be empty, but is not: \n%s", string(json))
	}
}

func TestFilterTimelineByCheckpoints(t *testing.T) {
	p := initializeValues()

	c := NewCheckpoints()
	work1 := NewSection(time.UnixMilli(work1StartMs))
	work1.SetEnd(time.UnixMilli(work1EndMs))
	c.AddSectionForTesting("work1", work1)
	work2 := NewSection(time.UnixMilli(work2StartMs))
	work2.SetEnd(time.UnixMilli(work2EndMs))
	c.AddSectionForTesting("work2", work2)
	work3 := NewSection(time.UnixMilli(work3StartMs))
	work3.SetEnd(time.UnixMilli(work3EndMs))
	c.AddSectionForTesting("work3", work3)

	saveAndCompare(t, p.FilterTimelineByCheckpoints(c), "testdata/TestFilterTimelineByCheckpoints.json")
}

// One Checkpoint name maps to multiple Sections.
func TestFilterTimelineByCheckpointsOverlap(t *testing.T) {
	p := initializeValues()

	c := NewCheckpoints()
	work1 := NewSection(time.UnixMilli(work1StartMs))
	work1.SetEnd(time.UnixMilli(work1EndMs))
	c.AddSectionForTesting("work1", work1)
	work2 := NewSection(time.UnixMilli(work2StartMs))
	work2.SetEnd(time.UnixMilli(work2EndMs))
	c.AddSectionForTesting("work1", work2)
	work3 := NewSection(time.UnixMilli(work3StartMs))
	work3.SetEnd(time.UnixMilli(work3EndMs))
	c.AddSectionForTesting("work1", work3)

	saveAndCompare(t, p.FilterTimelineByCheckpoints(c), "testdata/TestFilterTimelineByCheckpointsOverlap.json")
}

func TestFilterTimelineByCheckpointsEmpty(t *testing.T) {
	p := initializeValues()
	c := NewCheckpoints()
	filtered := p.FilterTimelineByCheckpoints(c)
	checkValuesEmpty(t, filtered)
}

// Checkpoints do not cover the timestamps in Values.
func TestFilterTimelineByCheckpointsFrontBack(t *testing.T) {
	p := initializeValues()

	c := NewCheckpoints()
	idle1 := NewSection(time.UnixMilli(idle1StartMs))
	idle1.SetEnd(time.UnixMilli(idle1EndMs))
	c.AddSectionForTesting("idle1", idle1)
	idle2 := NewSection(time.UnixMilli(idle2StartMs))
	idle2.SetEnd(time.UnixMilli(idle2EndMs))
	c.AddSectionForTesting("idle2", idle2)

	filtered := p.FilterTimelineByCheckpoints(c)
	checkValuesEmpty(t, filtered)
}

// TestFilterTimelineByCheckpointsOnTimestamp tests the filtering behavior when
// the start and end of a Checkpoint Section each fall on a different timestamp.
// Example:
// Data timestamp: t1 t2 t3 t4 t5
// Checkpoint Section: [t2, t4]
// Then t2 & t3 count in this Section, while t4 is not. Only the left end is
// included in tagging.
func TestFilterTimelineByCheckpointsOnTimestamp(t *testing.T) {
	p := initializeValues()

	c := NewCheckpoints()
	s := NewSection(time.UnixMilli(tsStartMs))
	s.SetEnd(time.UnixMilli(tsEndMs))
	c.AddSectionForTesting("work", s)

	saveAndCompare(t, p.FilterTimelineByCheckpoints(c), "testdata/TestFilterTimelineByCheckpointsOnTimestamp.json")
}

// TestCheckpointTagsOnSameTimestamp tests the filtering behavior when the start
// and end of a Checkpoint Section both fall on the same timestamp.
// Example:
// Data timestamp: t1 t2 t3 t4 t5
// Checkpoint Section: [t2, t2]
// Then no data point get counted in this Section.
func TestFilterTimelineByCheckpointsOnSameTimestamp(t *testing.T) {
	p := initializeValues()

	c := NewCheckpoints()
	s := NewSection(time.UnixMilli(tsStartMs))
	c.AddSectionForTesting("work", s)

	filtered := p.FilterTimelineByCheckpoints(c)
	checkValuesEmpty(t, filtered)
}

func saveAsAndCompare(t *testing.T, p *Values, goldenPath string, format Format, expectedFileName string) {
	t.Helper()

	td := testutil.TempDir(t)
	defer os.RemoveAll(td)

	runGenGUID = func(context.Context) (string, error) { return "FAKE-GUID", nil }
	if err := p.SaveAs(context.Background(), td, format); err != nil {
		t.Fatal("Failed saving JSON: ", err)
	}

	path := filepath.Join(td, expectedFileName)
	if err := jsonEquals(path, goldenPath); err != nil {
		data, _ := ioutil.ReadFile(path)
		t.Fatalf("%v; output:\n%s", err, string(data))
	}
}

func saveFormat(t *testing.T, format Format, expectedOutput, expectedFileName string) {
	// Note: format=Chromeperf does not currently support multiple variants.
	var (
		metric1 = Metric{Name: "metric1", Unit: "unit1", Direction: SmallerIsBetter}
		metric2 = Metric{Name: "metric2", Unit: "unit2", Direction: SmallerIsBetter, Multiple: true}
		metric3 = Metric{Name: "metric3", Unit: "bytes", Direction: BiggerIsBetter}
	)

	p := NewValues()
	p.Set(metric1, 100)
	p.Set(metric2, 200, 201, 202)
	p.Set(metric3, 300)

	saveAsAndCompare(t, p, expectedOutput, format, expectedFileName)
}

func TestSaveAsCrosbolt(t *testing.T) {
	saveFormat(t, Crosbolt, "testdata/TestSaveAsCrosbolt.json", "results-chart.json")
}

func TestSaveAsChromeperf(t *testing.T) {
	saveFormat(t, Chromeperf, "testdata/TestSaveAsChromeperf.json", "perf_results.json")
}
