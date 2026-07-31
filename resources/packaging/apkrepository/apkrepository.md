---
subcategory: Packages
---

# ApkRepository

A package source in `/etc/apk/repositories`. The object's name is the URL
itself, because that is all an apk repository is: the file holds one URL per
line and nothing else identifies them.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: ApkRepository
metadata:
  name: https://dl-cdn.alpinelinux.org/alpine/edge/testing
spec:
  enabled: false
  tag: testing
```

```console
$ whoctl get linux/apkrepo
NAME                                                    ENABLED   TAG
https://dl-cdn.alpinelinux.org/alpine/v3.24/community   true      -
https://dl-cdn.alpinelinux.org/alpine/v3.24/main        true      -

$ whoctl apply -f testing.yaml
linux/apkrepository/https://dl-cdn.alpinelinux.org/alpine/edge/testing created

$ tail -1 /etc/apk/repositories
#@testing https://dl-cdn.alpinelinux.org/alpine/edge/testing
```

Because the name is a URL and URLs contain slashes, the whole reference form
works here too — whoctl only splits the first two segments off:

```console
$ whoctl get linux/apkrepository/https://dl-cdn.alpinelinux.org/alpine/v3.24/main
NAME                                               ENABLED   TAG
https://dl-cdn.alpinelinux.org/alpine/v3.24/main   true      -
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `enabled` | boolean | optional | Whether apk should use this repository. A disabled one is kept in the file, commented out, so the URL is not lost. |
| `tag` | string | optional | Optional tag, which makes the repository addressable as pkg@tag when installing. Example: `edge`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `url` | string | The repository URL, same as metadata.name. Example: `https://dl-cdn.alpinelinux.org/alpine/v3.22/main`. |
| `enabled` | boolean | Whether the line is active or commented out. |
| `tag` | string | The tag the repository is addressable by. Example: `edge`. |
| `file` | string | The file the entry was read from. Example: `/etc/apk/repositories`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `ENABLED` | always |
| `TAG` | always |
| `FILE` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Disabled means commented out, not deleted.** `enabled: false` rewrites the
line with a leading `#`, which is how apk users have always parked a repository
they may want back. `delete` is the operation that actually removes the URL.

**A commented-out URL is a disabled repository, not a comment.** The parser
tells the two apart by looking at what follows the `#`: something that starts
with a scheme or a `/` is a repository, and ordinary prose is left alone. Both
are preserved on rewrite either way.

**Position is kept.** apk queries repositories in file order, so a change is
written back in place rather than appended, and only a genuinely new repository
goes at the end.

**`spec.tag` is what makes `apk add package@tag` work.** Tagging the edge
repositories is the usual reason to add one to a stable Alpine, and pinning the
tag is what stops every package from being pulled from it.

**The file is rewritten by whoctl, not by apk**, because apk has no command that
edits it. As with `/etc/resolv.conf`, the write is atomic where the filesystem
allows it and in place where it does not, and `--dry-run` and `-v` report it the
same way they report a command.

See [ApkPackage](apkpackage.md) for the packages these repositories provide.
