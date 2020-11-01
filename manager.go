package astiws

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type ClientAdapter func(c *Client)

type Manager struct {
	clients    map[interface{}]*Client
	mutex      *sync.RWMutex
	Upgrader   websocket.Upgrader
}

type ManagerConfiguration struct {
	MaxMessageSize int `toml:"max_message_size"`
}

func NewManager(c ManagerConfiguration) *Manager {
	return &Manager{
		clients: make(map[interface{}]*Client),
		mutex:   &sync.RWMutex{},
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  c.MaxMessageSize,
			WriteBufferSize: c.MaxMessageSize,
		},
	}
}

func (m *Manager) AutoRegisterClient(c *Client) {
	m.RegisterClient(c, c)
	return
}

func (m *Manager) Client(k interface{}) (c *Client, ok bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	c, ok = m.clients[k]
	return
}

// executes a function on every client. It stops if an error is returned.
func (m *Manager) Clients(fn func(k interface{}, c *Client) error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	for k, c := range m.clients {
		if err := fn(k, c); err != nil {
			return
		}
	}
}

func (m *Manager) Close() error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for k, c := range m.clients {
		_ = c.Close()
		delete(m.clients, k)
	}
	return nil
}

func (m *Manager) CountClients() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.clients)
}

// loops in clients and execute a function for each of them
func (m *Manager) Loop(fn func(k interface{}, c *Client)) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, c := range m.clients {
		fn(k, c)
	}
}

func (m *Manager) RegisterClient(k interface{}, c *Client) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.clients[k] = c
}

// handles an HTTP request and returns an error unlike an http.Handler
// We don't want to register the client yet, since we may want to index the map of clients with an information we don't
// have yet
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request, adapter ClientAdapter) (err error) {
	var client = NewClient(ClientConfiguration{
		MaxMessageSize: m.Upgrader.WriteBufferSize,
	})
	if client.conn, err = m.Upgrader.Upgrade(w, r, nil); err != nil {
		err = fmt.Errorf("upgrading conn failed: %w", err)
		return
	}

	adapter(client)

	if err = client.Read(); err != nil {
		err = fmt.Errorf("reading failed: %w", err)
		return
	}
	return
}

// astiws.disconnected event is a good place to call this function
func (m *Manager) UnregisterClient(k interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.clients, k)
}
