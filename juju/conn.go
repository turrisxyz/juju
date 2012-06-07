package juju

import (
	"launchpad.net/juju-core/juju/environs"
	"launchpad.net/juju-core/juju/state"
	"regexp"
	"sync"
)

var (
	ValidService = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	ValidUnit    = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
)

// Conn holds a connection to a juju.
type Conn struct {
	Environ environs.Environ
	state   *state.State
	mu      sync.Mutex
}

// NewConn returns a Conn pointing at the environName environment, or the
// default environment if not specified.
func NewConn(environName string) (*Conn, error) {
	environs, err := environs.ReadEnvirons("")
	if err != nil {
		return nil, err
	}
	environ, err := environs.Open(environName)
	if err != nil {
		return nil, err
	}
	return &Conn{Environ: environ}, nil
}

// Bootstrap initializes the Conn's environment and makes it ready to deploy
// services.
func (c *Conn) Bootstrap(uploadTools bool) error {
	return c.Environ.Bootstrap(uploadTools)
}

// Destroy destroys the Conn's environment and all its instances.
func (c *Conn) Destroy() error {
	return c.Environ.Destroy(nil)
}

// State returns the environment state associated with c. Closing the
// obtained state will have undefined consequences; Close c instead.
func (c *Conn) State() (*state.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil {
		info, err := c.Environ.StateInfo()
		if err != nil {
			return nil, err
		}
		st, err := state.Open(info)
		if err != nil {
			return nil, err
		}
		c.state = st
	}
	return c.state, nil
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state
	c.state = nil
	if state != nil {
		return state.Close()
	}
	return nil
}
