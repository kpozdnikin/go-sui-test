package gormdb

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/kpozdnikin/go-sui-test/app/internal/domain"
)

// ChirpTransactionModel represents the database model for CHIRP transactions
type ChirpTransactionModel struct {
	ID              uint      `gorm:"primaryKey;autoIncrement"`
	Digest          string    `gorm:"uniqueIndex;not null"`
	Sender          string    `gorm:"index;not null"`
	Recipient       string    `gorm:"index"`
	Amount          string    `gorm:"not null"`
	TransactionType string    `gorm:"index;not null"`
	Checkpoint      uint64    `gorm:"index"`
	Timestamp       time.Time `gorm:"index;not null"`
	Success         bool      `gorm:"not null"`
	GasFee          string    `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ChirpTransactionRepository handles database operations for CHIRP transactions
type ChirpTransactionRepository struct {
	db *gorm.DB
}

func NewChirpTransactionRepository(db *gorm.DB) *ChirpTransactionRepository {
	return &ChirpTransactionRepository{
		db: db,
	}
}

// TableName specifies the table name for ChirpTransactionModel
func (ChirpTransactionModel) TableName() string {
	return "chirp_transactions"
}

// toDomainTransaction converts database model to domain model
func toDomainTransaction(model *ChirpTransactionModel) *domain.ChirpTransaction {
	return &domain.ChirpTransaction{
		ID:              fmt.Sprintf("%d", model.ID),
		Digest:          model.Digest,
		Sender:          model.Sender,
		Recipient:       model.Recipient,
		Amount:          model.Amount,
		TransactionType: model.TransactionType,
		Checkpoint:      model.Checkpoint,
		Timestamp:       model.Timestamp,
		Success:         model.Success,
		GasFee:          model.GasFee,
	}
}

// toDBModel converts domain model to database model
func toDBModel(tx *domain.ChirpTransaction) *ChirpTransactionModel {
	return &ChirpTransactionModel{
		Digest:          tx.Digest,
		Sender:          tx.Sender,
		Recipient:       tx.Recipient,
		Amount:          tx.Amount,
		TransactionType: tx.TransactionType,
		Checkpoint:      tx.Checkpoint,
		Timestamp:       tx.Timestamp,
		Success:         tx.Success,
		GasFee:          tx.GasFee,
	}
}

// CreateTransaction creates a new transaction record
func (r *ChirpTransactionRepository) CreateTransaction(ctx context.Context, tx *domain.ChirpTransaction) error {
	model := toDBModel(tx)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("creating transaction: %w", err)
	}
	tx.ID = fmt.Sprintf("%d", model.ID)
	return nil
}

// CreateTransactionsBatch creates multiple transactions in a batch
func (r *ChirpTransactionRepository) CreateTransactionsBatch(ctx context.Context, txs []*domain.ChirpTransaction) error {
	if len(txs) == 0 {
		return nil
	}

	models := make([]ChirpTransactionModel, len(txs))
	for i, tx := range txs {
		models[i] = *toDBModel(tx)
	}

	if err := r.db.WithContext(ctx).CreateInBatches(models, 100).Error; err != nil {
		return fmt.Errorf("creating transactions batch: %w", err)
	}

	return nil
}

// GetTransactionByDigest retrieves a transaction by its digest
func (r *ChirpTransactionRepository) GetTransactionByDigest(ctx context.Context, digest string) (*domain.ChirpTransaction, error) {
	var model ChirpTransactionModel
	if err := r.db.WithContext(ctx).Where("digest = ?", digest).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting transaction by digest: %w", err)
	}
	return toDomainTransaction(&model), nil
}

// GetTransactionsByAddress retrieves transactions for a specific address
func (r *ChirpTransactionRepository) GetTransactionsByAddress(ctx context.Context, address string, limit int, offset int) ([]*domain.ChirpTransaction, error) {
	var models []ChirpTransactionModel
	query := r.db.WithContext(ctx).
		Where("sender = ? OR recipient = ?", address, address).
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("getting transactions by address: %w", err)
	}

	txs := make([]*domain.ChirpTransaction, len(models))
	for i := range models {
		txs[i] = toDomainTransaction(&models[i])
	}
	return txs, nil
}

// GetTransactionsByTimeRange retrieves transactions within a time range
func (r *ChirpTransactionRepository) GetTransactionsByTimeRange(ctx context.Context, start, end time.Time) ([]*domain.ChirpTransaction, error) {
	var models []ChirpTransactionModel
	if err := r.db.WithContext(ctx).
		Where("timestamp >= ? AND timestamp <= ?", start, end).
		Order("timestamp ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("getting transactions by time range: %w", err)
	}

	txs := make([]*domain.ChirpTransaction, len(models))
	for i := range models {
		txs[i] = toDomainTransaction(&models[i])
	}
	return txs, nil
}

// GetStatistics calculates token statistics for a given time period
func (r *ChirpTransactionRepository) GetStatistics(ctx context.Context, start, end time.Time) (*domain.TokenStatistics, error) {
	log.Printf("DB: GetStatistics called for period %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))
	
	stats := &domain.TokenStatistics{
		PeriodStart: start,
		PeriodEnd:   end,
	}

	// Get total transaction count
	var totalCount int64
	
	if err := r.db.WithContext(ctx).Model(&ChirpTransactionModel{}).
		Where("timestamp >= ? AND timestamp <= ? AND success = ?", start, end, true).
		Count(&totalCount).Error; err != nil {
		log.Printf("DB: Error counting transactions: %v", err)
		return nil, fmt.Errorf("counting transactions: %w", err)
	}
	stats.TotalTxCount = totalCount
	log.Printf("DB: Found %d total transactions", totalCount)

	// Get unique holders count
	var uniqueHolders int64
	if err := r.db.WithContext(ctx).Model(&ChirpTransactionModel{}).
		Where("timestamp >= ? AND timestamp <= ?", start, end).
		Distinct("sender").
		Count(&uniqueHolders).Error; err != nil {
		log.Printf("DB: Error counting unique holders: %v", err)
		return nil, fmt.Errorf("counting unique holders: %w", err)
	}
	stats.UniqueHolders = uniqueHolders
	log.Printf("DB: Found %d unique holders", uniqueHolders)

	// Aggregate amounts by transaction type
	type AggResult struct {
		TransactionType string
		TotalAmount     string
	}

	var results []AggResult
	if err := r.db.WithContext(ctx).Model(&ChirpTransactionModel{}).
		Select("transaction_type, SUM(CAST(amount AS DECIMAL)) as total_amount").
		Where("timestamp >= ? AND timestamp <= ? AND success = ?", start, end, true).
		Group("transaction_type").
		Scan(&results).Error; err != nil {
		log.Printf("DB: Error aggregating amounts: %v", err)
		return nil, fmt.Errorf("aggregating amounts: %w", err)
	}
	log.Printf("DB: Aggregation results: %+v", results)

	// Map results to statistics
	for _, result := range results {
		switch result.TransactionType {
		case "claim":
			stats.TotalClaimed = result.TotalAmount
		case "transfer":
			stats.TotalTransferred = result.TotalAmount
		case "stake":
			stats.TotalStaked = result.TotalAmount
		case "buy":
			stats.TotalBought = result.TotalAmount
		case "sell":
			stats.TotalSold = result.TotalAmount
		}
	}

	// Set defaults for empty values
	if stats.TotalClaimed == "" {
		stats.TotalClaimed = "0"
	}
	if stats.TotalTransferred == "" {
		stats.TotalTransferred = "0"
	}
	if stats.TotalStaked == "" {
		stats.TotalStaked = "0"
	}
	if stats.TotalBought == "" {
		stats.TotalBought = "0"
	}
	if stats.TotalSold == "" {
		stats.TotalSold = "0"
	}

	log.Printf("DB: Final statistics: TxCount=%d, Holders=%d, Claimed=%s, Transferred=%s, Staked=%s, Bought=%s, Sold=%s",
		stats.TotalTxCount, stats.UniqueHolders, stats.TotalClaimed, stats.TotalTransferred, 
		stats.TotalStaked, stats.TotalBought, stats.TotalSold)

	return stats, nil
}

// GetLatestCheckpoint retrieves the latest checkpoint number from stored transactions
func (r *ChirpTransactionRepository) GetLatestCheckpoint(ctx context.Context) (uint64, error) {
	var checkpoint *uint64
	if err := r.db.WithContext(ctx).Model(&ChirpTransactionModel{}).
		Select("MAX(checkpoint)").
		Scan(&checkpoint).Error; err != nil {
		log.Printf("DB: Error getting latest checkpoint: %v", err)
		return 0, fmt.Errorf("getting latest checkpoint: %w", err)
	}
	
	// If table is empty, MAX returns NULL
	if checkpoint == nil {
		log.Println("DB: No checkpoints found in database (table is empty), starting from 0")
		return 0, nil
	}
	
	log.Printf("DB: Latest checkpoint in database: %d", *checkpoint)
	return *checkpoint, nil
}
