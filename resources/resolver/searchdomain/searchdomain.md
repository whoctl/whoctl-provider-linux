---
subcategory: Resolver
---

# SearchDomain

One entry of the `search` directive in `/etc/resolv.conf`, the list of domains
an unqualified name is tried against. The object's name is the domain.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: SearchDomain
metadata:
  name: lab.internal
spec:
  priority: 1
```

```console
$ whoctl get linux/searchdomains
NAME           PRIORITY   EFFECTIVE
lab.internal   1          true
home.arpa      2          true
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `priority` | integer | optional | 1-based position in the search list. Unqualified names are tried against each domain in order, so it decides which one wins. Omitted, an existing entry keeps its place and a new one is appended. Example: `1`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `domain` | string | The domain, same as metadata.name. |
| `priority` | integer | 1-based position in the search list. |
| `effective` | boolean | False for entries past the resolver's limit, which are never consulted. |
| `file` | string | The file the entry was read from. Example: `/etc/resolv.conf`. |
| `managedBy` | string | The resolver daemon that owns the file, when there is one. Example: `NetworkManager`. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `PRIORITY` | always |
| `EFFECTIVE` | always |
| `FILE` | `-o wide` |
| `MANAGED-BY` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**One line, many entries.** A search list lives on a single `search` line, so
however many objects there are, a rewrite keeps them on one line rather than
emitting one per domain. Removing the last entry removes the directive entirely.

**Priority works exactly as it does for [Nameserver](nameserver.html)**: apply
one to move an entry, omit it to append. Domains past the sixth are reported as
not effective, which is glibc's `MAXDNSRCH`; musl bounds the list by total
length instead, so treat the flag as the conservative answer.

**The file's other directives are left alone**, and a `/etc/resolv.conf` owned by
a resolver daemon is not written at all — see
[Nameserver](nameserver.html#behaviour) for both.
