package p2p

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Peer is the persistent cryptographic identity of this P2P node.
// The private key is loaded from local storage and never transmitted to peers.
type Peer struct {
	ID         string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// LoadOrCreatePeer keeps the same identity across restarts. The key file is
// created with restrictive permissions and contains the raw Ed25519 private key encoded as hexadecimal.
func LoadOrCreatePeer(path string) (*Peer, error) {
	if path == "" {
		return nil, fmt.Errorf("peer key path is empty")
	}
	if data, err := os.ReadFile(path); err == nil {
		raw, err := hex.DecodeString(string(data))
		if err != nil || len(raw) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid peer key file")
		}
		priv := ed25519.PrivateKey(raw)
		pub := priv.Public().(ed25519.PublicKey)
		return &Peer{ID: hex.EncodeToString(pub), PublicKey: append(ed25519.PublicKey(nil), pub...), PrivateKey: append(ed25519.PrivateKey(nil), priv...)}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(priv)), 0600); err != nil {
		return nil, err
	}
	return &Peer{ID: hex.EncodeToString(pub), PublicKey: pub, PrivateKey: priv}, nil
}
