# whoctl-provider-linux

Accounts, services, packages, repositories and the resolver, on the local
system. A binary speaking whoctl's protocol over stdio, built on
`github.com/whoctl/whoctl-sdk-go`.

**This provider creates and deletes real accounts and installs real packages.**
The safety rules are in the workspace `CLAUDE.md` and they are not optional
here. `go test ./...` is safe on the host: it reads `testdata/` and never
`/etc`. Everything else runs in a container.

```sh
make test                # unit tests + e2e across every distro
make e2e DISTRO=fedora   # one distro
make e2e TOOLSET=busybox # alpine without shadow-utils
```

**One distro per package manager**, because a package manager cannot be
exercised anywhere but on its own system. `TOOLSET` means something only on
Alpine, the one distro shipping BusyBox's applets instead of shadow-utils. Only
Alpine can run an init system in a container, so the service tests run there and
the other three assert the provider refuses clearly instead of guessing.

**The container needs `--init`.** A daemon started through the provider is
orphaned when the process exits; with a shell as PID 1 nothing reaps it, and
OpenRC's `start-stop-daemon` then reports "process refused to stop" on the next
restart.

## Layout: families

A kind lives in `resources/<family>/<kind>/` with its spec, its handler, its
tests, its page and its example. Adding one is adding a directory plus one line
in `internal/linux`.

**What a family shares sits at the family's root**, so the tree says who shares
what: `resources/account/etcfiles` is read by the account kinds and by nobody
else. `internal/` is left holding no domain logic at all — the assembly, the
fixtures, and a `provider` that is a filesystem root and a runner.

Two consequences worth keeping:

- `internal/linux` is a **third** package on purpose. Every resource needs the
  shared state and the list of resources needs every resource, so putting the
  list in `provider` would be a cycle.
- Shared state read by one kind is state in the wrong place. The init system
  moved into `resources/service` and the passwd parsing into `resources/account`
  for exactly that reason; the same mistake is easy to make again.

The eight packaging kinds share two handlers (`pkgkind`, `repokind`) and still
have a directory each. The implementation is shared; the kind is not.

## Decisions somebody would otherwise undo

**One kind per package manager, not one Package kind.** The opposite of the
choice made for accounts, deliberately. `User` hides shadow-utils and BusyBox
because an account is an account either way. A package is not: the same software
is `openssh-server` under apt and `openssh` under apk, the version grammars do
not travel, and pacman cannot pin a version at all. A shared kind would produce
manifests that look portable and are not.

Every manager is registered on every machine, so a kind whose binary is missing
fails with `core.CodeUnavailable` rather than reporting an empty list: "nothing
is installed" and "this manager does not run here" are different facts.

**Reads parse files, writes shell out.** Reading `/etc/passwd` is fast and
predictable; writing goes through `useradd`/`adduser` so distro behaviour
(skeleton files, locking, PAM) is preserved and `/etc` is never corrupted by a
half-written line.

`resolvconf` and the repository files are the exceptions, because no native tool
owns them end to end. They keep every line, key and comment the model does not
cover verbatim. pacman's `[options]` section is excluded outright: it configures
pacman itself, and rewriting it as a repository would break the machine.

**Package databases are read, not queried — except rpm**, whose store is binary
with no stable on-disk format. The dnf backend shells out to `rpm -qa` with an
explicit query format. That is the one read that runs a command.

**One owner per directive.** `Nameserver` owns the `nameserver` lines,
`SearchDomain` the `search` list, `ResolverOption` the `options` list. Two kinds
writing the same directive means the second silently undoes the first, which is
why there is no `Resolver` singleton.

**Init detection needs the tooling, not the directory.** Debian keeps sysvinit
scripts in `/etc/init.d` with none of OpenRC's commands, so matching on the
directory hands it a backend whose every mutation fails with `rc-service: not
found`.

**Spec is round-trippable.** `get` fills `spec` with the *observed* state, so
`get -o yaml | apply -f -` reports `unchanged`. The e2e asserts it; keep it true
when adding fields.

**Credentials are write-only.** `spec.passwordHash` applies but never reads
back — the shadow parser keeps the hash unexported and exposes only
`HashEquals`, so an exported manifest cannot leak one while `apply` stays
idempotent.

**nil is not empty.** An absent list means "not managed here"; a present empty
one means "make it empty". `spec.groups: []` removes every supplementary group.

## Documentation

The prose is written by hand in each kind's `<kind>.md`; the tables come from
`doc` tags on the spec and status structs and are injected between HTML-comment
markers, so the page still reads on GitHub. `docExample` shows a value,
`docFlags` marks what the type cannot say.

`make docs` writes the bundle a release publishes. `TestConformance` runs the
docs check, so `go test ./...` fails on a field with no doc tag, a kind with no
page, or a page whose tables are stale. Do not "fix" it by deleting the check.
