package parser

import (
	"log"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/parserQueue"
	"github.com/lejaynd/webcrawler/internals/scheduler"
)

type ParserWorker struct {
	ParserQueue *parserQueue.ParserQueue
	Scheduler   *scheduler.Scheduler
}

func NewParserWorker(pq *parserQueue.ParserQueue, s *scheduler.Scheduler) *ParserWorker {
	return &ParserWorker{
		ParserQueue: pq,
		Scheduler:   s,
	}
}

func (p *ParserWorker) Run() {
	for {
		task := <-p.ParserQueue.Queue
		p.process(task)
	}
}

func (p *ParserWorker) process(task models.ParseTask) {
	doc, err := html.Parse(strings.NewReader(task.HTMLContent))
	if err != nil {
		log.Printf("[Parser] Error parsing HTML for %s: %v\n", task.URL, err)
		return
	}

	base, err := url.Parse(task.URL)
	if err != nil {
		return
	}

	var extractedURLs []string
	var textBuilder strings.Builder

	extractedURLCnt := 0

	var f func(*html.Node)
	f = func(n *html.Node) {
		if extractedURLCnt > 100 {
			return
		}

		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					link, err := base.Parse(a.Val)
					if err == nil {
						if link.Scheme == "http" || link.Scheme == "https" {
							extractedURLs = append(extractedURLs, link.String())
							extractedURLCnt++

							if extractedURLCnt > 100 {
								return
							}
						}
					}
					break
				}
			}
		} else if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				textBuilder.WriteString(text + " ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
				continue
			}
			f(c)
		}
	}

	f(doc)

	normalizedText := textBuilder.String()
	if len(normalizedText) > 100 {
		normalizedText = normalizedText[:100] + "..."
	}
	log.Printf("[Parser] Parsed %s - Extracted %d URLs. Text preview: %s\n", task.URL, len(extractedURLs), normalizedText)

	//fwd to scheduler
	for _, u := range extractedURLs {
		p.Scheduler.Incoming <- models.CrawlTask{
			URL:   u,
			Depth: task.Depth + 1,
		}
	}
}
