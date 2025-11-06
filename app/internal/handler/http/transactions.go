package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kpozdnikin/go-sui-test/app/internal/service"
)

type TransactionsHTTPHandler struct {
	service *service.ChirpTransactionService
}

func NewTransactionsHTTPHandler(service *service.ChirpTransactionService) *TransactionsHTTPHandler {
	return &TransactionsHTTPHandler{
		service: service,
	}
}

func (h *TransactionsHTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/transactions/address", h.GetTransactionsByAddress)
	mux.HandleFunc("/api/v1/statistics/weekly", h.GetWeeklyStatistics)
	mux.HandleFunc("/api/v1/statistics", h.GetStatistics)
	mux.HandleFunc("/api/v1/sync", h.SyncTransactions)
	mux.HandleFunc("/health", h.HealthCheck)
}

func (h *TransactionsHTTPHandler) GetTransactionsByAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "address parameter is required", http.StatusBadRequest)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	txs, err := h.service.GetTransactionsByAddress(r.Context(), address, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"transactions": txs,
		"total":        len(txs),
		"limit":        limit,
		"offset":       offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *TransactionsHTTPHandler) GetWeeklyStatistics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := h.service.GetWeeklyStatistics(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *TransactionsHTTPHandler) GetStatistics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		http.Error(w, "start and end parameters are required (RFC3339 format)", http.StatusBadRequest)
		return
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		http.Error(w, "invalid start time format, use RFC3339", http.StatusBadRequest)
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		http.Error(w, "invalid end time format, use RFC3339", http.StatusBadRequest)
		return
	}

	stats, err := h.service.GetStatistics(r.Context(), start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *TransactionsHTTPHandler) SyncTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := h.service.SyncTransactions(r.Context())
	
	response := map[string]interface{}{
		"success": err == nil,
	}

	if err != nil {
		response["message"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		response["message"] = "Synchronization completed successfully"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *TransactionsHTTPHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
