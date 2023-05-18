# Contribute to power tests in Tast

## Naming convention

For parameterized test names, the recommendation is to follow the following
order:

**test specific (video format, websites to test, etc) -> browser type -> other
 browser options -> hardware options**

For example:
- VideoPlayback.h264_4k_60fps_lacros: h264_4k_60fps (test specific) ->
  lacros (browser type)
- ExampleUI.ash_gaia: ash (browser type) -> gaia (other browser options)
- ExampleUI.ash_kbbl: ash (browser type) -> kbbl (hardware options)
