package notification

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// memRepo is an in-memory notification store for unit tests.
type memRepo struct {
	mu    sync.Mutex
	store map[string]*Notification
}

func newTestRepo(_ *testing.T) *memRepo {
	return &memRepo{store: make(map[string]*Notification)}
}

func (m *memRepo) Create(_ context.Context, n *Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	copy := *n
	m.store[n.ID] = &copy
	return nil
}

func (m *memRepo) GetByAccount(_ context.Context, accountID string, unreadOnly bool) ([]Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Notification
	for _, n := range m.store {
		if n.AccountID != accountID {
			continue
		}
		if unreadOnly && n.Read {
			continue
		}
		out = append(out, *n)
	}
	return out, nil
}

func (m *memRepo) GetUnreadCount(_ context.Context, accountID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, n := range m.store {
		if n.AccountID == accountID && !n.Read {
			count++
		}
	}
	return count, nil
}

func (m *memRepo) MarkRead(_ context.Context, notifID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.store[notifID]
	if !ok {
		return fmt.Errorf("notification %s not found", notifID)
	}
	n.Read = true
	return nil
}

func (m *memRepo) MarkAllRead(_ context.Context, accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.store {
		if n.AccountID == accountID {
			n.Read = true
		}
	}
	return nil
}
