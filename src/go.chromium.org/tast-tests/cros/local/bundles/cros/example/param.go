// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

type animal struct {
	numLegs int
	crying  string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Param,
		Desc:         "Parameterized test example",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		Params: []testing.Param{{
			Name: "dog",
			Val: animal{
				numLegs: 4,
				crying:  "bow-wow",
			},
		}, {
			Name: "duck",
			Val: animal{
				numLegs: 2,
				crying:  "quack",
			},
		}},
	})
}

func Param(ctx context.Context, s *testing.State) {
	s.Log("Value: ", s.Param().(animal))
}
