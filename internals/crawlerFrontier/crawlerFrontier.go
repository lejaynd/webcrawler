package crawlerFrontier

import (
	"container/heap"
	"time"

	"github.com/lejaynd/webcrawler/internals/models"
)

// crawlHeap implements heap.Interface and holds CrawlTasks
type crawlHeap []models.CrawlTask

func (h crawlHeap) Len() int           { return len(h) }
func (h crawlHeap) Less(i, j int) bool { return h[i].NextRunTime.Before(h[j].NextRunTime) }
func (h crawlHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *crawlHeap) Push(x interface{}) {
	*h = append(*h, x.(models.CrawlTask))
}

func (h *crawlHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

type Frontier struct {
	PushChan  chan models.CrawlTask
	ReadyChan chan models.CrawlTask
}

func NewFrontier() *Frontier {
	return &Frontier{
		PushChan:  make(chan models.CrawlTask, 1000),
		ReadyChan: make(chan models.CrawlTask, 1000),
	}
}

func (f *Frontier) Run() {
	pq := &crawlHeap{}
	heap.Init(pq)

	var timer *time.Timer
	var timerChan <-chan time.Time

	for {
		//empty all tasks in pushchan into readychan
		if pq.Len() > 0 {
			nextTask := (*pq)[0]
			now := time.Now()
			if nextTask.NextRunTime.Before(now) || nextTask.NextRunTime.Equal(now) {
				task := heap.Pop(pq).(models.CrawlTask)
				f.ReadyChan <- task
				continue
			} else {
				timer = time.NewTimer(nextTask.NextRunTime.Sub(now))
				timerChan = timer.C
			}
		} else {
			timerChan = nil
		}

		select {
		case task := <-f.PushChan:
			heap.Push(pq, task)
			if timer != nil {
				timer.Stop()
			}
		case <-timerChan:
		}
	}
}
