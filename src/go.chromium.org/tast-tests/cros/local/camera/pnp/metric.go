// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pnp provides fixture for stable power and performance evaluation.
package pnp

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
)

// FunctionMetric is parsed from a metric JSON file.
type FunctionMetric struct {
	FunctionName   string `json:"function_name"`
	MetricName     string `json:"metric_name"`
	Unit           string `json:"unit"`
	Value          int64  `json:"value"`
	BiggerIsBetter bool   `json:"bigger_is_better"`
}

// SetMetric adds a metric to a perf value.
func SetMetric(pv *perf.Values, name, unit string, value float64, biggerIsBetter bool) {
	// perf.Values valid metric names only allow "^[a-zA-Z0-9._-]{1,256}$".
	name = strings.Replace(name, "~", "Destructor-", -1)
	re := regexp.MustCompile("[^a-zA-Z0-9._-]")
	name = re.ReplaceAllString(name, "-")

	perfDirection := perf.SmallerIsBetter
	if biggerIsBetter {
		perfDirection = perf.BiggerIsBetter
	}

	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: perfDirection,
	}, value)
}

// SetMetricFromFunction adds a metric from pnp.FunctionMetric to a perf value.
func SetMetricFromFunction(pv *perf.Values, fm FunctionMetric, prefix string) {
	SetMetric(
		pv, fmt.Sprintf("%s%s_%s", prefix, fm.FunctionName, fm.MetricName),
		fm.Unit, float64(fm.Value), fm.BiggerIsBetter)
}
