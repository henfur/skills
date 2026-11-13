// Package provenance signs audit reports so a client can check, without lacarta and without
// trusting the auditor, that a report is the one lacarta produced, unchanged, with its
// AI-assistance disclosure. Signatures use Ed25519 in OpenSSH's SSHSIG format, so
// `ssh-keygen -Y verify` checks them.
package provenance

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Namespace = "lacarta-report"

// Key is the machine's report-signing key.
type Key struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

// LoadOrCreate reads the key from dir (mode 0600), creating it on first use.
func LoadOrCreate(dir string) (*Key, error) {
	path := filepath.Join(dir, "report-signing.pem")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		block := pem.EncodeToMemory(&pem.Block{Type: "LACARTA ED25519 SEED", Bytes: priv.Seed()})
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(block); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		return &Key{priv, pub}, nil
	}
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil || len(block.Bytes) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a lacarta signing key", path)
	}
	priv := ed25519.NewKeyFromSeed(block.Bytes)
	return &Key{priv, priv.Public().(ed25519.PublicKey)}, nil
}

func sshString(b []byte) []byte {
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	return out
}

func pubBlob(pub ed25519.PublicKey) []byte {
	return append(sshString([]byte("ssh-ed25519")), sshString(pub)...)
}

// AuthorizedKey is the public key in OpenSSH's one-line format.
func AuthorizedKey(pub ed25519.PublicKey) string {
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(pubBlob(pub))
}

// Fingerprint is OpenSSH's SHA256 fingerprint of the key.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256Sum(pubBlob(pub))
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum), "=")
}

// AllowedSigners is a line for ssh-keygen's allowed_signers file.
func AllowedSigners(identity string, pub ed25519.PublicKey) string {
	return fmt.Sprintf("%s namespaces=%q %s", strings.ReplaceAll(identity, " ", "_"), Namespace, AuthorizedKey(pub))
}

func signedData(msg []byte) []byte {
	h := sha512.Sum512(msg)
	var b bytes.Buffer
	b.WriteString("SSHSIG")
	b.Write(sshString([]byte(Namespace)))
	b.Write(sshString(nil))
	b.Write(sshString([]byte("sha512")))
	b.Write(sshString(h[:]))
	return b.Bytes()
}

// Sign returns an armored SSHSIG signature of msg.
func (k *Key) Sign(msg []byte) string {
	sig := ed25519.Sign(k.Private, signedData(msg))
	var b bytes.Buffer
	b.WriteString("SSHSIG")
	_ = binary.Write(&b, binary.BigEndian, uint32(1))
	b.Write(sshString(pubBlob(k.Public)))
	b.Write(sshString([]byte(Namespace)))
	b.Write(sshString(nil))
	b.Write(sshString([]byte("sha512")))
	b.Write(sshString(append(sshString([]byte("ssh-ed25519")), sshString(sig)...)))
	enc := base64.StdEncoding.EncodeToString(b.Bytes())
	var out strings.Builder
	out.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for len(enc) > 70 {
		out.WriteString(enc[:70] + "\n")
		enc = enc[70:]
	}
	out.WriteString(enc + "\n-----END SSH SIGNATURE-----\n")
	return out.String()
}

func readString(b []byte) ([]byte, []byte, error) {
	if len(b) < 4 {
		return nil, nil, errors.New("truncated signature")
	}
	n := binary.BigEndian.Uint32(b)
	if uint32(len(b)-4) < n {
		return nil, nil, errors.New("truncated signature")
	}
	return b[4 : 4+n], b[4+n:], nil
}

// Verify checks an armored SSHSIG signature of msg and returns the signing public key.
func Verify(msg []byte, armored string) (ed25519.PublicKey, error) {
	body := strings.TrimSpace(armored)
	body = strings.TrimPrefix(body, "-----BEGIN SSH SIGNATURE-----")
	body = strings.TrimSuffix(body, "-----END SSH SIGNATURE-----")
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body), ""))
	if err != nil || !bytes.HasPrefix(raw, []byte("SSHSIG")) || len(raw) < 10 {
		return nil, errors.New("not an SSH signature")
	}
	rest := raw[10:] // magic and version
	var pub, ns, hashAlg, sig []byte
	if pub, rest, err = readString(rest); err != nil {
		return nil, err
	}
	if ns, rest, err = readString(rest); err != nil {
		return nil, err
	}
	if _, rest, err = readString(rest); err != nil {
		return nil, err
	}
	if hashAlg, rest, err = readString(rest); err != nil {
		return nil, err
	}
	if sig, _, err = readString(rest); err != nil {
		return nil, err
	}
	if string(ns) != Namespace || string(hashAlg) != "sha512" {
		return nil, fmt.Errorf("signature for %q/%s, not a lacarta report", ns, hashAlg)
	}
	keyType, keyRest, err := readString(pub)
	if err != nil || string(keyType) != "ssh-ed25519" {
		return nil, errors.New("not an Ed25519 key")
	}
	pk, _, err := readString(keyRest)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return nil, errors.New("bad public key")
	}
	sigType, sigRest, err := readString(sig)
	if err != nil || string(sigType) != "ssh-ed25519" {
		return nil, errors.New("not an Ed25519 signature")
	}
	s, _, err := readString(sigRest)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pk, signedData(msg), s) {
		return nil, errors.New("the signature doesn't match: the file was changed after signing")
	}
	return ed25519.PublicKey(pk), nil
}
