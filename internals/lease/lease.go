package lease

import (
	"log"
	"sync"
	"time"

	"github.com/lejaynd/webcrawler/internals/models"
)

type LeaseManager struct {
	inflight map[string]*models.CrawlStateRecord
	lock     sync.Mutex
	requeue  chan<- models.CrawlTask
}

func NewLeaseManager(requeue chan<- models.CrawlTask) *LeaseManager {
	return &LeaseManager{
		inflight: make(map[string]*models.CrawlStateRecord),
		requeue:  requeue,
	}
}

func (l *LeaseManager) MarkInFlight(task models.CrawlTask) {
	l.lock.Lock()
	defer l.lock.Unlock()

	l.inflight[task.URL] = &models.CrawlStateRecord{
		State:         models.StateInFlight,
		TimeRemaining: 60,
		Task:          task,
	}
}

func (l *LeaseManager) MarkSuccess(url string) {
	l.lock.Lock()
	defer l.lock.Unlock()

	delete(l.inflight, url)
}

func (l *LeaseManager) RecoverExpired() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	//TimeRemaining reduces by 5 every 5 seconds

	for range ticker.C {
		l.lock.Lock()
		for url, rec := range l.inflight {
			rec.TimeRemaining -= 5
			if rec.TimeRemaining <= 0 {
				log.Printf("[Lease Recovery] Requeueing expired task: %s\n", url)
				rec.Task.NextRunTime = time.Now()

				select {
				case l.requeue <- rec.Task:
					delete(l.inflight, url)
				default:
					rec.TimeRemaining = 0
				}
			}
		}
		l.lock.Unlock()
	}
}
