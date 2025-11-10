package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kpozdnikin/go-sui-test/app/internal/config"
	grpcHandler "github.com/kpozdnikin/go-sui-test/app/internal/handler/grpc"
	httpHandler "github.com/kpozdnikin/go-sui-test/app/internal/handler/http"
	"github.com/kpozdnikin/go-sui-test/app/internal/infrastructure/blockchain"
	"github.com/kpozdnikin/go-sui-test/app/internal/infrastructure/storage/gormdb"
	"github.com/kpozdnikin/go-sui-test/app/internal/service"
	pb "github.com/kpozdnikin/go-sui-test/app/pkg/transactions/v1"
)

func main() {
	// Load configuration
	cfg, err := config.GetConfig("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting %s v%s", cfg.App.Name, cfg.App.Version)

	// Initialize database
	db, err := initDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Get blockchain info
	suiClient := blockchain.NewSuiClient("https://fullnode.mainnet.sui.io:443")
	blockchainInfo, err := suiClient.GetBlockchainInfo(context.Background())
	if err != nil {
		log.Fatalf("Failed to get blockchain info: %v", err)
	}

	log.Printf("Connected to SUI %s network", blockchainInfo.CurrentNetwork)
	log.Printf("CHIRP Token: %s", blockchainInfo.ChirpCurrency)

	// Initialize repository
	repo := gormdb.NewChirpTransactionRepository(db)

	// Initialize service
	txService := service.NewChirpTransactionService(
		repo,
		suiClient,
		blockchainInfo.ChirpCurrency,
		blockchainInfo.ClaimAddress,
	)

	// Start background sync (optional - can be triggered via API)
	go func() {
		ticker := time.NewTicker(cfg.Sync.Interval)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("Running scheduled transaction sync...")
			if err := txService.SyncTransactions(context.Background()); err != nil {
				log.Printf("Sync error: %v", err)
			}
		}
	}()

	// Start gRPC server
	grpcServer := initGRPCServer(cfg, txService)
	go func() {
		lis, err := net.Listen("tcp", cfg.GRPC.Port)
		if err != nil {
			log.Fatalf("Failed to listen on %s: %v", cfg.GRPC.Port, err)
		}

		log.Printf("gRPC server listening on %s", cfg.GRPC.Port)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	// Start HTTP server
	httpServer := initHTTPServer(cfg, txService)
	go func() {
		log.Printf("HTTP server listening on %s", cfg.HTTP.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to serve HTTP: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	// Graceful shutdown
	grpcServer.GracefulStop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Servers stopped")
}

func initDatabase(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PostgreSQL.Host,
		cfg.PostgreSQL.Port,
		cfg.PostgreSQL.User,
		cfg.PostgreSQL.Password,
		cfg.PostgreSQL.DBName,
		cfg.PostgreSQL.SSLMode,
	)

	db, err := gormdb.NewDB(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("Database connected and migrated successfully")
	return db, nil
}

func initGRPCServer(cfg *config.Config, txService *service.ChirpTransactionService) *grpc.Server {
	grpcServer := grpc.NewServer()

	// Register services
	handler := grpcHandler.NewTransactionsHandler(txService)
	pb.RegisterTransactionsServiceServer(grpcServer, handler)

	// Enable reflection for development
	if cfg.GRPC.EnableReflection {
		reflection.Register(grpcServer)
	}

	return grpcServer
}

func initHTTPServer(cfg *config.Config, txService *service.ChirpTransactionService) *http.Server {
	mux := http.NewServeMux()

	// Register HTTP handlers
	handler := httpHandler.NewTransactionsHTTPHandler(txService)
	handler.RegisterRoutes(mux)

	return &http.Server{
		Addr:    cfg.HTTP.Port,
		Handler: mux,
	}
}