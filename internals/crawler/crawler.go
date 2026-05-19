package crawler

import (
	"io"
	"log"
	"net/http"

	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/lease"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/parserQueue"
	"github.com/lejaynd/webcrawler/internals/scheduler"
	"github.com/lejaynd/webcrawler/internals/utils"
)

type CrawlerWorker struct {
	Frontier     *crawlerFrontier.Frontier
	ParserQueue  *parserQueue.ParserQueue
	Scheduler    *scheduler.Scheduler
	LeaseManager *lease.LeaseManager
}

func NewCrawlerWorker(f *crawlerFrontier.Frontier, pq *parserQueue.ParserQueue, s *scheduler.Scheduler, lm *lease.LeaseManager) *CrawlerWorker {
	return &CrawlerWorker{
		Frontier:     f,
		ParserQueue:  pq,
		Scheduler:    s,
		LeaseManager: lm,
	}
}

func (c *CrawlerWorker) Run() {
	for {
		task := <-c.Frontier.ReadyChan
		c.process(task)
	}
}

func (c *CrawlerWorker) process(task models.CrawlTask) {
	task.URL = utils.NormalizeURL(task.URL)

	//processing
	c.LeaseManager.MarkInFlight(task)

	//fetch html
	log.Printf("[Crawler] Fetching %s\n", task.URL)
	resp, err := http.Get(task.URL)
	if err != nil {
		log.Printf("[Crawler] Error fetching %s: %v\n", task.URL, err)
		c.LeaseManager.MarkSuccess(task.URL)
		c.Scheduler.Retries <- task
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Crawler] Non-OK status %d for %s\n", resp.StatusCode, task.URL)
		c.LeaseManager.MarkSuccess(task.URL)
		c.Scheduler.Retries <- task
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Crawler] Error reading body %s: %v\n", task.URL, err)
		c.LeaseManager.MarkSuccess(task.URL)
		c.Scheduler.Retries <- task
		return
	}

	//fwd to parsing queue
	c.ParserQueue.Queue <- models.ParseTask{
		URL:         task.URL,
		Depth:       task.Depth,
		HTMLContent: string(bodyBytes),
	}

	//success
	c.LeaseManager.MarkSuccess(task.URL)
}
