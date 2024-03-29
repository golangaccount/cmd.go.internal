// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package fix implements the ``go fix'' command.
package fix

import (
	"github.com/golangaccount/cmd.go.internal/base"
	"github.com/golangaccount/cmd.go.internal/cfg"
	"github.com/golangaccount/cmd.go.internal/load"
	"github.com/golangaccount/cmd.go.internal/str"
)

var CmdFix = &base.Command{
	Run:       runFix,
	UsageLine: "fix [packages]",
	Short:     "run go tool fix on packages",
	Long: `
Fix runs the Go fix command on the packages named by the import paths.

For more about fix, see 'go doc cmd/fix'.
For more about specifying packages, see 'go help packages'.

To run fix with specific options, run 'go tool fix'.

See also: go fmt, go vet.
	`,
}

func runFix(cmd *base.Command, args []string) {
	for _, pkg := range load.Packages(args) {
		// Use pkg.gofiles instead of pkg.Dir so that
		// the command only applies to this package,
		// not to packages in subdirectories.
		files := base.FilterDotUnderscoreFiles(base.RelPaths(pkg.Internal.AllGoFiles))
		base.Run(str.StringList(cfg.BuildToolexec, base.Tool("fix"), files))
	}
}
