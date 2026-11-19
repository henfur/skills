package cli

import (
	"os"

	"lacarta/internal/core"
	"lacarta/internal/hooks"
)

func trackCommands() []*Cmd {
	return []*Cmd{
		{Name: "track", Help: "Show this session in the console, and every later session in this folder: run it from the " +
			"session (the agent can), or from a terminal in the folder for the next sessions.", Run: cmdTrack,
			Opts: []Opt{{Name: "name", Help: "The task's name, saved in .lacarta/config.json (default: the git repository's or the folder's name)."}}},
	}
}

func cmdTrack(a *Args) error {
	cwd, _ := os.Getwd()
	if core.FindRoot(cwd) != "" || core.AuditFor(cwd) != "" {
		return core.Gate("This folder is already a lacarta project: its sessions join the console on their own.")
	}
	sid := os.Getenv("LACARTA_SESSION")
	if h, err := core.HubRead(); err == nil && sid != "" {
		if pid, ok := h.Sessions.Get(sid); ok {
			a.Printf("This session is already in the console as %s.\n", pid)
			return nil
		}
	}
	if sid == "" && core.ProjectConfigPath(core.Single, &core.Store{Root: cwd}) == "" {
		return core.Gate("Nothing to remember in the home folder: run `track` from the session itself to add it.")
	}
	if id := core.FolderID(cwd); sid == "" && id != "" {
		a.Printf("Already tracked: sessions started in %s join the console as %s.\n", core.LocalRoot(cwd), id)
		return nil
	}
	var pid string
	var s *core.Store
	var err error
	if sid != "" {
		pid, s, err = core.RegisterSession(sid, cwd, os.Getenv("LACARTA_TRANSCRIPT"), "startup")
	} else {
		pid, s, err = core.CreateSingle(cwd, "")
	}
	if err != nil {
		return err
	}
	if name := a.Str("name"); name != "" {
		st, err := s.Read()
		if err != nil {
			return err
		}
		if err := core.ProjectConfigSetName(st, name); err != nil {
			return err
		}
	}
	st, err := s.Read()
	if err != nil {
		return err
	}
	if sid == "" {
		a.Printf("Tracked: sessions started in %s now appear in the console as \"%s\" (%s).\n", core.LocalRoot(cwd), st.Project.Name, pid)
		return nil
	}
	hooks.EnsureDaemon()
	ctx, err := hooks.SessionContext(pid, s)
	if err != nil {
		return err
	}
	later := "Later sessions in " + core.LocalRoot(cwd) + " join it on their own."
	if core.ProjectConfigPath(core.Single, &core.Store{Root: cwd}) == "" {
		later = "Only this session: lacarta keeps nothing in the home folder."
	}
	a.Printf("Added this session to the console. %s\n\n%s\n", later, ctx)
	return nil
}
