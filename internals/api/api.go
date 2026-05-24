package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lejaynd/webcrawler/internals/llm"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/scheduler"
)

type Server struct {
	schedulerIncoming chan models.CrawlTask
	contextForLLM     *llm.ContextForLLM
	groqClient        *llm.GroqClient
	scheduler         *scheduler.Scheduler

	mu       sync.Mutex
	status   string //idle, crawling, ready
	stopChan chan struct{}
	crawlEnd time.Time
	duration int
}

func NewServer(incoming chan models.CrawlTask, ctx *llm.ContextForLLM, groq *llm.GroqClient, scheduler *scheduler.Scheduler) *Server {
	return &Server{
		schedulerIncoming: incoming,
		contextForLLM:     ctx,
		groqClient:        groq,
		scheduler:         scheduler,
		status:            "idle",
	}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("frontend")))

	mux.HandleFunc("/api/crawl", s.handleCrawl)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/ask", s.handleAsk)

	log.Printf("[API] Server starting on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

type crawlRequest struct {
	SeedURL  string `json:"seedURL"`
	Duration int    `json:"duration"` //seconds
}

type statusResponse struct {
	Status       string `json:"status"`
	PagesCrawled int    `json:"pagesCrawled"`
	Remaining    int    `json:"remaining"` //seconds left
}

type askRequest struct {
	Question string `json:"question"`
}

type askResponse struct {
	Answer    string   `json:"answer"`
	Citations []string `json:"citations"`
}

func (s *Server) handleCrawl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req crawlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SeedURL == "" {
		http.Error(w, "seedURL is required", http.StatusBadRequest)
		return
	}

	duration := 30
	if req.Duration > 10 {
		duration = req.Duration
	} else {
		http.Error(w, "Duration should be > 10 Seconds", http.StatusBadRequest)
	}

	s.mu.Lock()

	if s.stopChan != nil {
		close(s.stopChan)
	}

	s.contextForLLM.Reset()
	s.status = "crawling"
	s.stopChan = make(chan struct{})
	s.duration = duration
	s.crawlEnd = time.Now().Add(time.Duration(duration) * time.Second)

	stopCh := s.stopChan
	s.scheduler.Resume()
	s.mu.Unlock()

	s.schedulerIncoming <- models.CrawlTask{
		URL:   req.SeedURL,
		Depth: 0,
	}

	log.Printf("[API] Crawl started with seed: %s, duration: %ds\n", req.SeedURL, duration)

	//after duration mark as ready and stop scheduler
	go func() {
		select {
		case <-time.After(time.Duration(duration) * time.Second):
			s.mu.Lock()
			s.status = "ready"
			s.mu.Unlock()
			s.scheduler.Stop()
			log.Printf("[API] Crawl finished. %d pages in context.\n", s.contextForLLM.PageCount())
		case <-stopCh:
			//new crawl started, abort this timer
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "crawling"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	st := s.status
	end := s.crawlEnd
	s.mu.Unlock()

	remaining := 0
	if st == "crawling" {
		remaining = int(time.Until(end).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}

	resp := statusResponse{
		Status:       st,
		PagesCrawled: s.contextForLLM.PageCount(),
		Remaining:    remaining,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	st := s.status
	s.mu.Unlock()

	if st != "ready" {
		http.Error(w, "Crawl not finished yet", http.StatusTooEarly)
		return
	}

	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		http.Error(w, "question is required", http.StatusBadRequest)
		return
	}

	context := s.contextForLLM.GetContexts()

	log.Printf("[API] Asking Groq: %q (context length: %d chars)\n", req.Question, len(context))

	answer, citations, err := s.groqClient.AskWithContext(req.Question, context)
	if err != nil {
		log.Printf("[API] Groq error: %v\n", err)
		http.Error(w, fmt.Sprintf("LLM error: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[API] Groq answered: %d chars, %d citations\n", len(answer), len(citations))

	resp := askResponse{
		Answer:    answer,
		Citations: citations,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
