#!/bin/sh
# Opens a shell on a throwaway machine with this provider ready to use, or runs
# one command there.
#
# The container harness is whoctl's — running whoctl and some providers on a
# throwaway machine is its job, and it does not care which distro it is. What
# stays here is what is this provider's: which distro, which account tooling,
# and the examples the suite applies.
#
# # Why a sandbox at all, for this provider above every other
#
# It creates and deletes real accounts and installs real packages. There is no
# --root that makes a mutation safe, because the mutation goes through
# useradd and apk, and those write to the machine they are running on. So the
# machine has to be one nobody minds losing. That is not a convenience here, it
# is the only way any of these verbs can be exercised at all.
#
# Usage:
#   scripts/sandbox.sh                          # a shell, on Alpine
#   scripts/sandbox.sh whoctl get linux/users   # one command
#   DISTRO=fedora scripts/sandbox.sh
#   TOOLSET=busybox scripts/sandbox.sh          # alpine only
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

# The examples are staged for this container to mount, and only for that: they
# are not in the release, because nobody installing a provider wants nine YAML
# files in ~/.whoctl.
( cd "$root" && make --no-print-directory build examples ) >&2

# TOOLSET is this provider's alone: Alpine is the only distro that ships
# BusyBox's account applets instead of shadow-utils, and both have to be tested.
packages=""
if [ "$distro" = alpine ] && [ "$toolset" = busybox ]; then
	# PACKAGES replaces the harness's list outright, so the four every sandbox
	# has are spelled again here. What is deliberately missing is `shadow`:
	# testing BusyBox's account applets means the machine must not have
	# shadow-utils on it, which is the whole point of this toolset.
	packages="bash vim jq yq openrc busybox-openrc"
fi

# The distro is in the name because this provider is the one that runs the same
# code against four of them, and which one you are in is the first thing you
# need to know when something behaves differently.
PROVIDERS=linux \
SANDBOX_NAME="linux-$distro" \
PACKAGES="$packages" \
MOUNTS="-v $root/scripts:/scripts:ro -v $root/bin/examples:/examples:ro" \
ENV="-e TOOLSET=$toolset" \
DISTRO="$distro" \
	exec "$sandbox" "$@"
