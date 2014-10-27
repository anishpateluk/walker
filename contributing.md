# Contributing

We welcome contributions in the form of pull requests. They should:
- Have passing unit tests
- Be well documented and formatted with `go fmt`
- Match the general coding style of the project
- Provide features or bug fixes in keeping with Walker's design goals

# Tests

The basic tests can be run with:

```sh
cd $GOPATH/src/github.com/iParadigms/walker
go test ./test
```

But most Walker tests require dependencies to work and won't run by default. We have included some build tags to run these (see `script/test.sh` for the full `go test` command).

### Tag: sudo

The fetch manager tests, in order to test with URLs that do not have a port number, try to listen locally on port 80. This requires elevated privileges. These use the `sudo` build tag, and `script/test.sh` calls the tests using `sudo -E` to run them.

### Tag: cassandra

The datastore tests require a local Cassandra instance to be running. They automatically set up a `walker_test` keyspace for testing, so shouldn't interfere with existing data (nonetheless running tests with your production Cassandra instance is not a good idea). See [the README](README.md) for simple setup instructions.

# Logging

Logging can also be configured by having a `log4go.xml` file in the same location as `walker.yaml`. See `walker/log4go.xml.sample` for an example.
