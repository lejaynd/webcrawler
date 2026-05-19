package main

import (
	"log"
	"time"

	"github.com/lejaynd/webcrawler/internals/buffer"
	"github.com/lejaynd/webcrawler/internals/crawler"
	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/dlq"
	"github.com/lejaynd/webcrawler/internals/lease"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/parser"
	"github.com/lejaynd/webcrawler/internals/parserQueue"
	"github.com/lejaynd/webcrawler/internals/scheduler"
)

type System struct {
	frontier        *crawlerFrontier.Frontier
	deadLetterQueue *dlq.InMemoryDLQ
	buffer          *buffer.InMemoryBuffer
	scheduler       *scheduler.Scheduler
	parserQueue     *parserQueue.ParserQueue
	leaseManager    *lease.LeaseManager
}

func initSystem() *System {
	return &System{
		frontier:        crawlerFrontier.NewFrontier(),
		deadLetterQueue: dlq.NewInMemoryDLQ(),
		buffer:          buffer.NewInMemoryBuffer(),
		parserQueue:     parserQueue.NewParserQueue(),
	}
}

func main() {
	log.Println("Starting Web Crawler...")

	system := initSystem()
	system.scheduler = scheduler.NewScheduler(system.frontier, system.deadLetterQueue, system.buffer)
	system.leaseManager = lease.NewLeaseManager(system.frontier.PushChan)

	go system.frontier.Run()
	go system.scheduler.Run()
	go system.leaseManager.RecoverExpired()

	for i := 0; i < 10; i++ {
		p := parser.NewParserWorker(system.parserQueue, system.scheduler)
		go p.Run()
	}

	for i := 0; i < 5; i++ {
		c := crawler.NewCrawlerWorker(system.frontier, system.parserQueue, system.scheduler, system.leaseManager)
		go c.Run()
	}

	//seeding crawler
	seedURL := "https://example.com"
	log.Printf("Seeding crawler with: %s\n", seedURL)
	system.scheduler.Incoming <- models.CrawlTask{
		URL:   seedURL,
		Depth: 0,
	}

	time.Sleep(60 * time.Second)
	log.Println("Crawler shutting down after 60 seconds.")
}
