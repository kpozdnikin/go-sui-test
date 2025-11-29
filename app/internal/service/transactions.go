package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kpozdnikin/go-sui-test/app/internal/domain"
	"github.com/kpozdnikin/go-sui-test/app/internal/infrastructure/blockchain"
	"github.com/kpozdnikin/go-sui-test/app/internal/infrastructure/storage/gormdb"
)

// ChirpTransactionService handles business logic for CHIRP token transactions
type ChirpTransactionService struct {
	repo               *gormdb.ChirpTransactionRepository
	suiClient          *blockchain.SuiClient
	chirpToken         string
	claimAddr          string
	initialSyncDays    int
	monitoringAddresses []string
	pageLimit          int
	batchSize          int
}

func NewChirpTransactionService(
	repo *gormdb.ChirpTransactionRepository,
	suiClient *blockchain.SuiClient,
	chirpToken string,
	claimAddr string,
	initialSyncDays int,
	monitoringAddresses []string,
	pageLimit int,
	batchSize int,
) *ChirpTransactionService {
	return &ChirpTransactionService{
		repo:               repo,
		suiClient:          suiClient,
		chirpToken:         chirpToken,
		claimAddr:          claimAddr,
		initialSyncDays:    initialSyncDays,
		monitoringAddresses: monitoringAddresses,
		pageLimit:          pageLimit,
		batchSize:          batchSize,
	}
}

// SyncTransactions synchronizes transactions from SUI blockchain to database
func (s *ChirpTransactionService) SyncTransactions(ctx context.Context) error {
	syncStart := time.Now()
	log.Println("========================================")
	log.Println("Starting transaction synchronization using checkpoint-based sync...")

	// Determine last processed checkpoint from database
	lastCheckpoint, err := s.repo.GetLatestCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("getting latest checkpoint from database: %w", err)
	}
	log.Printf("Last checkpoint in database: %d", lastCheckpoint)

	// Get latest checkpoint from blockchain
	latestCheckpoint, err := s.suiClient.GetLatestCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("getting latest checkpoint from blockchain: %w", err)
	}
	log.Printf("Latest checkpoint on blockchain: %d", latestCheckpoint)

	// Determine range to sync
	startCheckpoint := lastCheckpoint
	if lastCheckpoint > 0 {
		startCheckpoint = lastCheckpoint + 1
	}

	if startCheckpoint > latestCheckpoint {
		log.Println("No new checkpoints to synchronize. Database is up to date.")
		log.Println("========================================")
		return nil
	}

	log.Printf("Synchronizing checkpoints from %d to %d", startCheckpoint, latestCheckpoint)
	totalTxCount, chirpTxCount, err := s.syncCheckpointRange(ctx, startCheckpoint, latestCheckpoint)
	if err != nil {
		return err
	}

	log.Printf("========================================")
	log.Printf("Synchronization completed in %v", time.Since(syncStart))
	log.Printf("Total transactions processed: %d", totalTxCount)
	log.Printf("Total CHIRP transactions saved: %d", chirpTxCount)
	log.Println("========================================")
	return nil
}

// buildMonitoringAddressList creates a list of addresses to monitor
func (s *ChirpTransactionService) buildMonitoringAddressList() []string {
	addresses := make([]string, 0)
	

	// Add configured monitoring addresses
	for _, addr := range s.monitoringAddresses {
		// Skip empty addresses and duplicates
		if addr == "" {
			continue
		}
		isDuplicate := false
		for _, existing := range addresses {
			if existing == addr {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			addresses = append(addresses, addr)
		}
	}
	
	return addresses
}

// syncTransactionsByAddress syncs transactions for a specific address using parallel processing
func (s *ChirpTransactionService) syncTransactionsByAddress(ctx context.Context, address string, isFrom bool) (int, error) {
	var cursor *string
	totalCount := 0
	limit := s.pageLimit
	if limit <= 0 {
		limit = 5000
	}
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	for {
		// Query transactions
		var response *blockchain.TransactionQueryResponse
		var err error
		
		if isFrom {
			response, err = s.suiClient.QueryTransactionsByAddress(ctx, address, cursor, limit)
		} else {
			// For TO address, we need a different filter
			response, err = s.queryTransactionsToAddress(ctx, address, cursor, limit)
		}
		
		if err != nil {
			return totalCount, fmt.Errorf("querying transactions: %w", err)
		}

		// Process transactions in batches
		for i := 0; i < len(response.Data); i += batchSize {
			end := i + batchSize
			if end > len(response.Data) {
				end = len(response.Data)
			}
			batch := response.Data[i:end]
			
			// Process batch in parallel
			count, err := s.processBatchParallel(ctx, batch)
			if err != nil {
				log.Printf("ERROR: Failed to process batch: %v", err)
			} else {
				totalCount += count
			}
		}

		// Check if there are more pages
		if !response.HasNextPage || response.NextCursor == nil {
			break
		}
		cursor = response.NextCursor
		
		log.Printf("Processed %d transactions so far, fetching next page...", totalCount)
		time.Sleep(500 * time.Millisecond) // Rate limiting
	}

	return totalCount, nil
}

// processBatchParallel processes a batch of transactions in parallel using WaitGroup
func (s *ChirpTransactionService) processBatchParallel(ctx context.Context, batch []blockchain.TransactionBlock) (int, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	allChirpTxs := make([]*domain.ChirpTransaction, 0)
	
	// Process each transaction in parallel
	for _, txBlock := range batch {
		wg.Add(1)
		go func(tx blockchain.TransactionBlock) {
			defer wg.Done()
			
			// Check if already exists
			existing, err := s.repo.GetTransactionByDigest(ctx, tx.Digest)
			if err != nil {
				log.Printf("ERROR: Failed to check existing transaction %s: %v", tx.Digest, err)
				return
			}
			if existing != nil {
				return // Already processed
			}

			// Parse timestamp
			var timestamp time.Time
			if tx.TimestampMs != nil {
				timestamp, err = s.parseTimestamp(*tx.TimestampMs)
				if err != nil {
					log.Printf("ERROR: Failed to parse timestamp for %s: %v", tx.Digest, err)
					return
				}
			}

			// Extract CHIRP transactions
			var checkpoint uint64
			if tx.Checkpoint != nil {
				fmt.Sscanf(*tx.Checkpoint, "%d", &checkpoint)
			}
			
			chirpTxs := s.extractChirpTransactions(&tx, checkpoint, timestamp)
			
			// Add to collection (thread-safe)
			if len(chirpTxs) > 0 {
				mu.Lock()
				allChirpTxs = append(allChirpTxs, chirpTxs...)
				mu.Unlock()
			}
		}(txBlock)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	
	// Deduplicate by digest before saving
	uniqueTxs := s.deduplicateTransactions(allChirpTxs)
	
	// Save all collected transactions in one batch
	if len(uniqueTxs) > 0 {
		log.Printf("Saving batch of %d CHIRP transaction(s) to database (deduplicated from %d)", len(uniqueTxs), len(allChirpTxs))
		if err := s.repo.CreateTransactionsBatch(ctx, uniqueTxs); err != nil {
			return 0, fmt.Errorf("saving transactions batch: %w", err)
		}
	}
	
	return len(uniqueTxs), nil
}

// deduplicateTransactions removes duplicate transactions by digest, keeping the first occurrence
func (s *ChirpTransactionService) deduplicateTransactions(txs []*domain.ChirpTransaction) []*domain.ChirpTransaction {
	seen := make(map[string]bool)
	unique := make([]*domain.ChirpTransaction, 0, len(txs))
	
	for _, tx := range txs {
		key := tx.Digest + "|" + tx.Recipient + "|" + tx.Amount + "|" + tx.TransactionType
		if !seen[key] {
			seen[key] = true
			unique = append(unique, tx)
		}
	}
	
	return unique
}

// queryTransactionsToAddress queries transactions TO a specific address
func (s *ChirpTransactionService) queryTransactionsToAddress(ctx context.Context, address string, cursor *string, limit int) (*blockchain.TransactionQueryResponse, error) {
	return s.suiClient.QueryTransactionsToAddress(ctx, address, cursor, limit)
}

func (s *ChirpTransactionService) syncCheckpointRange(ctx context.Context, start, end uint64) (int, int, error) {
	totalTxCount := 0
	chirpTxCount := 0

	for checkpoint := start; checkpoint <= end; checkpoint++ {
		cpData, err := s.suiClient.GetCheckpoint(ctx, checkpoint)
		if err != nil {
			log.Printf("ERROR: Failed to get checkpoint %d: %v", checkpoint, err)
			return totalTxCount, chirpTxCount, fmt.Errorf("getting checkpoint %d: %w", checkpoint, err)
		}

		txInCheckpoint := len(cpData.Transactions)
		totalTxCount += txInCheckpoint

		if txInCheckpoint > 0 {
			log.Printf("Checkpoint %d: %d transactions", checkpoint, txInCheckpoint)
		}

		// Process each transaction in the checkpoint
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, txDigest := range cpData.Transactions {
			wg.Add(1)
			go func(digest string) {
				defer wg.Done()
				chirpCount, err := s.processTransaction(ctx, digest, checkpoint, cpData.TimestampMs)
				if err != nil {
					log.Printf("ERROR: Failed to process transaction %s in checkpoint %d: %v", digest, checkpoint, err)
					return
				}
				if chirpCount > 0 {
					mu.Lock()
					chirpTxCount += chirpCount
					mu.Unlock()
					log.Printf("Found %d CHIRP transaction(s) in tx %s", chirpCount, digest)
				}
			}(txDigest)
		}
		wg.Wait()
	}

	return totalTxCount, chirpTxCount, nil
}

func (s *ChirpTransactionService) processTransaction(ctx context.Context, digest string, checkpoint uint64, timestampMs string) (int, error) {
	// Check if transaction already exists
	existing, err := s.repo.GetTransactionByDigest(ctx, digest)
	if err != nil {
		return 0, fmt.Errorf("checking existing transaction: %w", err)
	}
	if existing != nil {
		return 0, nil // Already processed
	}

	// Get transaction details
	txBlock, err := s.suiClient.GetTransactionBlock(ctx, digest)
	if err != nil {
		return 0, fmt.Errorf("getting transaction block: %w", err)
	}

	// Parse timestamp
	timestamp, err := s.parseTimestamp(timestampMs)
	if err != nil {
		return 0, fmt.Errorf("parsing timestamp: %w", err)
	}

	// Extract CHIRP token transactions
	chirpTxs := s.extractChirpTransactions(txBlock, checkpoint, timestamp)

	// Save to database
	if len(chirpTxs) > 0 {
		log.Printf("Saving %d CHIRP transaction(s) from tx %s", len(chirpTxs), digest)
		for i, tx := range chirpTxs {
			log.Printf("  [%d] Type: %s, Sender: %s, Recipient: %s, Amount: %s", 
				i+1, tx.TransactionType, tx.Sender, tx.Recipient, tx.Amount)
		}
		if err := s.repo.CreateTransactionsBatch(ctx, chirpTxs); err != nil {
			return 0, fmt.Errorf("saving transactions: %w", err)
		}
	}

	return len(chirpTxs), nil
}

func (s *ChirpTransactionService) extractChirpTransactions(txBlock *blockchain.TransactionBlock, checkpoint uint64, timestamp time.Time) []*domain.ChirpTransaction {
	var transactions []*domain.ChirpTransaction

	if txBlock.Transaction == nil || txBlock.Effects == nil {
		return transactions
	}

	sender := txBlock.Transaction.Data.Sender
	success := txBlock.Effects.Status.Status == "success"
	gasFee := s.calculateGasFee(txBlock.Effects.GasUsed)

	// Check balance changes for CHIRP token
	// Only log transactions with balance changes for debugging (limit to reduce noise)
	if len(txBlock.BalanceChanges) > 0 && checkpoint%100 == 0 {
		// Log sample transactions to see what tokens are being processed
		log.Printf("Sample TX %s has %d balance changes. Looking for CHIRP token: %s", txBlock.Digest[:16], len(txBlock.BalanceChanges), s.chirpToken)
		for i, balanceChange := range txBlock.BalanceChanges {
			if i < 2 { // Log first 2 balance changes for debugging
				log.Printf("  Balance change %d: CoinType=%s, Amount=%s", i, balanceChange.CoinType, balanceChange.Amount)
			}
		}
	}
	
	for _, balanceChange := range txBlock.BalanceChanges {
		log.Printf("TX %s BalanceChange: CoinType=%s, Amount=%s", txBlock.Digest, balanceChange.CoinType, balanceChange.Amount)
		// Check if this is a CHIRP token (case-insensitive and check for "chirp" keyword)
		coinTypeLower := strings.ToLower(balanceChange.CoinType)
		chirpTokenLower := strings.ToLower(s.chirpToken)
		
		isChirp := strings.Contains(coinTypeLower, chirpTokenLower) || 
		           strings.Contains(coinTypeLower, "::chirp::")
		
		if !isChirp {
			continue
		}
		
		log.Printf("✓ FOUND CHIRP TRANSACTION! TX: %s, CoinType: %s, Amount: %s", txBlock.Digest, balanceChange.CoinType, balanceChange.Amount)

		recipient := s.extractOwnerAddress(balanceChange.Owner)
		amount := balanceChange.Amount

		// Determine transaction type
		txType := s.determineTransactionType(txBlock, amount)

		tx := &domain.ChirpTransaction{
			Digest:          txBlock.Digest,
			Sender:          sender,
			Recipient:       recipient,
			Amount:          amount,
			TransactionType: txType,
			Checkpoint:      checkpoint,
			Timestamp:       timestamp,
			Success:         success,
			GasFee:          gasFee,
		}

		transactions = append(transactions, tx)
	}

	return transactions
}

func (s *ChirpTransactionService) determineTransactionType(txBlock *blockchain.TransactionBlock, amount string) string {
	// Check if it's a claim transaction
	if txBlock.Transaction != nil {
		txKind := txBlock.Transaction.Data.Transaction
		if txKind.Kind == "ProgrammableTransaction" {
			// Check for claim function call
			for _, tx := range txKind.Transactions {
				if txMap, ok := tx.(map[string]interface{}); ok {
					if moveCall, ok := txMap["MoveCall"].(map[string]interface{}); ok {
						if function, ok := moveCall["function"].(string); ok {
							if function == "claim" {
								return "claim"
							}
						}
					}
				}
			}
		}
	}

	// Check events for staking
	for _, event := range txBlock.Events {
		if strings.Contains(event.Type, "StakeProof") || strings.Contains(event.Type, "stake") {
			return "stake"
		}
		if strings.Contains(event.Type, "unstake") {
			return "unstake"
		}
	}

	// Determine buy/sell based on amount sign (simplified logic)
	if strings.HasPrefix(amount, "-") {
		return "sell"
	} else if amount != "0" {
		// Could be buy or transfer, default to transfer
		return "transfer"
	}

	return "transfer"
}

func (s *ChirpTransactionService) extractOwnerAddress(owner blockchain.Owner) string {
	if owner.AddressOwner != "" {
		return owner.AddressOwner
	}
	if owner.ObjectOwner != "" {
		return owner.ObjectOwner
	}
	return ""
}

func (s *ChirpTransactionService) calculateGasFee(gasUsed blockchain.GasUsed) string {
	comp, _ := strconv.ParseInt(gasUsed.ComputationCost, 10, 64)
	storage, _ := strconv.ParseInt(gasUsed.StorageCost, 10, 64)
	rebate, _ := strconv.ParseInt(gasUsed.StorageRebate, 10, 64)

	total := comp + storage - rebate
	return fmt.Sprintf("%d", total)
}

func (s *ChirpTransactionService) parseTimestamp(timestampMs string) (time.Time, error) {
	ms, err := strconv.ParseInt(timestampMs, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, ms*int64(time.Millisecond)), nil
}

// GetTransactionsByAddress retrieves transactions for a specific address
func (s *ChirpTransactionService) GetTransactionsByAddress(ctx context.Context, address string, limit, offset int) ([]*domain.ChirpTransaction, error) {
	return s.repo.GetTransactionsByAddress(ctx, address, limit, offset)
}

// GetWeeklyStatistics retrieves statistics for the last week
func (s *ChirpTransactionService) GetWeeklyStatistics(ctx context.Context) (*domain.TokenStatistics, error) {
	end := time.Now()
	start := end.AddDate(0, 0, -7) // 7 days ago

	return s.repo.GetStatistics(ctx, start, end)
}

// GetAllTimeStatistics retrieves statistics for the entire history
func (s *ChirpTransactionService) GetAllTimeStatistics(ctx context.Context) (*domain.TokenStatistics, error) {
	end := time.Now()
	start := time.Time{}

	return s.repo.GetStatistics(ctx, start, end)
}

// GetStatistics retrieves statistics for a custom time period
func (s *ChirpTransactionService) GetStatistics(ctx context.Context, start, end time.Time) (*domain.TokenStatistics, error) {
	return s.repo.GetStatistics(ctx, start, end)
}