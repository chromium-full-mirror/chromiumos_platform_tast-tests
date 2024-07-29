// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
