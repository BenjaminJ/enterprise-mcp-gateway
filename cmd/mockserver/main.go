package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/goschan/enterprise-mcp-gateway/pkg/connector/mock"
)

func main() {
	port := flag.Int("port", 8081, "Mock server port")
	flag.Parse()

	handler := mock.NewMockHandler()
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	log.Printf("[INFO] Mock Enterprise Backend listening on http://%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Mock server failed: %v", err)
	}
}
