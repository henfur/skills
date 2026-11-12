package hooks

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"lacarta/internal/core"
)

// Listen is `lacarta listen`: the agent runs it as a background command, and Claude Code wakes
// the agent when it exits. It returns as soon as the user writes from the console, answers a
// question or uses "Talk about this", and prints what happened. The terminal stays free meanwhile.
//
// One listener per project: a newer one takes over and the older one exits quietly. The
// listener's pid is recorded, so a killed listener stops counting at once.
func Listen(s *core.Store, w io.Writer, timeout time.Duration) error {
	me := os.Getpid()
	var off bool
	answered, threads := map[string]bool{}, map[string]int{}
	err := s.Edit(func(st *core.State) error {
		if !core.RemoteOn(st) {
			off = true
			return nil
		}
		since := core.Now()
		if st.Listening != nil && st.Listening.Since != "" {
			since = st.Listening.Since
		}
		st.Listening = &core.Listening{Since: since, PID: me}
		for _, q := range st.Questions {
			if q.Status != "open" {
				answered[q.ID] = true
			}
			threads[q.ID] = len(q.Thread)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if off {
		fmt.Fprintln(w, "Remote control is off for this project: there is nothing to listen for. Don't start the listener again.")
		return nil
	}
	again := fmt.Sprintf("\n\nWhen you have handled this, start `%s listen` again as a background command.", Self)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigs)
	// Whatever ends the wait, a listener that is still the registered one unregisters.
	defer func() {
		_ = s.Edit(func(st *core.State) error {
			if st.Listening != nil && st.Listening.PID == me {
				st.Listening.PID = 0
			}
			return nil
		})
	}()
	var deadline <-chan time.Time
	if timeout > 0 {
		deadline = time.After(timeout)
	}
	tick := time.NewTicker(PollEvery)
	defer tick.Stop()
	for {
		select {
		case <-sigs:
			return nil
		case <-deadline:
			fmt.Fprintln(w, "Nothing from the console yet."+again)
			return nil
		case <-tick.C:
		}
		st, err := s.Read()
		if err != nil {
			return err
		}
		if st.Listening == nil || st.Listening.PID != me {
			fmt.Fprintln(w, "Another listener took over for this project. Don't start a new one.")
			return nil
		}
		if !core.RemoteOn(st) {
			fmt.Fprintln(w, "The user turned remote control off for this project. Don't start the listener again.")
			return nil
		}
		if st.Stage == "ended" {
			return nil
		}
		if len(core.PendingMessages(st)) > 0 {
			var msg string
			if err := s.Edit(func(st *core.State) error {
				msg = core.TakeMessages(st)
				a := core.MustAgent(st, core.Orchestrator)
				a.Status, a.UpdatedAt = "working", core.Now()
				return nil
			}); err != nil {
				return err
			}
			if msg != "" { // a hook may have delivered it a moment earlier
				fmt.Fprintln(w, msg+again)
				return nil
			}
			continue
		}
		news := []string{}
		for _, q := range st.Questions {
			if !answered[q.ID] && q.Status == "answered" {
				ans, _ := core.PyDumpsValue(q.Answer)
				news = append(news, fmt.Sprintf("%s (%s) was answered: %s", q.ID, q.Title, ans))
			} else if core.Discussing(q) && len(q.Thread) > threads[q.ID] {
				news = append(news, fmt.Sprintf("On %s (%s) the user chose \"Talk about this\" and wrote: %s\nReply with "+
					"`reply %s --message ...`; it is not answered.", q.ID, q.Title, q.Thread[len(q.Thread)-1].Text, q.ID))
			}
		}
		if len(news) > 0 {
			fmt.Fprintln(w, core.ConsoleTag+" Activity in the lacarta console (a background `wait` may report the same "+
				"thing; it is the same event):\n\n"+strings.Join(news, "\n\n")+again)
			return nil
		}
	}
}
