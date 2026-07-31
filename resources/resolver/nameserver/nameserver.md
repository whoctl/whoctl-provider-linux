---
subcategory: Resolver
---

# Nameserver

One DNS resolver address in `/etc/resolv.conf`. The object's name is the address
itself, and its only field is where it sits in the list.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: Nameserver
metadata:
  name: 1.1.1.1
spec:
  priority: 1
```

```console
$ whoctl get linux/nameservers
NAME              PRIORITY   FAMILY   EFFECTIVE
1.1.1.1           1          IPv4     true
192.168.1.1       2          IPv4     true
9.9.9.9           3          IPv4     true
8.8.8.8           4          IPv4     false
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `priority` | integer | optional | 1-based position in /etc/resolv.conf. Resolvers are tried in order, so it decides which one answers first. Omitted, an existing entry keeps its place and a new one is appended. Example: `1`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `address` | string | The resolver address, same as metadata.name. |
| `family` | string | IPv4 or IPv6. |
| `priority` | integer | 1-based position among the nameserver lines. |
| `effective` | boolean | False for entries past MAXNS, which the C library never reaches. |
| `file` | string | The file the entry was read from. Example: `/etc/resolv.conf`. |
| `managedBy` | string | The resolver daemon that owns the file, when there is one. Example: `systemd-resolved`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `PRIORITY` | always |
| `FAMILY` | always |
| `EFFECTIVE` | always |
| `FILE` | `-o wide` |
| `MANAGED-BY` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Order is the whole point.** Resolvers are tried in order and the C library
only ever consults the first three (`MAXNS`), so `status.effective` says outright
which entries the system will never reach. Applying a `priority` moves an
existing entry; omitting it appends a new one at the end and leaves an existing
one where it is.

**Each directive has exactly one owner.** `Nameserver` owns the `nameserver`
lines, [SearchDomain](searchdomain.html) owns `search` and
[ResolverOption](resolveroption.html) owns `options`. There is deliberately no
single object covering the whole file: two kinds writing the same directive
would mean the second one silently undoing the first.

**Everything else in the file is preserved.** Comments, blank lines, `domain`
and `sortlist` survive a rewrite untouched, and each block stays where it was.

**A file with an owner is not written.** When `/etc/resolv.conf` is a
systemd-resolved symlink, or carries a NetworkManager header, whoctl refuses the
write and names the daemon in `status.managedBy`, because the change would be
reverted anyway.
