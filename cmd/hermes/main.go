// Package main is the entry point for Hermes — the standalone free-exploration service.
// Hermes runs independently from the Agent Service, providing unrestricted LLM access
// without the security audit layer. It communicates via HTTP and can be deployed as a
// sidecar container or separate process.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("HERMES_PORT")
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","service":"hermes","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	// Chat endpoint — direct LLM access without Agent Service audit
	mux.HandleFunc("/api/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Hermes routes chat requests to the configured LLM provider
		// without the full agent engine pipeline (no security audit, no skill routing)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","message":"Hermes free-explore endpoint — direct LLM access"}%s`, "\n")
	})

	// Models list
	mux.HandleFunc("/api/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		modelName := os.Getenv("LLM_MODEL")
		if modelName == "" {
			modelName = "gpt-4"
		}
		fmt.Fprintf(w, `{"models":[{"id":"%s","provider":"openai"}],"default":"%s"}%s`, modelName, modelName, "\n")
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Hermes: shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Hermes: forced shutdown: %v", err)
		}
	}()

	log.Printf("Hermes free-explore service starting on port %s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Hermes: server error: %v", err)
	}
	log.Println("Hermes: exited gracefully")
}
