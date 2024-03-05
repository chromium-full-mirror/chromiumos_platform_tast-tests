// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cujrecorder has utilities for CUJ-style UI performance tests.
package cujrecorder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/cpu"
	perfSrc "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type metricGroup string

const (
	deprecatedGroupSmoothness metricGroup = "AnimationSmoothness"
	deprecatedGroupLatency    metricGroup = "InputLatency"
)

type histogramType int

const (
	countHistogram histogramType = iota
	enumHistogram
	cumulativeHistogram
)

const (
	tpsMetricPrefix    = "TPS."
	powerMetricPrefix  = "Power."
	memoryMetricPrefix = "Memory."
)

const checkInterval = 5 * time.Second

// SystemTraceConfigFile is a perfetto tracing config.
const SystemTraceConfigFile = "perfetto/system_trace_config.pbtxt"

const (
	// CPUCoolDownTimeout is the time to wait for CPU cool down.
	CPUCoolDownTimeout = 7 * time.Minute
	// CPUIdleTimeout is the time to wait for CPU utilization to go down.
	// This value should match waitIdleCPUTimeout in cpu/idle.go.
	CPUIdleTimeout = 2 * time.Minute
	// CPUPkgStateIdleTimeout is the max amount of time needed for CPU
	// pkg state activity to drop.
	CPUPkgStateIdleTimeout = 3 * time.Minute
	// CooldownTimeout is the time to wait for the CPU to idle if
	// CooldownBeforeRun is set.
	CooldownTimeout = CPUCoolDownTimeout + CPUIdleTimeout + CPUPkgStateIdleTimeout
)

// Annotation regex must follow the same formatting rules as perf.Metric.Name.
// However, the length of the annotation must be less, to accommodate for the
// Annotation.<annotation count> prefix. Additionally, periods are not allowed
// to avoid confusion between the three sections of this metric name.
var annotationRe = regexp.MustCompile("^[a-zA-Z0-9_-]{1,240}$")

// metricRecordMethod determines how the collected metrics are recorded.
type metricRecordMethod int

const (
	// metricRecordMethodNeutral records metrics by treating all samples from
	// different sources equally. Used for metrics such as latency, smoothness
	// where samples does not happen on top of each other and average represents
	// overall system status.
	metricRecordMethodNeutral = iota
	// metricRecordMethodSumAverageBySource records metrics from different sources
	// by adding up the average of the samples from each source. Used for metrics
	// such as memory where the samples should be added up to represent the system
	// status.
	metricRecordMethodSumAverageBySource
)

// MetricConfig is the configuration for the recorder.
type MetricConfig struct {
	// The name of the histogram to be recorded.
	histogramName string

	// The unit of the histogram, like "percent" or "ms".
	unit string

	// The direction of the histogram.
	direction perf.Direction

	// There are two special classes of metrics that are reported outside
	// of test scenarios: BootTime.* and ShutdownTime.*. This way the
	// regular "metrics difference" will be empty for these because
	// usually Chrome will load these before a test is started.
	// To collect these metrics we need to fetch absolute metrics values.
	bootAndShutdown bool

	// The type of the histogram. Currently supports count histograms
	// and enum histograms.
	histogramType histogramType

	// The map between enum values and names.
	enumValues map[int64]string

	// Determines how samples from different sources (i.e. browsers) should be
	// recorded.
	recordMethod metricRecordMethod
}

// NewSmoothnessMetricConfig creates a new MetricConfig instance for collecting
// animation smoothness data for the given histogram name. The whole data of all
// smoothness metrics will be aggregated into the "AnimationSmoothness" entry at
// the end.
func NewSmoothnessMetricConfig(histogramName string) MetricConfig {
	return MetricConfig{histogramName: histogramName, unit: "percent", direction: perf.BiggerIsBetter, bootAndShutdown: false, histogramType: countHistogram}
}

// NewLatencyMetricConfig creates a new MetricConfig instance for collecting
// input latency data for the given histogram name. The whole data of all input
// latency metrics will be aggregated into the "InputLatency" entry at the end.
func NewLatencyMetricConfig(histogramName string) MetricConfig {
	return MetricConfig{histogramName: histogramName, unit: "ms", direction: perf.SmallerIsBetter, bootAndShutdown: false, histogramType: countHistogram}
}

// NewCumulativeMetricConfig creates a new MetricConfig instance for cumulative
// metrics with a given histogram name. These cumulative metrics have the
// cumulativeHistogram type, and thus the value reported is the value of the
// max histogram bucket reported.
func NewCumulativeMetricConfig(histogramName, unit string, direction perf.Direction) MetricConfig {
	return MetricConfig{histogramName: histogramName, unit: unit, direction: direction, bootAndShutdown: false, histogramType: cumulativeHistogram}
}

// NewCustomMetricConfig creates a new MetricConfig for the given histogram
// name, unit, and direction. The data are reported as-is but
// not aggregated with other histograms.
func NewCustomMetricConfig(histogramName, unit string, direction perf.Direction) MetricConfig {
	return MetricConfig{histogramName: histogramName, unit: unit, direction: direction, bootAndShutdown: false, histogramType: countHistogram}
}

// NewBootAndShutdownCustomMetricConfig creates a new MetricConfig with
// bootAndShutdown flag set, for the given histogram name, unit, and direction.
// The data are reported as-is but not aggregated with other histograms.
func NewBootAndShutdownCustomMetricConfig(histogramName, unit string, direction perf.Direction) MetricConfig {
	return MetricConfig{histogramName: histogramName, unit: unit, direction: direction, bootAndShutdown: true, histogramType: countHistogram}
}

// NewEnumCustomMetricConfig creates a new MetricConfig for the enum histogram type
// and the given histogram name and enum values map as defined in tools/metrics/histograms/enums.xml.
func NewEnumCustomMetricConfig(histogramName string, enumValues map[int64]string) MetricConfig {
	return MetricConfig{histogramName: histogramName, bootAndShutdown: false, histogramType: enumHistogram, enumValues: enumValues}
}

// NewMemoryMetricConfig creates a new MetricConfig for the given histogram
// name, unit, and direction. The data is recorded using
// metricRecordMethodSumAverageBySource.
func NewMemoryMetricConfig(histogramName, unit string) MetricConfig {
	return MetricConfig{
		histogramName:   histogramName,
		unit:            unit,
		direction:       perf.SmallerIsBetter,
		bootAndShutdown: false,
		histogramType:   countHistogram,
		recordMethod:    metricRecordMethodSumAverageBySource}
}

type record struct {
	config     MetricConfig
	totalCount int64

	// Sum is the sum of the all entries in the histogram.
	Sum int64 `json:"sum"`

	// Buckets contains ranges of reported values. It's the concatenated histogram buckets from multiple runs.
	Buckets []histogram.Bucket `json:"buckets"`
}

// combine combines another record into an existing one.
func (rec *record) combine(newRec *record) error {
	if !reflect.DeepEqual(rec.config, newRec.config) {
		return errors.New("records with different config cannot be combined")
	}
	rec.totalCount += newRec.totalCount
	rec.Sum += newRec.Sum
	rec.Buckets = append(rec.Buckets, newRec.Buckets...)
	return nil
}

// saveMetric records the metric into the perf values.
func (rec *record) saveMetric(pv *perf.Values, name string) {
	if rec.totalCount == 0 {
		return
	}
	switch rec.config.histogramType {
	case enumHistogram:
		for _, bucket := range rec.Buckets {
			// For each enum value (which equals to `bucket.Min`), create a metric
			// with the metric name as "<name>.<enum value or enum name>"
			var metricName string
			enumName, ok := rec.config.enumValues[bucket.Min]
			if ok {
				metricName = fmt.Sprintf("%s.%s", name, enumName)
			} else {
				metricName = fmt.Sprintf("%s.%v", name, bucket.Min)
			}
			pv.Set(perf.Metric{
				Name: metricName,
				Unit: "count",
			}, float64(bucket.Count))
		}
	case cumulativeHistogram:
		var maxValue int64
		if len(rec.Buckets) != 0 {
			maxValue = rec.Buckets[len(rec.Buckets)-1].Min
		}
		pv.Set(perf.Metric{
			Name:      name + ".Max",
			Unit:      rec.config.unit,
			Direction: rec.config.direction,
		}, float64(maxValue))
	case countHistogram:
		fallthrough
	default:
		// If rec.config.histogramType is not set,
		// treat it as count histograms by default.
		pv.Set(perf.Metric{
			Name:      name,
			Unit:      rec.config.unit,
			Variant:   "average",
			Direction: rec.config.direction,
		}, float64(rec.Sum)/float64(rec.totalCount))
	}
}

// Recorder is a utility to measure various metrics for CUJ-style tests.
type Recorder struct {
	cr    *chrome.Chrome
	tconn *chrome.TestConn
	arc   *arc.ARC

	// Metrics names keyed by relevant browser type.
	names map[browser.Type][]string

	// Keep one TestConn instance for each browser type.
	tconns map[browser.Type]*chrome.TestConn

	options RecorderOptions

	// Metric records keyed by relevant browser type.
	// Its value is a map keyed by metric name.
	records map[browser.Type]map[string]*record

	traceDir        string
	perfettoCfgPath string
	// The active or inactive tracing sessions.
	// Key is the full path of the directory where the
	// trace file will be saved, and value is the corresponding
	// tracing session.
	sessions map[string]*tracing.Session
	// The currently active tracing session.
	activeSession *tracing.Session

	// duration is the total running time of the recorder.
	duration time.Duration

	// Total number of times that the test has been successfully run.
	testCyclesCount int64

	// Time when recording was started.
	// Defined only for the running recorder.
	startedAtTm time.Time

	// Running recorder has these metrics recorders initialized for each metric
	// Defined only for the running recorder.
	mr map[browser.Type]*metrics.Recorder

	// A function to clean up started recording.
	// Defined only for the running recorder.
	cleanup func(ctx context.Context) error

	// powerSetupCleanup cleans up power setup.
	powerSetupCleanup func(ctx context.Context) error

	// batteryDischarge is true if battery discharge was successfully induced.
	batteryDischarge bool

	// screenRecorderStart starts the screen recorder, if present.
	screenRecorderStart action.Action

	// screenRecorderCleanup cleans up the screen recorder, if present.
	screenRecorderCleanup func(ctx context.Context)

	// perfettoTmpConfigCleanup cleans up the temporary perfetto config file, if present.
	perfettoTmpConfigCleanup func(ctx context.Context)

	screenshotRecorder uiauto.ScreenshotRecorder

	tpsTimeline        *perf.Timeline
	powerTimeline      *perf.Timeline
	memoryTimeline     *perf.Timeline
	gpuDataSource      *perfSrc.GPUDataSource
	frameDataTracker   *perfSrc.FrameDataTracker
	zramInfoTracker    *perfSrc.ZramInfoTracker
	batteryInfoTracker *perfSrc.BatteryInfoTracker
	memInfoTracker     *perfSrc.MemoryInfoTracker
	loginEventRecorder *perfSrc.LoginEventRecorder
	powertopRecorder   *perfSrc.PowertopRecorder
	profilerRecorder   *perfSrc.ProfilerRecorder
	// Additional timelines registered by the test.
	perfTimelines []*perf.Timeline

	pv *perf.Values

	// annotationCount is the number of annotations that have been added
	// to this recorder.
	annotationCount int

	takingSnapshot bool

	chromeosFlexTesting bool
}

// RecorderMode specifies which mode to run the recorder in. Each mode
// activates different timelines and collection utilities, which in turn
// changes the amount of overhead the recorder will have during recorder.Run.
type RecorderMode int

const (
	// CUJ runs the recorder with all of the collection utilities enabled.
	// This gives the device about 5% CPU overhead on low-end, few cored
	// devices, and a 1% overhead on medium to higher-end devices.
	CUJ RecorderMode = iota

	// Perf runs the recorder with only the TPS timeline enabled. The
	// overhead for this mode should be only around 1% for low-end devices,
	// and negligible overhead for higher end devices.
	Perf

	// Benchmark runs the recorder with no timelines enabled. There should be
	// negligible overhead associated with this recorder mode.
	Benchmark

	// BenchmarkDisableDPTFPowerd is the same as benchmark mode, except it
	// disables powerd and DPTF as part of the power test.
	BenchmarkDisableDPTFPowerd
)

// RecorderOptions contains options to control the recorder setup.
// The options are determined based on the test needs.
type RecorderOptions struct {
	// CooldownBeforeRun, if set, will wait for the CPU to idle and cooldown
	// before running the function in recorder.Run.
	CooldownBeforeRun bool

	// Mode specifies which mode to run the recorder in.
	Mode RecorderMode

	// TurnOffDisplay, if set, will turn the display off before running the
	// function in recorder.Run and turn it back on in recorder.StopRecording
	TurnOffDisplay bool

	// RunOnBattery forces the device to run on battery, regardless of the
	// set RecorderMode.
	RunOnBattery bool

	// DischargeThreshold is the battery discharge threshold.
	// If not set, defaultDischargeThreshold will be used.
	DischargeThreshold *float64
	// FailOnDischargeErr, if set, will cause test to fail on battery discharge error.
	// NoBatteryError is not considered as dischage error, though.
	FailOnDischargeErr   bool
	DoNotChangeWifi      bool
	DoNotChangePowerd    bool
	DoNotChangeDPTF      bool
	DoNotChangeAudio     bool
	DoNotChangeBluetooth bool
}

var performanceCUJDischargeThreshold = 25.0

// NewPerformanceCUJOptions indicates the power test settings for performance CUJs run by partners.
func NewPerformanceCUJOptions() RecorderOptions {
	return RecorderOptions{
		Mode:               CUJ,
		DischargeThreshold: &performanceCUJDischargeThreshold,
		FailOnDischargeErr: true,
		DoNotChangeWifi:    true,
		DoNotChangePowerd:  true,
		DoNotChangeDPTF:    true,
		DoNotChangeAudio:   true,
	}
}

// AddPerfTimelines adds additional performance timelines to the recorder.
func (r *Recorder) AddPerfTimelines(timelines ...*perf.Timeline) {
	r.perfTimelines = append(r.perfTimelines, timelines...)
}

// AddCollectedMetrics adds |configs| to the collected metrics for browser |bt| using the |tconn|
// as test connection.
func (r *Recorder) AddCollectedMetrics(tconn *chrome.TestConn, bt browser.Type, configs ...MetricConfig) error {
	if tconn == nil {
		return errors.New("tconn must never be nil")
	}
	if !r.startedAtTm.IsZero() {
		return errors.New("canont modify list of collected metrics after recording was started")
	}

	// Keep the tconn so it can be used to collect metrics from the browser.
	r.tconns[bt] = tconn

	for _, config := range configs {
		if config.histogramName == string(deprecatedGroupLatency) || config.histogramName == string(deprecatedGroupSmoothness) {
			return errors.Errorf("invalid histogram name: %s", config.histogramName)
		}
		r.names[bt] = append(r.names[bt], config.histogramName)
		if _, ok := r.records[bt]; !ok {
			r.records[bt] = make(map[string]*record)
		}
		r.records[bt][config.histogramName] = &record{config: config}
	}
	return nil
}

// AddCommonMetrics adds MetricConfigs defined by CommonMetrics to the collected metrics.
// tconn will be used as the test connection for Ash and AnyChrome common metrics. bTconn
// will be used for the Browser common metrics. If bTconn is different than tconn, then
// another set of AnyChrome common metrics will be collected with bTconn used as the test
// connection.
func (r *Recorder) AddCommonMetrics(tconn, bTconn *chrome.TestConn) error {
	var bt browser.Type
	if *tconn == *bTconn {
		bt = browser.TypeAsh
	} else {
		bt = browser.TypeLacros
	}
	if err := r.AddCollectedMetrics(tconn, browser.TypeAsh, CUJAshCommonMetricConfigs()...); err != nil {
		return errors.Wrap(err, "failed to add Ash common metrics")
	}
	if err := r.AddCollectedMetrics(bTconn, bt, CUJBrowserCommonMetricConfigs()...); err != nil {
		return errors.Wrapf(err, "failed to add Browser(%s) common metrics", bt)
	}
	if err := r.AddCollectedMetrics(tconn, browser.TypeAsh, CUJAnyChromeCommonMetricConfigs()...); err != nil {
		return errors.Wrap(err, "failed to add Ash AnyChrome common metrics")
	}
	if bt == browser.TypeLacros {
		if err := r.AddCollectedMetrics(bTconn, browser.TypeLacros, CUJLacrosCommonMetricConfigs()...); err != nil {
			return errors.Wrap(err, "failed to add Lacros common metrics")
		}
		if err := r.AddCollectedMetrics(bTconn, browser.TypeLacros, CUJAnyChromeCommonMetricConfigs()...); err != nil {
			return errors.Wrap(err, "failed to add Lacros AnyChrome common metrics")
		}
	}
	return nil
}

// addScreenRecorder creates a screen recorder that will record the
// device during the execution of recorder.Run.
func (r *Recorder) addScreenRecorder(ctx context.Context, tconn *chrome.TestConn) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok || dir == "" {
		return errors.New("failed to get the out directory to save the screen recording")
	}

	screenRecorder, err := uiauto.NewScreenRecorder(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to create ScreenRecorder")
	}

	r.screenRecorderStart = func(ctx context.Context) error {
		return screenRecorder.Start(ctx, tconn)
	}
	r.screenRecorderCleanup = func(ctx context.Context) {
		// GoBigSleepLint Sleep before stopping the screen recorder, to ensure
		// any last actions that are performed right as the recorder stops are
		// captured properly. In an abrupt crash, the recording can cut out
		// important context for what caused the crash.
		if err := testing.Sleep(ctx, 2*time.Second); err != nil {
			testing.ContextLog(ctx, "Failed to sleep")
		}
		uiauto.ScreenRecorderStopSaveRelease(ctx, screenRecorder, filepath.Join(dir, "record.webm"))
	}
	return nil
}

// AddScreenshotRecorder creates a screenshot recorder that takes
// screenshots at every |interval| while executing recorder.Run.
// |maxImages| is the maximum number of images the screenshot recorder
// is allowed to take.
func (r *Recorder) AddScreenshotRecorder(ctx context.Context, interval time.Duration, maxImages int) error {
	var err error
	r.screenshotRecorder, err = uiauto.NewScreenshotRecorder(ctx, interval, maxImages)
	if err != nil {
		return errors.Wrap(err, "failed to setup new screenshot recorder")
	}
	return nil
}

// CustomScreenshot takes a screenshot of the display. This function
// can only be called after |AddScreenshotRecorder|, but can be called
// before and after |Run|.
func (r *Recorder) CustomScreenshot(ctx context.Context) {
	if r.screenshotRecorder == nil {
		testing.ContextLog(ctx, "Failed to take custom screenshot, screenshot recorder was not added to the recorder")
		return
	}
	r.screenshotRecorder.TakeScreenshot(ctx)
}

// Annotate creates a metric in the following form:
//
// Annotation.<numbered annotation>.<annotation description>
//
// This annotation is used to describe what event occurred at certain
// times during recorder.Run. The value of this metric is the number
// of seconds from the start of the test. Annotation must be a string
// consistening of alphanumeric characters, underscore, or dash. It must
// also be less than 240 characters, since the length of a name for
// perf.Metric has a limit of 256 characters.
//
// If the given annotation string is invalid, this function returns false.
// Otherwise, it returns true. The output of this function can be ignored,
// as failure logs are self contained.
//
// Soft rules for the annotation should be that each word should be
// separated by an underscore.
//
// Examples:
// 1. Open_Gmail
// 2. Switch_windows_by_hotseat
// 3. Start_switching_CNN_tabs
func (r *Recorder) Annotate(ctx context.Context, annotation string) bool {
	if r.startedAtTm.IsZero() {
		testing.ContextLog(ctx, "Failed to add annotation because the recorder hasn't started yet")
		return false
	}
	if !annotationRe.MatchString(annotation) {
		testing.ContextLog(ctx, "Failed to add annotation because of invalid annotation string: ", annotation)
		return false
	}

	r.annotationCount++
	r.pv.Set(perf.Metric{
		Name: fmt.Sprintf("Annotation.%d.%s", r.annotationCount, annotation),
		Unit: "s",
	}, time.Since(r.startedAtTm).Seconds())

	testing.ContextLog(ctx, "Annotation: ", strings.ReplaceAll(annotation, "_", " "))

	return true
}

// AnnotateSection creates an annotation metric with the name |annotation|,
// and returns a function that will both create an annotation when this cleanup
// function is called, and create a metric that records how long this
// annotation section took in seconds.
// The start metric that is created will be in the form |annotation|_start, and
// the end metric created will be in the form |annotation|_end. For example, if
// the annotation representing what we are doing is "Split_screen_windows", the
// two metrics created will look like:
// Annotation.Split_screen_windows_start,
// Annotation.Split_screen_windows_end,
// AnnotationSectionDuration.Split_screen_windows.
func (r *Recorder) AnnotateSection(ctx context.Context, annotation string) func(ctx context.Context) {
	startTime := time.Now()
	if ok := r.Annotate(ctx, annotation+"_start"); !ok {
		return func(ctx context.Context) {}
	}

	return func(ctx context.Context) {
		r.Annotate(ctx, annotation+"_end")
		r.pv.Set(perf.Metric{
			Name:      "AnnotationSectionDuration." + annotation,
			Unit:      "s",
			Direction: perf.SmallerIsBetter,
		}, float64(time.Since(startTime).Seconds()))
	}
}

// NewRecorderWithTestConn creates a Recorder. It also aggregates the metrics of each
// category (animation smoothness and input latency) and creates the aggregated
// reports.
func NewRecorderWithTestConn(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, bTconn *chrome.TestConn, a *arc.ARC, options RecorderOptions) (*Recorder, error) {
	if tconn == nil {
		return nil, errors.New("tconn must never be nil")
	}
	// Keep one TestConn for each browser.
	tconns := make(map[browser.Type]*chrome.TestConn)
	tconns[browser.TypeAsh] = tconn
	if bTconn != nil && *tconn != *bTconn {
		tconns[browser.TypeLacros] = bTconn
	}
	r := &Recorder{
		cr:                  cr,
		tconn:               tconn,
		tconns:              tconns,
		arc:                 a,
		options:             options,
		sessions:            make(map[string]*tracing.Session),
		chromeosFlexTesting: strings.ToLower(chromeosFlexTesting.Value()) == "true",
	}

	// Perf and CUJ tests both include the TPS timeline, which requires the
	// GPU data source.
	if r.options.Mode == Perf || r.options.Mode == CUJ {
		r.gpuDataSource = perfSrc.NewGPUDataSource(r.tconns)
	}

	if err := r.Reset(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

// NewRecorder creates a Recorder based on the configs. It also aggregates the
// metrics of each category (animation smoothness and input latency) and creates
// the aggregated reports.
func NewRecorder(ctx context.Context, cr *chrome.Chrome, bTconn *chrome.TestConn, a *arc.ARC, options RecorderOptions) (*Recorder, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}
	return NewRecorderWithTestConn(ctx, tconn, cr, bTconn, a, options)
}

// Reset creates new trackers and sets recorder values that are supposed to be
// independent from run to run to their default values. Reset clears the
// recorder pv, so Record should be called prior to calling Reset.
func (r *Recorder) Reset(ctx context.Context) error {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return errors.New("failed to get the out directory")
	}

	var err error
	r.batteryInfoTracker, err = perfSrc.NewBatteryInfoTracker(ctx, r.options.Mode != CUJ /*skipPollingPower*/, tpsMetricPrefix)
	if err != nil {
		return errors.Wrap(err, "failed to create BatteryInfoTracker")
	}

	if r.options.Mode == CUJ {
		r.frameDataTracker, err = perfSrc.NewFrameDataTracker(tpsMetricPrefix)
		if err != nil {
			return errors.Wrap(err, "failed to create FrameDataTracker")
		}

		r.zramInfoTracker, err = perfSrc.NewZramInfoTracker(tpsMetricPrefix)
		if err != nil {
			return errors.Wrap(err, "failed to create ZramInfoTracker")
		}

		if r.chromeosFlexTesting != true {
			r.memInfoTracker = perfSrc.NewMemoryTracker(r.arc)
		}

		r.loginEventRecorder = perfSrc.NewLoginEventRecorder(tpsMetricPrefix)

		r.profilerRecorder, err = perfSrc.NewProfilerRecorder(ctx, tpsMetricPrefix, 5*time.Second, outDir)
		if err != nil {
			return errors.Wrap(err, "failed to create ProfilerRecorder")
		}

		// loginEventRecorder.Prepare() may not be needed because we usually start
		// Chrome with --keep-login-events-for-testing flag that will start
		// LoginEventRecorder data collection automatically. But we do it here
		// just in case Chrome was started with different parameters.
		if err := r.loginEventRecorder.Prepare(ctx, r.tconn); err != nil {
			return errors.Wrap(err, "failed to start recording login event data")
		}
	}

	// Only run powertop on models in |powertopModels|, and only on CUJ tests.
	forcePowertopOn := strings.ToLower(runPowertop.Value()) == "true"
	if r.options.Mode == CUJ && forcePowertopOn {
		r.powertopRecorder, err = perfSrc.NewPowertopRecorder(ctx,
			&perfSrc.PowertopRecorderOptions{
				Interval:       5 * time.Second,
				OutDir:         filepath.Join(outDir, "powertop"),
				IgnoreCPUScale: forcePowertopOn,
			})
		if err != nil {
			return errors.Wrap(err, "failed to create PowertopRecorder")
		}
	}

	if strings.ToLower(screenRecord.Value()) == "true" {
		if err := r.addScreenRecorder(ctx, r.tconn); err != nil {
			return errors.Wrap(err, "failed to add the screen recorder")
		}
	}

	r.names = make(map[browser.Type][]string)
	r.records = make(map[browser.Type]map[string]*record)

	r.pv = perf.NewValues()

	return nil
}

// StartTracing starts a new system tracing session. Should be used with StopTracing.
// It allows recording trace data using perfettoCfgPath for any part of
// the test scenario and the trace.data.gz file will be saved to outDir later.
//
// It is the caller's responsibility to make sure:
// (1) Only one tracing session is running at one time. Stop the former tracing session
// first before starting another one.
// (2) StopTracing has to be called after StartTracing to clean up, otherwise it may
// affect other following tests.
// (3) StartTracing should be called inside of recorder.Run when no following deferred
// StopTracing is called. This can make sure the clean up function stops potential
// running tracing session when recorder.Run stops.
// (4) When start/stop multiple tracing sessions, make sure they call StartTracingWithName
// and use different trace names, otherwise it would return errors. Be careful of this rule
// especially when using recorder.RunFor.
//
// Example:
//
//	if err := recorder.StartTracing(ctx, s.OutDir(), perfettoCfgPath); err != nil {
//		s.Fatal("Failed to start tracing: ", err)
//	}
//	defer recorder.StopTracing(ctx)
//	...
//	if err := recorder.SaveTraceFiles(ctx) {
//		s.Fatal("Failed to save trace files: ", err)
//	}
func (r *Recorder) StartTracing(ctx context.Context, outDir, perfettoCfgPath string) error {
	// trace.data.gz is the default trace file name.
	return r.StartTracingWithExtraCategories(ctx, outDir, "trace.data.gz", perfettoCfgPath)
}

// StartTracingWithName starts a new system tracing session with a custom trace file name.
//
// Example:
//
//	if err := recorder.StartTracingWithName(ctx, s.OutDir(), "trace-1.data.gz", perfettoCfgPath); err != nil {
//		s.Fatal("Failed to start tracing: ", err)
//	}
//	defer recorder.StopTracing(ctx)
//	...
//	if err := recorder.SaveTraceFiles(ctx) {
//		s.Fatal("Failed to save trace files: ", err)
//	}
func (r *Recorder) StartTracingWithName(ctx context.Context, outDir, traceName, perfettoCfgPath string) error {
	return r.StartTracingWithExtraCategories(ctx, outDir, traceName, perfettoCfgPath)
}

// StartTracingWithExtraCategories is like StartTracingWithName, with
// |extraCategories| added to the perfetto config.
func (r *Recorder) StartTracingWithExtraCategories(ctx context.Context, outDir, traceName, perfettoCfgPath string, categories ...string) error {
	tracePath := filepath.Join(outDir, traceName)
	if _, exists := r.sessions[tracePath]; exists {
		return errors.New("trace session for the given tracePath already exists")
	}
	if r.activeSession != nil {
		return errors.New("there is a tracing session currently running")
	}

	// If cujrecorder.extraChromeCategoriesForTracing is not empty, add
	// those categories to the perfetto config.
	extraCategories := extraChromeCategoriesForTracing.Value() + strings.Join(categories, ",")
	testing.ContextLog(ctx, "Adding the following extra categories to the perfetto trace config: ", extraCategories)
	if extraCategories != "" {
		cleanup, configPath, err := addExtraChromeTraceCategories(ctx, perfettoCfgPath, extraCategories)
		if err != nil {
			return err
		}
		r.perfettoTmpConfigCleanup = cleanup
		perfettoCfgPath = configPath
	}

	testing.ContextLogf(ctx,
		"Starting system tracing session (with config %s); Make sure to explicitly call StopTracing afterwards",
		perfettoCfgPath)
	sess, err := tracing.StartSession(ctx, perfettoCfgPath, tracing.WithTraceDataPath(tracePath), tracing.WithCompression())
	if err != nil {
		return errors.Wrap(err, "failed to start tracing")
	}
	r.Annotate(ctx, "Start_tracing")
	r.sessions[tracePath] = sess
	r.activeSession = sess
	return nil
}

// StopTracing stops the active system tracing session.
func (r *Recorder) StopTracing(ctx context.Context) error {
	if r.activeSession == nil {
		return errors.New("can't stop tracing because there is no active session")
	}
	testing.ContextLog(ctx, "Stopping active system tracing session")
	if err := r.activeSession.Stop(ctx); err != nil {
		return errors.Wrap(err, "failed to stop tracing")
	}
	r.Annotate(ctx, "Stop_tracing")
	r.activeSession = nil
	return nil
}

// SaveTraceFiles read temporary trace result files, saves all the trace files to
// output directory, and cleans up the temporary trace result files.
// This could be resource-consuming so it should be called after Run() or RunFor()
// to avoid metrics snapshots timing out.
func (r *Recorder) SaveTraceFiles(ctx context.Context) error {
	// Save trace files from all tracing sessions.
	for tracePath, sess := range r.sessions {
		testing.ContextLog(ctx, "Finalizing (and compressing) the trace data at ", tracePath)
		if err := sess.Finalize(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to save the trace data: ", err)
		}
	}

	return nil
}

// Close clears states for all trackers.
func (r *Recorder) Close(ctx context.Context) error {
	var firstErr error
	if r.options.TurnOffDisplay {
		testing.ContextLog(ctx, "Turning on display")
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOn); err != nil {
			testing.ContextLog(ctx, "Failed to turn on display: ", err)
			firstErr = errors.Wrap(err, "failed to turn on display")
		}
	}
	if r.powerSetupCleanup != nil {
		if err := r.powerSetupCleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up power setup: ", err)
			if firstErr == nil {
				firstErr = errors.Wrap(err, "failed to clean up power setup")
			}
		}
	}

	if r.gpuDataSource != nil {
		r.gpuDataSource.Close()
	}

	if r.frameDataTracker != nil {
		if err := r.frameDataTracker.Close(ctx, r.tconn); err != nil {
			testing.ContextLog(ctx, "Failed to close frame data tracker: ", err)
			if firstErr == nil {
				firstErr = errors.Wrap(err, "failed to close frame data tracker")
			}

		}
	}
	return firstErr
}

// filterBootAndShutdownMetricNames returns metric names having |bootAndShutdown| metric
// config flag value equal to |bootAndShutdownFlagValue|.
func (r *Recorder) filterBootAndShutdownMetricNames(bootAndShutdownFlagValue bool, bt browser.Type) ([]string, error) {
	metrics, ok := r.records[bt]
	if !ok {
		return nil, errors.Errorf("no expected metrics for the given browser %q", bt)
	}
	var result []string
	for name, record := range metrics {
		if record.config.bootAndShutdown == bootAndShutdownFlagValue {
			result = append(result, name)
		}
	}
	return result, nil
}

func (r *Recorder) getInTestMetricNames(bt browser.Type) ([]string, error) {
	return r.filterBootAndShutdownMetricNames(false, bt)
}

func (r *Recorder) getBootAndShutdownMetricNames(bt browser.Type) ([]string, error) {
	return r.filterBootAndShutdownMetricNames(true, bt)
}

// startRecording starts to record CUJ data.
//
// In:
// * context to initialize data recording (and tracing if needed).
//
// Out:
// * New context (with reduced timeout) that should be used to run the test
// function.
// * Error
func (r *Recorder) startRecording(ctx context.Context) (runCtx context.Context, e error) {
	if !r.startedAtTm.IsZero() {
		return nil, errors.New("start requested on the started recorder")
	}
	if r.mr != nil || r.cleanup != nil {
		return nil, errors.New("start requested but some parameters are already initialized:" + fmt.Sprintf(" mr=%v, r.cleanup=%p", r.mr, r.cleanup))
	}

	const traceCleanupDuration = 2 * time.Second
	runCtx, cancelRunCtx := ctxutil.Shorten(ctx, traceCleanupDuration)
	cancel := func(ctx context.Context) error {
		cancelRunCtx()
		return nil
	}
	defer func(ctx context.Context) {
		// If this function finishes without errors, cleanup will happen in Run.
		if e == nil {
			return
		}
		if err := cancel(ctx); err != nil {
			// We cannot overwrite e here.
			testing.ContextLogf(ctx, "Failed to cleanup after startRecording: %s", err)
		}
		r.cleanup = nil
		r.startedAtTm = time.Time{} // Reset to zero.
		r.mr = nil
	}(ctx)

	var success bool
	powerStateCleanup := func(context.Context) error { return nil }
	// Since we are not changing Powerd, the display might turn off. Request
	// that the display stays on before cooling down.
	if r.options.DoNotChangePowerd || r.options.Mode == Benchmark {
		cleanupWithTconn, err := power.RequestKeepAwake(ctx, r.tconn, power.Display)
		if err != nil {
			return nil, errors.Wrap(err, "failed to request to keep the system awake")
		}
		powerStateCleanup = func(ctx context.Context) error {
			testing.ContextLog(ctx, "Resetting power state")
			return cleanupWithTconn(ctx, r.tconn)
		}
		// Cleanup the power state if the recorder setup fails.
		defer func(ctx context.Context) {
			if success {
				return
			}
			if err := powerStateCleanup(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to reset power state: ", err)
			}
		}(ctx)
	}

	skipCooldown := strings.ToLower(isLocalVar.Value()) == "true"
	if skipCooldown {
		testing.ContextLog(ctx, "Skipping cooldown because cujrecorder.isLocalVar is set")
	}

	if !skipCooldown && r.options.CooldownBeforeRun {
		WaitForCPUStabilization(ctx)
	}

	if r.screenRecorderStart != nil {
		if err := r.screenRecorderStart(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start screen recorder")
		}
	}

	if r.screenshotRecorder != nil {
		if err := r.screenshotRecorder.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start screenshot recorder")
		}
	}

	// Remember when recording started. This start time is before all metric
	// timelines have started, so that we can align the start time of these
	// timelines to a base "zero" time.
	r.startedAtTm = time.Now()

	powerTestCleanup, err := r.setUpPowerTest(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up the power test")
	}
	// fullPowerTestCleanup cleans up both the normal power setup,
	// as well as any changes to the power state (display always on or off).
	fullPowerTestCleanup := func(ctx context.Context) error {
		var firstErr error
		if powerTestCleanup != nil {
			firstErr = powerTestCleanup(ctx)
		}
		if err := powerStateCleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to reset power state: ", err)
			if firstErr == nil {
				firstErr = errors.Wrap(err, "failed to reset power state")
			}
		}
		return firstErr
	}
	defer func(ctx context.Context) {
		// If the recorder setup fails, and there are cleanup actions that are
		// still left over, run the cleanup here. Otherwise, save the cleanup
		// actions to r.powerSetupCleanup.
		if !success && powerTestCleanup != nil {
			powerTestCleanup(ctx)
			return
		}
		r.powerSetupCleanup = fullPowerTestCleanup
	}(ctx)

	if r.options.TurnOffDisplay {
		testing.ContextLog(ctx, "Turning off display")
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOff); err != nil {
			return nil, errors.Wrap(err, "failed to turn off display")
		}
	} else {
		testing.ContextLog(ctx, "Turning on display")
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOn); err != nil {
			return nil, errors.Wrap(err, "failed to turn on display")
		}
	}
	defer func(ctx context.Context) {
		if success || !r.options.TurnOffDisplay {
			return
		}
		testing.ContextLog(ctx, "Turning on display")
		if err := power.SetDisplayPower(ctx, power.DisplayPowerAllOn); err != nil {
			testing.ContextLog(ctx, "Failed to turn on display: ", err)
		}
	}(ctx)

	// Start metrics recording per browser.
	r.mr = make(map[browser.Type]*metrics.Recorder)
	for bt := range r.names {
		// Watch in-test metrics only.
		inTestMetrics, err := r.getInTestMetricNames(bt)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get in-test metric names")
		}
		r.mr[bt], err = metrics.StartRecorder(ctx, r.tconns[bt], inTestMetrics...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start metrics recorder for browser %v", bt)
		}
		bootMetrics, err := r.getBootAndShutdownMetricNames(bt)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to query boot metrics for browser %v", bt)
		}
		if len(bootMetrics) > 0 {
			// Some of BootTime.* metrics are reported only once after reboot.
			// Force reporting them here in case they are already gone.
			if err := testexec.CommandContext(
				ctx,
				"sh",
				"-c",
				"start send-boot-metrics || true",
			).Run(testexec.DumpLogOnError); err != nil {
				return nil, errors.Wrap(err, "failed to force send-boot-metrics to be reported again")
			}
		}
	}

	if r.options.Mode == Perf || r.options.Mode == CUJ {
		// Create a TPS timeline aligned with r.startedAtTm.
		r.tpsTimeline, err = perf.NewTimeline(ctx, []perf.TimelineDatasource{
			perfSrc.NewCPUUsageSource("CPU"),
			perfSrc.NewThermalDataSource(),
			perfSrc.NewThermalStateDataSource(true /*ignoreChargerType*/),
			r.gpuDataSource,
			perfSrc.NewMemoryDataSource("RAM.Absolute", "RAM.Diff.Absolute", "RAM"),
		}, perf.Interval(checkInterval), perf.Prefix(tpsMetricPrefix), perf.EnableGracePeriod(), perf.WithCustomStartTime(r.startedAtTm))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create TPS timeline")
		}
		if err := r.tpsTimeline.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start TPS timeline")
		}
		if err := r.tpsTimeline.StartRecording(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start recording TPS timeline data")
		}

		for i, tl := range r.perfTimelines {
			if err := tl.Start(ctx); err != nil {
				return nil, errors.Wrapf(err, "failed to start timeline %d", i)
			}
			if err := tl.StartRecording(ctx); err != nil {
				return nil, errors.Wrapf(err, "failed to start recording timeline %d", i)
			}
		}
	}

	if err := r.batteryInfoTracker.Start(ctx, r.startedAtTm); err != nil {
		return nil, errors.Wrap(err, "failed to start BatteryInfoTracker")
	}

	if r.options.Mode == CUJ {
		if err := r.frameDataTracker.Start(ctx, r.tconn, r.startedAtTm); err != nil {
			return nil, errors.Wrap(err, "failed to start FrameDataTracker")
		}

		defer func(ctx context.Context) {
			if success {
				return
			}
			if err := r.frameDataTracker.Stop(ctx, r.tconn); err != nil {
				testing.ContextLog(ctx, "Failed to stop frame data tracker: ", err)
			}
		}(ctx)

		if err := r.zramInfoTracker.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start ZramInfoTracker")
		}

		// Create a power timeline aligned with r.startedAtTm.
		// Some DUTs have no ChromeEC which cannot support battery metrics collecting.
		// Assign different metrics accordingly by checking whether /dev/cros exists.
		var powerTestMetrics []perf.TimelineDatasource
		if util.ChromeECInfo(ctx) != nil {
			testing.ContextLog(ctx, "Device has ChromeEC")
			powerTestMetrics = pm.TestMetrics()
		} else {
			testing.ContextLog(ctx, "Device has no ChromeEC")
			powerTestMetrics = pm.TestMetricsWithoutBatteryInfo()
		}
		r.powerTimeline, err = perf.NewTimeline(ctx,
			powerTestMetrics,
			perf.Interval(checkInterval), perf.Prefix(powerMetricPrefix), perf.EnableGracePeriod(), perf.WithCustomStartTime(r.startedAtTm))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create power timeline")
		}
		if err := r.powerTimeline.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start power timeline")
		}
		if err := r.powerTimeline.StartRecording(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start recording power timeline data")
		}

		// Create a memory timeline aligned with r.startedAtTm. This memory
		// timeline tracks relatively inexpensive memory metrics, like PSI.
		r.memoryTimeline, err = perf.NewTimeline(ctx, []perf.TimelineDatasource{
			perfSrc.NewPSIDataSource(r.arc),
		}, perf.Interval(checkInterval), perf.Prefix(memoryMetricPrefix), perf.EnableGracePeriod(), perf.WithCustomStartTime(r.startedAtTm))
		if err != nil {
			return nil, errors.Wrap(err, "failed to create memory timeline")
		}
		if err := r.memoryTimeline.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start memory timeline")
		}
		if err := r.memoryTimeline.StartRecording(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start recording memory timeline data")
		}

		// memInfoTracker tracks more expensive memory metrics, and thus is
		// polled significantly less frequently.
		if r.memInfoTracker != nil {
			if err := r.memInfoTracker.Start(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to start recording memory data")
			}
		}

		if r.powertopRecorder != nil {
			if err := r.powertopRecorder.Start(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to start powertop recorder")
			}
		}

		if err := r.profilerRecorder.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start ProfilerRecorder")
		}
	}

	r.cleanup = cancel
	success = true
	return runCtx, nil
}

// setUpPowerTest sets up the power test based on the options provided to the
// recorder. This function returns a cleanup function that is only non-nil
// when there is no error.
func (r *Recorder) setUpPowerTest(ctx context.Context) (func(ctx context.Context) error, error) {
	if strings.ToLower(skipPowerTest.Value()) == "true" {
		testing.ContextLog(ctx, "Skipping power test because cujrecorder.skipPowerTest is set")
		return nil, nil
	}

	powerTestOptions := setup.PowerTestOptions{
		// The default for the following options is to disable these settings.
		Wifi:               setup.DisableWifiInterfaces,
		NightLight:         setup.DisableNightLight,
		Audio:              setup.Mute,
		Bluetooth:          setup.DisableBluetoothInterfaces,
		Powerd:             setup.DisablePowerd,
		DPTF:               setup.DisableDPTF,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}
	// Check recorder options and don't change them when required.
	if r.options.DoNotChangeWifi || strings.ToLower(keepWifi.Value()) == "true" {
		powerTestOptions.Wifi = setup.DoNotChangeWifiInterfaces
	}
	if r.options.DoNotChangeAudio {
		powerTestOptions.Audio = setup.DoNotChangeAudio
	}
	if r.options.DoNotChangeBluetooth {
		powerTestOptions.Bluetooth = setup.DoNotChangeBluetooth
	}
	dischargeThreshold := setup.DefaultDischargeThreshold
	if r.options.DischargeThreshold != nil {
		dischargeThreshold = *r.options.DischargeThreshold
	}

	// Don't change Powerd or DPTF for Benchmark tests, because in order to get
	// as high a score as possible, we want to make sure we are properly
	// cooling the CPU.
	if r.options.DoNotChangePowerd || r.options.Mode == Benchmark {
		testing.ContextLog(ctx, "Not changing Powerd")
		powerTestOptions.Powerd = setup.DoNotChangePowerd
	}
	if r.options.DoNotChangeDPTF || r.options.Mode == Benchmark {
		testing.ContextLog(ctx, "Not changing DPTF")
		powerTestOptions.DPTF = setup.DoNotChangeDPTF
	}

	// Create batteryDischarge with both discharge and ignoreErr set to true.
	// Only discharge for CUJ tests, since they are the only test that run for
	// long enough for the power test to give meaningful values.
	batteryDischarge := setup.NewBatteryDischarge(
		strings.ToLower(keepCharging.Value()) != "true" && (r.options.Mode == CUJ || r.options.RunOnBattery),
		true, dischargeThreshold)

	var err error
	setupCleanup, err := setup.PowerTest(ctx, r.tconn, powerTestOptions, batteryDischarge)

	batteryDischargeErr := batteryDischarge.Err()

	if batteryDischargeErr != nil {
		testing.ContextLog(ctx, "Failed to induce battery discharge: ", batteryDischargeErr)
	} else if r.options.Mode == CUJ {
		r.batteryDischarge = true
	}
	if err != nil {
		return nil, errors.Wrap(err, "power setup failed")
	}

	// cleanup cleans up the power setup.
	cleanup := func(ctx context.Context) error {
		testing.ContextLog(ctx, "Cleaning up power test")
		if err := setupCleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up power setup: ", err)
			return errors.Wrap(err, "failed to clean up power setup")
		}
		return nil
	}

	// Check options.FailOnDischargeErr after the deferred function is set.
	if batteryDischargeErr != nil && r.options.FailOnDischargeErr &&
		!errors.Is(batteryDischargeErr, pm.ErrNoBattery) {
		if err := cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up power test: ", err)
		}
		return nil, errors.Wrap(batteryDischargeErr, "battery discharge failed")
	}

	return cleanup, nil
}

// stopRecording stops CUJ data recording.
//
// In:
// * context used to initialise recording (the one that was passed to the
// startRecording above).
// * shorted context returned from the startRecording()
//
// Out:
// * Error
func (r *Recorder) stopRecording(ctx, runCtx context.Context) (e error) {
	// Ensure any active session stopped when recorder stopped.
	r.StopTracing(ctx)

	if r.startedAtTm.IsZero() {
		return errors.New("Stop requested on the stopped recorder")
	}
	if r.mr == nil || r.cleanup == nil {
		return errors.New("Stop requested but recorder was not fully started: " + fmt.Sprintf(" mr=%v, r.cleanup=%p", r.mr, r.cleanup))
	}

	r.duration += time.Now().Sub(r.startedAtTm)
	r.pv.Set(perf.Metric{Name: "TestMetrics.StartedAtTime", Variant: "summary", Unit: "Unix"}, float64(r.startedAtTm.Unix()))
	r.startedAtTm = time.Time{} // Reset to zero.

	if r.screenRecorderCleanup != nil {
		r.screenRecorderCleanup(ctx)
	}
	if r.perfettoTmpConfigCleanup != nil {
		r.perfettoTmpConfigCleanup(ctx)
	}

	if r.screenshotRecorder != nil {
		if err := r.screenshotRecorder.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop screenshot recorder: ", err)
		}
	}

	// Collects metrics per browser type.
	tHists := make(map[browser.Type][]*histogram.Histogram)
	for bt, rr := range r.mr {
		tconn := r.tconns[bt]
		inTestHistograms, err := rr.Histogram(runCtx, tconn)
		if err != nil {
			return errors.Wrapf(err, "failed to collect metrics from browser %v", bt)
		}
		// Boot and shutdown metrics must be collected separately.
		bootAndShutdownMetrics, err := r.getBootAndShutdownMetricNames(bt)
		if err != nil {
			return errors.Wrap(err, "failed to get boot and shutdown metric names")
		}

		if r.chromeosFlexTesting != true && len(bootAndShutdownMetrics) > 0 {
			// Some BootTime.* metrics are only reported once after
			// a reboot. We forced them to be reported again after
			// the recorder started, but ChromeOS metrics are
			// collected every 30 seconds, so we may need to wait a
			// while for them to appear.
			testing.ContextLog(ctx, "Waiting for BootTime.Total2 metrics to be reported")
			if _, err := metrics.WaitForHistogram(
				ctx,
				tconn,
				"BootTime.Total2",
				time.Minute,
			); err != nil {
				return errors.Wrap(err, "failed to wait until BootTime.Total2 metrics is reported")
			}
		}

		bootAndShutdownHistograms, err := metrics.GetHistograms(runCtx, tconn, bootAndShutdownMetrics)
		if err != nil {
			return errors.Wrap(err, "failed to fetch boot and shutdown metrics")
		}

		testing.ContextLogf(ctx, "The following in-test metrics are collected from %q: %v", bt+"-Chrome", histsWithSamples(inTestHistograms))
		testing.ContextLogf(ctx, "The following boot and shutdown metrics are collected from %q: %v", bt+"-Chrome", histsWithSamples(bootAndShutdownHistograms))
		tHists[bt] = append(tHists[bt], inTestHistograms...)
		tHists[bt] = append(tHists[bt], bootAndShutdownHistograms...)
	}
	// Reset recorders and context.
	r.mr = nil

	for bt, hists := range tHists {
		for _, hist := range hists {
			if hist.TotalCount() == 0 {
				continue
			}
			// Combine histogram result to the record.
			if err := r.records[bt][hist.Name].combine(&record{config: r.records[bt][hist.Name].config,
				totalCount: hist.TotalCount(), Sum: hist.Sum, Buckets: hist.Buckets}); err != nil {
				return err
			}
		}
	}
	return nil
}

// stopMetrics stops the performance metrics timelines and sets the values into r.pv.
// When this function fails, it may have already set some useful values into r.pv.
func (r *Recorder) stopMetrics(ctx context.Context) error {
	// We want to conduct all of Stop tasks even when some of them fails.
	// Return an error when one of them has failed.
	var stopErr error
	if r.tpsTimeline != nil {
		if tpsData, err := r.tpsTimeline.StopRecording(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop TPS timeline: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop TPS timeline")
			}
		} else {
			r.pv.Merge(tpsData)
		}
	}
	for i, tl := range r.perfTimelines {
		if d, err := tl.StopRecording(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to stop timeline %d: %v", i, err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop timeline")
			}
		} else {
			r.pv.Merge(d)
		}
	}

	if r.frameDataTracker != nil {
		if err := r.frameDataTracker.Stop(ctx, r.tconn); err != nil {
			testing.ContextLog(ctx, "Failed to stop FrameDataTracker: ", err)
			if strings.ToLower(ignoreFrameDataError.Value()) != "true" {
				stopErr = errors.Wrap(err, "failed to stop FrameDataTracker")
			}
		} else {
			r.frameDataTracker.Record(r.pv)
		}
	}

	if r.zramInfoTracker != nil {
		if err := r.zramInfoTracker.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop ZramInfoTracker: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop ZramInfoTracker")
			}
		} else {
			r.zramInfoTracker.Record(r.pv)
		}
	}

	if r.batteryInfoTracker != nil {
		if err := r.batteryInfoTracker.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop BatteryInfoTracker: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop BatteryInfoTracker")
			}
		} else {
			r.batteryInfoTracker.Record(r.pv)
		}
	}

	if r.powerTimeline != nil {
		if powerData, err := r.powerTimeline.StopRecording(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop power timeline: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop power timeline")
			}
		} else {
			r.pv.Merge(powerData)

			if strings.ToLower(powerLogVar.Value()) == "true" {
				dir, ok := testing.ContextOutDir(ctx)
				if !ok || dir == "" {
					return errors.New("failed to get the out directory")
				}

				// Save power_log.json and power_log.html.
				if err := power.GeneratePowerLog(ctx, dir, "", powerData); err != nil {
					return errors.Wrap(err, "failed to generate power log")
				}
			}
		}
	}

	if r.memoryTimeline != nil {
		if memoryData, err := r.memoryTimeline.StopRecording(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop memory timeline: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop memory timeline")
			}
		} else {
			r.pv.Merge(memoryData)
		}
	}

	if r.memInfoTracker != nil {
		if err := r.memInfoTracker.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop MemInfoTracker: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop MemInfoTracker")
			}
		} else {
			r.memInfoTracker.Record(r.pv)
		}
	}

	if r.loginEventRecorder != nil {
		if err := r.loginEventRecorder.FetchLoginEvents(ctx, r.tconn); err != nil {
			testing.ContextLog(ctx, "Failed to fetch login events date: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to fetch login events")
			}
		} else {
			r.loginEventRecorder.Record(ctx, r.pv)
		}
	}

	if r.profilerRecorder != nil {
		if err := r.profilerRecorder.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop ProfilerRecorder: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop ProfilerRecorder")
			}
		} else {
			r.profilerRecorder.Record(r.pv)
		}
	}

	if r.powertopRecorder != nil {
		if err := r.powertopRecorder.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop powertop recorder: ", err)
			if stopErr == nil {
				stopErr = errors.Wrap(err, "failed to stop powertop recorder")
			}
		}
	}

	if stopErr != nil {
		return stopErr
	}

	displayInfo, err := perfSrc.NewDisplayInfo(ctx, r.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get display info")
	}
	displayInfo.Record(r.pv)

	// Combined records from all browsers.
	allRecords, err := processMetricRecords(r.records)
	if err != nil {
		return err
	}

	var crasUnderruns float64
	// Record combined records from all tconns.
	for name, rec := range allRecords {
		if name == "Cras.UnderrunsPerDevice" {
			crasUnderruns = float64(rec.Sum)
			// We are not interested in reporting Cras.UnderrunsPerDevice but will use this value
			// to derive UnderrunsPerDevicePerMinute. Continue the loop.
			continue
		}
		// Metric name recorded is the original histogram name. For example:
		// - EventLatency.TotalLatency
		// - PageLoad.InteractiveTiming.InputDelay3
		rec.saveMetric(r.pv, name)
	}

	// Derive Cras.UnderrunsPerDevicePerMinute. Ideally, the audio playing time and number of CRAS audio device
	// should be captured. For now use the recorder running duration and assume there is only one device.
	r.pv.Set(perf.Metric{
		Name:      "Media.Cras.UnderrunsPerDevicePerMinute",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, crasUnderruns/r.duration.Minutes())

	var batteryDischargeReport float64
	if r.batteryDischarge {
		batteryDischargeReport = 1
	}
	r.pv.Set(perf.Metric{
		Name:      powerMetricPrefix + "MetricsCollectedWithBatteryDischarge",
		Unit:      "unitless",
		Direction: perf.BiggerIsBetter,
	}, batteryDischargeReport)

	r.pv.Set(perf.Metric{
		Name:      "TestMetrics.TestCyclesCount",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(r.testCyclesCount))

	r.pv.Set(perf.Metric{
		Name: "TestMetrics.TotalTestRunTime",
		Unit: "s",
		// Longer runtime correlates to better performance data, so bigger is better
		Direction: perf.BiggerIsBetter,
	}, r.duration.Seconds())

	// Reset duration for any future recorder.Run.
	r.duration = 0

	collectCLCP(ctx, r.pv)
	collectMSPH(ctx, r.pv)

	return nil
}

// processMetricRecords iterates through `records` and generate a list of to be recorded.
func processMetricRecords(perBrowserRecords map[browser.Type]map[string]*record) (map[string]*record, error) {
	allRecords := make(map[string]*record)

	accumulatedAverage := make(map[string]float64)
	for bt, records := range perBrowserRecords {
		for name, rec := range records {
			if rec.totalCount == 0 {
				continue
			}

			// Append metric name with browser type as the new metric name, for example:
			// - EventLatency.TotalLatency_ash-Chrome,
			// - PageLoad.InteractiveTiming.InputDelay3_lacros-Chrome
			perBrowserName := fmt.Sprintf("%s_%s-Chrome", name, bt)
			allRecords[perBrowserName] = rec

			// Combine the record from different browsers.
			if _, ok := allRecords[name]; !ok {
				allRecords[name] = &record{config: rec.config}
			}
			switch rec.config.recordMethod {
			case metricRecordMethodSumAverageBySource:
				allRecords[name].totalCount = 1
				accumulatedAverage[name] += float64(rec.Sum) / float64(rec.totalCount)
			case metricRecordMethodNeutral:
				fallthrough
			default:
				if err := allRecords[name].combine(rec); err != nil {
					return nil, err
				}
			}
		}
	}

	// Update `Sum` from `accumulatedAverage`.
	for name, averageSum := range accumulatedAverage {
		allRecords[name].Sum = int64(averageSum)
	}
	return allRecords, nil
}

// Run conducts the test scenario f, and collects the related metrics for the
// test scenario, and updates the internal data.
func (r *Recorder) Run(ctx context.Context, f func(ctx context.Context) error) (e error) {
	runCtx, err := r.startRecording(ctx)
	if err != nil {
		return err
	}

	defer func(ctx context.Context) {
		if err := r.cleanup(ctx); err != nil {
			if e == nil {
				e = errors.Wrap(err, "failed to clean up after stopRecording")
			} else {
				testing.ContextLog(ctx, "Failed to clean up after stopRecording: ", err)
			}
		}
		r.cleanup = nil
	}(ctx)

	if errF := f(runCtx); errF != nil {
		if errStopRecording := r.stopRecording(ctx, runCtx); errStopRecording != nil {
			testing.ContextLog(ctx, "Failed to stop recording: ", errStopRecording)
		}
		return errF
	}
	r.testCyclesCount++
	if errStopRecording := r.stopRecording(ctx, runCtx); errStopRecording != nil {
		return errors.Wrap(errStopRecording, "failed to stop recording")
	}

	// Use a short timeout value so it can return fast in case of failure.
	ctxStopMetrics, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := r.stopMetrics(ctxStopMetrics); err != nil {
		return errors.Wrap(err, "failed to stop metrics")
	}

	return nil
}

// RunFor conducts the test scenario f repeatedly for a given minimum
// duration. It may exceed that duration to complete the last call to f.
func (r *Recorder) RunFor(ctx context.Context, f func(ctx context.Context) error, minimumDuration time.Duration) error {
	return r.Run(ctx, func(ctx context.Context) error {
		for end := time.Now().Add(minimumDuration); time.Now().Before(end); {
			if err := f(ctx); err != nil {
				return err
			}
			r.testCyclesCount++
		}

		// Decrement test cycles to prevent double counting,
		// as Run increments the cycle count independently.
		r.testCyclesCount--

		return nil
	})
}

// Record sets the performance metrics timeline values into pv.
// TODO(b/263167309): Remove the unused ctx and unused return value.
func (r *Recorder) Record(ctx context.Context, pv *perf.Values) error {
	pv.Merge(r.pv)
	return nil
}

// SaveHistograms saves histogram raw data to a given directory in a
// file named "recorder_histograms.json" by marshal the recorders.
func (r *Recorder) SaveHistograms(outDir string) error {
	saveJSONFile := func(fileName string, records map[string]*record) error {
		filePath := path.Join(outDir, fileName+".json")
		j, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "failed to marshall data for %s json file: %v", fileName, records)
		}
		if err := ioutil.WriteFile(filePath, j, 0644); err != nil {
			return errors.Wrapf(err, "failed to write %s json file", fileName)
		}
		return nil
	}

	const histogramFileName = "recorder_histograms"
	allRecords := make(map[string]*record) // Combined records from all browsers.

	for bt, records := range r.records {
		// File for browser based histogram will be appended with the browser name.
		// For example:
		//   - recorder_histograms_ash-Chrome.json
		//   - recorder_histograms_lacros-Chrome.json
		fileName := fmt.Sprintf("%s_%s-Chrome", histogramFileName, bt)
		if err := saveJSONFile(fileName, records); err != nil {
			return err
		}
		for name, rec := range records {
			if _, ok := allRecords[name]; !ok {
				allRecords[name] = &record{config: rec.config}
			}
			// Combine the record.
			if err := allRecords[name].combine(rec); err != nil {
				return err
			}
		}
	}
	return saveJSONFile(histogramFileName, allRecords)
}

// NewConn is a wrapper around browser.NewConn that opens a tab for |url|,
// using the browser options |opts|. This wrapper records how long it took to
// open up this specific url. The metric name is constructed as follows:
//
// PageLoadTime.{shortTitle}
//
// |shortTitle| must follow all of the rules for perf.Metric names, including
// only alphanumeric characters, periods, dashes, and underscores.
func (r *Recorder) NewConn(ctx context.Context, br *browser.Browser, shortTitle, url string, opts ...browser.CreateTargetOption) (*browser.Conn, error) {
	start := time.Now()
	conn, err := br.NewConn(ctx, url, opts...)
	if err != nil {
		return nil, err
	}
	r.pv.Set(perf.Metric{
		Name:      fmt.Sprintf("PageLoadTime.%s", shortTitle),
		Unit:      "s",
		Direction: perf.SmallerIsBetter,
	}, float64(time.Since(start).Seconds()))
	return conn, nil
}

// StartSnapshot takes a snapshot of the given Ash and browser metrics, in order
// to compare them later. This function returns a snapshot stop function that must
// be called before calling StartSnapshot again. This stop function takes a new
// snapshot of the Ash and browser metrics, and saves the diff between the metrics.
// These metrics are saved under the original metric name prepended with |prefix|.
// |prefix| is separated from the rest of the metric name automatically with a
// period, so prefix should not include any period at the end of the string.
//
// For example, if we wanted to StartSnapshot for "EventLatency.TotalLatency", we
// would do the following:
//
// stopSnapshot, err := recorder.StartSnapshot(ctx, "MyPrefix", []string{}, []string{"EventLatency.TotalLatency"})
// ... test code
// err := stopSnapshot(ctx)
//
// After the test has been completed, there would be a new metric added to
// results-chart.json called "MyPrefix.EventLatency.TotalLatency". This metric would
// be the average "EventLatency.TotalLatency" between the start/stop snapshot.
//
// StartSnapshot only works if all metrics within |ashMetrics| and |browserMetrics|
// were added to the recorder prior to recorder.Run, either using AddCollectedMetric
// or AddCommonMetrics.
func (r *Recorder) StartSnapshot(ctx context.Context, prefix string, ashMetrics, browserMetrics []string) (func(ctx context.Context) error, error) {
	if r.takingSnapshot {
		return nil, errors.New("existing snapshot already in progress")
	}
	r.takingSnapshot = true

	// Add in a period to separate the prefix and the rest of the metric.
	prefixRe := regexp.MustCompile(`^.+\.$`)
	if prefixRe.MatchString(prefix) {
		testing.ContextLogf(ctx, "prefix %s should not include a period separator at the end", prefix)
	} else {
		prefix += "."
	}

	// If we have more than 1 tconn saved, then the browser metrics
	// must be Lacros based.
	bt := browser.TypeAsh
	if len(r.tconns) > 1 {
		bt = browser.TypeLacros
	}

	// Get the initial Ash/Browser histograms.
	ashHists, err := metrics.GetHistograms(ctx, r.tconns[browser.TypeAsh], ashMetrics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial Ash snapshot histograms")
	}

	browserHists, err := metrics.GetHistograms(ctx, r.tconns[bt], browserMetrics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial browser snapshot histograms")
	}

	// Return the stop function that compares a new snapshot with the initial
	// snapshot, and saves the new metrics in r.pv.
	return func(ctx context.Context) error {
		// Take new snapshots of the given metrics.
		newAshHists, err := metrics.GetHistograms(ctx, r.tconns[browser.TypeAsh], ashMetrics)
		if err != nil {
			return errors.Wrap(err, "failed to get final Ash snapshot histograms")
		}

		newBrowserHists, err := metrics.GetHistograms(ctx, r.tconns[bt], browserMetrics)
		if err != nil {
			return errors.Wrap(err, "failed to get final Browser snapshot histograms")
		}

		// Get the histogram diffs from when StartSnapshot was called.
		ashDiff, err := histogram.DiffHistograms(ashHists, newAshHists)
		if err != nil {
			return errors.Wrapf(err, "failed to diff old and new Ash histograms for snapshot, old had length %d; new had length %d", len(ashHists), len(newAshHists))
		}

		browserDiff, err := histogram.DiffHistograms(browserHists, newBrowserHists)
		if err != nil {
			return errors.Wrapf(err, "failed to diff old and new browser histograms for snapshot, old had length %d; new had length %d", len(browserHists), len(newBrowserHists))
		}

		// For each metric and its corresponding browser type, create
		// a new record with the new histogram diff.
		browserTypes := []browser.Type{browser.TypeAsh, bt}
		for i, diffs := range [][]*histogram.Histogram{ashDiff, browserDiff} {
			for _, hist := range diffs {
				metric, ok := r.records[browserTypes[i]][hist.Name]
				if !ok {
					return errors.Wrapf(err, "metric %q is not being recorded by the recorder for %v", hist.Name, browserTypes[i])
				}

				newRecord := &record{
					config:     metric.config,
					totalCount: hist.TotalCount(),
					Sum:        hist.Sum,
					Buckets:    hist.Buckets,
				}

				// Save the new histogram with the existing histogram name
				// prefixed with |prefix|.
				newRecord.saveMetric(r.pv, prefix+hist.Name)
			}
		}

		r.takingSnapshot = false

		return nil
	}, nil
}

// CPUCoolDownConfig returns a cpu.CoolDownConfig to be used for CUJ tests.
func CPUCoolDownConfig() cpu.CoolDownConfig {
	cdConfig := cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)
	cdConfig.PollTimeout = CPUCoolDownTimeout
	return cdConfig
}

// CPUPkgIdleConfig returns a cpu.IdleConfig to be used for CUJ tests.
func CPUPkgIdleConfig() cpu.IdleConfig {
	pkgIdleConfig := cpu.DefaultPkgIdleConfig()
	pkgIdleConfig.Timeout = CPUPkgStateIdleTimeout
	return pkgIdleConfig
}

// WaitForCPUStabilization stabilizes the DUT by waiting for cpu to cool down
// and idle.
func WaitForCPUStabilization(ctx context.Context) {
	if _, err := cpu.WaitUntilStabilized(ctx, CPUCoolDownConfig()); err != nil {
		// Log the CPU stabilizing wait failure instead of make it fatal.
		// TODO(b/213238698): Include the error as part of test data.
		testing.ContextLog(ctx, "Failed to wait for CPU to become idle: ", err)
	}

	// Usually takes longer than cpu.WaitUntilIdle(). Check x86 microarchitecture as well.
	if arch := runtime.GOARCH; arch != "arm" && arch != "arm64" {
		if err := cpu.WaitUntilPkgStateIdleWithConfig(ctx, CPUPkgIdleConfig()); err != nil {
			testing.ContextLog(ctx, "CPU package c-state failed to idle: ", err)
		}
	}
}

// histsWithSamples returns the names of the histograms that have at least one sample.
func histsWithSamples(hists []*histogram.Histogram) []string {
	var histNames []string
	for _, hist := range hists {
		if hist.TotalCount() > 0 {
			histNames = append(histNames, hist.Name)
		}
	}
	return histNames
}

// collectCLCP collects cLCP(LCP2-TTFB) values.
func collectCLCP(ctx context.Context, pv *perf.Values) {
	testing.ContextLog(ctx, "Collect cLCP metric")

	var (
		lcp2Metric = perf.Metric{Name: "PageLoad.PaintTiming.NavigationToLargestContentfulPaint2", Variant: "average", Unit: "ms", Direction: 0, Multiple: false}
		ttfbMetric = perf.Metric{Name: "PageLoad.Experimental.NavigationTiming.NavigationStartToFirstResponseStart", Variant: "average", Unit: "ms", Direction: 0, Multiple: false}
	)
	lcp2Values := pv.GetValueByMetric(lcp2Metric)
	ttfbValues := pv.GetValueByMetric(ttfbMetric)
	if len(lcp2Values) == 0 || len(ttfbValues) == 0 {
		return
	}
	pv.Set(perf.Metric{
		Name:      "TPS.Browser.cLCP",
		Variant:   "average",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, lcp2Values[0]-ttfbValues[0])
}

// collectMSPH collects Memory Stall-time Per Hour values.
func collectMSPH(ctx context.Context, pv *perf.Values) {
	testing.ContextLog(ctx, "Collect MSPH metric")

	var (
		psiFullMetric = perf.Metric{
			Name:      "ChromeOS.CWP.PSIMemPressure.Full",
			Variant:   "average",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}
		psiSomeMetric = perf.Metric{
			Name:      "ChromeOS.CWP.PSIMemPressure.Some",
			Variant:   "average",
			Unit:      "percent",
			Direction: perf.SmallerIsBetter,
		}
	)
	psiFullValues := pv.GetValueByMetric(psiFullMetric)
	psiSomeValues := pv.GetValueByMetric(psiSomeMetric)

	// psiFullValues is the stall time percentage * 100 (i.e. 20% = 2000).
	// AvgStallTimePerHour is that percentage applied to 1 hour (3600s).
	// So psiFullValues[0] / 10000 * 3600 = psiFullValues[0] * 0.36.
	if len(psiFullValues) != 1 {
		testing.ContextLogf(ctx, "Unexpected number of psiFullMetric values, got %d; expected 1", len(psiFullValues))
	} else {
		pv.Set(perf.Metric{
			Name:      "Memory.PSIFull.AvgStallTimePerHour",
			Unit:      "s",
			Direction: perf.SmallerIsBetter,
		}, psiFullValues[0]*0.36)
	}
	if len(psiSomeValues) != 1 {
		testing.ContextLogf(ctx, "Unexpected number of psiSomeMetric values, got %d; expected 1", len(psiSomeValues))
	} else {
		pv.Set(perf.Metric{
			Name:      "Memory.PSISome.AvgStallTimePerHour",
			Unit:      "s",
			Direction: perf.SmallerIsBetter,
		}, psiSomeValues[0]*0.36)
	}

}

// readAll reads from `filePath` and returns the content as a string.
func readAll(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open file %s", filePath)
	}
	buf, err := io.ReadAll(file)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read from file %s", filePath)
	}
	return string(buf), nil
}

// addExtraChromeTraceCategories adds `extraCategories` based on `templateConfigFilePath`,
// saves the new config file and returns the clean up function and the path to the new config file.
func addExtraChromeTraceCategories(
	ctx context.Context, templateConfigFilePath, extraCategories string) (func(context.Context), string, error) {
	configStr, err := readAll(templateConfigFilePath)
	if err != nil {
		return nil, "", err
	}

	categoriesReg := regexp.MustCompile(`included_categories\\":\[(.*)\]`)
	subMatches := categoriesReg.FindAllStringSubmatch(configStr, -1)
	if len(subMatches) != 2 && len(subMatches) != 3 {
		return nil, "", errors.Errorf("expect 2 (or 3) chrome category in data sources, but found %v sources",
			len(subMatches))
	}

	// Ensure the matches from `org.chromium.trace_event`,
	// `org.chromium.trace_metadata`, and `track_event` are exactly the same.
	if subMatches[0][1] != subMatches[1][1] {
		return nil, "", errors.Errorf("%s != %s; expect two chrome data sources to have same categories", subMatches[0][1], subMatches[1][1])
	}
	if len(subMatches) == 3 && subMatches[0][1] != subMatches[2][1] {
		return nil, "", errors.Errorf("%s != %s; expect two chrome data sources to have same categories", subMatches[0][1], subMatches[2][1])
	}
	categoriesStr := subMatches[0][1]
	newCategories := strings.Split(extraCategories, ",")
	newCategoriesStr := ""
	for _, category := range newCategories {
		newCategoriesStr += `,\"` + category + `\"`
	}
	newConfigStr := strings.ReplaceAll(configStr, categoriesStr, categoriesStr+newCategoriesStr)

	// Update `track_event` categories if any.
	trackEventMetaCatReg := regexp.MustCompile(`\n(\s+)(enabled_categories: \"\_\_metadata\"\n)`)
	metaCatMatch := trackEventMetaCatReg.FindAllStringSubmatch(newConfigStr, -1)
	if len(metaCatMatch) == 1 {
		leadingWhiteSapce := metaCatMatch[0][1]
		newCategoriesStr = "\n"
		for _, category := range newCategories {
			newCategoriesStr += leadingWhiteSapce + "enabled_categories: \"" + category + "\"\n"
		}
		newCategoriesStr += leadingWhiteSapce + metaCatMatch[0][2]
		newConfigStr = strings.Replace(newConfigStr, metaCatMatch[0][0], newCategoriesStr, 1)
	}

	f, err := os.CreateTemp("", "perfetto_config.pbtxt")
	perfettoTmpConfigCleanup := func(ctx context.Context) {
		if err := os.Remove(f.Name()); err != nil {
			testing.ContextLog(ctx, "Failed to remove the perfetto config tmp file")
		}
		testing.ContextLog(ctx, "Removed the perfetto config tmp file")
	}
	if _, err := f.WriteString(newConfigStr); err != nil {
		perfettoTmpConfigCleanup(ctx)
		return nil, "", errors.Wrap(err, "failed to write new config to config tmp file")
	}
	return perfettoTmpConfigCleanup, f.Name(), nil
}
