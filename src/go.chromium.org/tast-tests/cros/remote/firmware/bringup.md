# FAFT for bringup (go/faft-bringup)

[TOC]

## Setup

You need a servo_v4 or servo_v4.1, probably a servo micro, or a C2D2 micro, and
the board you are testing.

## Running a single test

Since TAST can't connect to the DUT to probe for hardware information, you need
to provide it on the command line. The option is a text proto of type
[tast.core.HardwareFeatures](https://chromium.googlesource.com/chromiumos/platform/tast/+/main/src/go.chromium.org/tast/core/framework/protocol/dutfeatures.proto)

The options that are known to be needed are

```textproto
deprecated_device_config <
  cpu:X86_64 # Or X86, ARM, ARM64
  id<
    platform:'boardname'
    model:'modelname'
  >
>
hardware_features <
  embedded_controller <
    present:PRESENT ec_type:EC_CHROME
  >
  form_factor <
    form_factor: CLAMSHELL # Or CONVERTIBLE, DETACHABLE, CHROMEBOX, CHROMESLATE, etc.
  >
>
```

(inside chroot)
```shell
SERVO=localhost:9999:nossh
BOARD=grunt
MODEL=treeya
tast run -var servo=${SERVO?} \
-hwdeps="deprecated_device_config <cpu:X86_64 \
  id<platform:'${BOARD?}' model:'${MODEL?}'>> \
  hardware_features<embedded_controller<present:PRESENT ec_type:EC_CHROME> \
    form_factor<form_factor:CLAMSHELL> \
  >" \
  - firmware.BootTime*
```

## Running all tests

(inside chroot)
```shell
SERVO=localhost:9999:nossh
BOARD=grunt
MODEL=treeya
tast run -var servo=${SERVO?} \
-hwdeps="deprecated_device_config <cpu:X86_64 \
  id<platform:'${BOARD?}' model:'${MODEL?}'>> \
  hardware_features<embedded_controller<present:PRESENT ec_type:EC_CHROME>
      form_factor<form_factor:CLAMSHELL> \
  >" \
  - '("group:firmware" && firmware_bringup)'
```

## Adapting a firmware test for bringup

Follow the instructions for creating a [FAFT test in Tast](codelab/README.md).
For the bringup case, you must not make any SSH calls to the DUT, and that means
that you can't use any [Tast gRPC
services](https://chromium.googlesource.com/chromiumos/platform/tast/+/HEAD/docs/writing_tests.md#calling-grpc-services)
either.

### Hardware dependencies

The test's hardware deps won't work without ssh to probe the hardware features.
Either change the test to not use any hardware dependencies, or pass the
necessary hardware info on the command line using the -hwdeps flag.

### Getting board specific configs

Set the board and model explicitly from command line vars. This is probably
not necessary if you passed the `-hwdeps` flag to `tast run`.

```
func init() {
	testing.AddTest(&testing.Test{
                Func: MyTest,
		Vars: []string{"board", "model"},
                // ...
	})
}

func MyTest(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	board, _ := s.Var("board")
	model, _ := s.Var("model")
	h.OverridePlatform(ctx, board, model)

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

        // At this point h.Config, h.Board, and h.Model are usable.
}
```
