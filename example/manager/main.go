package main

import (
	"context"
	"flag"
	"net/http"

	"encoding/json"

	"github.com/julienschmidt/httprouter"
	"github.com/molotovtv/go-astilog"
	"github.com/molotovtv/go-astiws"
)

// Constants
const (
	contextKeyManager = "manager"
)

// Flags
var (
	addr = flag.String("a", "localhost:4000", "addr")
)

func main() {
	// Init
	flag.Parse()
	astilog.FlagInit()

	// Init manager
	var m = astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 1024})
	defer m.Close()

	// Init router
	var r = httprouter.New()
	r.GET("/", Handle)

	// Serve
	astilog.Debugf("Listening and serving on %s", *addr)
	if err := http.ListenAndServe(*addr, AdaptHandler(r, m)); err != nil {
		astilog.Fatal(err)
	}
}

// Handle returns the handler
func Handle(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Retrieve manager
	var m = ManagerFromContext(r.Context())

	// Serve
	if err := m.ServeHTTP(rw, r, AdaptClient); err != nil {
		astilog.Error(err)
		return
	}
}

// AdaptHandle adapts a handler
func AdaptHandler(h http.Handler, m *astiws.Manager) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.Background())
		r = r.WithContext(NewContextWithManager(r.Context(), m))
		h.ServeHTTP(rw, r)
	})
}

// NewContextWithManager creates a context with the manager
func NewContextWithManager(ctx context.Context, m *astiws.Manager) context.Context {
	return context.WithValue(ctx, contextKeyManager, m)
}

// ManagerFromContext retrieves the manager from the context
func ManagerFromContext(ctx context.Context) *astiws.Manager {
	if l, ok := ctx.Value(contextKeyManager).(*astiws.Manager); ok {
		return l
	}
	return &astiws.Manager{}
}

// AdaptClient adapts a client
func AdaptClient(c *astiws.Client) {
	// Set up listeners
	c.SetListener("asticode", HandleAsticode)
}

// HandleAsticode handles asticode events
func HandleAsticode(c *astiws.Client, eventName string, payload json.RawMessage) (err error) {
	// Unmarshal payload
	var b bool
	if errUnmarshal := json.Unmarshal(payload, &b); errUnmarshal != nil {
		astilog.Errorf("%s while unmarshaling payload %s", err, string(payload))
		return
	}

	// Answer
	if b {
		if err = c.Write("asticoded", 2); err != nil {
			astilog.Error(err)
			return
		}
	} else {
		astilog.Error("Payload should be true")
	}
	return
}
