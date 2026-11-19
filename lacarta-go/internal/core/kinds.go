package core

// Behaviour is what one kind of project does its own way. Everything else is common to every
// kind: agents, questions (with "Talk about this"), decisions, activity, the inbox, the chat and
// remote control. The catalog entry (label, roles) is in Kinds; the CLI, the hooks and the console
// register their own per-kind parts (commands, session context, views) beside this.
type Behaviour interface {
	// Refresh advances the stage once its exit conditions are met. It runs after every write.
	Refresh(st *State, s *Store) error
	// Next is the `Next:` line, once the cases common to every kind are handled (a stopped
	// project, console messages, a question the user wants to talk about).
	Next(st *State, s *Store, waiting []*Question) (string, error)
	// Derive adds the kind's own fields to what the console and `status --json` receive.
	Derive(st *State, s *Store, d *Derived) error
}

var behaviours = map[string]Behaviour{
	Single:   singleKind{},
	Software: softwareKind{},
	Audit:    auditKind{},
	Desk:     deskKind{},
	Incident: incidentKind{},
}

// BehaviourOf is the behaviour of the project's kind; an unknown kind behaves as a single-agent task.
func BehaviourOf(st *State) Behaviour {
	if b, ok := behaviours[st.Template]; ok {
		return b
	}
	return singleKind{}
}

type singleKind struct{}

func (singleKind) Refresh(*State, *Store) error { return nil }
func (singleKind) Next(st *State, _ *Store, waiting []*Question) (string, error) {
	return singleNext(st, waiting), nil
}
func (singleKind) Derive(*State, *Store, *Derived) error { return nil }

type softwareKind struct{}

func (softwareKind) Refresh(st *State, s *Store) error { return specRefresh(st, s) }
func (softwareKind) Next(st *State, s *Store, waiting []*Question) (string, error) {
	return softwareNext(st, s, waiting)
}
func (softwareKind) Derive(st *State, s *Store, d *Derived) error { return softwareDerive(st, s, d) }

type auditKind struct{}

func (auditKind) Refresh(st *State, s *Store) error                       { return AuditRefresh(st, s) }
func (auditKind) Next(st *State, s *Store, _ []*Question) (string, error) { return AuditNext(st, s) }
func (auditKind) Derive(st *State, s *Store, d *Derived) error {
	d.Audit = DeriveAudit(st, s)
	return nil
}

// A desk or a case whose own record is missing (a damaged state file) falls back to the software
// project's rules, as it always did.

type deskKind struct{}

func (deskKind) Refresh(*State, *Store) error { return nil }
func (deskKind) Next(st *State, s *Store, waiting []*Question) (string, error) {
	if st.Desk == nil {
		return softwareNext(st, s, waiting)
	}
	return DeskNext(st), nil
}
func (deskKind) Derive(st *State, s *Store, d *Derived) error {
	if st.Desk == nil {
		return softwareDerive(st, s, d)
	}
	d.Desk = DeriveDesk(st)
	return nil
}

type incidentKind struct{}

func (incidentKind) Refresh(*State, *Store) error { return nil }
func (incidentKind) Next(st *State, s *Store, waiting []*Question) (string, error) {
	if st.Incident == nil {
		return softwareNext(st, s, waiting)
	}
	return IncidentNext(st), nil
}
func (incidentKind) Derive(st *State, s *Store, d *Derived) error {
	if st.Incident == nil {
		return softwareDerive(st, s, d)
	}
	d.Incident = DeriveIncident(st)
	return nil
}
