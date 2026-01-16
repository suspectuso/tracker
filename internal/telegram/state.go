package telegram

import "sync"

// UserState represents the current state of a user's conversation
type UserState struct {
	State string
	Data  map[string]interface{}
}

// StateManager manages user states for FSM
type StateManager struct {
	mu     sync.RWMutex
	states map[int64]*UserState
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	return &StateManager{
		states: make(map[int64]*UserState),
	}
}

// Set sets a user's state
func (sm *StateManager) Set(userID int64, state string, data map[string]interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if data == nil {
		data = make(map[string]interface{})
	}
	sm.states[userID] = &UserState{
		State: state,
		Data:  data,
	}
}

// Get returns a user's current state
func (sm *StateManager) Get(userID int64) *UserState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.states[userID]
}

// Clear removes a user's state
func (sm *StateManager) Clear(userID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.states, userID)
}

// State constants
const (
	StateWaitName     = "wait_name"
	StateWaitAddress  = "wait_address"
	StateWaitMinAmount = "wait_min_amount"
)
