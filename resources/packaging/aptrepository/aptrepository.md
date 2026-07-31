---
subcategory: Packages
---

# AptRepository

An apt source under `/etc/apt/sources.list.d`. The object's name is the file's
base name without its extension, because that is the identity apt users already
work with: you add a repository by dropping `docker.list` into the directory and
remove it by deleting that file. A source line has no id of its own.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: AptRepository
metadata:
  name: example
spec:
  uri: https://deb.example.com/debian
  suite: bookworm
  components: [main]
  options:
    signed-by: /etc/apt/keyrings/example.asc
```

```console
$ whoctl apply -f example.yaml
linux/aptrepository/example created

$ cat /etc/apt/sources.list.d/example.list
deb [signed-by=/etc/apt/keyrings/example.asc] https://deb.example.com/debian bookworm main

$ whoctl get linux/aptrepo -o wide
NAME     ENABLED   SUITE             URI                                     FORMAT   FILE
debian   true      stable-security   http://deb.debian.org/debian-security   deb822   /etc/apt/sources.list.d/debian.sources
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | optional | Whether apt should use this source. A disabled one is commented out rather than removed. |
| `type` | string | optional | deb for binary packages, deb-src for sources. Defaults to deb. Example: `deb`. |
| `uri` | string | **required** | Where the archive lives. Example: `https://download.docker.com/linux/debian`. |
| `suite` | string | **required** | The distribution the packages are built for. Example: `bookworm`. |
| `components` | list of string | optional | Which parts of the archive to use. Example: `main`. |
| `options` | map of string | optional | Bracketed options on the source line, such as signed-by or arch. Example: `signed-by: /etc/apt/keyrings/docker.asc`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `enabled` | boolean | Whether the source line is active. |
| `type` | string | deb or deb-src. Example: `deb`. |
| `uri` | string | Where the archive lives. |
| `suite` | string | The distribution the packages are built for. Example: `bookworm`. |
| `components` | list of string | Which parts of the archive are in use. |
| `options` | map of string | The bracketed options on the source line. |
| `format` | string | one-line for a .list file, deb822 for the stanza format, which whoctl reads but does not rewrite. Example: `one-line`. |
| `file` | string | The file the entry was read from. Example: `/etc/apt/sources.list.d/docker.list`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `ENABLED` | always |
| `SUITE` | always |
| `URI` | always |
| `FORMAT` | `-o wide` |
| `FILE` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Two formats, one of them read-only.** The one-line format — `deb [options] uri
suite components` — is what whoctl writes. The deb822 stanza format that Debian
12 and later ship is read, so those repositories show up in `get` with
`status.format: deb822`, but applying to one is refused rather than attempted: a
stanza can carry several URIs and suites at once, and squeezing that into a
single-entry model would quietly throw part of it away.

```console
$ whoctl apply -f debian.yaml
error: linux/aptrepository/debian: repository "debian" is in the deb822 format, which whoctl reads but does not rewrite: edit /etc/apt/sources.list.d/debian.sources by hand
```

**One source line per object.** A `.list` file holding more than one active line
is listed, but applying to it is refused for the same reason. This matches how
`sources.list.d` is used in practice — one repository per file — without
pretending the other shape does not exist.

**`/etc/apt/sources.list` is read but never deleted.** It appears under the name
`sources`; removing the distribution's own source list is not something `delete`
should do on a whim, so it is an error rather than a silent unlink.

**`spec.options` is where `signed-by` goes**, and with it `arch`, `trusted` and
anything else apt accepts in the brackets. Options are written in sorted order,
so a re-apply produces a byte-identical line rather than one that differs only
by shuffling.

**Disabled means commented out.** `enabled: false` comments the line rather than
emptying the file, so the URI, suite and key are all still there to turn back on.

See [AptPackage](aptpackage.md) for the packages these sources provide.
