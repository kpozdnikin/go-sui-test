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
	repo       *gormdb.ChirpTransactionRepository
	suiClient  *blockchain.SuiClient
	chirpToken string
	claimAddr  string
}

func NewChirpTransactionService(
	repo *gormdb.ChirpTransactionRepository,
	suiClient *blockchain.SuiClient,
	chirpToken string,
	claimAddr string,
) *ChirpTransactionService {
	return &ChirpTransactionService{
		repo:       repo,
		suiClient:  suiClient,
		chirpToken: chirpToken,
		claimAddr:  claimAddr,
	}
}

// SyncTransactions synchronizes transactions from SUI blockchain to database
func (s *ChirpTransactionService) SyncTransactions(ctx context.Context) error {
	log.Println("Starting transaction synchronization...")

	// Get latest checkpoint from database
	latestDBCheckpoint, err := s.repo.GetLatestCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("getting latest checkpoint from DB: %w", err)
	}

	// Get latest checkpoint from blockchain
	latestChainCheckpoint, err := s.suiClient.GetLatestCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("getting latest checkpoint from chain: %w", err)
	}

	log.Printf("DB checkpoint: %d, Chain checkpoint: %d", latestDBCheckpoint, latestChainCheckpoint)

	// If we're up to date, nothing to do
	if latestDBCheckpoint >= latestChainCheckpoint {
		log.Println("Already up to date")
		return nil
	}

	// Sync checkpoints in batches
	startCheckpoint := latestDBCheckpoint + 1
	batchSize := uint64(100)

	for checkpoint := startCheckpoint; checkpoint <= latestChainCheckpoint; checkpoint += batchSize {
		endCheckpoint := checkpoint + batchSize - 1
		if endCheckpoint > latestChainCheckpoint {
			endCheckpoint = latestChainCheckpoint
		}

		if err := s.syncCheckpointRange(ctx, checkpoint, endCheckpoint); err != nil {
			log.Printf("Error syncing checkpoints %d-%d: %v", checkpoint, endCheckpoint, err)
			// Continue with next batch
		}

		log.Printf("Synced checkpoints %d-%d", checkpoint, endCheckpoint)
	}

	log.Println("Transaction synchronization completed")
	return nil
}

func (s *ChirpTransactionService) syncCheckpointRange(ctx context.Context, start, end uint64) error {
	for checkpoint := start; checkpoint <= end; checkpoint++ {
		cpData, err := s.suiClient.GetCheckpoint(ctx, checkpoint)
		if err != nil {
			return fmt.Errorf("getting checkpoint %d: %w", checkpoint, err)
		}

		// Process each transaction in the checkpoint
		for _, txDigest := range cpData.Transactions {
			if err := s.processTransaction(ctx, txDigest, checkpoint, cpData.TimestampMs); err != nil {
				log.Printf("Error processing transaction %s: %v", txDigest, err)
				// Continue with next transaction
			}
		}
	}

	return nil
}

func (s *ChirpTransactionService) processTransaction(ctx context.Context, digest string, checkpoint uint64, timestampMs string) error {
	// Check if transaction already exists
	existing, err := s.repo.GetTransactionByDigest(ctx, digest)
	if err != nil {
		return fmt.Errorf("checking existing transaction: %w", err)
	}
	if existing != nil {
		return nil // Already processed
	}

	// Get transaction details
	txBlock, err := s.suiClient.GetTransactionBlock(ctx, digest)
	if err != nil {
		return fmt.Errorf("getting transaction block: %w", err)
	}

	// Parse timestamp
	timestamp, err := s.parseTimestamp(timestampMs)
	if err != nil {
		return fmt.Errorf("parsing timestamp: %w", err)
	}

	// Extract CHIRP token transactions
	chirpTxs := s.extractChirpTransactions(txBlock, checkpoint, timestamp)

	// Save to database
	if len(chirpTxs) > 0 {
		if err := s.repo.CreateTransactionsBatch(ctx, chirpTxs); err != nil {
			return fmt.Errorf("saving transactions: %w", err)
		}
	}

	return nil
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
	for _, balanceChange := range txBlock.BalanceChanges {
		if !strings.Contains(balanceChange.CoinType, s.chirpToken) {
			continue
		}

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