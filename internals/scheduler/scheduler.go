package scheduler

import (
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/lejaynd/webcrawler/internals/buffer"
	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/dlq"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/utils"
)

type Scheduler struct {
	Incoming chan models.CrawlTask
	Retries  chan models.CrawlTask

	Frontier *crawlerFrontier.Frontier
	DLQ      dlq.DLQ
	Buffer   buffer.Buffer

	CrawlDelay   time.Duration
	MaxDepth     int
	MaxPerDomain int //max crawls per domain

	visited     map[string]bool
	domainCount map[string]int
	visitedLock sync.Mutex
}

func NewScheduler(frontier *crawlerFrontier.Frontier, deadLetterQueue dlq.DLQ, bufferURL buffer.Buffer) *Scheduler {
	return &Scheduler{
		Incoming:     make(chan models.CrawlTask, 1000),
		Retries:      make(chan models.CrawlTask, 1000),
		Frontier:     frontier,
		DLQ:          deadLetterQueue,
		Buffer:       bufferURL,
		CrawlDelay:   10 * time.Second,
		MaxDepth:     3,
		MaxPerDomain: 100,
		visited:      make(map[string]bool),
		domainCount:  make(map[string]int),
	}
}

func (s *Scheduler) Run() {
	for {
		select {
		case inTask := <-s.Incoming:

			if inTask.Depth > s.MaxDepth {
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

			s.visitedLock.Lock()
			if s.visited[normalizedURL] {
				s.visitedLock.Unlock()
				continue
			}

			//too many crawls, can drop this request to bufferURL
			if s.domainCount[host] >= s.MaxPerDomain {
				log.Printf("[Buffer] Storing URL in Buffer after max per domain limit reached: %s\n", inTask.URL)
				s.Buffer.Push(inTask)
				s.visitedLock.Unlock()
				continue
			}

			s.visited[normalizedURL] = true
			s.domainCount[host]++
			s.visitedLock.Unlock()

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

			// Exponential backoff
			backoff := time.Duration(1<<task.RetryCount) * time.Second
			task.NextRunTime = time.Now().Add(backoff)
			s.Frontier.PushChan <- task
		}
	}
}
