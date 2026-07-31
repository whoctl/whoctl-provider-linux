---
subcategory: Packages
---

# ApkPackage

A package managed by Alpine's `apk`. The object's name is the package name, and
the spec says whether it should be there and, optionally, at which version.

There is one kind per package manager rather than a single `Package`, because
packages are the part of a system that does not travel: the same software is
`openssh` here and `openssh-server` under apt, and the two version strings are
written in different grammars. A manifest that says `ApkPackage` cannot be
mistaken for one that would work on Debian.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: ApkPackage
metadata:
  name: curl
spec:
  state: installed
```

```console
$ whoctl apply -f curl.yaml
linux/apkpackage/curl created

$ whoctl get linux/apk curl -o wide
NAME   STATE       VERSION     ARCH     ORIGIN   DESCRIPTION
curl   installed   8.21.0-r0   x86_64   curl     URL retrieval utility and library

$ whoctl get linux/apk curl -o yaml | whoctl apply -f -
linux/apkpackage/curl unchanged

$ whoctl delete linux/apk curl
linux/apkpackage/curl deleted
```

Pin a version by naming it exactly as apk does, release suffix included:

```yaml
spec:
  state: installed
  version: 8.21.0-r0
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `state` | string | optional | Whether the package should be present: installed or absent. Defaults to installed. Example: `installed`. |
| `version` | string | optional | Exact version to hold the package at, in the manager's own format. Omitted, whoctl installs the newest version the configured repositories offer and then leaves an already-installed package alone. Example: `8.12.1-r0`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `state` | string | Whether the package is installed or absent. |
| `version` | string | The version currently installed. Example: `8.12.1-r0`. |
| `architecture` | string | The architecture the installed package was built for. Example: `x86_64`. |
| `description` | string | One-line summary, as recorded by the package manager. |
| `origin` | string | Where the package came from, when the manager records it: the source package under apk, the vendor under rpm. |
| `manager` | string | The package manager this kind drives. Example: `apk`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `STATE` | always |
| `VERSION` | always |
| `ARCH` | `-o wide` |
| `ORIGIN` | `-o wide` |
| `DESCRIPTION` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**An omitted version means "any version", not "the newest".** A package that is
not installed is installed at whatever the configured repositories currently
offer; one that is already installed is left exactly as it is. That is what
keeps `apply` idempotent — the alternative would upgrade the machine every time
the manifest ran, which is an upgrade policy rather than a desired state.

**A pinned version has to still exist.** `spec.version` becomes `apk add
name=version`, resolved against the repositories configured right now. A version
that has aged out of the index fails with apk's own error instead of quietly
installing something near it:

```console
$ whoctl apply -f pinned.yaml
error: linux/apkpackage/curl: command `apk add --no-cache curl=0.0.1-r0` failed: ERROR: unable to select packages:
  curl-8.21.0-r0:
    breaks: world[curl=0.0.1-r0]
```

**Reads parse apk's own database.** Installed packages come from
`/lib/apk/db/installed` rather than from `apk info`, which reports each package
as a single `curl-8.21.0-r0` string: splitting the name back out of that is
ambiguous for any package whose own name ends in something version-shaped.

**`state: absent` and `delete` are the same operation**, both running `apk del`.
Deleting a package that is not installed is a not-found error; applying
`state: absent` to one is `unchanged`.

**This kind needs apk.** On a machine without it every verb fails with `apk is
not available on this system` instead of reporting an empty list. "Nothing is
installed" and "this manager does not run here" are different facts, and only
one of them is ever true.

See [ApkRepository](apkrepository.md) for where apk looks for these packages.
