package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

type Customer struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	SSN        string `json:"ssn"`
	CreditCard string `json:"creditCard"`
	Status     string `json:"status"`
}

type BillingStatement struct {
	CustomerID string   `json:"customerId"`
	Invoices   []string `json:"invoices"`
	CardNumber string   `json:"cardNumber"`
	BalanceDue float64  `json:"balanceDue"`
}

type SupportTicket struct {
	ID          string `json:"id"`
	CustomerID  string `json:"customerId"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
}

func NewMockHandler() http.Handler {
	mu := sync.Mutex{}
	customers := map[string]Customer{
		"cust-001": {
			ID:         "cust-001",
			Name:       "Alice Smith",
			Email:      "alice@enterprise.com",
			SSN:        "123-45-6789",
			CreditCard: "4111-1111-1111-1111",
			Status:     "active",
		},
		"cust-002": {
			ID:         "cust-002",
			Name:       "Bob Johnson",
			Email:      "bob.johnson@corp.internal",
			SSN:        "987-65-4321",
			CreditCard: "4242-4242-4242-4242",
			Status:     "inactive",
		},
	}

	tickets := []SupportTicket{
		{
			ID:          "tick-101",
			CustomerID:  "cust-001",
			Subject:     "Database latency issue",
			Description: "Customer reported timeout on invoice generation.",
			Priority:    "high",
			Status:      "open",
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/v1/customers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			defer mu.Unlock()
			list := make([]Customer, 0, len(customers))
			for _, c := range customers {
				list = append(list, c)
			}
			_ = json.NewEncoder(w).Encode(list)
		case http.MethodPost:
			var c Customer
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			if c.ID == "" {
				c.ID = fmt.Sprintf("cust-%03d", len(customers)+1)
			}
			c.Status = "active"
			customers[c.ID] = c
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(c)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/customers/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := strings.TrimPrefix(r.URL.Path, "/v1/customers/")
		mu.Lock()
		c, ok := customers[id]
		if !ok {
			if id == "cust-1" {
				c, ok = customers["cust-001"]
			} else if id == "cust-2" {
				c, ok = customers["cust-002"]
			}
		}
		mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"Customer not found"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	})

	mux.HandleFunc("/v1/billing/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := strings.TrimPrefix(r.URL.Path, "/v1/billing/")
		mu.Lock()
		c, ok := customers[id]
		if !ok {
			if id == "cust-1" {
				c, ok = customers["cust-001"]
			} else if id == "cust-2" {
				c, ok = customers["cust-002"]
			}
		}
		mu.Unlock()
		if !ok {
			http.Error(w, `{"error":"Customer not found"}`, http.StatusNotFound)
			return
		}
		stmt := BillingStatement{
			CustomerID: c.ID,
			Invoices:   []string{"INV-2026-001", "INV-2026-002"},
			CardNumber: c.CreditCard,
			BalanceDue: 1450.00,
		}
		_ = json.NewEncoder(w).Encode(stmt)
	})

	mux.HandleFunc("/v1/support/tickets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(tickets)
		case http.MethodPost:
			var t SupportTicket
			if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			t.ID = fmt.Sprintf("tick-%d", len(tickets)+101)
			t.Status = "open"
			tickets = append(tickets, t)
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(t)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/admin/reset-password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "success",
			"username": req["username"],
			"message":  "Password has been reset successfully.",
		})
	})

	return mux
}

// StartAutoMockServer starts an embedded mock server in the background if port 8081 is available.
func StartAutoMockServer(port int) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	server := &http.Server{Handler: NewMockHandler()}
	go func() {
		log.Printf("[INFO] Embedded mock backend server auto-started on http://%s", addr)
		_ = server.Serve(l)
	}()
}
