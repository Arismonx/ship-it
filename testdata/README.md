# testdata

`golden/` holds the expected outputs that tests compare against: the review screens, the MCP client files, and more. Rewrite them with `go test ./... -update` after an intended change, then read the diff before committing.

End-to-end scenarios live next to the binary, in `cmd/ship/*_test.go`: each one runs the real `ship` against a real git repository, with a local bare repository as its remote and a fake GitHub server where it publishes. Fuzz seed corpora live in the `testdata/fuzz/` folder of the package they belong to.
