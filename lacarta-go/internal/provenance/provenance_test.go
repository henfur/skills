package provenance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignVerify(t *testing.T) {
	k, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte(`{"ai_assisted": true}`)
	sig := k.Sign(msg)
	pub, err := Verify(msg, sig)
	if err != nil || !pub.Equal(k.Public) {
		t.Fatalf("verify: %v", err)
	}
	if _, err := Verify([]byte(`{"ai_assisted": false}`), sig); err == nil {
		t.Fatal("a changed manifest verified")
	}
}

func TestKeyIsReusedAndPrivate(t *testing.T) {
	dir := t.TempDir()
	a, _ := LoadOrCreate(dir)
	b, err := LoadOrCreate(dir)
	if err != nil || !a.Public.Equal(b.Public) {
		t.Fatalf("second load: %v", err)
	}
	fi, _ := os.Stat(filepath.Join(dir, "report-signing.pem"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", fi.Mode().Perm())
	}
}

// OpenSSH verifies what we sign: clients need nothing but ssh-keygen.
func TestOpenSSHVerifies(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not available")
	}
	dir := t.TempDir()
	k, _ := LoadOrCreate(dir)
	msg := []byte("manifest\n")
	signers := filepath.Join(dir, "allowed_signers")
	os.WriteFile(signers, []byte(AllowedSigners("Ada Auditor", k.Public)+"\n"), 0o644)
	sigFile := filepath.Join(dir, "m.sig")
	os.WriteFile(sigFile, []byte(k.Sign(msg)), 0o644)
	cmd := exec.Command("ssh-keygen", "-Y", "verify", "-f", signers, "-I", "Ada_Auditor", "-n", Namespace, "-s", sigFile)
	cmd.Stdin = strings.NewReader(string(msg))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	cmd = exec.Command("ssh-keygen", "-Y", "verify", "-f", signers, "-I", "Ada_Auditor", "-n", Namespace, "-s", sigFile)
	cmd.Stdin = strings.NewReader("changed\n")
	if err := cmd.Run(); err == nil {
		t.Fatal("ssh-keygen accepted a changed message")
	}
}
