package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"github.com/lejaynd/webcrawler/internals/api"
	"github.com/lejaynd/webcrawler/internals/buffer"
	"github.com/lejaynd/webcrawler/internals/crawler"
	"github.com/lejaynd/webcrawler/internals/crawlerFrontier"
	"github.com/lejaynd/webcrawler/internals/db"
	"github.com/lejaynd/webcrawler/internals/dlq"
	"github.com/lejaynd/webcrawler/internals/lease"
	"github.com/lejaynd/webcrawler/internals/llm"
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
	bloomFilter     *bloom.BloomFilter
	redisClient     *redis.Client
	s3Store         *db.S3Store
	sqlStore        *db.SQLStore
	contextForLLM   *llm.ContextForLLM
}

func initSystem(ctx context.Context) *System {

	//redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		log.Fatalf("Failed to flush Redis: %v", err)
	}
	log.Println("[Redis] Database flushed.")

	//aws S3
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("AWS_REGION")

	if bucket == "" {
		log.Fatal("S3_BUCKET env var not set")
	}
	if region == "" {
		region = "Region env var not set"
	}

	s3Store := db.NewS3Store(s3Client, bucket, region)

	//postgres
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/webcrawler?sslmode=disable"
	}

	sqlStore, err := db.NewSQLStore(dsn)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	if err := sqlStore.CreateTable(); err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	log.Println("[DB] crawl_records table ready.")

	return &System{
		frontier:        crawlerFrontier.NewFrontier(),
		deadLetterQueue: dlq.NewInMemoryDLQ(),
		buffer:          buffer.NewInMemoryBuffer(),
		parserQueue:     parserQueue.NewParserQueue(),
		bloomFilter:     bloom.NewWithEstimates(10000000, 0.0001),
		redisClient:     redisClient,
		s3Store:         s3Store,
		sqlStore:        sqlStore,
		contextForLLM:   llm.NewContextForLLM(),
	}
}

func main() {

	if err := godotenv.Load(".env"); err != nil {
		log.Printf("[WARN] .env file not loaded: %v\n", err)
	}

	log.Println("Starting Web Crawler...")
	ctx := context.Background()

	system := initSystem(ctx)

	system.leaseManager = lease.NewLeaseManager(system.frontier.PushChan)
	system.scheduler = scheduler.NewScheduler(system.frontier, system.deadLetterQueue, system.buffer, system.bloomFilter)

	go system.frontier.Run()
	go system.scheduler.Run()
	go system.leaseManager.RecoverExpired()

	for i := 0; i < 10; i++ {
		p := parser.NewParserWorker(system.parserQueue, system.scheduler, system.redisClient, system.s3Store, system.contextForLLM)
		go p.Run()
	}

	for i := 0; i < 10; i++ {
		c := crawler.NewCrawlerWorker(system.frontier, system.parserQueue, system.scheduler, system.leaseManager, system.s3Store, system.sqlStore)
		go c.Run()
	}

	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		log.Fatal("GROQ_API_KEY env var not set")
	}
	groqClient := llm.NewGroqClient(groqKey, "llama-3.1-8b-instant")
	log.Println("[LLM] Using Groq API with model: llama-3.1-8b-instant")

	//_ = models.CrawlTask{}
	server := api.NewServer(system.scheduler.Incoming, system.contextForLLM, groqClient, system.scheduler)
	log.Fatal(server.Start(":8080"))
}
