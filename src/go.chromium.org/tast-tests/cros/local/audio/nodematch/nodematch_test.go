// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nodematch

import (
	"testing"

	"go.chromium.org/tast-tests/cros/local/audio/types"
)

func TestString(t *testing.T) {
	for name, item := range map[string]struct {
		matcher Matcher
		want    string
	}{
		"name": {
			matcher: Name("foo"),
			want:    `Name("foo")`,
		},
		"type": {
			matcher: Type("bar"),
			want:    `Type("bar")`,
		},
		"direction": {
			matcher: Direction(types.InputStream),
			want:    `Direction(InputStream)`,
		},
		"all": {
			matcher: All(Name("foo"), Type("bar")),
			want:    `All(Name("foo"), Type("bar"))`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			actual := item.matcher.String()
			if actual != item.want {
				t.Errorf("matcher.String() = %q; want %q", actual, item.want)
			}
		})
	}
}
