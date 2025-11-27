# Filtering

Operations Center uses [expr-lang](https://expr-lang.org/) for filtering in
various places, including filtering of results fetched from the inventory
as well as selection of updates and update files.

See [Inventory](inventory) for more information about filtering inventory
results.
See [Update](update) for more information about update configuration.

Expr is an expression language that uses a Go-like syntax with some more
human-readable operators.

```{note}
For a full list of available functions and examples for more advanced expressions,
see the full language definition at https://expr-lang.org/docs/language-definition.
```

## Common Examples

### Filtering Results from Inventory

| Expression                                          | Entity  | Description                                                                          |
| :---                                                | :---    | :---                                                                                 |
| `name != "incusbr0" and name != "meshbr0"`          | network | Get all network except the automatically created ones named `incusbr0` and `meshbr0` |
| `object.config["ipv4.address"] == "10.115.25.1/24"` | network | Get all networks that have the IPv4 address `10.115.25.1/24` configured              |
| `"/1.0/profiles/default" in object.used_by`         | network | Get all networks that are used by the `default` profile                              |

```{note}
Field names of the entities in the inventory are the same as the `properties`
field of the JSON or YAML representation shown over the API or in the
`operations-center inventory <entity> list -f json` command, where `<entity>` is
one of `image`, `instance`, `network`, etc. Use `operations-center inventory -h`
to see the available entities.

Tip: to see the available fields of updates, run:
`operations-center inventory <entity> list -f json | jq -r '. | first | keys | sort | .[]'`
```

### Filtering Updates and Update Files

If a filter is defined, the filter needs to evaluate to `true` for the update
being fetched by Operations Center and `false` otherwise. A filter expression
resulting in an non boolean value is considered an error.
The empty filter expression does not filter at all, same effect as `true`.

| Expression                        | Description                                               |
| :---                              | :---                                                      |
| `"stable" in channels`            | Only download updates that are in the `stable` channel    |
| `AppliesToArchitecture("x86_64")` | Only download updates that apply to `x86_64` architecture |

```{note}
Operations Center has the following extended functions for filtering update files:

* `AppliesToArchitecture(string)` -- returns `true`, if the file applies to the
  specified architecture or if the file is architecture neutral.
```

```{note}
Field names of updates are the same as the `properties` field of the JSON or
YAML representation shown over the API or in the
`operations-center provisioning update list -f json` command.

Tip: to see the available fields of updates, run:
`operations-center provisioning update list -f json | jq -r '. | first | keys | sort | .[]'`
```
