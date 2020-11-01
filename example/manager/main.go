package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"

	"github.com/molotovtv/go-astiws"
)

const (
	contextKeyManager = "manager"
)

var (
	addr = flag.String("a", "localhost:4000", "addr")
)

func main() {
	flag.Parse()

	var m = astiws.NewManager(astiws.ManagerConfiguration{MaxMessageSize: 1024})
	defer func() { _ = m.Close() }()

	r := http.NewServeMux()
	r.HandleFunc("/", handle)

	if err := http.ListenAndServe(*addr, adaptHandler(r, m)); err != nil {
		panic(err)
	}
}

func handle(rw http.ResponseWriter, r *http.Request) {
	var m = managerFromContext(r.Context())

	if err := m.ServeHTTP(rw, r, adaptClient); err != nil {
		return
	}
}

func adaptHandler(h http.Handler, m *astiws.Manager) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.Background())
		r = r.WithContext(newContextWithManager(r.Context(), m))
		h.ServeHTTP(rw, r)
	})
}

func newContextWithManager(ctx context.Context, m *astiws.Manager) context.Context {
	return context.WithValue(ctx, contextKeyManager, m)
}

func managerFromContext(ctx context.Context) *astiws.Manager {
	if l, ok := ctx.Value(contextKeyManager).(*astiws.Manager); ok {
		return l
	}
	return &astiws.Manager{}
}

func adaptClient(c *astiws.Client) {
	c.SetListener("asticode", handleAsticodeEvents)
}

func handleAsticodeEvents(c *astiws.Client, _ string, payload json.RawMessage) error {
	var b bool

	if errUnmarshal := json.Unmarshal(payload, &b); errUnmarshal != nil {
		return nil
	}

	if b {
		if err := c.Write("asticoded", 2); err != nil {
			return err
		}
	}

	return nil
}
