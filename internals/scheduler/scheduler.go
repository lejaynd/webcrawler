package scheduler

import (
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/lejaynd/webcrawler/internals/buffer"
	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/dlq"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/utils"
)

type Scheduler struct {
	Incoming chan models.CrawlTask
	Retries  chan models.CrawlTask

	Frontier    *crawlerFrontier.Frontier
	DLQ         dlq.DLQ
	Buffer      buffer.Buffer
	BloomFilter *bloom.BloomFilter

	CrawlDelay   time.Duration
	MaxDepth     int
	MaxPerDomain int //max crawls per domain

	Visited     map[string]bool
	DomainCount map[string]int
	VisitedLock sync.Mutex

	Paused atomic.Bool
}

func NewScheduler(frontier *crawlerFrontier.Frontier, deadLetterQueue dlq.DLQ, bufferURL buffer.Buffer, bloomFilter *bloom.BloomFilter) *Scheduler {
	return &Scheduler{
		Incoming:     make(chan models.CrawlTask, 1000),
		Retries:      make(chan models.CrawlTask, 1000),
		Frontier:     frontier,
		DLQ:          deadLetterQueue,
		Buffer:       bufferURL,
		BloomFilter:  bloomFilter,
		CrawlDelay:   10 * time.Second,
		MaxDepth:     5,
		MaxPerDomain: 400,
		Visited:      make(map[string]bool),
		DomainCount:  make(map[string]int),
	}
}

func (s *Scheduler) Stop() {
	s.Paused.Store(true)
	log.Println("[Scheduler] Paused - no new URLs will be accepted.")
}

func (s *Scheduler) Resume() {
	s.Paused.Store(false)
	log.Println("[Scheduler] Resumed.")
}

func (s *Scheduler) Run() {
	for {
		select {
		case inTask := <-s.Incoming:
			if s.Paused.Load() {
				continue
			}

			normalizedURL := utils.NormalizeURL(inTask.URL)
			if normalizedURL == "" {
				continue
			}

			parsed, err := url.Parse(normalizedURL)
			if err != nil {
				continue
			}
			host := parsed.Host

			s.VisitedLock.Lock()

			if s.BloomFilter.TestString(normalizedURL) {
				if s.Visited[normalizedURL] {
					s.VisitedLock.Unlock()
					continue
				}
			}

			if inTask.Depth > s.MaxDepth {
				log.Printf("[Buffer] Storing URL in Buffer after max depth reached: %s\n", inTask.URL)
				s.Buffer.Push(inTask)
				s.VisitedLock.Unlock()
				continue
			}

			//too many crawls, can drop this request to bufferURL
			if s.DomainCount[host] >= s.MaxPerDomain {
				log.Printf("[Buffer] Storing URL in Buffer after max per domain limit reached: %s\n", inTask.URL)
				s.Buffer.Push(inTask)
				s.VisitedLock.Unlock()
				continue
			}

			s.Visited[normalizedURL] = true
			s.BloomFilter.AddString(normalizedURL)

			//visited[url] : URL has been passed to frontier

			s.DomainCount[host]++
			s.VisitedLock.Unlock()

			task := models.CrawlTask{
				URL:         normalizedURL,
				Depth:       inTask.Depth,
				RetryCount:  0,
				NextRunTime: time.Now().Add(s.CrawlDelay),
			}
			s.Frontier.PushChan <- task

		case task := <-s.Retries:
			task.RetryCount++
			if task.RetryCount >= 5 {
				log.Printf("[DLQ] Storing URL in DLQ after %d attempts: %s\n", task.RetryCount, task.URL)
				if s.DLQ != nil {
					s.DLQ.Push(task)
				}
				continue
			}

			//exponential backoff
			backoff := time.Duration(1<<task.RetryCount) * time.Second
			task.NextRunTime = time.Now().Add(backoff)
			s.Frontier.PushChan <- task
		}
	}
}
