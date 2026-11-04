package bundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// GenerateKeys writes an ed25519 key pair. The private key file is created with mode 0600.
func GenerateKeys(privPath, pubPath string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.WriteFile(privPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)
}

// Sign returns the detached signature of the bundle bytes, base64 encoded.
func Sign(data []byte, privPath string) (string, error) {
	priv, err := readKey(privPath, ed25519.PrivateKeySize)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), data)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// Verify checks a detached signature against the public key file. It fails closed.
func Verify(data []byte, sigB64 string, pubPath string) error {
	pub, err := readKey(pubPath, ed25519.PublicKeySize)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return errors.New("signature file is not base64")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return errors.New("signature does not match the bundle")
	}
	return nil
}

func readKey(path string, size int) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("key file %s is not base64", path)
	}
	if len(key) != size {
		return nil, fmt.Errorf("key file %s has %d bytes, expected %d", path, len(key), size)
	}
	return key, nil
}
