package parser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/net/html"

	"github.com/lejaynd/webcrawler/internals/db"
	"github.com/lejaynd/webcrawler/internals/llm"
	"github.com/lejaynd/webcrawler/internals/models"
	"github.com/lejaynd/webcrawler/internals/parserQueue"
	"github.com/lejaynd/webcrawler/internals/scheduler"
)

type ParserWorker struct {
	ParserQueue   *parserQueue.ParserQueue
	Scheduler     *scheduler.Scheduler
	RedisClient   *redis.Client
	S3Store       *db.S3Store
	contextForLLM *llm.ContextForLLM
}

func NewParserWorker(pq *parserQueue.ParserQueue, s *scheduler.Scheduler, rc *redis.Client, s3 *db.S3Store, l *llm.ContextForLLM) *ParserWorker {
	return &ParserWorker{
		ParserQueue:   pq,
		Scheduler:     s,
		RedisClient:   rc,
		S3Store:       s3,
		contextForLLM: l,
	}
}

func (p *ParserWorker) Run() {
	for {
		task := <-p.ParserQueue.Queue
		if p.Scheduler.Paused.Load() {
			continue
		}
		p.process(task)
	}
}

func (p *ParserWorker) process(task models.ParseTask) {
	ctx := context.Background()

	obj, err := p.S3Store.GetObject(ctx, task.S3Key)
	if err != nil {
		log.Printf("[Parser] Failed to get S3 object for %s: %v\n", task.URL, err)
		return
	}

	doc, err := html.Parse(strings.NewReader(obj.HTMLContent))
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

	textContent := textBuilder.String()

	normalizedText := strings.ToLower(textContent)
	normalizedText = strings.Join(strings.Fields(normalizedText), " ")

	hash := sha256.Sum256([]byte(normalizedText))
	contentHash := hex.EncodeToString(hash[:])

	//content dedup redis
	key := "content:" + contentHash
	set, err := p.RedisClient.SetNX(ctx, key, task.URL, 24*time.Hour).Result()
	if err != nil {
		log.Printf("[Parser] Redis error for %s: %v, skipping dedup check\n", task.URL, err)
	} else if !set {
		log.Printf("[Parser] Duplicate content skipped for %s (hash: %s...)\n", task.URL, contentHash[:8])
	}

	//unique content
	if set {
		if err := p.S3Store.UpdateTextContent(ctx, task.S3Key, textContent); err != nil {
			log.Printf("[Parser] S3 update failed for %s: %v\n", task.URL, err)
		} else {
			preview := textContent
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			log.Printf("[Parser] Parsed %s - %d URLs extracted. Preview: %s\n", task.URL, len(extractedURLs), preview)

			forLLMContent := " "

			if len(textContent) > 550 {
				forLLMContent = textContent[:550]
			} else {
				forLLMContent = textContent
			}

			p.contextForLLM.AddContext(forLLMContent, task.URL)
		}
	}

	for _, u := range extractedURLs {
		p.Scheduler.Incoming <- models.CrawlTask{
			URL:   u,
			Depth: task.Depth + 1,
		}
	}
}
