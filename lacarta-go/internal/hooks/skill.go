package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"lacarta/skill"
	"os"
	"path/filepath"
	"strings"
)

// skillMark is the file that tells lacarta a skill folder is its own: it holds the hash of the
// embedded skill it was written from. A folder without it (a link to a checkout, a copy the user
// edits) is never touched without --force.
const skillMark = ".lacarta-skill"

// SkillDir is where Claude Code finds the lacarta skill, beside the settings file.
func SkillDir(settings string) string {
	if settings == "" {
		settings = filepath.Join(claudeDir(), "settings.json")
	}
	return filepath.Join(filepath.Dir(settings), "skills", "lacarta")
}

func skillHash() string {
	h := sha256.New()
	_ = fs.WalkDir(skill.Agent, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := skill.Agent.ReadFile(p)
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// skillOwner says who owns the folder: "" (nothing there), "lacarta" (written by install, with the
// hash it was written from), or a sentence for the user about what is there instead.
func skillOwner(dir string) (owner, hash string) {
	info, err := os.Lstat(dir)
	if err != nil {
		return "", ""
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		to, _ := os.Readlink(dir)
		return fmt.Sprintf("%s is a link to %s", dir, to), ""
	}
	b, err := os.ReadFile(filepath.Join(dir, skillMark))
	if err != nil {
		return fmt.Sprintf("%s was not written by lacarta", dir), ""
	}
	return "lacarta", strings.TrimSpace(string(b))
}

func writeSkill(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	err := fs.WalkDir(skill.Agent, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := skill.Agent.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, skillMark), []byte(skillHash()+"\n"), 0o644)
}

// InstallSkill writes the embedded skill (or with uninstall, removes it). A folder lacarta didn't
// write is left alone unless force.
func InstallSkill(settings string, uninstall, force bool, w io.Writer) error {
	dir := SkillDir(settings)
	owner, _ := skillOwner(dir)
	switch {
	case owner != "" && owner != "lacarta" && !force:
		fmt.Fprintf(w, "The skill was not written: %s. lacarta leaves it as it is; pass --force to replace it.\n", owner)
		return nil
	case uninstall && owner == "":
		return nil
	case uninstall && owner != "lacarta":
		fmt.Fprintf(w, "The skill was not removed: %s.\n", owner)
		return nil
	case uninstall:
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Fprintf(w, "Removed the lacarta skill from %s.\n", dir)
		return nil
	}
	if owner != "" && owner != "lacarta" {
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) { // a link: only the link goes
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
	}
	if err := writeSkill(dir); err != nil {
		return err
	}
	fmt.Fprintf(w, "Installed the lacarta skill in %s.\n", dir)
	return nil
}

// SyncSkill rewrites an installed skill when this binary carries another version of it, so the
// instructions agents read always match the program that enforces them. Folders lacarta didn't
// write are left alone.
func SyncSkill() {
	dir := SkillDir("")
	if owner, hash := skillOwner(dir); owner == "lacarta" && hash != skillHash() {
		if err := writeSkill(dir); err != nil {
			logFailure("skill", err.Error())
		}
	}
}
