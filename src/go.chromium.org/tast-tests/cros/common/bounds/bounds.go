// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bounds contains utility functions for checking that performance metrics
// fall within acceptable bounds.
package bounds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"

	"go.chromium.org/tast/core/errors"
)

// Bounds represents a range of acceptable values.
// Given a performance value, a Bounds reports whether that value is acceptable or not,
// and gives a human-readable explanation for why the value is acceptable or not.
type Bounds func(float64) (bool, string)

// Min returns a Bounds that matches values greater than *or equal to* a the provided lower bound.
func Min(min float64) Bounds {
	return func(value float64) (bool, string) {
		if value >= min {
			return true, fmt.Sprintf("measured value %v >= lower bound %v", value, min)
		}
		return false, fmt.Sprintf("measured value %v < lower bound %v", value, min)
	}
}

// Max returns a Bounds that matches values less than *or equal to* the provided upper bound.
func Max(max float64) Bounds {
	return func(value float64) (bool, string) {
		if value <= max {
			return true, fmt.Sprintf("measured value %v <= upper bound %v", value, max)
		}
		return false, fmt.Sprintf("measured value %v > upper bound %v", value, max)
	}
}

// Between returns a Bounds that matches values that fall within the provided range.
// This range is inclusive: if the value is exactly equal to the lower bound or upper bound,
// it is considered to be within the range.
func Between(min, max float64) Bounds {
	return func(value float64) (bool, string) {
		if value < min {
			return false, fmt.Sprintf("measured value %v < lower bound %v", value, min)
		}
		if value > max {
			return false, fmt.Sprintf("measured value %v > upper bound %v", value, max)
		}
		return true, fmt.Sprintf("measured value %v is within range [%v, %v]", value, min, max)
	}
}

// Equals returns a Bounds that matches values that are equal to the provided target.
func Equals(target float64) Bounds {
	return func(value float64) (bool, string) {
		if value == target {
			return true, fmt.Sprintf("measured value %v = target value %v", value, target)
		}
		return false, fmt.Sprintf("measured value %v != target value %v", value, target)
	}
}

// Matcher represents a pattern for matching test names and metric names that Bounds should apply to.
type Matcher *regexp.Regexp

// MatchExact creates a Matcher that matches the provided string exactly.
func MatchExact(s string) Matcher {
	return Matcher(regexp.MustCompile("^" + regexp.QuoteMeta(s) + "$"))
}

// MatchRegexp creates a Matcher that matches the provided regexp string.
// Panics if the regexp string is invalid.
func MatchRegexp(s string) Matcher {
	return Matcher(regexp.MustCompile(s))
}

// MetricBounds associates metrics and tests with lower/upper bounds.
// If a test or metric matcher is not set,
// then the MetricBounds matches all tests or metrics respectively.
type MetricBounds struct {
	Test   Matcher
	Metric Matcher
	Bounds
}

// reportEntry should be either a matchedMetric or an unmatchedMetric.
type reportEntry any

type matchedMetric struct {
	MatchedMetricName string
	ActualValue       float64
	InBounds          bool
	Reason            string
}

type unmatchedMetric struct {
	UnmatchedMetricRegexp string
}

const reportFileName = "bounds-check.json"

func saveReport(outDir string, entries []reportEntry) error {
	b, err := json.Marshal(entries)
	if err != nil {
		return errors.Wrap(err, "marshal report entries")
	}
	return os.WriteFile(path.Join(outDir, reportFileName), b, 0666)
}

func checkAgainstLoadedScalars(testName string, bounds []MetricBounds, scalars []scalar) ([]reportEntry, error) {
	var entries []reportEntry
	var errs []error
	boundsForTest := filterForTest(bounds, testName)

	unmatched := make(map[*regexp.Regexp]bool, len(boundsForTest))
	for _, b := range boundsForTest {
		unmatched[b.Metric] = true
	}

	for _, scalar := range scalars {
		for _, bound := range boundsForTest {
			metricRegexp := (*regexp.Regexp)(bound.Metric)
			if metricRegexp == nil {
				metricRegexp = regexp.MustCompile("")
			}
			if !metricRegexp.MatchString(scalar.name) {
				continue
			}
			delete(unmatched, metricRegexp)
			inBounds, reason := (bound.Bounds)(scalar.value)
			entries = append(entries, matchedMetric{
				MatchedMetricName: scalar.name,
				ActualValue:       scalar.value,
				InBounds:          inBounds,
				Reason:            reason,
			})
			if !inBounds {
				errs = append(errs, errors.Errorf("metric %q: %v", scalar.name, reason))
			}
		}
	}

	for r := range unmatched {
		errs = append(errs, errors.Errorf("no metric found matching pattern %q", r.String()))
		entries = append(entries, unmatchedMetric{
			UnmatchedMetricRegexp: r.String(),
		})
	}

	return entries, errors.Join(errs...)
}

func filterForTest(bounds []MetricBounds, testName string) []MetricBounds {
	var out []MetricBounds
	for _, bound := range bounds {
		testRegexp := (*regexp.Regexp)(bound.Test)
		if testRegexp == nil {
			testRegexp = regexp.MustCompile("")
		}
		if testRegexp.MatchString(testName) {
			out = append(out, bound)
		}
	}
	return out
}

// EvaluateResults compares measured performance values against the provided bounds.
// Returns an error if any measured values do not fall within acceptable bounds
// or if no measured value was recorded for bounds associated with the currently running test.
//
// Only call EvaluateResults after performance values have been saved to disk.
//
// outDir should be set to (*testing.State).OutDir().
func EvaluateResults(ctx context.Context, bounds []MetricBounds, outDir string) error {
	testName := path.Base(outDir)
	scalars, err := loadScalars(outDir)
	if err != nil {
		return err
	}
	report, errs := checkAgainstLoadedScalars(testName, bounds, scalars)
	// intentionally write report before handling above "errors"
	err = saveReport(outDir, report)
	if err != nil {
		return errors.Wrap(err, "failed to save report")
	}

	return errs
}
