package p2p

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	OfferSellXMR = "SELL_XMR"
	OfferBuyXMR  = "BUY_XMR"
)

// OfferPayload is intentionally a public matching envelope. It contains only
// fields needed for discovery and matching; private negotiation data belongs in a
// later end-to-end encrypted transport session.
type OfferPayload struct {
	Type            string `json:"type"`
	AmountAtomic    uint64 `json:"amount_atomic"`
	FiatAmountCents int64  `json:"fiat_amount_cents"`
	FiatCurrency    string `json:"fiat_currency"`
	PaymentType     string `json:"payment_type"`
}

type OfferBlob struct {
	OfferID   string `json:"offer_id"`
	PeerID    string `json:"peer_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

func NewOffer(peer *Peer, payload OfferPayload, ttl time.Duration) (OfferBlob, error) {
	if peer == nil {
		return OfferBlob{}, fmt.Errorf("peer is nil")
	}
	if payload.Type != OfferSellXMR && payload.Type != OfferBuyXMR {
		return OfferBlob{}, fmt.Errorf("invalid offer type")
	}
	if payload.AmountAtomic == 0 {
		return OfferBlob{}, fmt.Errorf("amount_atomic must be positive")
	}
	if ttl <= 0 {
		return OfferBlob{}, fmt.Errorf("offer ttl must be positive")
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return OfferBlob{}, err
	}
	now := time.Now().Unix()
	p, err := json.Marshal(payload)
	if err != nil {
		return OfferBlob{}, err
	}
	o := OfferBlob{OfferID: hex.EncodeToString(raw), PeerID: peer.ID, Payload: string(p), CreatedAt: now, ExpiresAt: now + int64(ttl.Seconds())}
	o.Signature = hex.EncodeToString(ed25519.Sign(peer.PrivateKey, o.signingBytes()))
	return o, nil
}

func (o OfferBlob) signingBytes() []byte {
	return []byte(fmt.Sprintf("monero-pay-offer-v1|%s|%s|%s|%d|%d", o.OfferID, o.PeerID, o.Payload, o.CreatedAt, o.ExpiresAt))
}

func (o OfferBlob) Verify(now time.Time) bool {
	pub, err := hex.DecodeString(o.PeerID)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := hex.DecodeString(o.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	if o.OfferID == "" || o.CreatedAt <= 0 || o.ExpiresAt <= o.CreatedAt || now.Unix() >= o.ExpiresAt {
		return false
	}
	var payload OfferPayload
	if err := json.Unmarshal([]byte(o.Payload), &payload); err != nil {
		return false
	}
	if payload.Type != OfferSellXMR && payload.Type != OfferBuyXMR || payload.AmountAtomic == 0 || payload.FiatAmountCents < 0 {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), o.signingBytes(), sig)
}

func (o OfferBlob) IDHash() string {
	sum := sha256.Sum256([]byte(o.OfferID))
	return hex.EncodeToString(sum[:])
}
