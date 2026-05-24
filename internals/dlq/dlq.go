package dlq

import (
	"sync"

	"github.com/lejaynd/webcrawler/internals/models"
)

type DLQ interface {
	Push(task models.CrawlTask) error
	GetAll() []models.CrawlTask
}

type InMemoryDLQ struct {
	tasks []models.CrawlTask
	lock  sync.Mutex
}

func NewInMemoryDLQ() *InMemoryDLQ {
	return &InMemoryDLQ{
		tasks: make([]models.CrawlTask, 0),
	}
}

func (d *InMemoryDLQ) Push(task models.CrawlTask) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.tasks = append(d.tasks, task)
	return nil
}

func (d *InMemoryDLQ) GetAll() []models.CrawlTask {
	d.lock.Lock()
	defer d.lock.Unlock()

	copied := make([]models.CrawlTask, len(d.tasks))
	copy(copied, d.tasks)
	return copied
}
