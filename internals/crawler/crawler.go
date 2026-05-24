package crawler

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/db"
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
	S3Store      *db.S3Store
	SQLStore     *db.SQLStore
}

func NewCrawlerWorker(
	f *crawlerFrontier.Frontier,
	pq *parserQueue.ParserQueue,
	s *scheduler.Scheduler,
	lm *lease.LeaseManager,
	s3 *db.S3Store,
	sql *db.SQLStore,
) *CrawlerWorker {
	return &CrawlerWorker{
		Frontier:     f,
		ParserQueue:  pq,
		Scheduler:    s,
		LeaseManager: lm,
		S3Store:      s3,
		SQLStore:     sql,
	}
}

func (c *CrawlerWorker) Run() {
	for {
		task := <-c.Frontier.ReadyChan
		if c.Scheduler.Paused.Load() {
			continue
		}
		c.process(task)
	}
}

func (c *CrawlerWorker) process(task models.CrawlTask) {
	task.URL = utils.NormalizeURL(task.URL)
	ctx := context.Background()

	c.LeaseManager.MarkInFlight(task)

	//fetch HTML
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

	//upload HTML to s3
	s3Key, s3Link, err := c.S3Store.UploadHTML(ctx, task.URL, string(bodyBytes))
	if err != nil {
		log.Printf("[Crawler] S3 upload failed for %s: %v\n", task.URL, err)
		c.LeaseManager.MarkSuccess(task.URL)
		c.Scheduler.Retries <- task
		return
	}
	log.Printf("[Crawler] Uploaded %s → %s\n", task.URL, s3Link)

	//insert record into SQL
	if err := c.SQLStore.InsertRecord(task.URL, s3Link, task.Depth); err != nil {
		log.Printf("[Crawler] SQL insert failed for %s: %v\n", task.URL, err)
	}

	c.ParserQueue.Queue <- models.ParseTask{
		URL:   task.URL,
		S3Key: s3Key,
		Depth: task.Depth,
	}

	c.LeaseManager.MarkSuccess(task.URL)
}
