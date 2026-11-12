// Package server is the console daemon: one per host, serving every project's state to the
// console page and applying the user's actions through the same state code as the CLI.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"lacarta/internal/core"
	"lacarta/internal/hooks"
	"lacarta/web"
)

var loopback = map[string]bool{"127.0.0.1": true, "localhost": true, "::1": true}
var wildcard = map[string]bool{"0.0.0.0": true, "::": true, "": true}

func urlHost(h string) string {
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}

// ------------------------------------------------------------------ payloads

type roleOut struct {
	Label   string `json:"label"`
	Stage   string `json:"stage"`
	About   string `json:"about"`
	Default string `json:"default"`
}

type kindOut struct {
	Label string             `json:"label"`
	About string             `json:"about"`
	Roles core.OMap[roleOut] `json:"roles"`
}

func catalog() any {
	var models core.OMap[core.Model]
	for _, m := range core.Models {
		models.Set(m.Alias, m)
	}
	var kinds core.OMap[kindOut]
	for _, k := range core.Kinds {
		ko := kindOut{Label: k.Label, About: k.About}
		for _, r := range k.Roles {
			ko.Roles.Set(r.ID, roleOut{r.Label, r.Stage, r.About, r.Default})
		}
		kinds.Set(k.ID, ko)
	}
	return map[string]any{"models": models, "planned": core.PlannedKinds, "kinds": kinds}
}

type questionSummary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	AskedBy string `json:"asked_by"`
	Triage  bool   `json:"triage"`
	Replies int    `json:"replies"`
}

type agentSummary struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Avatar json.RawMessage `json:"avatar"`
}

type projectSummary struct {
	ID             string            `json:"id"`
	Template       string            `json:"template"`
	Name           string            `json:"name"`
	Where          string            `json:"where"`
	Archived       bool              `json:"archived"`
	Error          string            `json:"error,omitempty"`
	Stage          string            `json:"stage,omitempty"`
	Status         string            `json:"status,omitempty"`
	Agent          *agentSummary     `json:"agent,omitempty"`
	Session        bool              `json:"session"`
	Questions      []questionSummary `json:"questions"`
	Attention      *core.Attention   `json:"attention"`
	Listening      bool              `json:"listening"`
	ListeningSince *string           `json:"listening_since"`
	Blocked        *string           `json:"blocked"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
	MovedTo        *string           `json:"moved_to"`
}

// summary is what the home page and the notifications need, cheap enough for every project.
func summary(pid string, e *core.HubProject) projectSummary {
	out := projectSummary{ID: pid, Template: e.Template, Name: e.Name, Where: e.Root, Archived: e.Archived,
		Questions: []questionSummary{}}
	if out.Where == "" {
		out.Where = e.Cwd
	}
	if out.Name == "" {
		out.Name = pid
	}
	s, err := core.ProjectStore(pid)
	var st *core.State
	if err == nil {
		st, err = s.Read()
	}
	if err != nil {
		out.Error = "State unreadable: " + err.Error()
		return out
	}
	a := core.MustAgent(st, core.Orchestrator)
	names := map[string]string{}
	for _, x := range st.Agents {
		names[x.ID] = x.Name
	}
	for _, q := range core.OpenQuestions(st) {
		replies := 0
		for _, m := range q.Thread {
			if m.By != "user" {
				replies++
			}
		}
		out.Questions = append(out.Questions, questionSummary{q.ID, q.Title, names[q.AskedBy],
			q.Effect != nil && q.Effect.Type == "finding-triage", replies})
	}
	out.Name, out.Stage, out.Status = st.Project.Name, st.Stage, core.AgentStatus(st, a)
	out.Agent = &agentSummary{a.ID, a.Name, a.Avatar}
	out.Session = a.Session != nil && a.Session.EndedAt == nil
	out.Attention, out.Listening = st.Attention, core.IsListening(st)
	if out.Listening {
		out.ListeningSince = &st.Listening.Since
	}
	if st.Template == core.Software {
		if b := core.BlockedReason(st); b != "" {
			out.Blocked = &b
		}
	}
	out.UpdatedAt = st.Project.CreatedAt
	if n := len(st.Activity); n > 0 {
		out.UpdatedAt = st.Activity[n-1].At
	}
	if st.MovedTo != "" {
		out.MovedTo = &st.MovedTo
	}
	return out
}

type projectPayload struct {
	ID      string             `json:"id"`
	State   *core.State        `json:"state"`
	Derived *core.Derived      `json:"derived"`
	Specs   *map[string]string `json:"specs,omitempty"` // software projects only, even when empty
	Reports *map[string]string `json:"reports,omitempty"`
	Chat    []hooks.ChatItem   `json:"chat"`
}

func payload(pid string) (*projectPayload, error) {
	s, err := core.ProjectStore(pid)
	if err != nil {
		return nil, err
	}
	st, err := s.Read()
	if err != nil {
		return nil, err
	}
	d, err := core.DeriveState(st, s)
	if err != nil {
		return nil, err
	}
	out := &projectPayload{ID: pid, State: st, Derived: d}
	if st.Template == core.Software {
		specs, reports := map[string]string{}, map[string]string{}
		out.Specs, out.Reports = &specs, &reports
		for _, k := range core.Specs {
			b, err := os.ReadFile(core.SpecPath(s, k.K))
			if err != nil {
				return nil, err
			}
			specs[k.K] = string(b)
		}
		dir, _ := filepath.Abs(s.Dir)
		for _, r := range st.Reviews {
			p, _ := filepath.Abs(filepath.Join(s.Root, r.Report))
			if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
				if b, err := os.ReadFile(p); err == nil {
					reports[r.ID] = string(b)
				}
			}
		}
	}
	out.Chat = []hooks.ChatItem{}
	if a := core.MustAgent(st, core.Orchestrator); a.Session != nil {
		out.Chat = hooks.ReadChat(a.Session.Transcript, 400)
	}
	return out, nil
}

// ------------------------------------------------------------------ handler

type Config struct {
	Token      string
	Port       int
	ExtraHosts []string
	RequireKey bool // bound beyond loopback: every request needs the key
	CheckHost  bool
}

// Handler serves the console and its API.
//
// On loopback the page is open and only changes need the token. Bound to any other address,
// every request but the vendored scripts needs the token too (from `?key=` on the page URL,
// then the X-Lacarta-Token header), since anyone on the network can reach it.
func Handler(c Config) http.Handler {
	allowed := map[string]bool{}
	for h := range loopback {
		allowed[urlHost(h)+":"+strconv.Itoa(c.Port)] = true
	}
	for _, h := range c.ExtraHosts {
		allowed[urlHost(h)+":"+strconv.Itoa(c.Port)] = true
	}
	page, _ := web.Legacy.ReadFile("legacy/index.html")
	html := strings.ReplaceAll(string(page), "__LACARTA_TOKEN__", c.Token)
	keyOK := func(v string) bool {
		return !c.RequireKey || subtle.ConstantTimeCompare([]byte(v), []byte(c.Token)) == 1
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' "+
			"'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; connect-src 'self'; "+
			"img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		// Refuse DNS rebinding: only the host names we were started for are served.
		if c.CheckHost && !allowed[r.Host] {
			fail(w, 403, "Host not allowed. Add it with `serve --allow-host`.")
			return
		}
		switch r.Method {
		case http.MethodGet:
			get(w, r, html, keyOK)
		case http.MethodPost:
			post(w, r, c.Token)
		default:
			fail(w, 405, "Method not allowed.")
		}
	})
}

func send(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	b, _ := json.Marshal(v)
	_, _ = w.Write(b)
}

func fail(w http.ResponseWriter, code int, msg string) {
	send(w, code, map[string]string{"error": msg})
}

// projectPath parses /api/p/<id>/<action>; the id must be in the hub.
func projectPath(path string) (pid, action string) {
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[1] != "api" || parts[2] != "p" {
		return "", ""
	}
	h, err := core.HubRead()
	if err != nil {
		return "", ""
	}
	if _, ok := h.Projects.Get(parts[3]); !ok {
		return "", ""
	}
	return parts[3], parts[4]
}

func get(w http.ResponseWriter, r *http.Request, html string, keyOK func(string) bool) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/vendor/") {
		b, err := web.Legacy.ReadFile("legacy" + path)
		if err != nil || strings.Contains(path, "..") {
			fail(w, 404, "Not found.")
			return
		}
		// Vendored libraries never change for a given release; everything else is live state.
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write(b)
		return
	}
	if path == "/" {
		if !keyOK(r.URL.Query().Get("key")) {
			fail(w, 403, "Missing or wrong key. Open the URL `serve` printed.")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, html)
		return
	}
	if !keyOK(r.Header.Get("X-Lacarta-Token")) {
		fail(w, 403, "Missing or wrong key. Open the URL `serve` printed.")
		return
	}
	if path == "/api/hub" {
		h, err := core.HubRead()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		ps := []projectSummary{}
		for _, k := range h.Projects.Keys() {
			e, _ := h.Projects.Get(k)
			ps = append(ps, summary(k, e))
		}
		send(w, 200, map[string]any{"projects": ps, "settings": core.SettingsRead(), "catalog": catalog()})
		return
	}
	if pid, action := projectPath(path); pid != "" && action == "state" {
		p, err := payload(pid)
		if err != nil {
			fail(w, 500, "Cannot read this project: "+err.Error())
			return
		}
		send(w, 200, p)
		return
	}
	fail(w, 404, "Not found.")
}

func post(w http.ResponseWriter, r *http.Request, token string) {
	// Changes must come from the console page itself (same origin as the Host served).
	if r.Header.Get("Origin") != "http://"+r.Host {
		fail(w, 403, "Origin not allowed.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Lacarta-Token")), []byte(token)) != 1 {
		fail(w, 403, "Missing or wrong token. Reload the page.")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "JSON only.")
		return
	}
	if r.ContentLength > 64_000 {
		fail(w, 413, "Too long.")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 64_000)).Decode(&body); err != nil {
		fail(w, 400, "Malformed request.")
		return
	}
	core.Actor = "" // everything the console records comes from the user
	str := func(k string, max int) string {
		s, _ := body[k].(string)
		if r := []rune(s); max > 0 && len(r) > max {
			s = string(r[:max])
		}
		return s
	}
	if r.URL.Path == "/api/settings" {
		models := map[string]map[string]string{}
		if raw, ok := body["models"]; ok && raw != nil {
			m, ok := raw.(map[string]any)
			if !ok {
				fail(w, 400, "Malformed request.")
				return
			}
			for k, v := range m {
				roles, ok := v.(map[string]any)
				if !ok {
					fail(w, 400, "Malformed request.")
					return
				}
				models[k] = map[string]string{}
				for role, alias := range roles {
					models[k][role] = fmt.Sprint(alias)
				}
			}
		}
		var remote *bool
		if v, ok := body["remote_control"].(bool); ok {
			remote = &v
		}
		s, err := core.SettingsUpdate(models, remote)
		if err != nil {
			gateOr500(w, err)
			return
		}
		send(w, 200, map[string]any{"ok": true, "settings": s})
		return
	}
	pid, action := projectPath(r.URL.Path)
	if pid == "" {
		fail(w, 404, "Not found.")
		return
	}
	if action == "archive" {
		on, _ := body["archived"].(bool)
		err := core.HubEdit(func(h *core.Hub) error {
			p, _ := h.Projects.Get(pid)
			p.Archived = on
			return nil
		})
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		send(w, 200, map[string]any{"ok": true})
		return
	}
	s, err := core.ProjectStore(pid)
	if err != nil {
		gateOr500(w, err)
		return
	}
	var result any
	err = s.Edit(func(st *core.State) error {
		var err error
		switch action {
		case "answer":
			result, err = core.AnswerQuestion(st, str("id", 0), str("choice", 0), str("comment", 20_000), "user")
		case "discuss":
			result, err = core.DiscussQuestion(st, str("id", 0), str("message", 20_000))
		case "message":
			result, err = core.AddMessage(st, str("message", 20_000))
		case "remote":
			if on, ok := body["on"].(bool); ok {
				st.Remote = &on
			} else {
				st.Remote = nil
			}
			state := "off"
			if core.RemoteOn(st) {
				state = "on"
			}
			core.Log(st, "Remote control "+state+".", "chat")
			result = map[string]bool{"remote": core.RemoteOn(st)}
		default:
			return errNotFound
		}
		return err
	})
	if errors.Is(err, errNotFound) {
		fail(w, 404, "Not found.")
		return
	}
	if err != nil {
		gateOr500(w, err)
		return
	}
	send(w, 200, map[string]any{"ok": true, "result": result})
}

var errNotFound = errors.New("not found")

func gateOr500(w http.ResponseWriter, err error) {
	if core.IsGate(err) {
		fail(w, 409, err.Error())
		return
	}
	fail(w, 500, err.Error())
}

// ------------------------------------------------------------------ daemon

// Serve runs the console until interrupted. It prints the URL if one already runs.
func Serve(host string, port int, allowHosts []string, out io.Writer) error {
	if info := core.DaemonRead(); info != nil {
		fmt.Fprintf(out, "lacarta console already running: %s\n", info.OpenURL)
		return nil
	}
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	exposed, wild := !loopback[host], wildcard[host]
	extra := append([]string{}, allowHosts...)
	if exposed && !wild {
		extra = append(extra, host)
	}
	// A wildcard bind answers on every interface, so the Host check needs the names to expect;
	// without them the key alone guards the console.
	check := !wild || len(allowHosts) > 0
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	shown := urlHost(host)
	if wild {
		shown = "<this-machine's-address>"
		if len(allowHosts) > 0 {
			shown = urlHost(allowHosts[0])
		}
	}
	u := fmt.Sprintf("http://%s:%d/", shown, port)
	open := u
	if exposed {
		open += "?key=" + url.QueryEscape(token)
	}
	if err := os.MkdirAll(core.Home(), 0o755); err != nil {
		return err
	}
	infoPath := filepath.Join(core.Home(), "daemon.json")
	info := core.DaemonInfo{URL: u, OpenURL: open, PID: os.Getpid(), StartedAt: core.Now()}
	if err := core.WriteJSON(infoPath, info); err != nil {
		return err
	}
	_ = os.Chmod(infoPath, 0o600) // it can hold the key
	defer func() {
		if cur := core.DaemonRead(); cur != nil && cur.PID == os.Getpid() {
			_ = os.Remove(infoPath)
		}
	}()
	fmt.Fprintf(out, "lacarta console: %s\n", open)
	if exposed {
		fmt.Fprintln(out, "Reachable from the network. Keep the key private: anyone holding the URL can answer for you. "+
			"Traffic is plain HTTP; prefer a trusted network or an SSH tunnel.")
	}
	srv := &http.Server{Handler: Handler(Config{Token: token, Port: port, ExtraHosts: extra, RequireKey: exposed, CheckHost: check}),
		ReadHeaderTimeout: 10 * time.Second}
	ctx, cancel := signalContext()
	defer cancel()
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// EnsureDaemon starts the console in the background if none runs (one per host).
func EnsureDaemon(self string) *core.DaemonInfo {
	if info := core.DaemonRead(); info != nil || os.Getenv("LACARTA_NO_DAEMON") != "" { // tests run without a console
		return info
	}
	if err := os.MkdirAll(core.Home(), 0o755); err != nil {
		return nil
	}
	logf, err := os.OpenFile(filepath.Join(core.Home(), "daemon.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	defer logf.Close()
	cmd := exec.Command(self, "serve")
	cmd.Dir, cmd.Stdout, cmd.Stderr = core.Home(), logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if cmd.Start() != nil {
		return nil
	}
	_ = cmd.Process.Release()
	for range 50 {
		time.Sleep(100 * time.Millisecond)
		if info := core.DaemonRead(); info != nil {
			return info
		}
	}
	return nil
}
