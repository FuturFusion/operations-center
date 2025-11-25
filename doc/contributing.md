% Include content from [../CONTRIBUTING.md](../CONTRIBUTING.md)
```{include} ../CONTRIBUTING.md
```

## Building locally

Building Operations Center locally requires a recent version of the Go toolchain.
The UI additionally requires `yarn`.

After cloning the repository from GitHub, simply run:

    make

This will build the daemon and the multi architecture CLI.

To build the UI, run:

    make build-ui

Then, simply run `bin/operations-centerd` to start the daemon, and `bin/operations-center` for the CLI.

## Testing

To run the test suite, run:

    make test

## Static analysis

To run the static analysis tools, run:

    make static-analysis

or for just linting:

    make lint
