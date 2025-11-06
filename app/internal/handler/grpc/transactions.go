package grpc

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kpozdnikin/go-sui-test/app/internal/domain"
	"github.com/kpozdnikin/go-sui-test/app/internal/service"
	pb "github.com/kpozdnikin/go-sui-test/app/pkg/transactions/v1"
)

type TransactionsHandler struct {
	pb.UnimplementedTransactionsServiceServer
	service *service.ChirpTransactionService
}

func NewTransactionsHandler(service *service.ChirpTransactionService) *TransactionsHandler {
	return &TransactionsHandler{
		service: service,
	}
}

func (h *TransactionsHandler) GetTransactionsByAddress(ctx context.Context, req *pb.GetTransactionsByAddressRequest) (*pb.GetTransactionsByAddressResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	txs, err := h.service.GetTransactionsByAddress(ctx, req.Address, limit, offset)
	if err != nil {
		return nil, err
	}

	pbTxs := make([]*pb.ChirpTransaction, len(txs))
	for i, tx := range txs {
		pbTxs[i] = domainToProtoTransaction(tx)
	}

	return &pb.GetTransactionsByAddressResponse{
		Transactions: pbTxs,
		Total:        int32(len(pbTxs)),
	}, nil
}

func (h *TransactionsHandler) GetWeeklyStatistics(ctx context.Context, req *pb.GetWeeklyStatisticsRequest) (*pb.GetWeeklyStatisticsResponse, error) {
	stats, err := h.service.GetWeeklyStatistics(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.GetWeeklyStatisticsResponse{
		Statistics: domainToProtoStatistics(stats),
	}, nil
}

func (h *TransactionsHandler) GetStatistics(ctx context.Context, req *pb.GetStatisticsRequest) (*pb.GetStatisticsResponse, error) {
	start := req.StartTime.AsTime()
	end := req.EndTime.AsTime()

	stats, err := h.service.GetStatistics(ctx, start, end)
	if err != nil {
		return nil, err
	}

	return &pb.GetStatisticsResponse{
		Statistics: domainToProtoStatistics(stats),
	}, nil
}

func (h *TransactionsHandler) SyncTransactions(ctx context.Context, req *pb.SyncTransactionsRequest) (*pb.SyncTransactionsResponse, error) {
	err := h.service.SyncTransactions(ctx)
	if err != nil {
		return &pb.SyncTransactionsResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.SyncTransactionsResponse{
		Success: true,
		Message: "Synchronization completed successfully",
	}, nil
}

func domainToProtoTransaction(tx *domain.ChirpTransaction) *pb.ChirpTransaction {
	return &pb.ChirpTransaction{
		Id:              tx.ID,
		Digest:          tx.Digest,
		Sender:          tx.Sender,
		Recipient:       tx.Recipient,
		Amount:          tx.Amount,
		TransactionType: tx.TransactionType,
		Checkpoint:      tx.Checkpoint,
		Timestamp:       timestamppb.New(tx.Timestamp),
		Success:         tx.Success,
		GasFee:          tx.GasFee,
	}
}

func domainToProtoStatistics(stats *domain.TokenStatistics) *pb.TokenStatistics {
	return &pb.TokenStatistics{
		TotalClaimed:     stats.TotalClaimed,
		TotalTransferred: stats.TotalTransferred,
		TotalStaked:      stats.TotalStaked,
		TotalBought:      stats.TotalBought,
		TotalSold:        stats.TotalSold,
		UniqueHolders:    stats.UniqueHolders,
		TotalTxCount:     stats.TotalTxCount,
		PeriodStart:      timestamppb.New(stats.PeriodStart),
		PeriodEnd:        timestamppb.New(stats.PeriodEnd),
	}
}
