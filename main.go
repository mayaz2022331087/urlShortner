package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// urlStore holds the mapping between short codes and original URLs, in memory.
type urlStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newURLStore() *urlStore {
	return &urlStore{data: make(map[string]string)}
}

func (s *urlStore) save(code, original string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[code] = original
}

func (s *urlStore) get(code string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	original, ok := s.data[code]
	return original, ok
}

var store = newURLStore()

const codeLength = 6
const codeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateCode creates a random short code, e.g. "aZ3kD9".
func generateCode() (string, error) {
	b := make([]byte, codeLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// shortenRequest is the expected JSON body for POST /shorten.
type shortenRequest struct {
	URL string `json:"url"`
}

// shortenResponse is what we send back after creating a short URL.
type shortenResponse struct {
	ShortCode string `json:"short_code"`
	ShortURL  string `json:"short_url"`
	Original  string `json:"original_url"`
}

// errorResponse is a simple JSON error shape.
type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func isValidURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// shortenHandler handles POST /shorten — takes a long URL, returns a short code.
func shortenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}

	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	defer r.Body.Close()

	req.URL = strings.TrimSpace(req.URL)
	if !isValidURL(req.URL) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "url must start with http:// or https://"})
		return
	}

	code, err := generateCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "could not generate short code"})
		return
	}

	store.save(code, req.URL)

	resp := shortenResponse{
		ShortCode: code,
		ShortURL:  "http://" + r.Host + "/" + code,
		Original:  req.URL,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// redirectHandler handles GET /{code} — redirects to the original URL.
func redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "no code provided"})
		return
	}

	original, ok := store.get(code)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "short code not found"})
		return
	}

	http.Redirect(w, r, original, http.StatusFound)
}

// healthHandler responds to GET /health.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// rootRouter sends /shorten and /health to their handlers, everything else to redirect.
func rootRouter(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/shorten":
		shortenHandler(w, r)
	case r.URL.Path == "/health":
		healthHandler(w, r)
	default:
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}
		redirectHandler(w, r)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootRouter)

	addr := ":8080"
	log.Printf("Server starting on %s ...", addr)
	log.Printf("Shorten a URL:  curl -X POST -d '{\"url\":\"https://example.com\"}' http://localhost%s/shorten", addr)
	log.Printf("Use a short URL: curl -i http://localhost%s/<code>", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}