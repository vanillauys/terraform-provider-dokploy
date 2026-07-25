//go:build tools

// Package tools documents CLI tool dependencies pinned for this module.
//
// tfplugindocs cannot be pinned with the classic blank-import trick here:
// github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs is `package
// main`, and Go refuses to import a main package from another package
// ("is a program, not an importable package"). Instead it is pinned via
// go.mod's native `tool` directive (Go 1.24+):
//
//	go get -tool github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
//
// which records both the module requirement and a `tool` line in go.mod.
// `make docs` invokes it with `go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs`.
package tools
