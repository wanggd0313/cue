// Copyright 2018 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package load

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
	"unicode"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/internal/cueexperiment"
	"cuelang.org/go/internal/str"
	"cuelang.org/go/internal/tdtest"
)

func init() {
	// Ignore the value of CUE_EXPERIMENT for the purposes
	// of these tests, which we want to test both with the experiment
	// enabled and disabled.
	os.Setenv("CUE_EXPERIMENT", "")

	// Once we've called cueexperiment.Init, cueexperiment.Vars
	// will not be touched again, so we can set fields in it for the tests.
	cueexperiment.Init()
}

// TestLoad is an end-to-end test.
func TestLoad(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testdataDir := testdata("testmod")
	dirCfg := &Config{
		Dir:   testdataDir,
		Tools: true,
	}
	badModCfg := &Config{
		Dir: testdata("badmod"),
	}
	type loadTest struct {
		cfg  *Config
		args []string
		want string
	}

	args := str.StringList
	testCases := []loadTest{{
		cfg:  badModCfg,
		args: args("."),
		want: `err:    module: cannot use value 123 (type int) as string:
    $CWD/testdata/badmod/cue.mod/module.cue:2:9
path:   ""
module: ""
root:   ""
dir:    ""
display:""`,
	}, {
		// Even though the directory is called testdata, the last path in
		// the module is test. So "package test" is correctly the default
		// package of this directory.
		cfg:  dirCfg,
		args: nil,
		want: `path:   mod.test/test
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod
display:.
files:
    $CWD/testdata/testmod/test.cue
imports:
    mod.test/test/sub: $CWD/testdata/testmod/sub/sub.cue`}, {
		// Even though the directory is called testdata, the last path in
		// the module is test. So "package test" is correctly the default
		// package of this directory.
		cfg:  dirCfg,
		args: args("."),
		want: `path:   mod.test/test
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod
display:.
files:
    $CWD/testdata/testmod/test.cue
imports:
    mod.test/test/sub: $CWD/testdata/testmod/sub/sub.cue`}, {
		// TODO:
		// - path incorrect, should be mod.test/test/other:main.
		cfg:  dirCfg,
		args: args("./other/..."),
		want: `err:    import failed: relative import paths not allowed ("./file"):
    $CWD/testdata/testmod/other/main.cue:6:2
path:   ""
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    ""
display:""`}, {
		cfg:  dirCfg,
		args: args("./anon"),
		want: `err:    build constraints exclude all CUE files in ./anon:
    anon/anon.cue: no package name
path:   mod.test/test/anon
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/anon
display:./anon`}, {
		// TODO:
		// - paths are incorrect, should be mod.test/test/other:main.
		cfg:  dirCfg,
		args: args("./other"),
		want: `err:    import failed: relative import paths not allowed ("./file"):
    $CWD/testdata/testmod/other/main.cue:6:2
path:   mod.test/test/other:main
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/other
display:./other
files:
    $CWD/testdata/testmod/other/main.cue`}, {
		// TODO:
		// - incorrect path, should be mod.test/test/hello:test
		cfg:  dirCfg,
		args: args("./hello"),
		want: `path:   mod.test/test/hello:test
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/hello
display:./hello
files:
    $CWD/testdata/testmod/test.cue
    $CWD/testdata/testmod/hello/test.cue
imports:
    mod.test/test/sub: $CWD/testdata/testmod/sub/sub.cue`}, {
		// TODO:
		// - incorrect path, should be mod.test/test/hello:test
		cfg:  dirCfg,
		args: args("mod.test/test/hello:test"),
		want: `path:   mod.test/test/hello:test
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/hello
display:mod.test/test/hello:test
files:
    $CWD/testdata/testmod/test.cue
    $CWD/testdata/testmod/hello/test.cue
imports:
    mod.test/test/sub: $CWD/testdata/testmod/sub/sub.cue`}, {
		// TODO:
		// - incorrect path, should be mod.test/test/hello:test
		cfg:  dirCfg,
		args: args("mod.test/test/hello:nonexist"),
		want: `err:    build constraints exclude all CUE files in mod.test/test/hello:nonexist:
    anon.cue: no package name
    test.cue: package is test, want nonexist
    hello/test.cue: package is test, want nonexist
path:   mod.test/test/hello:nonexist
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/hello
display:mod.test/test/hello:nonexist`}, {
		cfg:  dirCfg,
		args: args("./anon.cue", "./other/anon.cue"),
		want: `path:   ""
module: ""
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod
display:command-line-arguments
files:
    $CWD/testdata/testmod/anon.cue
    $CWD/testdata/testmod/other/anon.cue`}, {
		cfg: dirCfg,
		// Absolute file is normalized.
		args: args(filepath.Join(cwd, testdata("testmod", "anon.cue"))),
		want: `path:   ""
module: ""
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod
display:command-line-arguments
files:
    $CWD/testdata/testmod/anon.cue`}, {
		cfg:  dirCfg,
		args: args("-"),
		want: `path:   ""
module: ""
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod
display:command-line-arguments
files:
    -`}, {
		// NOTE: dir should probably be set to $CWD/testdata, but either way.
		cfg:  dirCfg,
		args: args("non-existing"),
		want: `err:    implied package identifier "non-existing" from import path "non-existing" is not valid
path:   non-existing
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/cue.mod/gen/non-existing
display:non-existing`,
	}, {
		cfg:  dirCfg,
		args: args("./empty"),
		want: `err:    no CUE files in ./empty
path:   mod.test/test/empty
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/empty
display:./empty`,
	}, {
		cfg:  dirCfg,
		args: args("./imports"),
		want: `path:   mod.test/test/imports
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/imports
display:./imports
files:
    $CWD/testdata/testmod/imports/imports.cue
imports:
    mod.test/catch: $CWD/testdata/testmod/cue.mod/pkg/mod.test/catch/catch.cue
    mod.test/helper:helper1: $CWD/testdata/testmod/cue.mod/pkg/mod.test/helper/helper1.cue`}, {
		cfg:  dirCfg,
		args: args("./toolonly"),
		want: `path:   mod.test/test/toolonly:foo
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/toolonly
display:./toolonly
files:
    $CWD/testdata/testmod/toolonly/foo_tool.cue`}, {
		cfg: &Config{
			Dir: testdataDir,
		},
		args: args("./toolonly"),
		want: `err:    build constraints exclude all CUE files in ./toolonly:
    anon.cue: no package name
    test.cue: package is test, want foo
    toolonly/foo_tool.cue: _tool.cue files excluded in non-cmd mode
path:   mod.test/test/toolonly:foo
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/toolonly
display:./toolonly`}, {
		cfg: &Config{
			Dir:  testdataDir,
			Tags: []string{"prod"},
		},
		args: args("./tags"),
		want: `path:   mod.test/test/tags
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/tags
display:./tags
files:
    $CWD/testdata/testmod/tags/prod.cue`}, {
		cfg: &Config{
			Dir:  testdataDir,
			Tags: []string{"prod", "foo=bar"},
		},
		args: args("./tags"),
		want: `path:   mod.test/test/tags
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/tags
display:./tags
files:
    $CWD/testdata/testmod/tags/prod.cue`}, {
		cfg: &Config{
			Dir:  testdataDir,
			Tags: []string{"prod"},
		},
		args: args("./tagsbad"),
		want: `err:    tag "prod" not used in any file
previous declaration here:
    $CWD/testdata/testmod/tagsbad/prod.cue:1:1
multiple @if attributes:
    $CWD/testdata/testmod/tagsbad/prod.cue:2:1
path:   mod.test/test/tagsbad
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/tagsbad
display:./tagsbad`}, {
		cfg: &Config{
			Dir: testdataDir,
		},
		args: args("./cycle"),
		want: `err:    import failed: import failed: import failed: package import cycle not allowed:
    $CWD/testdata/testmod/cycle/cycle.cue:3:8
    $CWD/testdata/testmod/cue.mod/pkg/mod.test/cycle/bar/bar.cue:3:8
    $CWD/testdata/testmod/cue.mod/pkg/mod.test/cycle/foo/foo.cue:3:8
path:   mod.test/test/cycle
module: mod.test/test
root:   $CWD/testdata/testmod
dir:    $CWD/testdata/testmod/cycle
display:./cycle
files:
    $CWD/testdata/testmod/cycle/cycle.cue`}}
	tdtest.Run(t, testCases, func(t *tdtest.T, tc *loadTest) {
		pkgs := Instances(tc.args, tc.cfg)

		buf := &bytes.Buffer{}
		err := pkgInfo.Execute(buf, pkgs)
		if err != nil {
			t.Fatal(err)
		}

		got := strings.TrimSpace(buf.String())
		got = strings.Replace(got, cwd, "$CWD", -1)
		// Errors are printed with slashes, so replace
		// the slash-separated form of CWD too.
		got = strings.Replace(got, filepath.ToSlash(cwd), "$CWD", -1)
		// Make test work with Windows.
		got = strings.Replace(got, string(filepath.Separator), "/", -1)

		t.Equal(got, tc.want)
	})
}

var pkgInfo = template.Must(template.New("pkg").Funcs(template.FuncMap{
	"errordetails": func(err error) string {
		s := errors.Details(err, &errors.Config{
			ToSlash: true,
		})
		s = strings.TrimSuffix(s, "\n")
		return s
	}}).Parse(`
{{- range . -}}
{{- if .Err}}err:    {{errordetails .Err}}{{end}}
path:   {{if .ImportPath}}{{.ImportPath}}{{else}}""{{end}}
module: {{with .Module}}{{.}}{{else}}""{{end}}
root:   {{with .Root}}{{.}}{{else}}""{{end}}
dir:    {{with .Dir}}{{.}}{{else}}""{{end}}
display:{{with .DisplayPath}}{{.}}{{else}}""{{end}}
{{if .Files -}}
files:
{{- range .Files}}
    {{.Filename}}
{{- end -}}
{{- end}}
{{if .Imports -}}
imports:
{{- range .Dependencies}}
    {{.ImportPath}}:{{range .Files}} {{.Filename}}{{end}}
{{- end}}
{{end -}}
{{- end -}}
`))

func TestOverlays(t *testing.T) {
	cwd, _ := os.Getwd()
	abs := func(path string) string {
		return filepath.Join(cwd, path)
	}
	c := &Config{
		Overlay: map[string]Source{
			// Not necessary, but nice to add.
			abs("cue.mod"): FromString(`module: "mod.test"`),

			abs("dir/top.cue"): FromBytes([]byte(`
			   package top
			   msg: "Hello"
			`)),
			abs("dir/b/foo.cue"): FromString(`
			   package foo

			   a: <= 5
			`),
			abs("dir/b/bar.cue"): FromString(`
			   package foo

			   a: >= 5
			`),
		},
	}
	want := []string{
		`{msg:"Hello"}`,
		`{a:5}`,
	}
	rmSpace := func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}
	for i, inst := range cue.Build(Instances([]string{"./dir/..."}, c)) {
		if inst.Err != nil {
			t.Error(inst.Err)
			continue
		}
		b, err := format.Node(inst.Value().Syntax(cue.Final()))
		if err != nil {
			t.Error(err)
			continue
		}
		if got := string(bytes.Map(rmSpace, b)); got != want[i] {
			t.Errorf("%s: got %s; want %s", inst.Dir, got, want[i])
		}
	}
}

func TestLoadInstancesConcurrent(t *testing.T) {
	// This test is designed to fail when run with the race detector
	// if there's an underlying race condition.
	// See https:/cuelang.org/issue/1746
	race(func() {
		Instances([]string{"."}, nil)
	})
}

func race(f func()) {
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			f()
			wg.Done()
		}()
	}
}
