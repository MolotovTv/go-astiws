package astiws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	EventNameDisconnect = "astiws.disconnect"
	PingPeriod          = (pingWait * 9) / 10
	pingWait            = 60 * time.Second
)

type ListenerFunc func(c *Client, eventName string, payload json.RawMessage) error

type Client struct {
	c          ClientConfiguration
	chanDone   chan bool
	conn       *websocket.Conn
	listeners  map[string][]ListenerFunc
	mutex      *sync.RWMutex
	errHandler func(err error)
}

type ClientConfiguration struct {
	MaxMessageSize int `toml:"max_message_size"`
}

type ClientOption func(client *Client)

func WithErrHandler(errHandler func(err error)) ClientOption {
	return func(client *Client) {
		client.errHandler = errHandler
	}
}

func NewClient(c ClientConfiguration, options ...ClientOption) *Client {
	client := &Client{
		c:          c,
		listeners:  make(map[string][]ListenerFunc),
		mutex:      &sync.RWMutex{},
		errHandler: func(_ error) {},
	}

	for _, opt := range options {
		opt(client)
	}

	return client
}

func (c *Client) Close() (err error) {
	if c.conn != nil {
		// Send a close frame and wait for the server to respond.
		if err = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
			err = fmt.Errorf("astiws: sending close frame failed %w", err)
			return
		}
		if c.chanDone != nil {
			<-c.chanDone
			c.chanDone = nil
		}
	}
	return
}

func (c *Client) Dial(addr string) error {
	return c.DialWithHeaders(addr, nil)
}

func (c *Client) DialWithHeaders(addr string, h http.Header) error {
	if c.conn != nil {
		_ = c.conn.Close()
	}

	if conn, _, err := websocket.DefaultDialer.Dial(addr, h); err != nil {
		return fmt.Errorf("dialing %s failed: %w", addr, err)
	} else {
		c.conn = conn
		return nil
	}
}

// represents the body of a message for read purposes
// Indeed when reading the body, we need the payload to be a json.RawMessage
type BodyMessageRead struct {
	BodyMessage
	Payload json.RawMessage `json:"payload"`
}

// writes a ping message in the connection
func (c *Client) ping(chanStop chan bool) {
	var t = time.NewTicker(PingPeriod)
	defer t.Stop()
	for {
		select {
		case <-chanStop:
			return
		case <-t.C:
			c.mutex.Lock()
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				c.errHandler(fmt.Errorf("sending ping message failed: %w", err))
			}
			c.mutex.Unlock()
		}
	}
}

func (c *Client) HandlePing() error {
	return c.conn.SetReadDeadline(time.Now().Add(pingWait))
}

func (c *Client) Read() (err error) {
	chanStopPing := make(chan bool)
	c.chanDone = make(chan bool)
	defer func() {
		close(c.chanDone)
		close(chanStopPing)
		_ = c.conn.Close()
		c.conn = nil
		_ = c.executeListeners(EventNameDisconnect, json.RawMessage{})
	}()

	// Update conn
	if c.c.MaxMessageSize > 0 {
		c.conn.SetReadLimit(int64(c.c.MaxMessageSize))
	}
	_ = c.HandlePing()
	c.conn.SetPingHandler(func(string) error { return c.HandlePing() })

	go c.ping(chanStopPing)

	for {
		var m []byte
		if _, m, err = c.conn.ReadMessage(); err != nil {
			err = fmt.Errorf("reading message failed: %w", err)
			return
		}

		var b BodyMessageRead
		if err = json.Unmarshal(m, &b); err != nil {
			err = fmt.Errorf("unmarshaling message failed: %w", err)
			return
		}

		// Execute listener callbacks
		_ = c.executeListeners(b.EventName, b.Payload)
	}
}

func (c *Client) executeListeners(eventName string, payload json.RawMessage) (err error) {
	if fs, ok := c.listeners[eventName]; ok {
		for _, f := range fs {
			if err = f(c, eventName, payload); err != nil {
				err = fmt.Errorf("executing listener for event %s failed: %w", eventName, err)
				return
			}
		}
	}
	return
}

type BodyMessage struct {
	EventName string      `json:"event_name"`
	Payload   interface{} `json:"payload"`
}

func (c *Client) Write(eventName string, payload interface{}) (err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.conn == nil {
		return fmt.Errorf("astiws: connection is not set for astiws client %p", c)
	}

	var b []byte
	if b, err = json.Marshal(BodyMessage{EventName: eventName, Payload: payload}); err != nil {
		err = fmt.Errorf("marshaling message failed: %w", err)
		return
	}

	if err = c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		err = fmt.Errorf("writing message failed: %w", err)
		return
	}
	return
}

func (c *Client) AddListener(eventName string, f ListenerFunc) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.listeners[eventName] = append(c.listeners[eventName], f)
}

func (c *Client) DelListener(eventName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.listeners, eventName)
}

func (c *Client) SetListener(eventName string, f ListenerFunc) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.listeners[eventName] = []ListenerFunc{f}
}
