package parserQueue

import "github.com/lejaynd/webcrawler/internals/models"

// ParserQueue is a wrapper around a channel for decoupling Crawler from Parser.
type ParserQueue struct {
	Queue chan models.ParseTask
}

func NewParserQueue() *ParserQueue {
	return &ParserQueue{
		Queue: make(chan models.ParseTask, 1000),
	}
}
