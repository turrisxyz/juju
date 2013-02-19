package hook_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/worker/uniter/hook"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type InfoSuite struct{}

var _ = Suite(&InfoSuite{})

var validateTests = []struct {
	info hook.Info
	err  string
}{
	{
		hook.Info{Kind: hooks.RelationJoined},
		`"relation-joined" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hooks.RelationChanged},
		`"relation-changed" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hooks.RelationDeparted},
		`"relation-departed" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hooks.Kind("grok")},
		`unknown hook kind "grok"`,
	},
	{hook.Info{Kind: hooks.Install}, ""},
	{hook.Info{Kind: hooks.Start}, ""},
	{hook.Info{Kind: hooks.ConfigChanged}, ""},
	{hook.Info{Kind: hooks.UpgradeCharm}, ""},
	{hook.Info{Kind: hooks.Stop}, ""},
	{hook.Info{Kind: hooks.RelationJoined, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hooks.RelationBroken}, ""},
}

func (s *InfoSuite) TestValidate(c *C) {
	for i, t := range validateTests {
		c.Logf("test %d", i)
		err := t.info.Validate()
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}
