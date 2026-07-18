package deployment

import "sync"

type ApplicationLocks struct {
	mu     sync.Mutex
	locked map[int64]*applicationLease
}

type applicationLease struct{}

func NewApplicationLocks() *ApplicationLocks {
	return &ApplicationLocks{locked: make(map[int64]*applicationLease)}
}

// TryAcquire returns a release function when no deployment is running for the application.
// The returned function is idempotent and cannot release a newer lease for the same application.
func (l *ApplicationLocks) TryAcquire(applicationID int64) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.locked[applicationID]; exists {
		return nil, false
	}
	lease := &applicationLease{}
	l.locked[applicationID] = lease
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.locked[applicationID] == lease {
				delete(l.locked, applicationID)
			}
		})
	}, true
}
