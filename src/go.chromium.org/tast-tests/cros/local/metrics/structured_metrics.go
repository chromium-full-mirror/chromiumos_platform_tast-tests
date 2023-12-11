// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"

	"google.golang.org/protobuf/proto"

	metricspb "go.chromium.org/tast-tests/cros/local/metrics/proto"
	"go.chromium.org/tast/core/errors"
)

const (
	structuredEventsDir = "/var/lib/metrics/structured/events/"
)

// CleanStructuredEvents removes all files in structuredEventsDir.
func CleanStructuredEvents(ctx context.Context) error {
	eventFiles, err := os.ReadDir(structuredEventsDir)
	if err != nil {
		return errors.Wrapf(err, "failed to read files in %s", structuredEventsDir)
	}

	for _, file := range eventFiles {
		eventPath := filepath.Join(structuredEventsDir, file.Name())
		if err := os.RemoveAll(eventPath); err != nil {
			return errors.Wrapf(err, "failed to RemoveAll(%q)", eventPath)
		}
	}
	return nil
}

// getNameHash returns the first 8 bytes of the MD5 hash of the name.
func getNameHash(ctx context.Context, name string) string {
	nameMD5 := md5.Sum([]byte(name))
	nameHash := hex.EncodeToString(nameMD5[:8])

	return nameHash
}

// HasStructuredEventBeenReported checks all existing structured metrics events
// and parses them to verify whether an event with the name eventName exists.
func HasStructuredEventBeenReported(ctx context.Context, eventName string) (bool, error) {
	eventFiles, err := os.ReadDir(structuredEventsDir)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read files in %s", structuredEventsDir)
	}

	for _, file := range eventFiles {
		eventPath := filepath.Join(structuredEventsDir, file.Name())
		fileContent, err := os.ReadFile(eventPath)
		if err != nil {
			return false, errors.Wrapf(err, "failed to read event file %s", eventPath)
		}

		structuredData := &metricspb.StructuredDataProto{}
		if err = proto.Unmarshal(fileContent, structuredData); err != nil {
			return false, errors.Wrapf(err, "failed to parse event file %s", eventPath)
		}

		// Each file in structuredEventsDir contains an encoded StructuredDataProto.
		// There is at most one event in each file, so we only need to check the
		// content of the first element of the Events array.
		if len(structuredData.Events) > 0 {
			eventNameHash := structuredData.Events[0].GetEventNameHash()
			if strconv.FormatUint(eventNameHash, 16) == getNameHash(ctx, eventName) {
				return true, nil
			}
		}
	}

	return false, nil
}
