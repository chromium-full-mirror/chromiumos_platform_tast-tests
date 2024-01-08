// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

var (
	// exampleStrVar demonstrates how to declare a runtime variable.
	exampleStrVar = testing.RegisterVarString(
		"example.strvar", // The name of the variable which should have "<pkg_name>." as prefix.
		"Default value",
		"An example variable of string type to demonstrate how to use runtime variable")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RuntimeVars,
		Desc:         "Runtime variables",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

func RuntimeVars(ctx context.Context, s *testing.State) {
	testing.ContextLogf(ctx, "Runtime variable %q has value of %q", exampleStrVar.Name(), exampleStrVar.Value())
}
