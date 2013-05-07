// A library for more intuitive flocking
//
// flock(2) command is implemented per file table entry.
// This is not as intuitive and problematic trying to flock similar files from
// multiple threads.
package easyflock

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
)

type _FileID struct {
	Dev uint64
	Ino uint64
}

type _FlockMap map[_FileID]*_Flock

type _FlockCache struct {
	mtx   sync.Mutex
	cache _FlockMap
}

var _flockCache *_FlockCache

type _LockState int

const (
	_LS_SHARED    = _LockState(syscall.LOCK_SH)
	_LS_EXCLUSIVE = _LockState(syscall.LOCK_EX)
	_LS_UNLOCKED  = _LockState(syscall.LOCK_UN)
)

type _Flock struct {
	mtx      sync.Mutex
	fd       int
	state    _LockState
	refcount int
	users    int
	fid      _FileID
}

func (f *_Flock) TryLock() bool {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.state != _LS_UNLOCKED {
		return false
	}

	err := syscall.Flock(f.fd, syscall.LOCK_NB|syscall.LOCK_EX)
	if err != nil {
		return false
	}

	f.state = _LS_EXCLUSIVE
	return true
}

func (f *_Flock) TryRLock() bool {
	f.mtx.Lock()
	defer f.mtx.Unlock()

	if f.state == _LS_EXCLUSIVE {
		return false
	}

	if f.state != _LS_SHARED {
		err := syscall.Flock(f.fd, syscall.LOCK_NB|syscall.LOCK_SH)
		if err != nil {
			return false
		}
	}
	f.users++
	f.state = _LS_SHARED
	return true
}

func (f *_Flock) Unlock() {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	switch f.state {
	case _LS_UNLOCKED:
		panic(fmt.Errorf("Tried to unlock an already unlocked Flock"))
	case _LS_SHARED:
		f.users--
		if f.users > 0 {
			return
		}
	}
	err := syscall.Flock(f.fd, syscall.LOCK_NB|syscall.LOCK_UN)
	if err != nil {
		panic(err)
	}
	f.state = _LS_UNLOCKED
}

func (f *_Flock) incref() {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.refcount++
}

func (f *_Flock) Close() {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	f.refcount--

	_flockCache.mtx.Lock()
	defer _flockCache.mtx.Unlock()

	if f.refcount > 0 {
		return
	}

	err := syscall.Close(f.fd)
	if err != nil {
		panic(err)
	}
	f.fd = -1
	delete(_flockCache.cache, f.fid)

	runtime.SetFinalizer(f, nil)
}

/* Flock is a wrapper around a flock "instance"
 * since flocks are assosciated with a file table entry and not an fd we need
 * to make sure that a user can only close it's instance of the lock once
 */

type Flock struct {
	*_Flock
	closed int32
}

func (f *Flock) Close() {
	if atomic.CompareAndSwapInt32(&f.closed, 0, 1) {
		f._Flock.Close()
		// For GC
		f._Flock = nil
	}
}

func getFid(fd int) (_FileID, error) {
	fdi := int(fd)
	var stat syscall.Stat_t
	err := syscall.Fstat(fdi, &stat)
	if err != nil {
		return _FileID{}, err
	}

	return _FileID{stat.Dev, stat.Ino}, nil
}

func wrapFlock(flock *_Flock) *Flock {
	flock.incref()
	res := &Flock{_Flock: flock}
	// Makes sure the file is freed if the user forgets to close the
	// instance
	runtime.SetFinalizer(res, func(f *Flock) {
		f.Close()
	})

	return res
}

func NewFlock(fd int) (*Flock, error) {
	fid, err := getFid(fd)
	if err != nil {
		return nil, err
	}

	_flockCache.mtx.Lock()
	defer _flockCache.mtx.Unlock()

	flock, ok := _flockCache.cache[fid]
	if ok {
		return wrapFlock(flock), nil
	}

	nfd, err := syscall.Dup(fd)
	if err != nil {
		return nil, err
	}

	flock = &_Flock{
		fid:   fid,
		state: _LS_UNLOCKED,
		fd:    nfd}

	return wrapFlock(flock), nil
}

func init() {
	_flockCache = &_FlockCache{
		cache: make(_FlockMap)}

	f, _ := NewFlock(2);
	f.TryLock();
}
