package main

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"
)

type AddressPool struct {
	db        *sql.DB
	rpcClient *RPCClient
	minSize   int
	isFilling bool
	fillMu    sync.Mutex
}

func NewAddressPool(db *sql.DB, rpc *RPCClient, minSize int) *AddressPool {
	return &AddressPool{
		db:        db,
		rpcClient: rpc,
		minSize:   minSize,
	}
}

func (ap *AddressPool) InitAndMaintain() {
	go func() {
		for {
			ap.replenishPool()
			time.Sleep(30 * time.Second)
		}
	}()
}

func (ap *AddressPool) replenishPool() {
	ap.fillMu.Lock()
	if ap.isFilling {
		ap.fillMu.Unlock()
		return
	}
	ap.isFilling = true
	ap.fillMu.Unlock()

	defer func() {
		ap.fillMu.Lock()
		ap.isFilling = false
		ap.fillMu.Unlock()
	}()

	var count int
	err := ap.db.QueryRow(`SELECT COUNT(*) FROM address_pool WHERE is_used = 0`).Scan(&count)
	if err != nil {
		return
	}

	needed := ap.minSize - count
	if needed <= 0 {
		return
	}

	log.Printf("[AddressPool] Replenishing %d pre-generated addresses...", needed)

	for i := 0; i < needed; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		addr, index, err := ap.rpcClient.CreateSubaddress(ctx)
		cancel()

		if err != nil {
			log.Printf("[AddressPool Error] Create address failed: %v", err)
			break
		}
		_, err = ap.db.Exec(`INSERT OR IGNORE INTO address_pool (address_index, address, is_used) VALUES (?, ?, 0)`, index, addr)
		if err != nil {
			break
		}
	}
}

func (ap *AddressPool) GetUnusedAddress() (string, uint64, error) {
	tx, err := ap.db.Begin()
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback()

	var index uint64
	var addr string
	// Atomically reserve an unused address before returning it to the caller.
	err = tx.QueryRow(`SELECT address_index, address FROM address_pool WHERE is_used = 0 LIMIT 1`).Scan(&index, &addr)
	if err != nil {
		// Fall back to creating a fresh RPC subaddress when the pool is empty.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return ap.rpcClient.CreateSubaddress(ctx)
	}

	_, err = tx.Exec(`UPDATE address_pool SET is_used = 1 WHERE address_index = ?`, index)
	if err != nil {
		return "", 0, err
	}

	if err := tx.Commit(); err != nil {
		return "", 0, err
	}

	return addr, index, nil
}
