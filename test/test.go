// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golangaccount/cmd.go.internal/base"
	"github.com/golangaccount/cmd.go.internal/cfg"
	"github.com/golangaccount/cmd.go.internal/load"
	"github.com/golangaccount/cmd.go.internal/str"
	"github.com/golangaccount/cmd.go.internal/work"
)

// Break init loop.
func init() {
	CmdTest.Run = runTest
}

const testUsage = "test [build/test flags] [packages] [build/test flags & test binary flags]"

var CmdTest = &base.Command{
	CustomFlags: true,
	UsageLine:   testUsage,
	Short:       "test packages",
	Long: `
'Go test' automates testing the packages named by the import paths.
It prints a summary of the test results in the format:

	ok   archive/tar   0.011s
	FAIL archive/zip   0.022s
	ok   compress/gzip 0.033s
	...

followed by detailed output for each failed package.

'Go test' recompiles each package along with any files with names matching
the file pattern "*_test.go".
Files whose names begin with "_" (including "_test.go") or "." are ignored.
These additional files can contain test functions, benchmark functions, and
example functions. See 'go help testfunc' for more.
Each listed package causes the execution of a separate test binary.

Test files that declare a package with the suffix "_test" will be compiled as a
separate package, and then linked and run with the main test binary.

The go tool will ignore a directory named "testdata", making it available
to hold ancillary data needed by the tests.

By default, go test needs no arguments. It compiles and tests the package
with source in the current directory, including tests, and runs the tests.

The package is built in a temporary directory so it does not interfere with the
non-test installation.

` + strings.TrimSpace(testFlag1) + ` See 'go help testflag' for details.

For more about build flags, see 'go help build'.
For more about specifying packages, see 'go help packages'.

See also: go build, go vet.
`,
}

const testFlag1 = `
In addition to the build flags, the flags handled by 'go test' itself are:

	-args
	    Pass the remainder of the command line (everything after -args)
	    to the test binary, uninterpreted and unchanged.
	    Because this flag consumes the remainder of the command line,
	    the package list (if present) must appear before this flag.

	-c
	    Compile the test binary to pkg.test but do not run it
	    (where pkg is the last element of the package's import path).
	    The file name can be changed with the -o flag.

	-exec xprog
	    Run the test binary using xprog. The behavior is the same as
	    in 'go run'. See 'go help run' for details.

	-i
	    Install packages that are dependencies of the test.
	    Do not run the test.

	-o file
	    Compile the test binary to the named file.
	    The test still runs (unless -c or -i is specified).

The test binary also accepts flags that control execution of the test; these
flags are also accessible by 'go test'.
`

// Usage prints the usage message for 'go test -h' and exits.
func Usage() {
	os.Stderr.WriteString(testUsage + "\n\n" +
		strings.TrimSpace(testFlag1) + "\n\n\t" +
		strings.TrimSpace(testFlag2) + "\n")
	os.Exit(2)
}

var HelpTestflag = &base.Command{
	UsageLine: "testflag",
	Short:     "description of testing flags",
	Long: `
The 'go test' command takes both flags that apply to 'go test' itself
and flags that apply to the resulting test binary.

Several of the flags control profiling and write an execution profile
suitable for "go tool pprof"; run "go tool pprof -h" for more
information. The --alloc_space, --alloc_objects, and --show_bytes
options of pprof control how the information is presented.

The following flags are recognized by the 'go test' command and
control the execution of any test:

	` + strings.TrimSpace(testFlag2) + `
`,
}

const testFlag2 = `
	-bench regexp
	    Run only those benchmarks matching a regular expression.
	    By default, no benchmarks are run. 
	    To run all benchmarks, use '-bench .' or '-bench=.'.
	    The regular expression is split by unbracketed slash (/)
	    characters into a sequence of regular expressions, and each
	    part of a benchmark's identifier must match the corresponding
	    element in the sequence, if any. Possible parents of matches
	    are run with b.N=1 to identify sub-benchmarks. For example,
	    given -bench=X/Y, top-level benchmarks matching X are run
	    with b.N=1 to find any sub-benchmarks matching Y, which are
	    then run in full.

	-benchtime t
	    Run enough iterations of each benchmark to take t, specified
	    as a time.Duration (for example, -benchtime 1h30s).
	    The default is 1 second (1s).

	-count n
	    Run each test and benchmark n times (default 1).
	    If -cpu is set, run n times for each GOMAXPROCS value.
	    Examples are always run once.

	-cover
	    Enable coverage analysis.
	    Note that because coverage works by annotating the source
	    code before compilation, compilation and test failures with
	    coverage enabled may report line numbers that don't correspond
	    to the original sources.

	-covermode set,count,atomic
	    Set the mode for coverage analysis for the package[s]
	    being tested. The default is "set" unless -race is enabled,
	    in which case it is "atomic".
	    The values:
		set: bool: does this statement run?
		count: int: how many times does this statement run?
		atomic: int: count, but correct in multithreaded tests;
			significantly more expensive.
	    Sets -cover.

	-coverpkg pkg1,pkg2,pkg3
	    Apply coverage analysis in each test to the given list of packages.
	    The default is for each test to analyze only the package being tested.
	    Packages are specified as import paths.
	    Sets -cover.

	-cpu 1,2,4
	    Specify a list of GOMAXPROCS values for which the tests or
	    benchmarks should be executed. The default is the current value
	    of GOMAXPROCS.

	-list regexp
	    List tests, benchmarks, or examples matching the regular expression.
	    No tests, benchmarks or examples will be run. This will only
	    list top-level tests. No subtest or subbenchmarks will be shown.

	-parallel n
	    Allow parallel execution of test functions that call t.Parallel.
	    The value of this flag is the maximum number of tests to run
	    simultaneously; by default, it is set to the value of GOMAXPROCS.
	    Note that -parallel only applies within a single test binary.
	    The 'go test' command may run tests for different packages
	    in parallel as well, according to the setting of the -p flag
	    (see 'go help build').

	-run regexp
	    Run only those tests and examples matching the regular expression.
	    For tests, the regular expression is split by unbracketed slash (/)
	    characters into a sequence of regular expressions, and each part
	    of a test's identifier must match the corresponding element in
	    the sequence, if any. Note that possible parents of matches are
	    run too, so that -run=X/Y matches and runs and reports the result
	    of all tests matching X, even those without sub-tests matching Y,
	    because it must run them to look for those sub-tests.

	-short
	    Tell long-running tests to shorten their run time.
	    It is off by default but set during all.bash so that installing
	    the Go tree can run a sanity check but not spend time running
	    exhaustive tests.

	-timeout d
	    If a test binary runs longer than duration d, panic.
	    The default is 10 minutes (10m).

	-v
	    Verbose output: log all tests as they are run. Also print all
	    text from Log and Logf calls even if the test succeeds.

The following flags are also recognized by 'go test' and can be used to
profile the tests during execution:

	-benchmem
	    Print memory allocation statistics for benchmarks.

	-blockprofile block.out
	    Write a goroutine blocking profile to the specified file
	    when all tests are complete.
	    Writes test binary as -c would.

	-blockprofilerate n
	    Control the detail provided in goroutine blocking profiles by
	    calling runtime.SetBlockProfileRate with n.
	    See 'go doc runtime.SetBlockProfileRate'.
	    The profiler aims to sample, on average, one blocking event every
	    n nanoseconds the program spends blocked. By default,
	    if -test.blockprofile is set without this flag, all blocking events
	    are recorded, equivalent to -test.blockprofilerate=1.

	-coverprofile cover.out
	    Write a coverage profile to the file after all tests have passed.
	    Sets -cover.

	-cpuprofile cpu.out
	    Write a CPU profile to the specified file before exiting.
	    Writes test binary as -c would.

	-memprofile mem.out
	    Write a memory profile to the file after all tests have passed.
	    Writes test binary as -c would.

	-memprofilerate n
	    Enable more precise (and expensive) memory profiles by setting
	    runtime.MemProfileRate. See 'go doc runtime.MemProfileRate'.
	    To profile all memory allocations, use -test.memprofilerate=1
	    and pass --alloc_space flag to the pprof tool.

	-mutexprofile mutex.out
	    Write a mutex contention profile to the specified file
	    when all tests are complete.
	    Writes test binary as -c would.

	-mutexprofilefraction n
	    Sample 1 in n stack traces of goroutines holding a
	    contended mutex.

	-outputdir directory
	    Place output files from profiling in the specified directory,
	    by default the directory in which "go test" is running.

	-trace trace.out
	    Write an execution trace to the specified file before exiting.

Each of these flags is also recognized with an optional 'test.' prefix,
as in -test.v. When invoking the generated test binary (the result of
'go test -c') directly, however, the prefix is mandatory.

The 'go test' command rewrites or removes recognized flags,
as appropriate, both before and after the optional package list,
before invoking the test binary.

For instance, the command

	go test -v -myflag testdata -cpuprofile=prof.out -x

will compile the test binary and then run it as

	pkg.test -test.v -myflag testdata -test.cpuprofile=prof.out

(The -x flag is removed because it applies only to the go command's
execution, not to the test itself.)

The test flags that generate profiles (other than for coverage) also
leave the test binary in pkg.test for use when analyzing the profiles.

When 'go test' runs a test binary, it does so from within the
corresponding package's source code directory. Depending on the test,
it may be necessary to do the same when invoking a generated test
binary directly.

The command-line package list, if present, must appear before any
flag not known to the go test command. Continuing the example above,
the package list would have to appear before -myflag, but could appear
on either side of -v.

To keep an argument for a test binary from being interpreted as a
known flag or a package name, use -args (see 'go help test') which
passes the remainder of the command line through to the test binary
uninterpreted and unaltered.

For instance, the command

	go test -v -args -x -v

will compile the test binary and then run it as

	pkg.test -test.v -x -v

Similarly,

	go test -args math

will compile the test binary and then run it as

	pkg.test math

In the first example, the -x and the second -v are passed through to the
test binary unchanged and with no effect on the go command itself.
In the second example, the argument math is passed through to the test
binary, instead of being interpreted as the package list.
`

var HelpTestfunc = &base.Command{
	UsageLine: "testfunc",
	Short:     "description of testing functions",
	Long: `
The 'go test' command expects to find test, benchmark, and example functions
in the "*_test.go" files corresponding to the package under test.

A test function is one named TestXXX (where XXX is any alphanumeric string
not starting with a lower case letter) and should have the signature,

	func TestXXX(t *testing.T) { ... }

A benchmark function is one named BenchmarkXXX and should have the signature,

	func BenchmarkXXX(b *testing.B) { ... }

An example function is similar to a test function but, instead of using
*testing.T to report success or failure, prints output to os.Stdout.
If the last comment in the function starts with "Output:" then the output
is compared exactly against the comment (see examples below). If the last
comment begins with "Unordered output:" then the output is compared to the
comment, however the order of the lines is ignored. An example with no such
comment is compiled but not executed. An example with no text after
"Output:" is compiled, executed, and expected to produce no output.

Godoc displays the body of ExampleXXX to demonstrate the use
of the function, constant, or variable XXX. An example of a method M with
receiver type T or *T is named ExampleT_M. There may be multiple examples
for a given function, constant, or variable, distinguished by a trailing _xxx,
where xxx is a suffix not beginning with an upper case letter.

Here is an example of an example:

	func ExamplePrintln() {
		Println("The output of\nthis example.")
		// Output: The output of
		// this example.
	}

Here is another example where the ordering of the output is ignored:

	func ExamplePerm() {
		for _, value := range Perm(4) {
			fmt.Println(value)
		}

		// Unordered output: 4
		// 2
		// 1
		// 3
		// 0
	}

The entire test file is presented as the example when it contains a single
example function, at least one other function, type, variable, or constant
declaration, and no test or benchmark functions.

See the documentation of the testing package for more information.
`,
}

var (
	testC            bool            // -c flag
	testCover        bool            // -cover flag
	testCoverMode    string          // -covermode flag
	testCoverPaths   []string        // -coverpkg flag
	testCoverPkgs    []*load.Package // -coverpkg flag
	testO            string          // -o flag
	testProfile      bool            // some profiling flag
	testNeedBinary   bool            // profile needs to keep binary around
	testV            bool            // -v flag
	testTimeout      string          // -timeout flag
	testArgs         []string
	testBench        bool
	testList         bool
	testStreamOutput bool // show output as it is generated
	testShowPass     bool // show passing output

	testKillTimeout = 10 * time.Minute
)

var testMainDeps = map[string]bool{
	// Dependencies for testmain.
	"testing":                   true,
	"testing/internal/testdeps": true,
	"os": true,
}

func runTest(cmd *base.Command, args []string) {
	var pkgArgs []string
	pkgArgs, testArgs = testFlags(args)

	work.FindExecCmd() // initialize cached result

	work.InstrumentInit()
	work.BuildModeInit()
	pkgs := load.PackagesForBuild(pkgArgs)
	if len(pkgs) == 0 {
		base.Fatalf("no packages to test")
	}

	if testC && len(pkgs) != 1 {
		base.Fatalf("cannot use -c flag with multiple packages")
	}
	if testO != "" && len(pkgs) != 1 {
		base.Fatalf("cannot use -o flag with multiple packages")
	}
	if testProfile && len(pkgs) != 1 {
		base.Fatalf("cannot use test profile flag with multiple packages")
	}

	// If a test timeout was given and is parseable, set our kill timeout
	// to that timeout plus one minute. This is a backup alarm in case
	// the test wedges with a goroutine spinning and its background
	// timer does not get a chance to fire.
	if dt, err := time.ParseDuration(testTimeout); err == nil && dt > 0 {
		testKillTimeout = dt + 1*time.Minute
	}

	// show passing test output (after buffering) with -v flag.
	// must buffer because tests are running in parallel, and
	// otherwise the output will get mixed.
	testShowPass = testV || testList

	// stream test output (no buffering) when no package has
	// been given on the command line (implicit current directory)
	// or when benchmarking.
	// Also stream if we're showing output anyway with a
	// single package under test or if parallelism is set to 1.
	// In these cases, streaming the output produces the same result
	// as not streaming, just more immediately.
	testStreamOutput = len(pkgArgs) == 0 || testBench ||
		(testShowPass && (len(pkgs) == 1 || cfg.BuildP == 1))

	// For 'go test -i -o x.test', we want to build x.test. Imply -c to make the logic easier.
	if cfg.BuildI && testO != "" {
		testC = true
	}

	var b work.Builder
	b.Init()

	if cfg.BuildI {
		cfg.BuildV = testV

		deps := make(map[string]bool)
		for dep := range testMainDeps {
			deps[dep] = true
		}

		for _, p := range pkgs {
			// Dependencies for each test.
			for _, path := range p.Imports {
				deps[path] = true
			}
			for _, path := range p.Vendored(p.TestImports) {
				deps[path] = true
			}
			for _, path := range p.Vendored(p.XTestImports) {
				deps[path] = true
			}
		}

		// translate C to runtime/cgo
		if deps["C"] {
			delete(deps, "C")
			deps["runtime/cgo"] = true
			if cfg.Goos == runtime.GOOS && cfg.Goarch == runtime.GOARCH && !cfg.BuildRace && !cfg.BuildMSan {
				deps["cmd/cgo"] = true
			}
		}
		// Ignore pseudo-packages.
		delete(deps, "unsafe")

		all := []string{}
		for path := range deps {
			if !build.IsLocalImport(path) {
				all = append(all, path)
			}
		}
		sort.Strings(all)

		a := &work.Action{}
		for _, p := range load.PackagesForBuild(all) {
			a.Deps = append(a.Deps, b.Action(work.ModeInstall, work.ModeInstall, p))
		}
		b.Do(a)
		if !testC || a.Failed {
			return
		}
		b.Init()
	}

	var builds, runs, prints []*work.Action

	if testCoverPaths != nil {
		// Load packages that were asked about for coverage.
		// packagesForBuild exits if the packages cannot be loaded.
		testCoverPkgs = load.PackagesForBuild(testCoverPaths)

		// Warn about -coverpkg arguments that are not actually used.
		used := make(map[string]bool)
		for _, p := range pkgs {
			used[p.ImportPath] = true
			for _, dep := range p.Deps {
				used[dep] = true
			}
		}
		for _, p := range testCoverPkgs {
			if !used[p.ImportPath] {
				fmt.Fprintf(os.Stderr, "warning: no packages being tested depend on %s\n", p.ImportPath)
			}
		}

		// Mark all the coverage packages for rebuilding with coverage.
		for _, p := range testCoverPkgs {
			// There is nothing to cover in package unsafe; it comes from the compiler.
			if p.ImportPath == "unsafe" {
				continue
			}
			p.Stale = true // rebuild
			p.StaleReason = "rebuild for coverage"
			p.Internal.Fake = true // do not warn about rebuild
			p.Internal.CoverMode = testCoverMode
			var coverFiles []string
			coverFiles = append(coverFiles, p.GoFiles...)
			coverFiles = append(coverFiles, p.CgoFiles...)
			coverFiles = append(coverFiles, p.TestGoFiles...)
			p.Internal.CoverVars = declareCoverVars(p.ImportPath, coverFiles...)
		}
	}

	// Prepare build + run + print actions for all packages being tested.
	for _, p := range pkgs {
		// sync/atomic import is inserted by the cover tool. See #18486
		if testCover && testCoverMode == "atomic" {
			ensureImport(p, "sync/atomic")
		}

		buildTest, runTest, printTest, err := builderTest(&b, p)
		if err != nil {
			str := err.Error()
			if strings.HasPrefix(str, "\n") {
				str = str[1:]
			}
			failed := fmt.Sprintf("FAIL\t%s [setup failed]\n", p.ImportPath)

			if p.ImportPath != "" {
				base.Errorf("# %s\n%s\n%s", p.ImportPath, str, failed)
			} else {
				base.Errorf("%s\n%s", str, failed)
			}
			continue
		}
		builds = append(builds, buildTest)
		runs = append(runs, runTest)
		prints = append(prints, printTest)
	}

	// Ultimately the goal is to print the output.
	root := &work.Action{Deps: prints}

	// Force the printing of results to happen in order,
	// one at a time.
	for i, a := range prints {
		if i > 0 {
			a.Deps = append(a.Deps, prints[i-1])
		}
	}

	// Force benchmarks to run in serial.
	if !testC && testBench {
		// The first run must wait for all builds.
		// Later runs must wait for the previous run's print.
		for i, run := range runs {
			if i == 0 {
				run.Deps = append(run.Deps, builds...)
			} else {
				run.Deps = append(run.Deps, prints[i-1])
			}
		}
	}

	// If we are building any out-of-date packages other
	// than those under test, warn.
	okBuild := map[*load.Package]bool{}
	for _, p := range pkgs {
		okBuild[p] = true
	}
	warned := false
	for _, a := range work.ActionList(root) {
		if a.Package == nil || okBuild[a.Package] {
			continue
		}
		okBuild[a.Package] = true // warn at most once

		// Don't warn about packages being rebuilt because of
		// things like coverage analysis.
		for _, p1 := range a.Package.Internal.Imports {
			if p1.Internal.Fake {
				a.Package.Internal.Fake = true
			}
		}

		if a.Func != nil && !okBuild[a.Package] && !a.Package.Internal.Fake && !a.Package.Internal.Local {
			if !warned {
				fmt.Fprintf(os.Stderr, "warning: building out-of-date packages:\n")
				warned = true
			}
			fmt.Fprintf(os.Stderr, "\t%s\n", a.Package.ImportPath)
		}
	}
	if warned {
		args := strings.Join(pkgArgs, " ")
		if args != "" {
			args = " " + args
		}
		extraOpts := ""
		if cfg.BuildRace {
			extraOpts = "-race "
		}
		if cfg.BuildMSan {
			extraOpts = "-msan "
		}
		fmt.Fprintf(os.Stderr, "installing these packages with 'go test %s-i%s' will speed future tests.\n\n", extraOpts, args)
	}

	b.Do(root)
}

// ensures that package p imports the named package
func ensureImport(p *load.Package, pkg string) {
	for _, d := range p.Internal.Deps {
		if d.Name == pkg {
			return
		}
	}

	a := load.LoadPackage(pkg, &load.ImportStack{})
	if a.Error != nil {
		base.Fatalf("load %s: %v", pkg, a.Error)
	}
	load.ComputeStale(a)

	p.Internal.Imports = append(p.Internal.Imports, a)
}

var windowsBadWords = []string{
	"install",
	"patch",
	"setup",
	"update",
}

func builderTest(b *work.Builder, p *load.Package) (buildAction, runAction, printAction *work.Action, err error) {
	if len(p.TestGoFiles)+len(p.XTestGoFiles) == 0 {
		build := b.Action(work.ModeBuild, work.ModeBuild, p)
		run := &work.Action{Package: p, Deps: []*work.Action{build}}
		print := &work.Action{Func: builderNoTest, Package: p, Deps: []*work.Action{run}}
		return build, run, print, nil
	}

	// Build Package structs describing:
	//	ptest - package + test files
	//	pxtest - package of external test files
	//	pmain - pkg.test binary
	var ptest, pxtest, pmain *load.Package

	var imports, ximports []*load.Package
	var stk load.ImportStack
	stk.Push(p.ImportPath + " (test)")
	for i, path := range p.TestImports {
		p1 := load.LoadImport(path, p.Dir, p, &stk, p.Internal.Build.TestImportPos[path], load.UseVendor)
		if p1.Error != nil {
			return nil, nil, nil, p1.Error
		}
		if len(p1.DepsErrors) > 0 {
			err := p1.DepsErrors[0]
			err.Pos = "" // show full import stack
			return nil, nil, nil, err
		}
		if str.Contains(p1.Deps, p.ImportPath) || p1.ImportPath == p.ImportPath {
			// Same error that loadPackage returns (via reusePackage) in pkg.go.
			// Can't change that code, because that code is only for loading the
			// non-test copy of a package.
			err := &load.PackageError{
				ImportStack:   testImportStack(stk[0], p1, p.ImportPath),
				Err:           "import cycle not allowed in test",
				IsImportCycle: true,
			}
			return nil, nil, nil, err
		}
		p.TestImports[i] = p1.ImportPath
		imports = append(imports, p1)
	}
	stk.Pop()
	stk.Push(p.ImportPath + "_test")
	pxtestNeedsPtest := false
	for i, path := range p.XTestImports {
		p1 := load.LoadImport(path, p.Dir, p, &stk, p.Internal.Build.XTestImportPos[path], load.UseVendor)
		if p1.Error != nil {
			return nil, nil, nil, p1.Error
		}
		if len(p1.DepsErrors) > 0 {
			err := p1.DepsErrors[0]
			err.Pos = "" // show full import stack
			return nil, nil, nil, err
		}
		if p1.ImportPath == p.ImportPath {
			pxtestNeedsPtest = true
		} else {
			ximports = append(ximports, p1)
		}
		p.XTestImports[i] = p1.ImportPath
	}
	stk.Pop()

	// Use last element of import path, not package name.
	// They differ when package name is "main".
	// But if the import path is "command-line-arguments",
	// like it is during 'go run', use the package name.
	var elem string
	if p.ImportPath == "command-line-arguments" {
		elem = p.Name
	} else {
		_, elem = path.Split(p.ImportPath)
	}
	testBinary := elem + ".test"

	// The ptest package needs to be importable under the
	// same import path that p has, but we cannot put it in
	// the usual place in the temporary tree, because then
	// other tests will see it as the real package.
	// Instead we make a _test directory under the import path
	// and then repeat the import path there. We tell the
	// compiler and linker to look in that _test directory first.
	//
	// That is, if the package under test is unicode/utf8,
	// then the normal place to write the package archive is
	// $WORK/unicode/utf8.a, but we write the test package archive to
	// $WORK/unicode/utf8/_test/unicode/utf8.a.
	// We write the external test package archive to
	// $WORK/unicode/utf8/_test/unicode/utf8_test.a.
	testDir := filepath.Join(b.WorkDir, filepath.FromSlash(p.ImportPath+"/_test"))
	ptestObj := work.BuildToolchain.Pkgpath(testDir, p)

	// Create the directory for the .a files.
	ptestDir, _ := filepath.Split(ptestObj)
	if err := b.Mkdir(ptestDir); err != nil {
		return nil, nil, nil, err
	}

	// Should we apply coverage analysis locally,
	// only for this package and only for this test?
	// Yes, if -cover is on but -coverpkg has not specified
	// a list of packages for global coverage.
	localCover := testCover && testCoverPaths == nil

	// Test package.
	if len(p.TestGoFiles) > 0 || localCover || p.Name == "main" {
		ptest = new(load.Package)
		*ptest = *p
		ptest.GoFiles = nil
		ptest.GoFiles = append(ptest.GoFiles, p.GoFiles...)
		ptest.GoFiles = append(ptest.GoFiles, p.TestGoFiles...)
		ptest.Internal.Target = ""
		ptest.Imports = str.StringList(p.Imports, p.TestImports)
		ptest.Internal.Imports = append(append([]*load.Package{}, p.Internal.Imports...), imports...)
		ptest.Internal.Pkgdir = testDir
		ptest.Internal.Fake = true
		ptest.Internal.ForceLibrary = true
		ptest.Stale = true
		ptest.StaleReason = "rebuild for test"
		ptest.Internal.Build = new(build.Package)
		*ptest.Internal.Build = *p.Internal.Build
		m := map[string][]token.Position{}
		for k, v := range p.Internal.Build.ImportPos {
			m[k] = append(m[k], v...)
		}
		for k, v := range p.Internal.Build.TestImportPos {
			m[k] = append(m[k], v...)
		}
		ptest.Internal.Build.ImportPos = m

		if localCover {
			ptest.Internal.CoverMode = testCoverMode
			var coverFiles []string
			coverFiles = append(coverFiles, ptest.GoFiles...)
			coverFiles = append(coverFiles, ptest.CgoFiles...)
			ptest.Internal.CoverVars = declareCoverVars(ptest.ImportPath, coverFiles...)
		}
	} else {
		ptest = p
	}

	// External test package.
	if len(p.XTestGoFiles) > 0 {
		pxtest = &load.Package{
			PackagePublic: load.PackagePublic{
				Name:       p.Name + "_test",
				ImportPath: p.ImportPath + "_test",
				Root:       p.Root,
				Dir:        p.Dir,
				GoFiles:    p.XTestGoFiles,
				Imports:    p.XTestImports,
				Stale:      true,
			},
			Internal: load.PackageInternal{
				LocalPrefix: p.Internal.LocalPrefix,
				Build: &build.Package{
					ImportPos: p.Internal.Build.XTestImportPos,
				},
				Imports:  ximports,
				Pkgdir:   testDir,
				Fake:     true,
				External: true,
			},
		}
		if pxtestNeedsPtest {
			pxtest.Internal.Imports = append(pxtest.Internal.Imports, ptest)
		}
	}

	// Action for building pkg.test.
	pmain = &load.Package{
		PackagePublic: load.PackagePublic{
			Name:       "main",
			Dir:        testDir,
			GoFiles:    []string{"_testmain.go"},
			ImportPath: "testmain",
			Root:       p.Root,
			Stale:      true,
		},
		Internal: load.PackageInternal{
			Build:     &build.Package{Name: "main"},
			Pkgdir:    testDir,
			Fake:      true,
			OmitDebug: !testC && !testNeedBinary,
		},
	}

	// The generated main also imports testing, regexp, and os.
	stk.Push("testmain")
	for dep := range testMainDeps {
		if dep == ptest.ImportPath {
			pmain.Internal.Imports = append(pmain.Internal.Imports, ptest)
		} else {
			p1 := load.LoadImport(dep, "", nil, &stk, nil, 0)
			if p1.Error != nil {
				return nil, nil, nil, p1.Error
			}
			pmain.Internal.Imports = append(pmain.Internal.Imports, p1)
		}
	}

	if testCoverPkgs != nil {
		// Add imports, but avoid duplicates.
		seen := map[*load.Package]bool{p: true, ptest: true}
		for _, p1 := range pmain.Internal.Imports {
			seen[p1] = true
		}
		for _, p1 := range testCoverPkgs {
			if !seen[p1] {
				seen[p1] = true
				pmain.Internal.Imports = append(pmain.Internal.Imports, p1)
			}
		}
	}

	// Do initial scan for metadata needed for writing _testmain.go
	// Use that metadata to update the list of imports for package main.
	// The list of imports is used by recompileForTest and by the loop
	// afterward that gathers t.Cover information.
	t, err := loadTestFuncs(ptest)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(ptest.GoFiles)+len(ptest.CgoFiles) > 0 {
		pmain.Internal.Imports = append(pmain.Internal.Imports, ptest)
		t.ImportTest = true
	}
	if pxtest != nil {
		pmain.Internal.Imports = append(pmain.Internal.Imports, pxtest)
		t.ImportXtest = true
	}

	if ptest != p && localCover {
		// We have made modifications to the package p being tested
		// and are rebuilding p (as ptest), writing it to the testDir tree.
		// Arrange to rebuild, writing to that same tree, all packages q
		// such that the test depends on q, and q depends on p.
		// This makes sure that q sees the modifications to p.
		// Strictly speaking, the rebuild is only necessary if the
		// modifications to p change its export metadata, but
		// determining that is a bit tricky, so we rebuild always.
		//
		// This will cause extra compilation, so for now we only do it
		// when testCover is set. The conditions are more general, though,
		// and we may find that we need to do it always in the future.
		recompileForTest(pmain, p, ptest, testDir)
	}

	if cfg.BuildContext.GOOS == "darwin" {
		if cfg.BuildContext.GOARCH == "arm" || cfg.BuildContext.GOARCH == "arm64" {
			t.NeedCgo = true
		}
	}

	for _, cp := range pmain.Internal.Imports {
		if len(cp.Internal.CoverVars) > 0 {
			t.Cover = append(t.Cover, coverInfo{cp, cp.Internal.CoverVars})
		}
	}

	if !cfg.BuildN {
		// writeTestmain writes _testmain.go. This must happen after recompileForTest,
		// because recompileForTest modifies XXX.
		if err := writeTestmain(filepath.Join(testDir, "_testmain.go"), t); err != nil {
			return nil, nil, nil, err
		}
	}

	load.ComputeStale(pmain)

	if ptest != p {
		a := b.Action(work.ModeBuild, work.ModeBuild, ptest)
		a.Objdir = testDir + string(filepath.Separator) + "_obj_test" + string(filepath.Separator)
		a.Objpkg = ptestObj
		a.Target = ptestObj
		a.Link = false
	}

	if pxtest != nil {
		a := b.Action(work.ModeBuild, work.ModeBuild, pxtest)
		a.Objdir = testDir + string(filepath.Separator) + "_obj_xtest" + string(filepath.Separator)
		a.Objpkg = work.BuildToolchain.Pkgpath(testDir, pxtest)
		a.Target = a.Objpkg
	}

	a := b.Action(work.ModeBuild, work.ModeBuild, pmain)
	a.Objdir = testDir + string(filepath.Separator)
	a.Objpkg = filepath.Join(testDir, "main.a")
	a.Target = filepath.Join(testDir, testBinary) + cfg.ExeSuffix
	if cfg.Goos == "windows" {
		// There are many reserved words on Windows that,
		// if used in the name of an executable, cause Windows
		// to try to ask for extra permissions.
		// The word list includes setup, install, update, and patch,
		// but it does not appear to be defined anywhere.
		// We have run into this trying to run the
		// go.codereview/patch tests.
		// For package names containing those words, use test.test.exe
		// instead of pkgname.test.exe.
		// Note that this file name is only used in the Go command's
		// temporary directory. If the -c or other flags are
		// given, the code below will still use pkgname.test.exe.
		// There are two user-visible effects of this change.
		// First, you can actually run 'go test' in directories that
		// have names that Windows thinks are installer-like,
		// without getting a dialog box asking for more permissions.
		// Second, in the Windows process listing during go test,
		// the test shows up as test.test.exe, not pkgname.test.exe.
		// That second one is a drawback, but it seems a small
		// price to pay for the test running at all.
		// If maintaining the list of bad words is too onerous,
		// we could just do this always on Windows.
		for _, bad := range windowsBadWords {
			if strings.Contains(testBinary, bad) {
				a.Target = filepath.Join(testDir, "test.test") + cfg.ExeSuffix
				break
			}
		}
	}
	buildAction = a

	if testC || testNeedBinary {
		// -c or profiling flag: create action to copy binary to ./test.out.
		target := filepath.Join(base.Cwd, testBinary+cfg.ExeSuffix)
		if testO != "" {
			target = testO
			if !filepath.IsAbs(target) {
				target = filepath.Join(base.Cwd, target)
			}
		}
		buildAction = &work.Action{
			Func:    work.BuildInstallFunc,
			Deps:    []*work.Action{buildAction},
			Package: pmain,
			Target:  target,
		}
		runAction = buildAction // make sure runAction != nil even if not running test
	}
	if testC {
		printAction = &work.Action{Package: p, Deps: []*work.Action{runAction}} // nop
	} else {
		// run test
		runAction = &work.Action{
			Func:       builderRunTest,
			Deps:       []*work.Action{buildAction},
			Package:    p,
			IgnoreFail: true,
		}
		cleanAction := &work.Action{
			Func:    builderCleanTest,
			Deps:    []*work.Action{runAction},
			Package: p,
		}
		printAction = &work.Action{
			Func:    builderPrintTest,
			Deps:    []*work.Action{cleanAction},
			Package: p,
		}
	}

	return buildAction, runAction, printAction, nil
}

func testImportStack(top string, p *load.Package, target string) []string {
	stk := []string{top, p.ImportPath}
Search:
	for p.ImportPath != target {
		for _, p1 := range p.Internal.Imports {
			if p1.ImportPath == target || str.Contains(p1.Deps, target) {
				stk = append(stk, p1.ImportPath)
				p = p1
				continue Search
			}
		}
		// Can't happen, but in case it does...
		stk = append(stk, "<lost path to cycle>")
		break
	}
	return stk
}

func recompileForTest(pmain, preal, ptest *load.Package, testDir string) {
	// The "test copy" of preal is ptest.
	// For each package that depends on preal, make a "test copy"
	// that depends on ptest. And so on, up the dependency tree.
	testCopy := map[*load.Package]*load.Package{preal: ptest}
	for _, p := range load.PackageList([]*load.Package{pmain}) {
		// Copy on write.
		didSplit := false
		split := func() {
			if didSplit {
				return
			}
			didSplit = true
			if p.Internal.Pkgdir != testDir {
				p1 := new(load.Package)
				testCopy[p] = p1
				*p1 = *p
				p1.Internal.Imports = make([]*load.Package, len(p.Internal.Imports))
				copy(p1.Internal.Imports, p.Internal.Imports)
				p = p1
				p.Internal.Pkgdir = testDir
				p.Internal.Target = ""
				p.Internal.Fake = true
				p.Stale = true
				p.StaleReason = "depends on package being tested"
			}
		}

		// Update p.Deps and p.Internal.Imports to use at test copies.
		for i, dep := range p.Internal.Deps {
			if p1 := testCopy[dep]; p1 != nil && p1 != dep {
				split()
				p.Internal.Deps[i] = p1
			}
		}
		for i, imp := range p.Internal.Imports {
			if p1 := testCopy[imp]; p1 != nil && p1 != imp {
				split()
				p.Internal.Imports[i] = p1
			}
		}
	}
}

var coverIndex = 0

// isTestFile reports whether the source file is a set of tests and should therefore
// be excluded from coverage analysis.
func isTestFile(file string) bool {
	// We don't cover tests, only the code they test.
	return strings.HasSuffix(file, "_test.go")
}

// declareCoverVars attaches the required cover variables names
// to the files, to be used when annotating the files.
func declareCoverVars(importPath string, files ...string) map[string]*load.CoverVar {
	coverVars := make(map[string]*load.CoverVar)
	for _, file := range files {
		if isTestFile(file) {
			continue
		}
		coverVars[file] = &load.CoverVar{
			File: filepath.Join(importPath, file),
			Var:  fmt.Sprintf("GoCover_%d", coverIndex),
		}
		coverIndex++
	}
	return coverVars
}

var noTestsToRun = []byte("\ntesting: warning: no tests to run\n")

// builderRunTest is the action for running a test binary.
func builderRunTest(b *work.Builder, a *work.Action) error {
	args := str.StringList(work.FindExecCmd(), a.Deps[0].Target, testArgs)
	a.TestOutput = new(bytes.Buffer)

	if cfg.BuildN || cfg.BuildX {
		b.Showcmd("", "%s", strings.Join(args, " "))
		if cfg.BuildN {
			return nil
		}
	}

	if a.Failed {
		// We were unable to build the binary.
		a.Failed = false
		fmt.Fprintf(a.TestOutput, "FAIL\t%s [build failed]\n", a.Package.ImportPath)
		base.SetExitStatus(1)
		return nil
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = a.Package.Dir
	cmd.Env = base.EnvForDir(cmd.Dir, cfg.OrigEnv)
	var buf bytes.Buffer
	if testStreamOutput {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	}

	// If there are any local SWIG dependencies, we want to load
	// the shared library from the build directory.
	if a.Package.UsesSwig() {
		env := cmd.Env
		found := false
		prefix := "LD_LIBRARY_PATH="
		for i, v := range env {
			if strings.HasPrefix(v, prefix) {
				env[i] = v + ":."
				found = true
				break
			}
		}
		if !found {
			env = append(env, "LD_LIBRARY_PATH=.")
		}
		cmd.Env = env
	}

	t0 := time.Now()
	err := cmd.Start()

	// This is a last-ditch deadline to detect and
	// stop wedged test binaries, to keep the builders
	// running.
	if err == nil {
		tick := time.NewTimer(testKillTimeout)
		base.StartSigHandlers()
		done := make(chan error)
		go func() {
			done <- cmd.Wait()
		}()
	Outer:
		select {
		case err = <-done:
			// ok
		case <-tick.C:
			if base.SignalTrace != nil {
				// Send a quit signal in the hope that the program will print
				// a stack trace and exit. Give it five seconds before resorting
				// to Kill.
				cmd.Process.Signal(base.SignalTrace)
				select {
				case err = <-done:
					fmt.Fprintf(&buf, "*** Test killed with %v: ran too long (%v).\n", base.SignalTrace, testKillTimeout)
					break Outer
				case <-time.After(5 * time.Second):
				}
			}
			cmd.Process.Kill()
			err = <-done
			fmt.Fprintf(&buf, "*** Test killed: ran too long (%v).\n", testKillTimeout)
		}
		tick.Stop()
	}
	out := buf.Bytes()
	t := fmt.Sprintf("%.3fs", time.Since(t0).Seconds())
	if err == nil {
		norun := ""
		if testShowPass {
			a.TestOutput.Write(out)
		}
		if bytes.HasPrefix(out, noTestsToRun[1:]) || bytes.Contains(out, noTestsToRun) {
			norun = " [no tests to run]"
		}
		fmt.Fprintf(a.TestOutput, "ok  \t%s\t%s%s%s\n", a.Package.ImportPath, t, coveragePercentage(out), norun)
		return nil
	}

	base.SetExitStatus(1)
	if len(out) > 0 {
		a.TestOutput.Write(out)
		// assume printing the test binary's exit status is superfluous
	} else {
		fmt.Fprintf(a.TestOutput, "%s\n", err)
	}
	fmt.Fprintf(a.TestOutput, "FAIL\t%s\t%s\n", a.Package.ImportPath, t)

	return nil
}

// coveragePercentage returns the coverage results (if enabled) for the
// test. It uncovers the data by scanning the output from the test run.
func coveragePercentage(out []byte) string {
	if !testCover {
		return ""
	}
	// The string looks like
	//	test coverage for encoding/binary: 79.9% of statements
	// Extract the piece from the percentage to the end of the line.
	re := regexp.MustCompile(`coverage: (.*)\n`)
	matches := re.FindSubmatch(out)
	if matches == nil {
		// Probably running "go test -cover" not "go test -cover fmt".
		// The coverage output will appear in the output directly.
		return ""
	}
	return fmt.Sprintf("\tcoverage: %s", matches[1])
}

// builderCleanTest is the action for cleaning up after a test.
func builderCleanTest(b *work.Builder, a *work.Action) error {
	if cfg.BuildWork {
		return nil
	}
	run := a.Deps[0]
	testDir := filepath.Join(b.WorkDir, filepath.FromSlash(run.Package.ImportPath+"/_test"))
	os.RemoveAll(testDir)
	return nil
}

// builderPrintTest is the action for printing a test result.
func builderPrintTest(b *work.Builder, a *work.Action) error {
	clean := a.Deps[0]
	run := clean.Deps[0]
	os.Stdout.Write(run.TestOutput.Bytes())
	run.TestOutput = nil
	return nil
}

// builderNoTest is the action for testing a package with no test files.
func builderNoTest(b *work.Builder, a *work.Action) error {
	fmt.Printf("?   \t%s\t[no test files]\n", a.Package.ImportPath)
	return nil
}

// isTestFunc tells whether fn has the type of a testing function. arg
// specifies the parameter type we look for: B, M or T.
func isTestFunc(fn *ast.FuncDecl, arg string) bool {
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 ||
		fn.Type.Params.List == nil ||
		len(fn.Type.Params.List) != 1 ||
		len(fn.Type.Params.List[0].Names) > 1 {
		return false
	}
	ptr, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	// We can't easily check that the type is *testing.M
	// because we don't know how testing has been imported,
	// but at least check that it's *M or *something.M.
	// Same applies for B and T.
	if name, ok := ptr.X.(*ast.Ident); ok && name.Name == arg {
		return true
	}
	if sel, ok := ptr.X.(*ast.SelectorExpr); ok && sel.Sel.Name == arg {
		return true
	}
	return false
}

// isTest tells whether name looks like a test (or benchmark, according to prefix).
// It is a Test (say) if there is a character after Test that is not a lower-case letter.
// We don't want TesticularCancer.
func isTest(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) { // "Test" is ok
		return true
	}
	rune, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(rune)
}

type coverInfo struct {
	Package *load.Package
	Vars    map[string]*load.CoverVar
}

// loadTestFuncs returns the testFuncs describing the tests that will be run.
func loadTestFuncs(ptest *load.Package) (*testFuncs, error) {
	t := &testFuncs{
		Package: ptest,
	}
	for _, file := range ptest.TestGoFiles {
		if err := t.load(filepath.Join(ptest.Dir, file), "_test", &t.ImportTest, &t.NeedTest); err != nil {
			return nil, err
		}
	}
	for _, file := range ptest.XTestGoFiles {
		if err := t.load(filepath.Join(ptest.Dir, file), "_xtest", &t.ImportXtest, &t.NeedXtest); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// writeTestmain writes the _testmain.go file for t to the file named out.
func writeTestmain(out string, t *testFuncs) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := testmainTmpl.Execute(f, t); err != nil {
		return err
	}

	return nil
}

type testFuncs struct {
	Tests       []testFunc
	Benchmarks  []testFunc
	Examples    []testFunc
	TestMain    *testFunc
	Package     *load.Package
	ImportTest  bool
	NeedTest    bool
	ImportXtest bool
	NeedXtest   bool
	NeedCgo     bool
	Cover       []coverInfo
}

func (t *testFuncs) CoverMode() string {
	return testCoverMode
}

func (t *testFuncs) CoverEnabled() bool {
	return testCover
}

// ImportPath returns the import path of the package being tested, if it is within GOPATH.
// This is printed by the testing package when running benchmarks.
func (t *testFuncs) ImportPath() string {
	pkg := t.Package.ImportPath
	if strings.HasPrefix(pkg, "_/") {
		return ""
	}
	if pkg == "command-line-arguments" {
		return ""
	}
	return pkg
}

// Covered returns a string describing which packages are being tested for coverage.
// If the covered package is the same as the tested package, it returns the empty string.
// Otherwise it is a comma-separated human-readable list of packages beginning with
// " in", ready for use in the coverage message.
func (t *testFuncs) Covered() string {
	if testCoverPaths == nil {
		return ""
	}
	return " in " + strings.Join(testCoverPaths, ", ")
}

// Tested returns the name of the package being tested.
func (t *testFuncs) Tested() string {
	return t.Package.Name
}

type testFunc struct {
	Package   string // imported package name (_test or _xtest)
	Name      string // function name
	Output    string // output, for examples
	Unordered bool   // output is allowed to be unordered.
}

var testFileSet = token.NewFileSet()

func (t *testFuncs) load(filename, pkg string, doImport, seen *bool) error {
	f, err := parser.ParseFile(testFileSet, filename, nil, parser.ParseComments)
	if err != nil {
		return base.ExpandScanner(err)
	}
	for _, d := range f.Decls {
		n, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if n.Recv != nil {
			continue
		}
		name := n.Name.String()
		switch {
		case name == "TestMain" && isTestFunc(n, "M"):
			if t.TestMain != nil {
				return errors.New("multiple definitions of TestMain")
			}
			t.TestMain = &testFunc{pkg, name, "", false}
			*doImport, *seen = true, true
		case isTest(name, "Test"):
			err := checkTestFunc(n, "T")
			if err != nil {
				return err
			}
			t.Tests = append(t.Tests, testFunc{pkg, name, "", false})
			*doImport, *seen = true, true
		case isTest(name, "Benchmark"):
			err := checkTestFunc(n, "B")
			if err != nil {
				return err
			}
			t.Benchmarks = append(t.Benchmarks, testFunc{pkg, name, "", false})
			*doImport, *seen = true, true
		}
	}
	ex := doc.Examples(f)
	sort.Slice(ex, func(i, j int) bool { return ex[i].Order < ex[j].Order })
	for _, e := range ex {
		*doImport = true // import test file whether executed or not
		if e.Output == "" && !e.EmptyOutput {
			// Don't run examples with no output.
			continue
		}
		t.Examples = append(t.Examples, testFunc{pkg, "Example" + e.Name, e.Output, e.Unordered})
		*seen = true
	}
	return nil
}

func checkTestFunc(fn *ast.FuncDecl, arg string) error {
	if !isTestFunc(fn, arg) {
		name := fn.Name.String()
		pos := testFileSet.Position(fn.Pos())
		return fmt.Errorf("%s: wrong signature for %s, must be: func %s(%s *testing.%s)", pos, name, name, strings.ToLower(arg), arg)
	}
	return nil
}

var testmainTmpl = template.Must(template.New("main").Parse(`
package main

import (
{{if not .TestMain}}
	"os"
{{end}}
	"testing"
	"testing/internal/testdeps"

{{if .ImportTest}}
	{{if .NeedTest}}_test{{else}}_{{end}} {{.Package.ImportPath | printf "%q"}}
{{end}}
{{if .ImportXtest}}
	{{if .NeedXtest}}_xtest{{else}}_{{end}} {{.Package.ImportPath | printf "%s_test" | printf "%q"}}
{{end}}
{{range $i, $p := .Cover}}
	_cover{{$i}} {{$p.Package.ImportPath | printf "%q"}}
{{end}}

{{if .NeedCgo}}
	_ "runtime/cgo"
{{end}}
)

var tests = []testing.InternalTest{
{{range .Tests}}
	{"{{.Name}}", {{.Package}}.{{.Name}}},
{{end}}
}

var benchmarks = []testing.InternalBenchmark{
{{range .Benchmarks}}
	{"{{.Name}}", {{.Package}}.{{.Name}}},
{{end}}
}

var examples = []testing.InternalExample{
{{range .Examples}}
	{"{{.Name}}", {{.Package}}.{{.Name}}, {{.Output | printf "%q"}}, {{.Unordered}}},
{{end}}
}

func init() {
	testdeps.ImportPath = {{.ImportPath | printf "%q"}}
}

{{if .CoverEnabled}}

// Only updated by init functions, so no need for atomicity.
var (
	coverCounters = make(map[string][]uint32)
	coverBlocks = make(map[string][]testing.CoverBlock)
)

func init() {
	{{range $i, $p := .Cover}}
	{{range $file, $cover := $p.Vars}}
	coverRegisterFile({{printf "%q" $cover.File}}, _cover{{$i}}.{{$cover.Var}}.Count[:], _cover{{$i}}.{{$cover.Var}}.Pos[:], _cover{{$i}}.{{$cover.Var}}.NumStmt[:])
	{{end}}
	{{end}}
}

func coverRegisterFile(fileName string, counter []uint32, pos []uint32, numStmts []uint16) {
	if 3*len(counter) != len(pos) || len(counter) != len(numStmts) {
		panic("coverage: mismatched sizes")
	}
	if coverCounters[fileName] != nil {
		// Already registered.
		return
	}
	coverCounters[fileName] = counter
	block := make([]testing.CoverBlock, len(counter))
	for i := range counter {
		block[i] = testing.CoverBlock{
			Line0: pos[3*i+0],
			Col0: uint16(pos[3*i+2]),
			Line1: pos[3*i+1],
			Col1: uint16(pos[3*i+2]>>16),
			Stmts: numStmts[i],
		}
	}
	coverBlocks[fileName] = block
}
{{end}}

func main() {
{{if .CoverEnabled}}
	testing.RegisterCover(testing.Cover{
		Mode: {{printf "%q" .CoverMode}},
		Counters: coverCounters,
		Blocks: coverBlocks,
		CoveredPackages: {{printf "%q" .Covered}},
	})
{{end}}
	m := testing.MainStart(testdeps.TestDeps{}, tests, benchmarks, examples)
{{with .TestMain}}
	{{.Package}}.{{.Name}}(m)
{{else}}
	os.Exit(m.Run())
{{end}}
}

`))
