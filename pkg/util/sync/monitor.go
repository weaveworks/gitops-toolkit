package sync

import (
	"errors"
	"sync"
)

// Monitor is a convenience wrapper around
// starting a goroutine with a wait group,
// which can be used to wait for the
// goroutine to stop.
type Monitor struct {
	wg  *sync.WaitGroup
	err error
}

func RunMonitor(f func() error) *Monitor {
	m := &Monitor{
		wg: new(sync.WaitGroup),
	}

	m.wg.Add(1)
	go func() {
		m.err = f()
		m.wg.Done()
	}()

	return m
}

func (m *Monitor) Wait() error {
	// TODO: Do we need this check?
	if m == nil {
		return errors.New("Monitor: invalid null pointer to m")
	}
	// TODO: maybe this could be easier implemented using just a channel?
	m.wg.Wait()
	return m.err
}
