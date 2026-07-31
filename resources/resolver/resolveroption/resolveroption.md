---
subcategory: Resolver
---

# ResolverOption

One entry of the `options` directive in `/etc/resolv.conf`. The object's name is
the option name and the value is separate, so `options ndots:2` is an object
named `ndots` whose `spec.value` is `2`.

## Example

```yaml
apiVersion: linux.whoctl.io/v1alpha1
kind: ResolverOption
metadata:
  name: ndots
spec:
  value: "2"
---
apiVersion: linux.whoctl.io/v1alpha1
kind: ResolverOption
metadata:
  name: rotate      # a flag option carries no value
```

```console
$ whoctl get linux/resolveroptions
NAME      VALUE
ndots     2
rotate    -
```

## Spec

<!-- whoctl:begin spec -->
| Field | Type | Notes | Description |
| --- | --- | --- | --- |
| `value` | string | optional | What follows the colon in the options line. Flag options such as rotate or edns0 carry none and leave it empty. Example: `2`. |
<!-- whoctl:end spec -->

## Status

<!-- whoctl:begin status -->
| Field | Type | Description |
| --- | --- | --- |
| `name` | string | The option name, same as metadata.name. |
| `value` | string | The value, empty for flag options. |
| `flag` | boolean | True for options that are merely present, with no value. |
| `file` | string | The file the entry was read from. Example: `/etc/resolv.conf`. |
| `managedBy` | string | The resolver daemon that owns the file, when there is one. |
<!-- whoctl:end status -->

## Columns

<!-- whoctl:begin columns -->
| Column | Shown |
| --- | --- |
| `NAME` | always |
| `VALUE` | always |
| `FILE` | `-o wide` |
| `MANAGED-BY` | `-o wide` |
<!-- whoctl:end columns -->

## Behaviour

**Options are unordered**, so there is no `priority` here. Applying an option
that already exists replaces its value in place instead of adding a duplicate,
which is what keeps `apply` idempotent for a directive where a repeated key
would be ambiguous.

**Values are strings.** `value: "2"` and `value: 2` both work in YAML, but
quoting is the honest spelling: what goes into the file is text either way.

**The file's other directives are left alone**, and a `/etc/resolv.conf` owned by
a resolver daemon is not written at all — see
[Nameserver](nameserver.html#behaviour) for both.
