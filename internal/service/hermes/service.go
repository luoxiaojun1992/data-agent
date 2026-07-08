package hermes

import (
	"io"
	"log"
	"net/http"
	"time"
)

// Service is a forwarding proxy for the Hermes free-explore mode.
type Service struct {
	hermesURL string
	client    *http.Client
}

// NewService creates a Hermes service.
func NewService(hermesURL string) *Service {
	return &Service{
		hermesURL: hermesURL,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Proxy forwards a request to the Hermes API and streams the response.
func (s *Service) Proxy(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest(r.Method, s.hermesURL+r.URL.Path, r.Body)
	if err != nil {
		http.Error(w, "proxy error", http.StatusInternalServerError)
		return
	}
	req.Header = r.Header

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "hermes unreachable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("hermes proxy: io.Copy error: %v", err)
	}
}
