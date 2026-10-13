package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RPCClient struct {
	rpcURL     string
	user       string
	pass       string
	httpClient *http.Client
}

type NodeSyncStatus struct {
	Status       string `json:"status"` // One of SYNCED, SYNCING, or OFFLINE.
	DaemonHeight uint64 `json:"daemon_height"`
	TargetHeight uint64 `json:"target_height"`
}

func NewRPCClient(url, user, pass string) *RPCClient {
	return &RPCClient{
		rpcURL: url,
		user:   user,
		pass:   pass,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CallRPC sends a JSON-RPC 2.0 request and rejects explicit Monero RPC errors.
func (r *RPCClient) CallRPC(ctx context.Context, method string, params interface{}, out interface{}) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "0",
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", r.rpcURL+"/json_rpc", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.user != "" {
		req.SetBasicAuth(r.user, r.pass)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rpc connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc HTTP error code: %d", resp.StatusCode)
	}

	var rawResp struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
		return fmt.Errorf("failed to decode RPC response: %w", err)
	}

	// Never interpret an explicit Monero RPC error as a zero-value result.
	if rawResp.Error != nil {
		return fmt.Errorf("monero RPC Error [%d]: %s", rawResp.Error.Code, rawResp.Error.Message)
	}

	if out != nil && len(rawResp.Result) > 0 {
		return json.Unmarshal(rawResp.Result, out)
	}

	return nil
}

func (r *RPCClient) GetBalance() (uint64, uint64, error) {
	var result struct {
		Balance         uint64 `json:"balance"`
		UnlockedBalance uint64 `json:"unlocked_balance"`
	}
	err := r.CallRPC(context.Background(), "get_balance", map[string]interface{}{"account_index": 0}, &result)
	if err != nil {
		return 0, 0, err
	}
	return result.Balance, result.UnlockedBalance, nil
}

func (r *RPCClient) CreateSubaddress(ctx context.Context) (string, uint64, error) {
	var result struct {
		Address      string `json:"address"`
		AddressIndex uint64 `json:"address_index"`
	}
	params := map[string]interface{}{"account_index": 0, "label": "Hosted Invoice Payment"}
	err := r.CallRPC(ctx, "create_address", params, &result)
	if err != nil {
		return "", 0, err
	}
	return result.Address, result.AddressIndex, nil
}

type TransferItem struct {
	TxID          string
	Amount        uint64
	Confirmations uint64
	SubaddrIndex  uint64
}

// GetAllIncomingTransfers fetches both confirmed incoming transfers and zero-confirmation pool transfers.
func (r *RPCClient) GetAllIncomingTransfers() ([]TransferItem, error) {
	params := map[string]interface{}{
		"in":            true,
		"pool":          true,
		"account_index": 0,
	}

	var result struct {
		In []struct {
			TxID          string `json:"txid"`
			Amount        uint64 `json:"amount"`
			Confirmations uint64 `json:"confirmations"`
			SubaddrIndex  struct {
				Minor uint64 `json:"minor"`
			} `json:"subaddr_index"`
		} `json:"in"`
		Pool []struct {
			TxID         string `json:"txid"`
			Amount       uint64 `json:"amount"`
			SubaddrIndex struct {
				Minor uint64 `json:"minor"`
			} `json:"subaddr_index"`
		} `json:"pool"`
	}

	err := r.CallRPC(context.Background(), "get_transfers", params, &result)
	if err != nil {
		return nil, err
	}

	var items []TransferItem
	for _, tx := range result.In {
		items = append(items, TransferItem{
			TxID:          tx.TxID,
			Amount:        tx.Amount,
			Confirmations: tx.Confirmations,
			SubaddrIndex:  tx.SubaddrIndex.Minor,
		})
	}
	for _, tx := range result.Pool {
		items = append(items, TransferItem{
			TxID:          tx.TxID,
			Amount:        tx.Amount,
			Confirmations: 0,
			SubaddrIndex:  tx.SubaddrIndex.Minor,
		})
	}

	return items, nil
}

// GetSyncStatus returns the current Monero daemon height and synchronization state.
func (r *RPCClient) GetSyncStatus(ctx context.Context) (NodeSyncStatus, error) {
	var res struct {
		Height       uint64 `json:"height"`
		TargetHeight uint64 `json:"target_height"`
	}
	err := r.CallRPC(ctx, "get_height", nil, &res)
	if err != nil {
		return NodeSyncStatus{Status: "OFFLINE"}, err
	}

	status := "SYNCED"
	if res.TargetHeight > 0 && res.Height < res.TargetHeight {
		status = "SYNCING"
	}

	return NodeSyncStatus{
		Status:       status,
		DaemonHeight: res.Height,
		TargetHeight: res.TargetHeight,
	}, nil
}
