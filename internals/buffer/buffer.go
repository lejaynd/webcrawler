package buffer

import (
	"sync"

	"github.com/lejaynd/webcrawler/internals/models"
)

type Buffer interface {
	Push(task models.CrawlTask)
	GetAll() []models.CrawlTask
}

type InMemoryBuffer struct {
	tasks []models.CrawlTask
	lock  sync.Mutex
}

func NewInMemoryBuffer() *InMemoryBuffer {
	return &InMemoryBuffer{
		tasks: make([]models.CrawlTask, 0),
	}
}

func (b *InMemoryBuffer) Push(task models.CrawlTask) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.tasks = append(b.tasks, task)
}

func (b *InMemoryBuffer) GetAll() []models.CrawlTask {
	b.lock.Lock()
	defer b.lock.Unlock()

	copied := make([]models.CrawlTask, len(b.tasks))
	copy(copied, b.tasks)
	return copied
}
