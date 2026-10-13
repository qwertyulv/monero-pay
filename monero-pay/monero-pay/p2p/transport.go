package p2p

// Transport intentionally defines the boundary for the next protocol stage.
// Discovery/relay in this first version never treats the relay as a trusted
// counterparty. A matched peer should establish an authenticated, encrypted
// session here before exchanging private payment details.
type Transport interface {
	Dial(peerAddress string) error
}
