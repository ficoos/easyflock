package easyflock

import (
	. "launchpad.net/gocheck"
	"testing"
	"syscall"
	"path/filepath"
	"os"
)

func Test(t *testing.T) { TestingT(t) }

type FlockTests struct{}
var _ = Suite(&FlockTests{})

func (s *FlockTests) TestBadFd(c *C) {
	_, err := NewFlock(-1)
	c.Assert(err, NotNil)
}

func (s *FlockTests) TestAnoymousFd(c *C) {
	_, err := NewFlock(1)
	c.Assert(err, IsNil)
}

func (s *FlockTests) TestExclusive(c *C) {
	l, err := NewFlock(1)
	c.Assert(err, IsNil)
	c.Assert(l.TryLock(), Equals, true)
	defer l.Unlock()
	c.Assert(l.TryLock(), Equals, false)
	c.Assert(l.TryRLock(), Equals, false)
}

func (s *FlockTests) TestShared(c *C) {
	l, err := NewFlock(1)
	c.Assert(err, IsNil)
	c.Assert(l.TryRLock(), Equals, true)
	defer l.Unlock()
	c.Assert(l.TryRLock(), Equals, true)
	defer l.Unlock()
	c.Assert(l.TryLock(), Equals, false)
	c.Assert(l.TryRLock(), Equals, true)
	defer l.Unlock()
}

func (s *FlockTests) TestBoth(c *C) {
	s.TestExclusive(c)
	s.TestShared(c)
	s.TestExclusive(c)
}

// Make sure that flock follows symlinks
func (s *FlockTests) TestSymlinks(c *C) {
	orig := "/etc/passwd"

	dir := c.MkDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	c.Assert(syscall.Symlink(orig, a), IsNil)
	c.Assert(syscall.Symlink(orig, b), IsNil)

	fa, err := os.Open(a)
	c.Assert(err, IsNil)
	defer fa.Close()
	fb, err := os.Open(b)
	c.Assert(err, IsNil)
	defer fb.Close()

	la, err := NewFlock(int(fa.Fd()))
	c.Assert(err, IsNil)
	lb, err := NewFlock(int(fb.Fd()))
	c.Assert(err, IsNil)

	c.Assert(la.TryLock(), Equals, true)
	defer la.Unlock()
	c.Assert(lb.TryLock(), Equals, false)
}
