// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type BootstrapSuite struct {
	testbase.LoggingSuite
	coretesting.MgoSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

type bootstrapRetryTest struct {
	info               string
	args               []string
	expectedAllowRetry []bool
	err                string
}

var bootstrapRetryTests = []bootstrapRetryTest{{
	info:               "no tools uploaded, first check has no retries; check after upload has retries",
	expectedAllowRetry: []bool{false, true},
	err:                "tools not found",
}, {
	info:               "new tools uploaded, so we want to allow retries to give them a chance at showing up",
	args:               []string{"--upload-tools"},
	expectedAllowRetry: []bool{true},
	err:                "no matching tools available",
}}

// Test test checks that bootstrap calls FindTools with the expected allowRetry flag.
func (s *BootstrapSuite) TestAllowRetries(c *gc.C) {
	for i, test := range bootstrapRetryTests {
		c.Logf("test %d: %s\n", i, test.info)
		s.runAllowRetriesTest(c, test)
	}
}

func (s *BootstrapSuite) runAllowRetriesTest(c *gc.C, test bootstrapRetryTest) {
	_, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	var findToolsRetryValues []bool

	mockFindTools := func(cloudInst environs.ConfigGetter, majorVersion, minorVersion int,
		filter coretools.Filter, allowRetry bool) (list coretools.List, err error) {
		findToolsRetryValues = append(findToolsRetryValues, allowRetry)
		return nil, errors.NotFoundf("tools")
	}

	restore := envtools.TestingPatchBootstrapFindTools(mockFindTools)
	defer restore()

	_, errc := runCommand(nil, new(BootstrapCommand), test.args...)
	err := <-errc
	c.Assert(findToolsRetryValues, gc.DeepEquals, test.expectedAllowRetry)
	c.Assert(err, gc.ErrorMatches, test.err)
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	dummy.Reset()
}

func (s *BootstrapSuite) TestTest(c *gc.C) {
	uploadTools = mockUploadTools
	defer func() { uploadTools = sync.Upload }()

	for i, test := range bootstrapTests {
		c.Logf("\ntest %d: %s", i, test.info)
		test.run(c)
	}
}

type bootstrapTest struct {
	info string
	// binary version string used to set version.Current
	version string
	sync    bool
	args    []string
	err     string
	// binary version strings for expected tools; if set, no default tools
	// will be uploaded before running the test.
	uploads     []string
	constraints constraints.Value
}

func (test bootstrapTest) run(c *gc.C) {
	// Prepare a mock storage for testing.
	restore := createToolsStore(c)
	defer restore()

	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	if test.version != "" {
		origVersion := version.Current
		version.Current = version.MustParseBinary(test.version)
		defer func() { version.Current = origVersion }()
	}

	uploadCount := len(test.uploads)
	if uploadCount == 0 {
		usefulVersion := version.Current
		usefulVersion.Series = env.Config().DefaultSeries()
		envtesting.UploadFakeToolsVersion(c, env.Storage(), usefulVersion)
	}

	// Run command and check for uploads.
	opc, errc := runCommand(nil, new(BootstrapCommand), test.args...)
	if uploadCount > 0 {
		for i := 0; i < uploadCount; i++ {
			c.Check((<-opc).(dummy.OpPutFile).Env, gc.Equals, "peckham")
		}
		list, err := envtools.FindTools(
			env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
		c.Check(err, gc.IsNil)
		c.Logf("found: " + list.String())
		urls := list.URLs()
		c.Check(urls, gc.HasLen, len(test.uploads))
		for _, v := range test.uploads {
			c.Logf("seeking: " + v)
			vers := version.MustParseBinary(v)
			_, found := urls[vers]
			c.Check(found, gc.Equals, true)
		}
	}

	// Check for remaining operations/errors.
	if test.err != "" {
		c.Check(<-errc, gc.ErrorMatches, test.err)
		return
	}
	if !c.Check(<-errc, gc.IsNil) {
		return
	}
	opPutBootstrapVerifyFile := (<-opc).(dummy.OpPutFile)
	c.Check(opPutBootstrapVerifyFile.Env, gc.Equals, "peckham")
	c.Check(opPutBootstrapVerifyFile.FileName, gc.Equals, "bootstrap-verify")

	opBootstrap := (<-opc).(dummy.OpBootstrap)
	c.Check(opBootstrap.Env, gc.Equals, "peckham")
	c.Check(opBootstrap.Constraints, gc.DeepEquals, test.constraints)

	// Check a CA cert/key was generated by reloading the environment.
	env, err := environs.NewFromName("peckham")
	c.Assert(err, gc.IsNil)
	_, hasCert := env.Config().CACert()
	c.Check(hasCert, gc.Equals, true)
	_, hasKey := env.Config().CAPrivateKey()
	c.Check(hasKey, gc.Equals, true)
}

var bootstrapTests = []bootstrapTest{{
	info: "no args, no error, no uploads, no constraints",
}, {
	info: "bad arg",
	args: []string{"twiddle"},
	err:  `unrecognized args: \["twiddle"\]`,
}, {
	info: "bad --constraints",
	args: []string{"--constraints", "bad=wrong"},
	err:  `invalid value "bad=wrong" for flag --constraints: unknown constraint "bad"`,
}, {
	info: "bad --series",
	args: []string{"--series", "bad1"},
	err:  `invalid value "bad1" for flag --series: invalid series name "bad1"`,
}, {
	info: "lonely --series",
	args: []string{"--series", "fine"},
	err:  `--series requires --upload-tools`,
}, {
	info: "bad environment",
	args: []string{"-e", "brokenenv"},
	err:  `environment configuration has no admin-secret`,
}, {
	info:        "constraints",
	args:        []string{"--constraints", "mem=4G cpu-cores=4"},
	constraints: constraints.MustParse("mem=4G cpu-cores=4"),
}, {
	info:    "--upload-tools picks all reasonable series",
	version: "1.2.3-saucy-hostarch",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-saucy-hostarch",   // from version.Current
		"1.2.3.1-raring-hostarch",  // from env.Config().DefaultSeries()
		"1.2.3.1-precise-hostarch", // from environs/config.DefaultSeries
	},
}, {
	info:    "--upload-tools only uploads each file once",
	version: "1.2.3-precise-hostarch",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.1-raring-hostarch",
		"1.2.3.1-precise-hostarch",
	},
}, {
	info:    "--upload-tools rejects invalid series",
	version: "1.2.3-saucy-hostarch",
	args:    []string{"--upload-tools", "--series", "ping,ping,pong"},
	uploads: []string{
		"1.2.3.1-saucy-hostarch",
		"1.2.3.1-ping-hostarch",
		"1.2.3.1-pong-hostarch",
	},
	err: `invalid series "ping"`,
}, {
	info:    "--upload-tools always bumps build number",
	version: "1.2.3.4-raring-hostarch",
	args:    []string{"--upload-tools"},
	uploads: []string{
		"1.2.3.5-raring-hostarch",
		"1.2.3.5-precise-hostarch",
	},
}}

func (s *BootstrapSuite) TestAutoSync(c *gc.C) {
	// Prepare a mock storage for testing and store the
	// dummy tools in there.
	restore := createToolsStore(c)
	defer restore()

	// Change the tools location to be the test location and also
	// the version and ensure their later restoring.
	origVersion := version.Current
	version.Current.Number = version.MustParse("1.2.3")
	defer func() { version.Current = origVersion }()

	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	// Bootstrap the environment now detects the missing
	// tools and automatically synchronizes them from the
	// storage above.
	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, nil)
	c.Check(code, gc.Equals, 0)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) TestAutoSyncLocalSource(c *gc.C) {
	// Prepare a tools directory for testing and store the
	// dummy tools in there.
	source := createToolsSource(c)

	// Change the version and ensure its later restoring.
	origVersion := version.Current
	version.Current.Number = version.MustParse("1.2.3")
	defer func() {
		version.Current = origVersion
	}()

	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	defer fake.Restore()

	// Bootstrap the environment with an invalid source.
	// The command returns with an error.
	ctx := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, []string{"--source", c.MkDir()})
	c.Check(code, gc.Equals, 1)

	// Now check that there are no tools available.
	_, err := envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
	c.Assert(err, gc.ErrorMatches, "no tools available")

	// Bootstrap the environment with the valid source. This time
	// the bootstrapping has to show no error, because the tools
	// are automatically synchronized.
	ctx = coretesting.Context(c)
	code = cmd.Main(&BootstrapCommand{}, ctx, []string{"--source", source})
	c.Check(code, gc.Equals, 0)

	// Now check the available tools which are the 1.2.0 envtools.
	checkTools(c, env, v120All)
}

func (s *BootstrapSuite) setupAutoUploadTest(c *gc.C, vers, series string) environs.Environ {
	uploadTools = mockUploadTools
	s.AddCleanup(func(*gc.C) { uploadTools = sync.Upload })

	// Prepare a mock storage for testing and store the
	// dummy tools in there.
	restore := createToolsStore(c)
	s.AddCleanup(func(*gc.C) { restore() })

	// Change the tools location to be the test location and also
	// the version and ensure their later restoring.
	// Set the current version to be something for which there are no tools
	// so we can test that an upload is forced.
	origVersion := version.Current
	version.Current.Number = version.MustParse(vers)
	version.Current.Series = series
	s.AddCleanup(func(*gc.C) { version.Current = origVersion })

	// Create home with dummy provider and remove all
	// of its envtools.
	env, fake := makeEmptyFakeHome(c)
	s.AddCleanup(func(*gc.C) { fake.Restore() })
	return env
}

func (s *BootstrapSuite) TestAutoUploadAfterFailedSync(c *gc.C) {
	otherSeries := "precise"
	if otherSeries == version.CurrentSeries() {
		otherSeries = "raring"
	}
	env := s.setupAutoUploadTest(c, "1.7.3", otherSeries)
	// Run command and check for that upload has been run for tools matching the current juju version.
	opc, errc := runCommand(nil, new(BootstrapCommand))
	c.Assert(<-errc, gc.IsNil)
	c.Assert((<-opc).(dummy.OpPutFile).Env, gc.Equals, "peckham")
	list, err := envtools.FindTools(env, version.Current.Major, version.Current.Minor, coretools.Filter{}, false)
	c.Assert(err, gc.IsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()
	c.Assert(urls, gc.HasLen, 2)
	expectedVers := []version.Binary{
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", otherSeries, version.Current.Arch)),
		version.MustParseBinary(fmt.Sprintf("1.7.3.1-%s-%s", version.CurrentSeries(), version.CurrentArch())),
	}
	for _, vers := range expectedVers {
		c.Logf("seeking: " + vers.String())
		_, found := urls[vers]
		c.Check(found, gc.Equals, true)
	}
}

func (s *BootstrapSuite) TestAutoUploadOnlyForDev(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	_, errc := runCommand(nil, new(BootstrapCommand))
	err := <-errc
	c.Assert(err, gc.ErrorMatches, "no matching tools available")
}

func (s *BootstrapSuite) TestMissingToolsError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.8.3", "precise")
	context := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, context, nil)
	c.Assert(code, gc.Equals, 1)
	errText := context.Stderr.(*bytes.Buffer).String()
	errText = strings.Replace(errText, "\n", "", -1)
	expectedErrText := strings.Replace(fmt.Sprintf(".*%s.*", NoToolsNoUploadMessage), "\n", "", -1)
	c.Assert(errText, gc.Matches, expectedErrText)
}

func uploadToolsAlwaysFails(stor storage.Storage, forceVersion *version.Number, series ...string) (*coretools.Tools, error) {
	return nil, fmt.Errorf("an error")
}

func (s *BootstrapSuite) TestMissingToolsUploadFailedError(c *gc.C) {
	s.setupAutoUploadTest(c, "1.7.3", "precise")
	uploadTools = uploadToolsAlwaysFails
	context := coretesting.Context(c)
	code := cmd.Main(&BootstrapCommand{}, context, nil)
	c.Assert(code, gc.Equals, 1)
	errText := context.Stderr.(*bytes.Buffer).String()
	errText = strings.Replace(errText, "\n", "", -1)
	expectedErrText := strings.Replace(fmt.Sprintf(".*%s.*", NoToolsMessage), "\n", "", -1)
	c.Assert(errText, gc.Matches, expectedErrText)
}

// createToolsStore creates the fake tools store.
func createToolsStore(c *gc.C) func() {
	stor, err := envtesting.NewEC2HTTPTestStorage("127.0.0.1")
	c.Assert(err, gc.IsNil)
	origLocation := sync.DefaultToolsLocation
	sync.DefaultToolsLocation = stor.Location()
	for _, vers := range vAll {
		stor.PutBinary(vers)
	}
	restore := func() {
		sync.DefaultToolsLocation = origLocation
	}
	return restore
}

// createToolsSource writes the mock tools into a temporary
// directory and returns it.
func createToolsSource(c *gc.C) string {
	source := c.MkDir()
	for _, vers := range vAll {
		data := vers.String()
		name := envtools.StorageName(vers)
		filename := filepath.Join(source, name)
		dir := filepath.Dir(filename)
		err := os.MkdirAll(dir, 0755)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(filename, []byte(data), 0666)
		c.Assert(err, gc.IsNil)
	}
	return source
}

// makeEmptyFakeHome creates a faked home without envtools.
func makeEmptyFakeHome(c *gc.C) (environs.Environ, *coretesting.FakeHome) {
	fake := coretesting.MakeFakeHome(c, envConfig)
	dummy.Reset()
	env, err := environs.PrepareFromName("peckham")
	c.Assert(err, gc.IsNil)
	envtesting.RemoveAllTools(c, env)
	return env, fake
}

// checkTools check if the environment contains the passed envtools.
func checkTools(c *gc.C, env environs.Environ, expected []version.Binary) {
	list, err := envtools.FindTools(
		env, version.Current.Major, version.Current.Minor, coretools.Filter{}, envtools.DoNotAllowRetry)
	c.Check(err, gc.IsNil)
	c.Logf("found: " + list.String())
	urls := list.URLs()
	c.Check(urls, gc.HasLen, len(expected))
}

var (
	v100d64 = version.MustParseBinary("1.0.0-raring-amd64")
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v120d64 = version.MustParseBinary("1.2.0-raring-amd64")
	v120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	v120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	v120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	v100All = []version.Binary{
		v100d64, v100p64, v100q64, v100q32,
	}
	v120All = []version.Binary{
		v120d64, v120p64, v120q64, v120q32,
	}
	vAll = []version.Binary{
		v100d64, v100p64, v100q32, v100q64,
		v120d64, v120p64, v120q32, v120q64,
		v190p32, v190q64,
		v200p64,
	}
)
