package main

import (
	"flag"
	"math/rand"
	"sync"
	"time"

	"github.com/molotovtv/go-astiws"
)

var (
	clientsCount = flag.Int("c", 2, "number of clients")
	managerAddr  = flag.String("m", "ws://localhost:4000", "manager addr")
	sleepError   = flag.Duration("s", 10*time.Second, "sleep duration before retrying")
	ticker       = flag.Duration("t", 5*time.Second, "ticker duration")
)

func main() {
	flag.Parse()

	clients, mutex, wg := make(map[int]*astiws.Client), &sync.RWMutex{}, &sync.WaitGroup{}

	for i := 0; i < *clientsCount; i++ {
		wg.Add(1)

		go func(i int) {
			// Init client
			var c = astiws.NewClient(astiws.ClientConfiguration{MaxMessageSize: 1024})
			defer func() { _ = c.Close() }()

			// Add client
			mutex.Lock()
			clients[i] = c
			mutex.Unlock()
			wg.Done()

			// Infinite loop to handle reconnection
			var err error
			for {
				// Dial
				if err = c.Dial(*managerAddr); err != nil {
					time.Sleep(*sleepError)
					continue
				}

				// Read
				if err = c.Read(); err != nil {
					time.Sleep(*sleepError)
					continue
				}
			}
		}(i)
	}
	wg.Wait()

	// Send random messages
	var t = time.NewTicker(*ticker)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			// Get random client
			var c = clients[rand.Intn(len(clients))]

			// Random payload
			var b bool
			if rand.Intn(2) == 1 {
				b = true
			}

			// Send asticode event
			_ = c.Write("asticode", b)
		}
	}
}
