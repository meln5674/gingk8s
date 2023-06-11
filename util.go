package gingk8s

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
)

var (
	randPortLock = make(chan struct{}, 1)
)

// GetRandomPort returns a port that is not currently in use
func GetRandomPort() int {
	return WithRandomPort[int](func(port int) int { return port })
}

// WithRandomPort calls a function with a port that is not currently in use
func WithRandomPort[T any](f func(int) T) T {
	return WithRandomPorts[T](1, func(ports []int) T { return f(ports[0]) })
}

// WithRandomPort calls a function a set of ports that are not currently in use
func WithRandomPorts[T any](count int, f func([]int) T) T {
	randPortLock <- struct{}{}
	defer func() { <-randPortLock }()

	listeners := make([]net.Listener, count)
	ports := make([]int, count)
	for ix := 0; ix < count; ix++ {

		listener, err := net.Listen("tcp", ":0")
		Expect(err).ToNot(HaveOccurred())
		defer listener.Close()

		listeners[ix] = listener
		ports[ix] = listener.Addr().(*net.TCPAddr).Port
	}
	for _, listener := range listeners {
		listener.Close()
	}

	return f(ports)
}

func tryLock(dir string) (bool, error) {
	err := os.Mkdir(dir, 0700)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func lock(dir string, poll time.Duration) error {
	for {
		locked, err := tryLock(dir)
		if err != nil {
			return err
		}
		if locked {
			return nil
		}
		time.Sleep(poll)
	}
}

func unlock(dir string) error {
	return os.Remove(dir)
}

func reversePath(offsetPath string, then ...string) string {
	dirParts := strings.Split(offsetPath, string(filepath.Separator))
	for ix := range dirParts {
		dirParts[ix] = ".."
	}
	dirParts = append(dirParts, then...)
	return filepath.Join(dirParts...)
}

// MultiSuiteLock allows for locking between multiple concurrent suites
type MultiSuiteLock struct {
	// LockDir is a directory that meets the following criteria:
	// * Must not exist before any suites execute
	// * Parent directory must exist
	// * Test user must have permissions to create
	// * Relative to module root
	LockDir string
}

func (m *MultiSuiteLock) fullLockPath(offsetPath string) string {
	return reversePath(offsetPath, m.LockDir)
}

// WithLock allows only one suite to execute a section at once.
// offsetPath must be the relative path from the module root to the test's package
func (m *MultiSuiteLock) WithLock(offsetPath string, poll time.Duration, f func() error) error {
	lockPath := m.fullLockPath(offsetPath)
	err := lock(lockPath, poll)
	if err != nil {
		return err
	}
	defer func() {
		unlockErr := unlock(lockPath)
		if err == nil {
			err = unlockErr
		}
	}()
	return f()
}

// Once attempts to create the lock, and only the suite executes the provided function, all other suites do nothing.
// Each call to Once requires its own Once
// offsetPath must be the relative path from the module root to the test's package
func (m *MultiSuiteLock) Once(offsetPath string, f func(unlock func() error) error) (bool, error) {
	lockPath := m.fullLockPath(offsetPath)
	locked, err := tryLock(lockPath)
	if err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}

	return true, f(func() error { return unlock(lockPath) })
}
