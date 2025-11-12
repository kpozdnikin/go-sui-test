package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
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
}

func NewChirpTransactionService(
	repo *gormdb.ChirpTransactionRepository,
	suiClient *blockchain.SuiClient,
	chirpToken string,
	claimAddr string,
	initialSyncDays int,
	monitoringAddresses []string,
) *ChirpTransactionService {
	return &ChirpTransactionService{
		repo:               repo,
		suiClient:          suiClient,
		chirpToken:         chirpToken,
		claimAddr:          claimAddr,
		initialSyncDays:    initialSyncDays,
		monitoringAddresses: monitoringAddresses,
	}
}

// SyncTransactions synchronizes transactions from SUI blockchain to database
func (s *ChirpTransactionService) SyncTransactions(ctx context.Context) error {
	syncStart := time.Now()
	log.Println("========================================")
	log.Println("Starting transaction synchronization using address-based query...")
	
	// Build list of addresses to monitor
	addressesToMonitor := s.buildMonitoringAddressList()
	log.Printf("Monitoring %d addresses:", len(addressesToMonitor))
	for i, addr := range addressesToMonitor {
		log.Printf("  %d. %s", i+1, addr)
	}

	totalChirpTransactions := 0

	// Query transactions for each address
	for _, address := range addressesToMonitor {
		log.Printf("Processing address: %s", address)
		
		// Query transactions FROM address
		fromCount, err := s.syncTransactionsByAddress(ctx, address, true)
		if err != nil {
			log.Printf("ERROR: Failed to sync FROM transactions for %s: %v", address, err)
		} else {
			totalChirpTransactions += fromCount
			log.Printf("  Found %d CHIRP transactions FROM address", fromCount)
		}

		// Query transactions TO address
		toCount, err := s.syncTransactionsByAddress(ctx, address, false)
		if err != nil {
			log.Printf("ERROR: Failed to sync TO transactions for %s: %v", address, err)
		} else {
			totalChirpTransactions += toCount
			log.Printf("  Found %d CHIRP transactions TO address", toCount)
		}
		
		log.Printf("  Total for this address: %d CHIRP transactions", fromCount+toCount)
	}

	log.Printf("========================================")
	log.Printf("Synchronization completed in %v", time.Since(syncStart))
	log.Printf("Total CHIRP transactions saved: %d", totalChirpTransactions)
	log.Println("========================================")
	return nil
}

// buildMonitoringAddressList creates a list of addresses to monitor
func (s *ChirpTransactionService) buildMonitoringAddressList() []string {
	addresses := make([]string, 0)
	
	// Add claim address if not empty
	if s.claimAddr != "" {
		addresses = append(addresses, s.claimAddr)
	}
	
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

// syncTransactionsByAddress syncs transactions for a specific address
func (s *ChirpTransactionService) syncTransactionsByAddress(ctx context.Context, address string, isFrom bool) (int, error) {
	var cursor *string
	totalCount := 0
	limit := 50

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

		// Process each transaction
		for _, txBlock := range response.Data {
			// Check if already exists
			existing, err := s.repo.GetTransactionByDigest(ctx, txBlock.Digest)
			if err != nil {
				log.Printf("ERROR: Failed to check existing transaction %s: %v", txBlock.Digest, err)
				continue
			}
			if existing != nil {
				continue // Already processed
			}

			// Parse timestamp
			var timestamp time.Time
			if txBlock.TimestampMs != nil {
				timestamp, err = s.parseTimestamp(*txBlock.TimestampMs)
				if err != nil {
					log.Printf("ERROR: Failed to parse timestamp for %s: %v", txBlock.Digest, err)
					continue
				}
			}

			// Extract CHIRP transactions
			var checkpoint uint64
			if txBlock.Checkpoint != nil {
				fmt.Sscanf(*txBlock.Checkpoint, "%d", &checkpoint)
			}
			
			chirpTxs := s.extractChirpTransactions(&txBlock, checkpoint, timestamp)
			
			// Save to database
			if len(chirpTxs) > 0 {
				log.Printf("Saving %d CHIRP transaction(s) from tx %s", len(chirpTxs), txBlock.Digest)
				if err := s.repo.CreateTransactionsBatch(ctx, chirpTxs); err != nil {
					log.Printf("ERROR: Failed to save transactions: %v", err)
				} else {
					totalCount += len(chirpTxs)
				}
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
		for _, txDigest := range cpData.Transactions {
			chirpCount, err := s.processTransaction(ctx, txDigest, checkpoint, cpData.TimestampMs)
			if err != nil {
				log.Printf("ERROR: Failed to process transaction %s in checkpoint %d: %v", txDigest, checkpoint, err)
				// Continue with next transaction
			} else {
				chirpTxCount += chirpCount
				if chirpCount > 0 {
					log.Printf("Found %d CHIRP transaction(s) in tx %s", chirpCount, txDigest)
				}
			}
		}
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

// GetStatistics retrieves statistics for a custom time period
func (s *ChirpTransactionService) GetStatistics(ctx context.Context, start, end time.Time) (*domain.TokenStatistics, error) {
	return s.repo.GetStatistics(ctx, start, end)
}