package blockchain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kpozdnikin/go-sui-test/app/internal/domain"
)

type SuiClient struct {
	rpcURL     string
	httpClient *http.Client
}

func NewSuiClient(rpcURL string) *SuiClient {
	return &SuiClient{
		rpcURL: rpcURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type JSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// GetBlockchainInfo fetches blockchain configuration from Chirp API
func (c *SuiClient) GetBlockchainInfo(ctx context.Context) (*domain.BlockchainInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://app.chirpwireless.io/api/public/blockchain-info", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching blockchain info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var info domain.BlockchainInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &info, nil
}

// QueryTransactionsByAddress queries transactions for a specific address
func (c *SuiClient) QueryTransactionsByAddress(ctx context.Context, address string, cursor *string, limit int) (*TransactionQueryResponse, error) {
	params := []interface{}{
		map[string]interface{}{
			"filter": map[string]interface{}{
				"FromAddress": address,
			},
			"options": map[string]interface{}{
				"showInput":          true,
				"showEffects":        true,
				"showEvents":         true,
				"showObjectChanges":  true,
				"showBalanceChanges": true,
			},
		},
	}

	if cursor != nil {
		params = append(params, *cursor)
	} else {
		params = append(params, nil)
	}
	params = append(params, limit)

	result, err := c.makeRPCCall(ctx, "suix_queryTransactionBlocks", params)
	if err != nil {
		return nil, err
	}

	var response TransactionQueryResponse
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	return &response, nil
}

// GetTransactionBlock gets details of a specific transaction
func (c *SuiClient) GetTransactionBlock(ctx context.Context, digest string) (*TransactionBlock, error) {
	params := []interface{}{
		digest,
		map[string]interface{}{
			"showInput":          true,
			"showEffects":        true,
			"showEvents":         true,
			"showObjectChanges":  true,
			"showBalanceChanges": true,
		},
	}

	result, err := c.makeRPCCall(ctx, "sui_getTransactionBlock", params)
	if err != nil {
		return nil, err
	}

	var txBlock TransactionBlock
	if err := json.Unmarshal(result, &txBlock); err != nil {
		return nil, fmt.Errorf("unmarshaling transaction block: %w", err)
	}

	return &txBlock, nil
}

// GetLatestCheckpoint gets the latest checkpoint number
func (c *SuiClient) GetLatestCheckpoint(ctx context.Context) (uint64, error) {
	result, err := c.makeRPCCall(ctx, "sui_getLatestCheckpointSequenceNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var checkpoint uint64
	if err := json.Unmarshal(result, &checkpoint); err != nil {
		return 0, fmt.Errorf("unmarshaling checkpoint: %w", err)
	}

	return checkpoint, nil
}

// GetCheckpoint gets checkpoint details by sequence number
func (c *SuiClient) GetCheckpoint(ctx context.Context, sequenceNumber uint64) (*Checkpoint, error) {
	params := []interface{}{
		fmt.Sprintf("%d", sequenceNumber),
	}

	result, err := c.makeRPCCall(ctx, "sui_getCheckpoint", params)
	if err != nil {
		return nil, err
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(result, &checkpoint); err != nil {
		return nil, fmt.Errorf("unmarshaling checkpoint: %w", err)
	}

	return &checkpoint, nil
}

func (c *SuiClient) makeRPCCall(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making RPC call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// TransactionQueryResponse represents the response from suix_queryTransactionBlocks
type TransactionQueryResponse struct {
	Data        []TransactionBlock `json:"data"`
	NextCursor  *string            `json:"nextCursor"`
	HasNextPage bool               `json:"hasNextPage"`
}

// TransactionBlock represents a SUI transaction block
type TransactionBlock struct {
	Digest                  string                   `json:"digest"`
	Transaction             *Transaction             `json:"transaction,omitempty"`
	Effects                 *TransactionEffects      `json:"effects,omitempty"`
	Events                  []Event                  `json:"events,omitempty"`
	ObjectChanges           []ObjectChange           `json:"objectChanges,omitempty"`
	BalanceChanges          []BalanceChange          `json:"balanceChanges,omitempty"`
	TimestampMs             *string                  `json:"timestampMs,omitempty"`
	Checkpoint              *string                  `json:"checkpoint,omitempty"`
}

type Transaction struct {
	Data   TransactionData `json:"data"`
	TxSignatures []string   `json:"txSignatures"`
}

type TransactionData struct {
	MessageVersion string        `json:"messageVersion"`
	Transaction    TransactionKind `json:"transaction"`
	Sender         string        `json:"sender"`
	GasData        GasData       `json:"gasData"`
}

type TransactionKind struct {
	Kind         string        `json:"kind"`
	Inputs       []interface{} `json:"inputs,omitempty"`
	Transactions []interface{} `json:"transactions,omitempty"`
}

type GasData struct {
	Payment []GasPayment `json:"payment"`
	Owner   string       `json:"owner"`
	Price   string       `json:"price"`
	Budget  string       `json:"budget"`
}

type GasPayment struct {
	ObjectID string `json:"objectId"`
	Version  int    `json:"version"`
	Digest   string `json:"digest"`
}

type TransactionEffects struct {
	Status          ExecutionStatus `json:"status"`
	GasUsed         GasUsed         `json:"gasUsed"`
	TransactionDigest string        `json:"transactionDigest"`
}

type ExecutionStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type GasUsed struct {
	ComputationCost string `json:"computationCost"`
	StorageCost     string `json:"storageCost"`
	StorageRebate   string `json:"storageRebate"`
	NonRefundableStorageFee string `json:"nonRefundableStorageFee"`
}

type Event struct {
	ID              EventID `json:"id"`
	PackageID       string  `json:"packageId"`
	TransactionModule string `json:"transactionModule"`
	Sender          string  `json:"sender"`
	Type            string  `json:"type"`
	ParsedJSON      map[string]interface{} `json:"parsedJson,omitempty"`
	Bcs             string  `json:"bcs,omitempty"`
	TimestampMs     string  `json:"timestampMs,omitempty"`
}

type EventID struct {
	TxDigest string `json:"txDigest"`
	EventSeq string `json:"eventSeq"`
}

type ObjectChange struct {
	Type         string `json:"type"`
	Sender       string `json:"sender,omitempty"`
	Owner        interface{} `json:"owner,omitempty"`
	ObjectType   string `json:"objectType,omitempty"`
	ObjectID     string `json:"objectId,omitempty"`
	Version      string `json:"version,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

type BalanceChange struct {
	Owner    Owner  `json:"owner"`
	CoinType string `json:"coinType"`
	Amount   string `json:"amount"`
}

type Owner struct {
	AddressOwner string `json:"AddressOwner,omitempty"`
	ObjectOwner  string `json:"ObjectOwner,omitempty"`
	Shared       *SharedOwner `json:"Shared,omitempty"`
}

type SharedOwner struct {
	InitialSharedVersion string `json:"initial_shared_version"`
}

// Checkpoint represents a SUI checkpoint
type Checkpoint struct {
	Epoch                      string   `json:"epoch"`
	SequenceNumber             string   `json:"sequenceNumber"`
	Digest                     string   `json:"digest"`
	NetworkTotalTransactions   string   `json:"networkTotalTransactions"`
	PreviousDigest             *string  `json:"previousDigest"`
	EpochRollingGasCostSummary GasCostSummary `json:"epochRollingGasCostSummary"`
	TimestampMs                string   `json:"timestampMs"`
	Transactions               []string `json:"transactions"`
	CheckpointCommitments      []interface{} `json:"checkpointCommitments"`
}

type GasCostSummary struct {
	ComputationCost         string `json:"computationCost"`
	StorageCost             string `json:"storageCost"`
	StorageRebate           string `json:"storageRebate"`
	NonRefundableStorageFee string `json:"nonRefundableStorageFee"`
}
