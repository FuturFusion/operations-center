# BIOS Profiles

## Example

An example of a BIOS profile, showing all the fields available for matching and
for the values to apply, can be found in
[`_dummy.yaml.example`](_dummy.yaml.example).

## Tests

If new profiles are added, it is advised to also update
[`../profiles_test.go`](../profiles_test.go) with additional tests for the
manufacturers, models and hardware combinations covered by the new profiles, to
make sure the profiles provide the expected outcome.
