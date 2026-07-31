#!/bin/sh
# Runs this provider's end-to-end suite for one distro.
#
# The container harness is whoctl's — running whoctl and some providers on a
# throwaway machine is its job, and it does not care which distro it is. What
# stays here is what is this provider's: the assertions, and the fact that a
# package manager cannot be exercised anywhere but on its own system.
#
# Usage:
#   scripts/e2e-run.sh                       # alpine, shadow-utils
#   DISTRO=fedora scripts/e2e-run.sh
#   TOOLSET=busybox scripts/e2e-run.sh       # alpine only
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
sandbox="${WHOCTL_SANDBOX:-$root/../whoctl/scripts/sandbox.sh}"
distro="${DISTRO:-alpine}"
toolset="${TOOLSET:-shadow}"

if [ ! -x "$sandbox" ]; then
	echo "no sandbox to run in: check out github.com/whoctl/whoctl beside this" >&2
	echo "repository, or set WHOCTL_SANDBOX to its scripts/sandbox.sh." >&2
	exit 1
fi

( cd "$root" && make --no-print-directory build ) >&2

# TOOLSET is this provider's alone: Alpine is the only distro that ships
# BusyBox's account applets instead of shadow-utils, and both have to be tested.
packages=""
if [ "$distro" = alpine ] && [ "$toolset" = busybox ]; then
	packages="bash vim jq openrc busybox-openrc"
fi

PROVIDERS=linux \
PACKAGES="$packages" \
MOUNTS="-v $root/scripts:/scripts:ro -v $root/bin/examples:/examples:ro" \
ENV="-e TOOLSET=$toolset" \
DISTRO="$distro" \
	exec "$sandbox" /scripts/e2e.sh
