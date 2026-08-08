#!/bin/sh
# Runs this provider's end-to-end suite for one distro.
#
# The machine it runs on is scripts/sandbox.sh, and everything about how that
# machine is built lives there. This is the suite and nothing else.
#
# Two ways of preparing the same container would be two things to drift, and
# the shell somebody opens by hand has to be the same one the assertions run
# against — otherwise reproducing a failure means reproducing the difference
# first, and the difference is the part nobody knows about.
#
# Usage:
#   scripts/e2e-run.sh                       # alpine, shadow-utils
#   DISTRO=fedora scripts/e2e-run.sh
#   TOOLSET=busybox scripts/e2e-run.sh       # alpine only
set -eu

exec "$(cd "$(dirname "$0")" && pwd)/sandbox.sh" /scripts/e2e.sh
