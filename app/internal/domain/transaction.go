package domain

import (
	"time"
)

// ChirpTransaction represents a CHIRP token transaction in the SUI network
type ChirpTransaction struct {
	ID              string    `json:"id"`
	Digest          string    `json:"digest"`
	Sender          string    `json:"sender"`
	Recipient       string    `json:"recipient"`
	Amount          string    `json:"amount"` // stored as string to preserve precision
	TransactionType string    `json:"transaction_type"` // "claim", "transfer", "stake", "unstake"
	Checkpoint      uint64    `json:"checkpoint"`
	Timestamp       time.Time `json:"timestamp"`
	Success         bool      `json:"success"`
	GasFee          string    `json:"gas_fee"`
}

// TokenStatistics represents aggregated statistics for CHIRP token
type TokenStatistics struct {
	TotalClaimed     string    `json:"total_claimed"`
	TotalTransferred string    `json:"total_transferred"`
	TotalStaked      string    `json:"total_staked"`
	TotalBought      string    `json:"total_bought"`
	TotalSold        string    `json:"total_sold"`
	UniqueHolders    int64     `json:"unique_holders"`
	TotalTxCount     int64     `json:"total_tx_count"`
	PeriodStart      time.Time `json:"period_start"`
	PeriodEnd        time.Time `json:"period_end"`
}

// BlockchainInfo represents the configuration from Chirp API
type BlockchainInfo struct {
	ChirpCurrency     string `json:"chirpCurrency"`
	ClaimAddress      string `json:"claimAddress"`
	VaultID           string `json:"vaultId"`
	StakingPackageID  string `json:"staking_package_id"`
	FountainObjectID  string `json:"fountain_object_id"`
	CurrentNetwork    string `json:"current_network"`
	SuiMainnetURL     string `json:"url_main"`
}
