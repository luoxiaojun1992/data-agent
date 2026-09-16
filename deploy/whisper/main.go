package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type transcribeRequest struct {
	Audio      string `json:"audio"`       // base64 int16 PCM 16kHz mono
	Lang       string `json:"lang"`        // "zh" / "auto"
	SampleRate int    `json:"sample_rate"` // 16000 (informational)
}

type textResponse struct {
	Text string `json:"text"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func main() {
	modelPath := envStr("WHISPER_MODEL_PATH", "/models/ggml-tiny.bin")
	minPool := envInt("WHISPER_MIN_CONCURRENCY", 2) // warm-up instances
	maxPool := envInt("WHISPER_MAX_CONCURRENCY", 4) // dynamic-growth cap
	threads := envInt("WHISPER_N_THREADS", 4)
	audioCtx := envInt("WHISPER_AUDIO_CTX", 768)
	internalToken := envStr("WHISPER_INTERNAL_TOKEN", "")
	maxBytes := envInt("WHISPER_MAX_BYTES", 10*1024*1024) // ~5min @16kHz int16
	port := envStr("PORT", "8081")

	tr, err := NewTranscriber(modelPath, minPool, maxPool, threads, audioCtx)
	if err != nil {
		log.Fatalf("init transcriber: %v", err)
	}
	defer tr.Close()
	log.Printf("whisper server ready: min_pool=%d max_pool=%d threads=%d audio_ctx=%d model=%s",
		minPool, maxPool, threads, audioCtx, modelPath)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/transcribe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if internalToken != "" && r.Header.Get("X-Internal-Token") != internalToken {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Bound the request body (base64 inflates by ~4/3).
		bodyLimit := int64(maxBytes)*4/3 + 1024
		var req transcribeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		raw, err := base64.StdEncoding.DecodeString(req.Audio)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid base64 audio")
			return
		}
		if len(raw) == 0 {
			writeError(w, http.StatusBadRequest, "empty audio")
			return
		}
		if len(raw) > maxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "audio too large")
			return
		}

		text, err := tr.Transcribe(r.Context(), raw, req.Lang)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "transcribe failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, textResponse{Text: text})
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // a 60s clip can take seconds to transcribe
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
