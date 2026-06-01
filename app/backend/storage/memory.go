// Здесь будет написана структура, которая реализует интерфес Storage для хранения данных в памяти (ОЗУ).
package storage

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("not found")
)

// memoryGame хранит данные игры и участников
type memoryGame struct {
	ID      string
	Payload interface{}
	Members map[string]struct{}
}

// memoryStorage реализует Storage
type memoryStorage struct {
	mu          sync.RWMutex
	gameCounter int64
	users       map[string]interface{}
	usersByName map[string]string // name -> id
	games       map[string]*memoryGame
}

// NewMemoryStorage создает новый in-memory storage
func NewMemoryStorage() Storage {
	return &memoryStorage{
		users:       make(map[string]interface{}),
		usersByName: make(map[string]string),
		games:       make(map[string]*memoryGame),
	}
}

// helper: try to extract name from a stored user object (map[string]interface{} or struct with Name field is not handled here)
func extractName(user interface{}) string {
	if user == nil {
		return ""
	}
	// try map[string]interface{}
	if m, ok := user.(map[string]interface{}); ok {
		if v, ok := m["name"].(string); ok {
			return v
		}
		if v, ok := m["Name"].(string); ok {
			return v
		}
	}
	// try map[string]string
	if ms, ok := user.(map[string]string); ok {
		if v, ok := ms["name"]; ok {
			return v
		}
		if v, ok := ms["Name"]; ok {
			return v
		}
	}
	return ""
}

// ---------------- UserStorage methods ----------------

func (m *memoryStorage) CreateUser(user interface{}) (string, error) {
	id := uuid.New().String()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[id] = user
	if name := extractName(user); name != "" {
		m.usersByName[name] = id
	}
	return id, nil
}

func (m *memoryStorage) GetUser(id string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (m *memoryStorage) GetUserByName(name string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.usersByName[name]
	if !ok {
		return nil, ErrNotFound
	}
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (m *memoryStorage) UpdateUser(id string, user interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[id]; !ok {
		return ErrNotFound
	}
	// remove old name index if existed
	// naive: rebuild usersByName mapping for this id
	for k, v := range m.usersByName {
		if v == id {
			delete(m.usersByName, k)
			break
		}
	}
	m.users[id] = user
	if name := extractName(user); name != "" {
		m.usersByName[name] = id
	}
	return nil
}

func (m *memoryStorage) DeleteUser(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[id]; !ok {
		return ErrNotFound
	}
	// remove from users map
	delete(m.users, id)
	// remove name index if present
	for k, v := range m.usersByName {
		if v == id {
			delete(m.usersByName, k)
			break
		}
	}
	// remove from any game members
	for _, g := range m.games {
		if _, ok := g.Members[id]; ok {
			delete(g.Members, id)
		}
	}
	return nil
}

// ---------------- GamesStorage methods ----------------

func (m *memoryStorage) CreateGame(game interface{}) (string, error) {
	id := fmt.Sprintf("%06d", atomic.AddInt64(&m.gameCounter, 1))
	m.mu.Lock()
	defer m.mu.Unlock()
	m.games[id] = &memoryGame{ID: id, Payload: game, Members: make(map[string]struct{})}
	return id, nil
}

func (m *memoryStorage) GetGame(id string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.games[id]
	if !ok {
		return nil, ErrNotFound
	}
	return g.Payload, nil
}

func (m *memoryStorage) ListGames() ([]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]interface{}, 0, len(m.games))
	for _, g := range m.games {
		out = append(out, g.Payload)
	}
	return out, nil
}

func (m *memoryStorage) UpdateGame(id string, game interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.games[id]
	if !ok {
		return ErrNotFound
	}
	g.Payload = game
	return nil
}

func (m *memoryStorage) DeleteGame(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.games[id]; !ok {
		return ErrNotFound
	}
	delete(m.games, id)
	return nil
}

func (m *memoryStorage) ValidateGameID(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.games[id]
	return ok
}

func (m *memoryStorage) JoinGame(gameID string, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.games[gameID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := m.users[userID]; !ok {
		return ErrNotFound
	}
	g.Members[userID] = struct{}{}
	return nil
}

func (m *memoryStorage) LeaveGame(gameID string, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.games[gameID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := g.Members[userID]; !ok {
		return ErrNotFound
	}
	delete(g.Members, userID)
	return nil
}
