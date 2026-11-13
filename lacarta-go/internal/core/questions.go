package core

import (
	"fmt"
	"strings"
)

func OpenQuestion(st *State, effect *Effect) *Question {
	for _, q := range st.Questions {
		if q.Status == "open" && q.Effect.Equal(effect) {
			return q
		}
	}
	return nil
}

func AddQuestion(st *State, kind, title, body string, options []string, effect *Effect, ctx Context,
	allowFree bool, help map[string]string) (*Question, error) {
	if effect != nil {
		if q := OpenQuestion(st, effect); q != nil {
			return q, nil
		}
	}
	asker := Orchestrator
	if a, err := ActorID(st); err != nil {
		return nil, err
	} else if a != nil {
		asker = *a
	}
	if options == nil {
		options = []string{}
	}
	if help == nil {
		help = map[string]string{}
	}
	q := &Question{
		ID: NextID(st, "Q"), Kind: kind, Title: title, Body: body, Options: options, OptionHelp: help,
		AllowFree: allowFree, Context: ctx, Effect: effect, Status: "open", AskedAt: Now(),
		AskedBy: asker, Thread: []*Msg{},
	}
	st.Questions = append(st.Questions, q)
	Log(st, fmt.Sprintf("%s asked: %s", q.ID, title), "question")
	return q, nil
}

func GetQuestion(st *State, id string) (*Question, error) {
	for _, q := range st.Questions {
		if q.ID == id {
			return q, nil
		}
	}
	return nil, Gate("No question %s.", id)
}

// ParseOptions turns `--option "Label|What it means"` values into labels and their help.
func ParseOptions(values []string) ([]string, map[string]string, error) {
	labels, help := []string{}, map[string]string{}
	for _, v := range values {
		label, text, _ := strings.Cut(v, "|")
		label = strings.TrimSpace(label)
		if label == "" {
			return nil, nil, Gate("Empty option label in %s.", pyStrRepr(v))
		}
		labels = append(labels, label)
		if t := strings.TrimSpace(text); t != "" {
			help[label] = t
		}
	}
	return labels, help, nil
}

// "Talk about this": the user can discuss an open question with the agent before answering it.
// The question stays open; the agent owes a reply while the user spoke last.

func Discussing(q *Question) bool {
	return q.Status == "open" && len(q.Thread) > 0 && q.Thread[len(q.Thread)-1].By == "user"
}

func DiscussQuestion(st *State, id, text string) (*Question, error) {
	q, err := GetQuestion(st, id)
	if err != nil {
		return nil, err
	}
	if q.Status != "open" {
		return nil, Gate("%s is already answered.", id)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, Gate("Write a message.")
	}
	q.Thread = append(q.Thread, &Msg{By: "user", Text: text, At: Now()})
	Log(st, fmt.Sprintf("%s: you asked the agent about it.", id), "question")
	return q, nil
}

func AnswerQuestion(st *State, id, choice, comment, by string) (*Question, error) {
	q, err := GetQuestion(st, id)
	if err != nil {
		return nil, err
	}
	if q.Status != "open" {
		return nil, Gate("%s is already answered.", id)
	}
	choice, comment = strings.TrimSpace(choice), strings.TrimSpace(comment)
	if len(q.Options) > 0 && !contains(q.Options, choice) {
		if !q.AllowFree || choice != "" || comment == "" {
			msg := "Choose one of " + PyStrList(q.Options)
			if q.AllowFree {
				msg += " or write an answer."
			} else {
				msg += "."
			}
			return nil, Gate("%s", msg)
		}
	}
	if len(q.Options) == 0 && comment == "" && choice == "" {
		return nil, Gate("Write an answer.")
	}
	if q.Effect != nil && q.Effect.Type == "finding-triage" && choice == TriageAccept && comment == "" {
		return nil, Gate("Accepting a high or critical risk needs a written rationale.")
	}
	q.Status = "answered"
	q.Answer = &Answer{Choice: choice, Comment: comment, By: by, At: Now()}
	decision, rationale := choice, ""
	if choice == "" {
		decision = comment
	} else {
		rationale = comment
	}
	d := &Decision{ID: NextID(st, "D"), Title: q.Title, Decision: decision, Rationale: rationale,
		Source: ptr(id), By: by, Phase: q.Context.Phase, At: Now()}
	if q.Context.Spec != "" {
		d.Spec = ptr(q.Context.Spec)
	}
	st.Decisions = append(st.Decisions, d)
	Log(st, fmt.Sprintf("%s answered: %s", id, decision), "decision")
	return q, applyEffect(st, q)
}

func applyEffect(st *State, q *Question) error {
	e := q.Effect
	if e == nil {
		return nil
	}
	choice, comment := q.Answer.Choice, q.Answer.Comment
	if strings.HasPrefix(e.Type, "audit-") {
		applyAuditEffect(st, q)
		return nil
	}
	switch e.Type {
	case "spec-approval":
		s := st.Spec(e.Spec)
		if choice == Approve {
			s.Status, s.ApprovedAt, s.ApprovedRevision = "approved", ptr(Now()), ptr(s.Revision)
		} else {
			s.Status = "changes-requested"
		}
		Log(st, fmt.Sprintf("%s spec %s.", SpecTitle(e.Spec), s.Status), "spec")
	case "plan-approval":
		if choice == Approve {
			st.Plan.Status, st.Plan.ApprovedAt = "approved", ptr(Now())
		} else {
			st.Plan.Status, st.Plan.ApprovedAt = "changes-requested", nil
		}
		Log(st, fmt.Sprintf("Phase plan %s.", st.Plan.Status), "plan")
	case "phase-approval":
		p, err := GetPhase(st, e.Phase.Num)
		if err != nil {
			return err
		}
		if choice == Approve {
			p.Status, p.CompletedAt = "done", ptr(Now())
			Log(st, fmt.Sprintf("Phase %d done.", p.ID), "phase")
		} else {
			p.Status = "in-progress"
			Log(st, fmt.Sprintf("Phase %d sent back: changes requested.", p.ID), "phase")
		}
	case "final-approval":
		if choice == Approve {
			st.Stage = "done"
			Log(st, "Final review approved. Project done.", "stage")
		} else {
			Log(st, "Final approval refused: changes requested.", "stage")
		}
	case "finding-triage":
		for _, f := range st.Findings {
			if f.ID != e.Finding {
				continue
			}
			switch choice {
			case TriageAccept:
				f.Status, f.Resolution, f.ResolvedAt = "accepted", comment, ptr(Now())
			case TriageRedesign:
				f.Status = "redesign"
			case TriageStop:
				st.Stage = "stopped"
			default:
				f.Status = "fixing"
			}
			what := choice
			if what == "" {
				what = comment
			}
			Log(st, fmt.Sprintf("%s triaged: %s.", f.ID, what), "finding")
		}
	}
	return nil
}

func Supersede(st *State, effect *Effect, why string) {
	if q := OpenQuestion(st, effect); q != nil {
		q.Status = "superseded"
		Log(st, fmt.Sprintf("%s withdrawn: %s", q.ID, why), "question")
	}
}

// WithdrawOpenQuestions: nobody is left to act on these answers.
func WithdrawOpenQuestions(st *State, why string) {
	for _, q := range st.Questions {
		if q.Status == "open" {
			q.Status = "superseded"
			Log(st, fmt.Sprintf("%s withdrawn: %s", q.ID, why), "question")
		}
	}
}

func OpenQuestions(st *State) []*Question {
	out := []*Question{}
	for _, q := range st.Questions {
		if q.Status == "open" {
			out = append(out, q)
		}
	}
	return out
}

// ---------------------------------------------------------------- inbox
// Messages typed in the console wait in the project's inbox until a hook hands them to the
// agent: after its next tool call, or when it ends its turn.

const ConsoleTag = "[lacarta console]"

func PendingMessages(st *State) []*InboxMsg {
	out := []*InboxMsg{}
	for _, m := range st.Inbox {
		if m.DeliveredAt == nil {
			out = append(out, m)
		}
	}
	return out
}

func AddMessage(st *State, text string) (*InboxMsg, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, Gate("Write a message.")
	}
	a, err := GetAgent(st, Orchestrator)
	if err != nil {
		return nil, err
	}
	if a.Session == nil || a.Session.EndedAt != nil {
		return nil, Gate("No agent session is connected to this project right now, so nothing would read it.")
	}
	m := &InboxMsg{ID: NextID(st, "M"), Text: text, At: Now()}
	st.Inbox = append(st.Inbox, m)
	if n := len(st.Inbox); n > 200 {
		st.Inbox = st.Inbox[n-200:]
	}
	Log(st, "You sent a message from the console.", "chat")
	return m, nil
}

// TakeMessages marks the pending messages delivered and returns them as one prompt ("" if none).
func TakeMessages(st *State) string {
	ms := PendingMessages(st)
	if len(ms) == 0 {
		return ""
	}
	texts := make([]string, len(ms))
	for i, m := range ms {
		m.DeliveredAt = ptr(Now())
		texts[i] = m.Text
	}
	return ConsoleTag + " The user wrote from the lacarta console. Treat it exactly as if they had typed it " +
		"here:\n\n" + strings.Join(texts, "\n\n")
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
