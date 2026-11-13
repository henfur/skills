package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lacarta/internal/core"
)

// Live updates: the console subscribes to /api/events (server-sent events). The state files are
// the source of truth and any process may write them, so a watcher compares their modification
// times and sizes; it runs only while someone is subscribed.

// WatchEvery is how often the watcher looks at the state files.
var WatchEvery = 500 * time.Millisecond

type event struct{ name, data string }

type broker struct {
	mu      sync.Mutex
	subs    map[chan event]struct{}
	running bool
	closed  chan struct{}
}

func newBroker() *broker {
	return &broker{subs: map[chan event]struct{}{}, closed: make(chan struct{})}
}

func (b *broker) subscribe() (chan event, func()) {
	ch := make(chan event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	if !b.running {
		b.running = true
		go b.watch()
	}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// close ends every stream (server shutdown).
func (b *broker) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
}

func (b *broker) publish(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // a slow client misses a notice; it re-fetches on the next one
		}
	}
}

type stamp struct {
	mod  time.Time
	size int64
}

func stat(path string) stamp {
	info, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	return stamp{info.ModTime(), info.Size()}
}

// snapshot stamps the hub and the settings, and every project's state file.
func snapshot() (hub [2]stamp, projects map[string]stamp) {
	hub = [2]stamp{stat(filepath.Join(core.Home(), "hub.json")), stat(filepath.Join(core.Home(), "settings.json"))}
	projects = map[string]stamp{}
	if hb, err := core.HubRead(); err == nil {
		for _, id := range hb.Projects.Keys() {
			if s, err := core.ProjectStore(id); err == nil {
				st := stat(s.Path())
				if p, _ := hb.Projects.Get(id); p.Template == core.Audit {
					// The agents write the skill's run files directly: they change the audit too.
					runs, _ := filepath.Glob(filepath.Join(s.Dir, "run-*", "*.json"))
					for _, f := range runs {
						x := stat(f)
						if x.mod.After(st.mod) {
							st.mod = x.mod
						}
						st.size += x.size
					}
				}
				projects[id] = st
			}
		}
	}
	return hub, projects
}

func (b *broker) watch() {
	hub, projects := snapshot()
	t := time.NewTicker(WatchEvery)
	defer t.Stop()
	for {
		select {
		case <-b.closed:
			return
		case <-t.C:
		}
		b.mu.Lock()
		if len(b.subs) == 0 {
			b.running = false
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		h, ps := snapshot()
		changed := h != hub
		for id, s := range ps {
			if projects[id] != s {
				b.publish(event{"project", id})
				changed = true // the hub's summaries come from the projects' state
			}
		}
		if changed || len(ps) != len(projects) {
			b.publish(event{"hub", ""})
		}
		hub, projects = h, ps
	}
}

func (b *broker) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "Streaming unsupported.")
		return
	}
	ch, cancel := b.subscribe()
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-b.closed:
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		case e := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data)
		}
		flusher.Flush()
	}
}
