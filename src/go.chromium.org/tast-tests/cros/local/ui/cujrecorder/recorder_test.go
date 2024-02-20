// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testutil"
)

func TestAddExtraChromeTraceCategories(t *testing.T) {
	const templateConfig = `
	# Chrome trace events.
	data_sources: {
    	config {
        	name: "org.chromium.trace_event"
        	chrome_config {
				# Categories: cc, benchmark, input.
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\"],\"memory_dump_config\":{}}"
			}
		}
	}

	# Chrome trace metadata.
	data_sources: {
	config {
			name: "org.chromium.trace_metadata"
			chrome_config {
				# Categories: cc, benchmark, input.
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\"],\"memory_dump_config\":{}}"
			}
		}
	}

	# Chrome trace events after client library enabled.
	data_sources: {
		config {
			name: "track_event"
			target_buffer: 2
			chrome_config {
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\"],\"memory_dump_config\":{}}"
			}
			track_event_config {
				disabled_categories: "*"
				enabled_categories: "cc"
				enabled_categories: "benchmark"
				enabled_categories: "input"
				enabled_categories: "__metadata"
			}
		}
	}
	`
	const extraCategories = "gpu,v8"
	const expectedNewConfig = `
	# Chrome trace events.
	data_sources: {
    	config {
        	name: "org.chromium.trace_event"
        	chrome_config {
				# Categories: cc, benchmark, input.
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\",\"gpu\",\"v8\"],\"memory_dump_config\":{}}"
			}
		}
	}

	# Chrome trace metadata.
	data_sources: {
	config {
			name: "org.chromium.trace_metadata"
			chrome_config {
				# Categories: cc, benchmark, input.
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\",\"gpu\",\"v8\"],\"memory_dump_config\":{}}"
			}
		}
	}

	# Chrome trace events after client library enabled.
	data_sources: {
		config {
			name: "track_event"
			target_buffer: 2
			chrome_config {
				trace_config: "{\"record_mode\":\"record-until-full\",\"included_categories\":[\"cc\",\"benchmark\",\"input\",\"gpu\",\"v8\"],\"memory_dump_config\":{}}"
			}
			track_event_config {
				disabled_categories: "*"
				enabled_categories: "cc"
				enabled_categories: "benchmark"
				enabled_categories: "input"
				enabled_categories: "gpu"
				enabled_categories: "v8"
				enabled_categories: "__metadata"
			}
		}
	}
	`

	ctx := context.Background()

	dir := testutil.TempDir(t)
	defer os.RemoveAll(dir)

	originalConfigPath := filepath.Join(dir, "original_perfetto_config.pbtxt")
	if err := os.WriteFile(originalConfigPath, []byte(templateConfig), 0644); err != nil {
		t.Fatal("Failed to create original_perfetto_config.pbtxt: ", err)
	}
	cleanup, newConfigPath, err := addExtraChromeTraceCategories(ctx, originalConfigPath, extraCategories)
	if cleanup != nil {
		defer cleanup(ctx)
	}
	if err != nil {
		t.Fatal("Failed to add extra chrome trace categories: ", err)
	}

	newConfigStr, err := readAll(newConfigPath)
	if err != nil {
		t.Fatal("Failed to read from the new config file: ", err)
	}
	if newConfigStr != expectedNewConfig {
		t.Fatalf("The new config is not expected; expect %s, got %s", expectedNewConfig, newConfigStr)
	}
}

func TestProcessMetricsRecord(t *testing.T) {
	metricCountName := "MetricCount"
	metricMemoryName := "MetricMemory"

	countMetricConfig := NewCustomMetricConfig(metricCountName, "count", perf.SmallerIsBetter)
	memoryMetricConfig := NewMemoryMetricConfig(metricMemoryName, "MB")

	records := map[browser.Type]map[string]*record{
		browser.TypeAsh: {
			metricCountName: &record{
				config:     countMetricConfig,
				totalCount: 1,
				Sum:        2,
			},
			metricMemoryName: &record{
				config:     memoryMetricConfig,
				totalCount: 5,
				Sum:        10,
			},
		},
		browser.TypeLacros: {
			metricCountName: &record{
				config:     countMetricConfig,
				totalCount: 3,
				Sum:        4,
			},
			metricMemoryName: &record{
				config:     memoryMetricConfig,
				totalCount: 6,
				Sum:        12,
			},
		},
	}

	processed, err := processMetricRecords(records)
	if err != nil {
		t.Fatal("Failed to process records: ", err)
	}

	expected := map[string]*record{
		"MetricCount": &record{
			config:     countMetricConfig,
			totalCount: 4, // Sum of totalCount from ash and lacro
			Sum:        6, // Sum of Sum from ash and lacros
		},
		"MetricCount_ash-Chrome": &record{
			config:     countMetricConfig,
			totalCount: 1,
			Sum:        2,
		},
		"MetricCount_lacros-Chrome": &record{
			config:     countMetricConfig,
			totalCount: 3,
			Sum:        4,
		},
		"MetricMemory": &record{
			config:     memoryMetricConfig,
			totalCount: 1, // Always 1
			Sum:        4, // Sum of the averages from ash and lacros
		},
		"MetricMemory_ash-Chrome": &record{
			config:     memoryMetricConfig,
			totalCount: 5,
			Sum:        10,
		},
		"MetricMemory_lacros-Chrome": &record{
			config:     memoryMetricConfig,
			totalCount: 6,
			Sum:        12,
		},
	}

	if !reflect.DeepEqual(processed, expected) {
		t.Error("Unexpected processed results")

		dump := func(r map[string]*record) {
			var keys []string
			for key := range r {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			fmt.Println("{")
			for _, key := range keys {
				fmt.Println("  ", key, " : ", r[key])
			}
			fmt.Println("}")
		}
		fmt.Println("expected:")
		dump(expected)
		fmt.Println("processed:")
		dump(processed)
	}
}
