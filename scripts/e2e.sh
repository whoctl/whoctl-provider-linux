#!/bin/sh
# End-to-end test for the linux provider. It creates, changes and deletes real
# accounts and installs real packages, so it refuses to start outside a
# container.
#
# Run it through scripts/e2e-run.sh, which borrows whoctl's container harness:
#   scripts/e2e-run.sh                  # alpine, shadow-utils
#   TOOLSET=busybox scripts/e2e-run.sh  # alpine, BusyBox applets
#   DISTRO=debian scripts/e2e-run.sh    # apt, shadow-utils
#
# What runs depends on what the distro has: only Alpine can run an init system
# in a container, and each distro has exactly one package manager.
set -u

if [ ! -f /.dockerenv ] && [ ! -f /run/.containerenv ] && [ "${WHOCTL_IN_CONTAINER:-}" != "1" ]; then
	echo "refusing to run: this test creates and deletes real user accounts." >&2
	echo "run it inside a container with scripts/e2e-run.sh" >&2
	exit 1
fi

distro="${DISTRO:-alpine}"
toolset="${TOOLSET:-shadow}"
passed=0
failed=0

# One package manager per distro, and one package plus one repository kind per
# manager. The *_ref variables are what apply and delete print: provider,
# singular resource, object name.
#
# The repository specs differ per manager because the managers do: apk names a
# repository by its URL, apt by a file with a suite and components, dnf by a
# section id, pacman by a section with servers.
case "$distro" in
alpine)
	pkg_res=linux/apkpackages; pkg_kind=ApkPackage; pkg_short=linux/apk; pkg_ref=linux/apkpackage
	repo_res=linux/apkrepositories; repo_kind=ApkRepository; repo_short=linux/apkrepo; repo_ref=linux/apkrepository
	repo_name='https://example.invalid/alpine/edge/testing'
	repo_spec='  tag: testing'
	repo_file=/etc/apk/repositories
	repo_grep='example.invalid/alpine/edge/testing'
	;;
debian)
	pkg_res=linux/aptpackages; pkg_kind=AptPackage; pkg_short=linux/apt; pkg_ref=linux/aptpackage
	repo_res=linux/aptrepositories; repo_kind=AptRepository; repo_short=linux/aptrepo; repo_ref=linux/aptrepository
	repo_name=whoctl-e2e
	repo_spec='  uri: https://example.invalid/debian
  suite: bookworm
  components: [main]'
	repo_file=/etc/apt/sources.list.d/whoctl-e2e.list
	repo_grep='example.invalid/debian bookworm main'
	;;
fedora)
	pkg_res=linux/dnfpackages; pkg_kind=DnfPackage; pkg_short=linux/dnf; pkg_ref=linux/dnfpackage
	repo_res=linux/dnfrepositories; repo_kind=DnfRepository; repo_short=linux/dnfrepo; repo_ref=linux/dnfrepository
	repo_name=whoctl-e2e
	repo_spec='  displayName: whoctl e2e
  baseurl: [https://example.invalid/fedora]
  gpgcheck: true'
	repo_file=/etc/yum.repos.d/whoctl-e2e.repo
	repo_grep='baseurl=https://example.invalid/fedora'
	;;
arch)
	pkg_res=linux/pacmanpackages; pkg_kind=PacmanPackage; pkg_short=linux/pacman; pkg_ref=linux/pacmanpackage
	repo_res=linux/pacmanrepositories; repo_kind=PacmanRepository; repo_short=linux/pacmanrepo; repo_ref=linux/pacmanrepository
	repo_name=whoctl-e2e
	repo_spec='  servers: [https://example.invalid/arch]
  sigLevel: Optional TrustAll'
	repo_file=/etc/pacman.conf
	repo_grep='Server = https://example.invalid/arch'
	;;
*)
	echo "unknown DISTRO '$distro'" >&2
	exit 1
	;;
esac

ok() {
	passed=$((passed + 1))
	echo "  ok    $1"
}

nok() {
	failed=$((failed + 1))
	echo "  FAIL  $1"
	[ $# -gt 1 ] && printf '        %s\n' "$2"
}

# expect_ok DESCRIPTION COMMAND...  — the command must succeed.
expect_ok() {
	desc=$1
	shift
	if out=$("$@" 2>&1); then
		ok "$desc"
	else
		nok "$desc" "$out"
	fi
}

# expect_fail DESCRIPTION COMMAND... — the command must fail.
expect_fail() {
	desc=$1
	shift
	if out=$("$@" 2>&1); then
		nok "$desc (unexpected success)" "$out"
	else
		ok "$desc"
	fi
}

# expect_match DESCRIPTION PATTERN COMMAND... — output must match PATTERN.
expect_match() {
	desc=$1
	pattern=$2
	shift 2
	out=$("$@" 2>&1)
	if printf '%s' "$out" | grep -qE "$pattern"; then
		ok "$desc"
	else
		nok "$desc" "expected /$pattern/ in: $out"
	fi
}

# expect_no_match DESCRIPTION PATTERN COMMAND...
expect_no_match() {
	desc=$1
	pattern=$2
	shift 2
	out=$("$@" 2>&1)
	if printf '%s' "$out" | grep -qE "$pattern"; then
		nok "$desc" "did not expect /$pattern/ in: $out"
	else
		ok "$desc"
	fi
}

section() { echo; echo "== $1"; }

echo "whoctl e2e — distro: $distro, toolset: $toolset, packages: $pkg_kind"

# the user example puts alice in wheel, which Alpine, Fedora and Arch all
# create and Debian does not. Making it a precondition keeps the example
# idiomatic instead of writing it around the least-equipped distro.
getent group wheel >/dev/null 2>&1 || whoctl apply -f - >/dev/null <<'YAML'
apiVersion: linux.whoctl.io/v1alpha1
kind: Group
metadata:
  name: wheel
YAML

section "basics"
expect_match "version prints a version" 'whoctl ' whoctl version
expect_match "api-resources lists users" '^linux/users' whoctl api-resources
expect_match "api-resources lists the package kind" "^$pkg_res" whoctl api-resources
expect_match "api-resources names the provider aliases" 'linux \(also nix\)' whoctl api-resources
expect_match "unknown resource is rejected" 'unknown resource' whoctl get linux/nonsense
# Naming a provider is what installs it, so an unknown one is no longer a
# rejection: whoctl goes looking for it. What has to be clear is that it is not
# installed — not whatever the network said on the way.
expect_match "an uninstalled provider says so" 'is not installed' whoctl get gcp/instances

section "the provider prefix is mandatory"
expect_match "an unprefixed resource is refused" 'needs its provider' whoctl get users
expect_match "and the error names the fix" 'linux/users' whoctl get users
expect_match "the provider alias resolves" '^root' whoctl get nix/users
expect_match "a short name resolves under the alias" '^root' whoctl get nix/usr root

section "reading the system"
expect_match "get users includes root" '^root' whoctl get linux/users
expect_match "get user root shows uid 0" 'uid: 0' whoctl get linux/user root -o yaml
expect_match "-o name prints the qualified reference" '^linux/user/root$' whoctl get linux/user root -o name
expect_match "a full reference is accepted as one argument" '^root' whoctl get linux/user/root
expect_match "-o wide adds the HOME column" 'HOME' whoctl get linux/users -o wide
expect_match "get on a missing user fails clearly" 'not found' whoctl get linux/user ghost
expect_match "describe renders spec and status" 'Status:' whoctl describe linux/user root

section "dry run"
expect_match "dry-run reports what it would do" 'linux/group/developers created \(dry run\)' \
	whoctl apply -f /examples/account-group.yaml --dry-run
expect_fail "dry-run really changed nothing" getent group developers

section "creating"
expect_match "apply creates the group" 'linux/group/developers created' whoctl apply -f /examples/account-group.yaml
expect_match "the group exists with the requested gid" '^developers:x:4000:' getent group developers
expect_match "re-applying is a no-op" 'linux/group/developers unchanged' whoctl apply -f /examples/account-group.yaml

expect_match "apply creates the user" 'linux/user/alice created' whoctl apply -f /examples/account-user.yaml
expect_match "the user has the requested uid" 'uid=4200' id alice
expect_match "the primary group was honoured" 'gid=4000\(developers\)' id alice
expect_match "supplementary groups were honoured" 'wheel' id alice
expect_ok "the home directory was created" test -d /home/alice
expect_match "re-applying the user is a no-op" 'linux/user/alice unchanged' whoctl apply -f /examples/account-user.yaml

section "export round-trip"
whoctl get linux/user alice -o yaml >/tmp/alice.yaml
expect_no_match "exported yaml never carries a password hash" 'passwordHash' cat /tmp/alice.yaml
expect_match "exported yaml can be applied back unchanged" 'linux/user/alice unchanged' whoctl apply -f /tmp/alice.yaml
expect_match "apply also reads stdin" 'linux/user/alice unchanged' sh -c 'whoctl get linux/user alice -o yaml | whoctl apply -f -'

section "multi-document manifests"
expect_match "a manifest with several objects applies in order" 'linux/user/carol created' whoctl apply -f /examples/team.yaml
expect_match "the locked flag was applied" 'locked: true' whoctl get linux/user carol -o yaml
expect_match "carol joined the developers group" 'developers' id carol

if [ "$toolset" = shadow ]; then
	section "updating (shadow-utils)"
	sed 's|shell: /bin/sh|shell: /bin/bash|' /examples/account-user.yaml >/tmp/alice-shell.yaml
	expect_match "changing the shell reports configured" 'linux/user/alice configured' whoctl apply -f /tmp/alice-shell.yaml
	expect_match "the new shell is in /etc/passwd" 'alice:.*:/bin/bash' getent passwd alice

	sed 's|- wheel|- platform|' /tmp/alice-shell.yaml >/tmp/alice-groups.yaml
	expect_match "group membership is reconciled" 'linux/user/alice configured' whoctl apply -f /tmp/alice-groups.yaml
	expect_match "alice joined platform" 'platform' id alice
	expect_no_match "alice left wheel" 'wheel' id alice

	printf '#!/bin/sh\nsed -i "s|comment: .*|comment: Edited By Test|" "$1"\n' >/tmp/fake-editor
	chmod +x /tmp/fake-editor
	expect_match "edit applies what the editor saved" 'linux/user/alice configured' \
		env WHOCTL_EDITOR=/tmp/fake-editor whoctl edit linux/user alice
	expect_match "the edited comment landed in /etc/passwd" 'Edited By Test' getent passwd alice

	printf '#!/bin/sh\ntrue\n' >/tmp/noop-editor
	chmod +x /tmp/noop-editor
	expect_match "an untouched editor cancels the edit" 'Edit cancelled' \
		env WHOCTL_EDITOR=/tmp/noop-editor whoctl edit linux/user alice
else
	section "updating (busybox)"
	sed 's|shell: /bin/sh|shell: /bin/bash|' /examples/account-user.yaml >/tmp/alice-shell.yaml
	expect_match "busybox explains it cannot update accounts" 'apk add shadow' \
		whoctl apply -f /tmp/alice-shell.yaml
fi

section "packages ($pkg_kind)"
cat >/tmp/pkg.yaml <<YAML
apiVersion: linux.whoctl.io/v1alpha1
kind: $pkg_kind
metadata:
  name: tree
spec:
  state: installed
YAML
expect_match "the package is not installed yet" 'not found' whoctl get "$pkg_short" tree
expect_match "apply installs it" "$pkg_ref/tree created" whoctl apply -f /tmp/pkg.yaml
# Presence is asserted through the manager's database, not through
# /usr/bin/tree. Alpine's BusyBox provides a `tree` applet of its own, so that
# path exists before the install and comes back as a symlink after the removal:
# it would pass both checks without the package ever being involved.
expect_match "get reports it installed" 'installed' whoctl get "$pkg_short" tree
expect_match "and records the version the manager chose" 'version: .+' whoctl get "$pkg_short" tree -o yaml
expect_match "-o wide shows the architecture" 'ARCH' whoctl get "$pkg_short" tree -o wide
expect_match "re-applying is a no-op" 'unchanged' whoctl apply -f /tmp/pkg.yaml
expect_match "export round-trips" 'unchanged' sh -c "whoctl get $pkg_short tree -o yaml | whoctl apply -f -"
expect_match "listing includes it" '^tree' sh -c "whoctl get $pkg_res | grep '^tree'"

if [ "$distro" = arch ]; then
	expect_match "pacman refuses a pinned version" 'cannot install a chosen version' \
		sh -c "sed 's|state: installed|state: installed\n  version: 1.2.3|' /tmp/pkg.yaml | whoctl apply -f -"
else
	expect_match "an impossible version is rejected by the manager" 'failed' \
		sh -c "sed 's|state: installed|state: installed\n  version: 0.0.0-nope|' /tmp/pkg.yaml | whoctl apply -f -"
fi

sed 's|state: installed|state: absent|' /tmp/pkg.yaml >/tmp/pkg-absent.yaml
expect_match "state absent removes it" 'configured' whoctl apply -f /tmp/pkg-absent.yaml
expect_match "the manager no longer has it" 'not found' whoctl get "$pkg_short" tree
expect_match "applying absent again is a no-op" 'unchanged' whoctl apply -f /tmp/pkg-absent.yaml
expect_match "deleting a package that is not installed fails" 'not found' whoctl delete "$pkg_short" tree
expect_match "the other managers report themselves unavailable" 'not available on this system' \
	sh -c "whoctl get linux/apkpackages linux/aptpackages 2>&1 | grep -v '^NAME' | head -1; whoctl get linux/pacmanpackages 2>&1"

section "repositories ($repo_kind)"
expect_match "get lists the distro's own repositories" 'ENABLED' whoctl get "$repo_res"
expect_match "every listed repository has a name" '^[a-z0-9/:._-]+' sh -c "whoctl get $repo_res | sed -n 2p"
cat >/tmp/repo.yaml <<YAML
apiVersion: linux.whoctl.io/v1alpha1
kind: $repo_kind
metadata:
  name: $repo_name
spec:
$repo_spec
YAML
expect_match "apply creates it" "$repo_ref/.* created" whoctl apply -f /tmp/repo.yaml
expect_match "it is in the file now" "$repo_grep" sh -c "cat $repo_file"
expect_match "re-applying is a no-op" 'unchanged' whoctl apply -f /tmp/repo.yaml
expect_match "export round-trips" 'unchanged' \
	sh -c "whoctl get $repo_short '$repo_name' -o yaml | whoctl apply -f -"
expect_match "disabling comments it out rather than dropping it" 'configured' \
	sh -c "sed 's|^spec:|spec:\n  enabled: false|' /tmp/repo.yaml | whoctl apply -f -"
expect_match "the disabled entry is still readable" 'enabled: false' \
	sh -c "whoctl get $repo_short '$repo_name' -o yaml"
expect_match "delete removes it" 'deleted' sh -c "whoctl delete $repo_short '$repo_name'"
expect_no_match "it is gone from the file" "$repo_grep" sh -c "cat $repo_file || true"
expect_match "deleting it again fails" 'not found' sh -c "whoctl delete $repo_short '$repo_name'"
expect_match "the distro's own repositories survived" 'ENABLED' whoctl get "$repo_res"

if [ "$distro" = alpine ]; then
	section "services (openrc)"
	expect_match "get services lists what is in /etc/init.d" '^crond' whoctl get linux/services
	expect_match "the init system is reported" 'initSystem: openrc' whoctl get linux/service crond -o yaml
	expect_match "shell libraries are not services" 'not found' whoctl get linux/service functions.sh
	expect_match "apply starts and enables a service" 'linux/service/crond configured' whoctl apply -f /examples/service.yaml
	expect_match "the service is enabled at boot" 'crond' sh -c 'rc-update show default'
	expect_ok "the service is running" test -e /run/openrc/started/crond
	expect_match "re-applying is a no-op" 'linux/service/crond unchanged' whoctl apply -f /examples/service.yaml
	expect_match "restart works" 'linux/service/crond restarted' whoctl restart linux/service crond
	expect_ok "the service is still running after a restart" test -e /run/openrc/started/crond

	cat >/tmp/crond-off.yaml <<'YAML'
apiVersion: linux.whoctl.io/v1alpha1
kind: Service
metadata:
  name: crond
spec:
  enabled: false
  state: stopped
YAML
	expect_match "apply stops and disables it" 'linux/service/crond configured' whoctl apply -f /tmp/crond-off.yaml
	expect_fail "the service is stopped" test -e /run/openrc/started/crond
	expect_no_match "the service left every runlevel" 'crond' sh -c 'rc-update show default'
	expect_match "services cannot be deleted" 'cannot be deleted' whoctl delete linux/service crond
	expect_match "an unknown service is reported as missing" 'not found' whoctl get linux/service nosuchservice
else
	section "services (none)"
	# Debian keeps sysvinit scripts in /etc/init.d without any of OpenRC's
	# tooling, and no container runs systemd, so the right answer here is a
	# clear refusal rather than a backend whose every mutation fails.
	expect_match "no init system is detected" 'no supported init system' whoctl get linux/services
fi

section "nameservers"
expect_match "get nameservers reads resolv.conf" 'PRIORITY' whoctl get linux/nameservers
expect_match "apply adds a resolver at the requested position" 'linux/nameserver/1.1.1.1 created' \
	whoctl apply -f /examples/resolver-nameserver.yaml
expect_match "it landed first in the file" '^nameserver 1.1.1.1' sh -c 'grep "^nameserver" /etc/resolv.conf | head -1'
expect_match "re-applying is a no-op" 'linux/nameserver/1.1.1.1 unchanged' whoctl apply -f /examples/resolver-nameserver.yaml
expect_match "a hostname is rejected" 'must be an IP address' \
	sh -c 'printf "apiVersion: linux.whoctl.io/v1alpha1\nkind: Nameserver\nmetadata:\n  name: dns.example.com\n" | whoctl apply -f -'
expect_match "export round-trips" 'linux/nameserver/1.1.1.1 unchanged' \
	sh -c 'whoctl get linux/nameserver 1.1.1.1 -o yaml | whoctl apply -f -'
expect_match "dry-run reports without writing" 'linux/nameserver/8.8.4.4 created \(dry run\)' \
	sh -c 'printf "apiVersion: linux.whoctl.io/v1alpha1\nkind: Nameserver\nmetadata:\n  name: 8.8.4.4\n" | whoctl apply -f - --dry-run'
expect_fail "dry-run really changed nothing" grep -q '^nameserver 8.8.4.4' /etc/resolv.conf
expect_match "delete removes the entry" 'linux/nameserver/1.1.1.1 deleted' whoctl delete linux/nameserver 1.1.1.1
expect_fail "the entry is gone from the file" grep -q '^nameserver 1.1.1.1' /etc/resolv.conf
expect_match "deleting a missing entry fails" 'not found' whoctl delete linux/nameserver 1.1.1.1

section "search domains and resolver options"
expect_match "apply adds search domains and options" 'linux/searchdomain/corp.example created' \
	whoctl apply -f /examples/resolver.yaml
expect_match "the search list is one single line" '^1$' \
	sh -c 'grep -c "^search " /etc/resolv.conf'
expect_match "the requested order was honoured" '^search lab.internal corp.example' \
	sh -c 'grep "^search " /etc/resolv.conf'
expect_match "options were written on one line" '^options .*ndots:2.*rotate' \
	sh -c 'grep "^options " /etc/resolv.conf'
expect_match "re-applying the whole manifest is a no-op" 'linux/resolveroption/rotate unchanged' \
	whoctl apply -f /examples/resolver.yaml
expect_match "short names resolve" '^lab.internal' whoctl get linux/search
expect_match "an option value is split from its name" '^ndots +2' whoctl get linux/resopt

cat >/tmp/ndots.yaml <<'YAML'
apiVersion: linux.whoctl.io/v1alpha1
kind: ResolverOption
metadata:
  name: ndots
spec:
  value: "5"
YAML
expect_match "changing an option value updates in place" 'linux/resolveroption/ndots configured' whoctl apply -f /tmp/ndots.yaml
expect_match "the option was not duplicated" '^1$' \
	sh -c 'grep -o "ndots:[0-9]*" /etc/resolv.conf | wc -l'
expect_match "the new value is in the file" 'ndots:5' sh -c 'grep "^options " /etc/resolv.conf'

expect_match "a domain with a space is rejected" 'cannot contain spaces' \
	sh -c 'printf "apiVersion: linux.whoctl.io/v1alpha1\nkind: SearchDomain\nmetadata:\n  name: two domains\n" | whoctl apply -f -'
expect_match "an option name carrying a colon is rejected" 'spec.value' \
	sh -c 'printf "apiVersion: linux.whoctl.io/v1alpha1\nkind: ResolverOption\nmetadata:\n  name: ndots:2\n" | whoctl apply -f -'

expect_match "export round-trips" 'linux/searchdomain/lab.internal unchanged' \
	sh -c 'whoctl get linux/searchdomain lab.internal -o yaml | whoctl apply -f -'
expect_match "deleting from a manifest works" 'linux/searchdomain/lab.internal deleted' \
	whoctl delete -f /examples/resolver.yaml
expect_no_match "the deleted domains left the search line" 'lab.internal|corp.example' \
	sh -c 'grep "^search " /etc/resolv.conf || true'
expect_no_match "the deleted options left the options line" 'rotate' \
	sh -c 'grep "^options " /etc/resolv.conf || true'
expect_match "nameservers survived all of that" '^nameserver ' sh -c 'grep "^nameserver " /etc/resolv.conf | head -1'

section "deleting"
expect_fail "a primary group cannot be deleted while in use" whoctl delete linux/group developers
expect_match "deleting a user works" 'linux/user/alice deleted' whoctl delete linux/user alice --cascade
expect_fail "the account is gone" id alice
expect_fail "--cascade removed the home directory" test -d /home/alice
expect_match "deleting a missing user fails" 'not found' whoctl delete linux/user alice
expect_ok "--ignore-not-found makes it idempotent" whoctl delete linux/user alice --ignore-not-found

expect_match "users can be deleted from a manifest" 'linux/user/bob deleted' whoctl delete -f /examples/team.yaml --ignore-not-found
expect_match "an unused group can be deleted" 'linux/group/developers deleted' whoctl delete linux/group developers

echo
echo "passed: $passed  failed: $failed"
[ "$failed" -eq 0 ]
