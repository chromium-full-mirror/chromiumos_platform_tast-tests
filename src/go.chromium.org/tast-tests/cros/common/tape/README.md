# TAPE Security
Note that TAPE requires local OAUTH for running locally, and uses known service
accounts when running in the lab. Context for this can be found here:
https://bugs.chromium.org/p/chromium/issues/detail?id=1339418. This is all
handled by the TAPE logic but requires a few manual steps when running tests
that use the TAPE service.

## Running tests locally
Check the internal documentation on how to run a test locally:
go/tape-tast-doc#local-execution
