package models

import "time"

type CrawlTask struct {
	URL          string
	Depth        int
	RetryCount   int
	NextRunTime  time.Time
}

type ParseTask struct {
	URL         string
	Depth       int
	HTMLContent string
}

type URLState int

const (
	StateUnseen URLState = iota
	StateInFlight
	StateSuccess
)

type CrawlStateRecord struct {
	State         URLState
	TimeRemaining int
	Task          CrawlTask
}
