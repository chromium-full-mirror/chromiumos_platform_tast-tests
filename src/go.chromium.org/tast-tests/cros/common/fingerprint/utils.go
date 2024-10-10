// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import "strings"

// ParseColonDelimitedOutput parses colon delimited information to a map.
func ParseColonDelimitedOutput(output string) map[string]string {
	ret := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		// Note that the ectool version build info line uses ':'s as time of
		// date delimiters.
		splits := strings.SplitN(line, ":", 2)
		if len(splits) != 2 {
			continue
		}
		ret[strings.TrimSpace(splits[0])] = strings.TrimSpace(splits[1])
	}
	return ret
}
