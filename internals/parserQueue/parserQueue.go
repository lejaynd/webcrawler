package parserQueue

import "github.com/lejaynd/webcrawler/internals/models"

type ParserQueue struct {
	Queue chan models.ParseTask
}

func NewParserQueue() *ParserQueue {
	return &ParserQueue{
		Queue: make(chan models.ParseTask, 1000),
	}
}
