// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package perf provides utilities to build a JSON file that can be uploaded to
// Chrome Performance Dashboard (https://chromeperf.appspot.com/).
//
// Measurements processed by this package are stored in
// tests/<test-name>/results-chart.json in the Tast results dir.  The data is
// typically read by the Autotest TKO parser. In order to have metrics
// uploaded, they have to be listed here:
// src/third_party/autotest/files/tko/perf_upload/perf_dashboard_config.json
//
// Chrome Performance Dashboard docs can be found here:
// https://github.com/catapult-project/catapult/tree/master/dashboard // nocheck
//
// Usage example:
//
//	pv := perf.NewValues()
//	pv.Set(perf.Metric{
//	    Name:       "mytest_important_quantity"
//	    Unit:       "gizmos"
//	    Direction:  perf.BiggerIsBetter
//	}, 42)
//	if err := pv.Save(s.OutDir()); err != nil {
//	    s.Error("Failed saving perf data: ", err)
//	}
//
// # Remote usage example
//
// Protocol buffer definition:
//
//	import "values.proto";
//	service ExampleService {
//	    rpc Method (google.protobuf.Empty)
//	        returns (tast.common.perf.perfpb.Values) {}
//	}
//
// In order to "import values.proto", add a -I argument pointing at
// src/go.chromium.org/tast-tests/cros/common/perf/perfpb/ to the protoc command in your
// service's gen.go file. See src/go.chromium.org/tast-tests/cros/services/cros/arc/gen.go
// for an example.
//
// Service:
//
//	import "go.chromium.org/tast-tests/cros/common/perf"
//	import "go.chromium.org/tast-tests/cros/common/perf/perfpb"
//	func (s *ExampleService) Method() (*perfpb.Values, error) {
//	    p := perf.NewValues()
//	    ... // Do some computation that generates perf values in p.
//	    return p.Proto(), nil
//	}
//
// Test:
//
//	import "go.chromium.org/tast-tests/cros/common/perf"
//	func TestMethod(ctx context.Context, s *testing.State) {
//	    ... // Set up gRPC, ExampleServiceClient.
//	    res, err := service.Method()
//	    if err != nil {
//	        s.Fatal("RPC failed: ", err)
//	    }
//	    if err := perf.NewValuesFromProto(res).Save(s.OutDir()); err != nil {
//	        s.Fatal("Failed to save perf results: ", err)
//	    }
//	}
package perf

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/google/uuid"

	"golang.org/x/exp/maps"

	"go.chromium.org/tast-tests/cros/common/perf/perfpb"
	"go.chromium.org/tast/core/errors"
)

var (
	// nameRe defines valid names (Name and Variant).
	nameRe = regexp.MustCompile("^[a-zA-Z0-9._-]{1,256}$")
	// unitRe defines valid units.
	unitRe = regexp.MustCompile("^[a-zA-Z0-9._-]{1,32}$")
	// intervalRe defines valid intervals. Interval can either be a metric
	// name ending in ".t", just "t", or a number followed by "s".
	intervalRe = regexp.MustCompile("t|(^[a-zA-Z0-9._-]{1,254}.t$)|(^[0-9(.)*[0-9]+s)$")
)

// DefaultVariantName is the default variant name treated specially by the dashboard.
const DefaultVariantName = "summary"

// genGUID generates a guid for diagnostic structs.
func genGUID(ctx context.Context) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// Overridable function pointer for tests.
var runGenGUID = genGUID

// Direction indicates which direction of change (bigger or smaller) means improvement
// of a performance metric.
type Direction int

const (
	// SmallerIsBetter means the performance metric is considered improved when it decreases.
	SmallerIsBetter Direction = iota

	// BiggerIsBetter means the performance metric is considered improved when it increases.
	BiggerIsBetter
)

// Metric defines the schema of a performance metric.
type Metric struct {
	// Name is the name of the chart this performance metric appears in.
	Name string

	// Variant is the name of this performance metric in a chart. If this is empty,
	// DefaultVariantName is used. It is treated specially by the dashboard.
	// Charts containing only one performance metric should stick with the default.
	Variant string

	// Unit is a unit name to describe values of this performance metric.
	Unit string

	// Direction indicates which direction of change (bigger or smaller) means improvement
	// of this performance metric.
	Direction Direction

	// Multiple specifies if this performance metric can contain multiple values at a time.
	Multiple bool

	// Interval is an optional field that specifies the timing of the measurements, if this
	// metric is a timeline with Multiple set to true. Interval can either be the name of
	// another metric, where that metric holds time values that directly correspond to the
	// performance values in this metric, or the number in seconds specifying how much time
	// passes in between performance values in this metric. This number in seconds must be
	// followed by "s". If Multiple is false, the Interval is ignored.
	Interval string

	// HasStartTs is true only if the current Metric is generated by
	// timestampSource. It is false otherwise.
	HasStartTs bool

	// StartTs is the timestamp at the start of the timeline. It is only valid
	// if HasStartTs is true. The combination of StartTs and timestampSource
	// provides Unix timestamps since Epoch for every data point.
	StartTs time.Time
}

func (s *Metric) setDefaults() {
	if len(s.Variant) == 0 {
		s.Variant = DefaultVariantName
	}
}

// Maps a Metric unit type to a histogram unit type. TODO(stevenjb) Investigate
var supportedUnits = map[string]string{
	"bytes":          "sizeInBytes",
	"bytesPerSecond": "bytesPerSecond",
	"J":              "J",
	"W":              "W",
	"count":          "count",
	"ms":             "ms",
	"n%":             "n%",
	"sigma":          "sigma",
	"tsMs":           "tsMs",
}

func (s *Metric) histogramUnit() string {
	unit, ok := supportedUnits[s.Unit]
	if !ok {
		// "unitless" is a valid histogram unit type. Returning "unitless" is
		// preferable to throwing an error here.
		return "unitless"
	}
	switch s.Direction {
	case BiggerIsBetter:
		unit += "_biggerIsBetter"
	case SmallerIsBetter:
		unit += "_smallerIsBetter"
	}
	return unit
}

// Values holds performance metric values.
type Values struct {
	values map[Metric][]float64
}

// NewValues returns a new empty Values.
func NewValues() *Values {
	return &Values{values: make(map[Metric][]float64)}
}

// GetValues returns stored metrics values.
func (p *Values) GetValues() map[Metric][]float64 {
	return p.values
}

// GetValueByMetric returns specific metric values.
func (p *Values) GetValueByMetric(metric Metric) []float64 {
	return p.values[metric]
}

// MergeWithSuffix merges all data points of vs into this Values structure
// optionally adding suffix to the value name.
func (p *Values) MergeWithSuffix(suffix string, vs ...*Values) {
	for _, val := range vs {
		if val == nil {
			continue
		}
		for k, v := range val.values {
			suffixedK := k
			suffixedK.Name += suffix
			if k.Multiple {
				p.Append(suffixedK, v...)
			} else {
				if vv, c := p.values[suffixedK]; c {
					panic(fmt.Sprint("MergeWithSuffix(suffix='", suffix, "'): Single-valued metric {", suffixedK, "} already present as {", vv, "}. Cannot merge with another value."))
				}
				p.Set(suffixedK, v...)
			}
		}
	}
}

// Merge merges all data points of vs into this Values structure.
func (p *Values) Merge(vs ...*Values) {
	p.MergeWithSuffix("", vs...)
}

// NewValuesFromProto creates a Values from a perfpf.Values.
func NewValuesFromProto(vs ...*perfpb.Values) *Values {
	p := NewValues()
	for _, val := range vs {
		for _, v := range val.Values {
			m := Metric{
				Name:      v.Name,
				Variant:   v.Variant,
				Unit:      v.Unit,
				Direction: Direction(v.Direction),
				Multiple:  v.Multiple,
			}
			if v.Multiple {
				p.Append(m, v.Value...)
			} else {
				p.Set(m, v.Value...)
			}
		}
	}
	return p
}

// Append appends performance metrics values. It can be called only for multi-valued
// performance metrics.
func (p *Values) Append(s Metric, vs ...float64) {
	s.setDefaults()
	if !s.Multiple {
		panic("Append must not be called for single-valued data series")
	}
	p.values[s] = append(p.values[s], vs...)
	validate(s, p.values[s])
}

// Set sets a performance metric value(s).
func (p *Values) Set(s Metric, vs ...float64) {
	s.setDefaults()
	p.values[s] = vs
	validate(s, p.values[s])
}

// mapTsMetricToData maps every timestampSource Metric to the data Metrics that
// use it. It assumes that Values come from perf.Timeline.
func (p *Values) mapTsMetricToData() map[Metric][]Metric {
	tsMetricToData := make(map[Metric][]Metric)
	for metric := range p.GetValues() {
		if metric.HasStartTs {
			tsMetricToData[metric] = make([]Metric, 0)
		}
	}
	for metric := range p.GetValues() {
		if metric.HasStartTs {
			continue
		}
		for tsMetric := range tsMetricToData {
			if metric.Interval == tsMetric.Name {
				tsMetricToData[tsMetric] = append(tsMetricToData[tsMetric], metric)
			}
		}
	}
	return tsMetricToData
}

// ConvertToUnixTss takes a timestampSource Metric, and translates the relative
// timestamps into Unix timestamps. It assumes that Values come from
// perf.Timeline.
func (p *Values) ConvertToUnixTss(tsMetric Metric) ([]time.Time, error) {
	if !tsMetric.HasStartTs {
		return nil, errors.Errorf("%s Metric does not have a start timestamp", tsMetric.Name)
	}
	unixTss := make([]time.Time, len(p.GetValueByMetric(tsMetric)))
	for i, relativeTs := range p.GetValueByMetric(tsMetric) {
		unixTss[i] = tsMetric.StartTs.Add(time.Duration(relativeTs * float64(time.Second)))
	}
	return unixTss, nil
}

// filterMetric creates a new Metric with suffix to indicate the Checkpoint.
func filterMetric(m Metric, checkpointName string) Metric {
	filteredMetric := m
	filteredMetric.Name = filteredMetric.Name + "." + checkpointName
	if !filteredMetric.HasStartTs {
		filteredMetric.Interval = filteredMetric.Interval + "." + checkpointName
	}
	return filteredMetric
}

// updateFilteredValues updates f Values. Iterate over m Metrics in p Values,
// for every data point at index i, add it to f Values if there is a Checkpoint
// name that is on at its Unix timestamp.
func updateFilteredValues(f, p *Values, m []Metric, i int, names map[string]int) {
	for name := range names {
		for _, metric := range m {
			filteredMetric := filterMetric(metric, name)
			f.Append(filteredMetric, p.GetValueByMetric(metric)[i])
		}
	}
}

// FilterTimelineByCheckpoints assumes that Values come from perf.Timeline.
// It filters perf.Values by Checkpoints, and generates a new perf.Values
// object. The number of Metrics in the new Values =
// (# of Metrics in the old Values * # of Checkpoints).
func (p *Values) FilterTimelineByCheckpoints(ckpts *Checkpoints) (*Values, error) {
	filtered := NewValues()
	btoi := map[bool]int{true: 1, false: -1}

	// Map a timestampSource Metric to the data Metrics that use it.
	tsMetricToData := p.mapTsMetricToData()

	events := ckpts.Flatten()
	for tsMetric, data := range tsMetricToData {
		// Calculate Unix timestamps from relative timestamps in perf.Timeline.
		unixTss, err := p.ConvertToUnixTss(tsMetric)
		if err != nil {
			return nil, err
		}

		// Compare Unix timestamps with Checkpoints. If a Unix timestamp is
		// within the Checkpoint coverage, add the corresponding data points to
		// the new perf.Values under the name "Metric.Checkpoint".
		j := 0
		started := make(map[string]int)

		for i, unixTs := range unixTss {
			for ; j < len(events) && !events[j].Ts.After(unixTs); j++ {
				// Count the Checkpoints that are turned on now for a timestamp.
				name := events[j].CheckpointName
				started[name] += btoi[events[j].IsStart]
				if started[name] == 0 {
					delete(started, name)
				}
			}
			// Update filtered Values. For every data point at index i, add it
			// to filtered Values if there is a Checkpoint that is on at its
			// Unix timestamp.
			updateFilteredValues(filtered, p, append(data, tsMetric), i, started)
		}
	}

	return filtered, nil
}

// Format describes the output format for perf data.
type Format int

const (
	// Crosbolt is used for ChromeOS infra dashboards (go/crosbolt).
	Crosbolt Format = iota
	// Chromeperf is used for ChromeOS infra dashboards (go/chromeperf).
	Chromeperf
	// DebugCSV is used for manual debugging and viewing result summary.
	DebugCSV
)

func (format Format) fileName() (string, error) {
	switch format {
	case Crosbolt:
		return "results-chart.json", nil
	case Chromeperf:
		return "perf_results.json", nil
	case DebugCSV:
		return "debug_summary.csv", nil
	default:
		return "", errors.Errorf("invalid perf format: %d", format)
	}
}

// traceData is a struct corresponding to a trace entry in Chrome Performance Dashboard JSON.
// See: https://github.com/catapult-project/catapult/blob/master/dashboard/docs/data-format.md // nocheck
type traceData struct {
	Units                string `json:"units"`
	ImprovementDirection string `json:"improvement_direction"`
	Type                 string `json:"type"`
	Interval             string `json:"interval,omitempty"`

	// These are pointers to permit us to include zero values in JSON representations.
	Value  *float64   `json:"value,omitempty"`
	Values *[]float64 `json:"values,omitempty"`
}

// diagnostic corresponds to the catapult Diagnostic struct preferred by
// go/chromeperf. For more info see:
// https://chromium.googlesource.com/catapult/+/HEAD/docs/histogram-set-json-format.md
// https://chromeperf.appspot.com/
type diagnostic struct {
	Type   string   `json:"type"`
	GUID   string   `json:"guid"`
	Values []string `json:"values"`
}

// diagnosticMap corresponds to the catapult DiagnosticMap struct.
type diagnosticMap struct {
	Benchmarks string `json:"benchmarks"`
}

// histogram corresponds to the catapult Histogram format preferred by
// go/chromeperf. See diagnostic struct for more info.
type histogram struct {
	Name         string        `json:"name"`
	Unit         string        `json:"unit"`
	Diagnostics  diagnosticMap `json:"diagnostics"`
	SampleValues []float64     `json:"sampleValues"`
	Running      [7]float64    `json:"running"`
	AllBins      [][]int       `json:"allBins"`
}

func (h *histogram) updateRunning() {
	sorted := h.SampleValues
	sort.Float64s(sorted)
	min := sorted[0]
	max := sorted[len(sorted)-1]
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	mean := sum / float64(len(sorted))
	variance := 0.0
	for _, v := range sorted {
		d := v - mean
		variance += d * d
	}
	h.Running = [7]float64{float64(len(sorted)), max, math.Log(mean), mean, min, sum, variance}
}

// toCrosbolt returns perf values formatted as json for crosbolt.
func (p *Values) toCrosbolt() ([]byte, error) {
	charts := &map[string]*map[string]*traceData{}

	for s := range p.values {
		// Need the original slice since we'll take a pointer to it.
		vs := p.values[s]

		// Avoid nil slices since they are encoded to null.
		if vs == nil {
			vs = []float64{}
		}

		traces, ok := (*charts)[s.Name]
		if !ok {
			traces = &map[string]*traceData{}
			(*charts)[s.Name] = traces
		}

		var t traceData
		t.Units = s.Unit
		if s.Direction == BiggerIsBetter {
			t.ImprovementDirection = "up"
		} else {
			t.ImprovementDirection = "down"
		}
		if s.Multiple {
			t.Type = "list_of_scalar_values"
			t.Interval = s.Interval
			t.Values = &vs
		} else {
			t.Type = "scalar"
			t.Value = &vs[0]
		}

		(*traces)[s.Variant] = &t
	}

	return json.MarshalIndent(charts, "", "  ")
}

// toChromeperf returns perf values formatted as json for chromeperf.
func (p *Values) toChromeperf(ctx context.Context) ([]byte, error) {
	guid, err := runGenGUID(ctx)
	if err != nil {
		return nil, err
	}

	diag := diagnostic{
		Type:   "GenericSet",
		GUID:   guid,
		Values: []string{"disk_image_size"},
	}

	hgrams := map[string]histogram{}
	for s, vs := range p.values {
		if vs == nil {
			continue
		}
		h, ok := hgrams[s.Name]
		if ok {
			// TODO(stevenjb): Handle Variances and resolve mismatched units.
			h.SampleValues = append(h.SampleValues, vs...)
			h.updateRunning()
		} else {
			h = histogram{
				Name:         s.Name,
				Unit:         s.histogramUnit(),
				Diagnostics:  diagnosticMap{Benchmarks: diag.GUID},
				SampleValues: vs,
				AllBins:      [][]int{{1}},
			}
			h.updateRunning()
			hgrams[s.Name] = h
		}
	}

	// The json file format is an array of diagnostic and histogram structs.
	var data []interface{}

	// Make diag the first entry.
	data = append(data, diag)

	// Append the hgrams entries in deterministic (Name) order.
	var keys []string
	for k := range hgrams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		data = append(data, hgrams[k])
	}

	return json.MarshalIndent(data, "", "  ")
}

// Save saves performance metric values as a JSON file named and formatted for
// crosbolt. outDir should be the output directory path obtained from
// testing.State.
func (p *Values) Save(outDir string) error {
	fileName, err := Crosbolt.fileName()
	if err != nil {
		return err
	}
	json, err := p.toCrosbolt()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(outDir, fileName), json, 0644)
}

// Proto converts this Values to something that can be passed in a gRPC call.
func (p *Values) Proto() *perfpb.Values {
	result := &perfpb.Values{}
	for k, v := range p.values {
		result.Values = append(result.Values, &perfpb.Value{
			Name:      k.Name,
			Variant:   k.Variant,
			Unit:      k.Unit,
			Direction: perfpb.Direction(k.Direction),
			Multiple:  k.Multiple,
			Interval:  k.Interval,
			Value:     v,
		})
	}
	return result
}

// SaveAs saves performance metric values in the format provided to outDir.
// outDir should be the output directory path obtained from testing.State.
// format must be either Crosbolt or Chromeperf.
func (p *Values) SaveAs(ctx context.Context, outDir string, format Format) error {
	fileName, err := format.fileName()
	if err != nil {
		return err
	}

	var json []byte
	switch format {
	case Crosbolt:
		json, err = p.toCrosbolt()
	case Chromeperf:
		json, err = p.toChromeperf(ctx)
	}

	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(outDir, fileName), json, 0644)
}

// SaveAsDebugCSV saves a CSV file of the value statistics to outDir. This is
// meant to help with manual debugging.
func (p *Values) SaveAsDebugCSV(outDir string) error {
	fileName, err := DebugCSV.fileName()
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(outDir, fileName))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	headers := []string{"name", "mean", "stddev", "min", "max", "# data"}
	if err = writer.Write(headers); err != nil {
		return errors.Wrapf(err, "Encountered error while writing %q", headers)
	}

	metrics := maps.Keys(p.GetValues())
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})
	for _, metric := range metrics {
		value := p.GetValueByMetric(metric)
		line := []string{metric.Name}
		if len(value) == 0 {
			nan := "NaN"
			line = append(line, nan, nan, nan, nan, "0")
		} else {
			sum := 0.0
			min := math.Inf(1)
			max := math.Inf(-1)
			for _, n := range value {
				sum += n
				if n < min {
					min = n
				}
				if n > max {
					max = n
				}
			}
			mean := sum / float64(len(value))
			stddev := 0.0
			for _, n := range value {
				stddev += math.Pow((n - mean), 2)
			}
			stddev = math.Sqrt(stddev / float64(len(value)))
			line = append(
				line,
				fmt.Sprintf("%f", mean),
				fmt.Sprintf("%f", stddev),
				fmt.Sprintf("%f", min),
				fmt.Sprintf("%f", max),
				fmt.Sprintf("%d", len(value)),
			)
		}
		if err := writer.Write(line); err != nil {
			return errors.Wrapf(err, "Encountered error while writing %q", line)
		}
	}
	return nil
}

func validate(s Metric, vs []float64) {
	if !nameRe.MatchString(s.Name) {
		panic(fmt.Sprintf("Metric has illegal Name: %v", s))
	}
	if !nameRe.MatchString(s.Variant) {
		panic(fmt.Sprintf("Metric has illegal Variant: %v", s))
	}
	if !unitRe.MatchString(s.Unit) {
		panic(fmt.Sprintf("Metric has illegal Unit: %v", s))
	}
	if !s.Multiple && len(vs) != 1 {
		panic(fmt.Sprintf("Metric requires single-valued: %v", s))
	}
	if s.Interval != "" {
		if !s.Multiple {
			panic("Metric Interval is non-empty but Multiple is false")
		}
		if !intervalRe.MatchString(s.Interval) {
			panic(fmt.Sprintf("Metric has illegal Interval: %v", s))
		}
	}
}
