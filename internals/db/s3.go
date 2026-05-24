package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// JSON for S3 storage
type CrawlObject struct {
	URL         string `json:"url"`
	HTMLContent string `json:"html_content"`
	TextContent string `json:"text_content"`
}

type S3Store struct {
	Client *s3.Client
	Bucket string
	Region string
}

func NewS3Store(client *s3.Client, bucket, region string) *S3Store {
	return &S3Store{Client: client, Bucket: bucket, Region: region}
}

func s3KeyForURL(pageURL string) string {
	h := sha256.Sum256([]byte(pageURL))
	return "crawls/" + hex.EncodeToString(h[:])
}

func (s *S3Store) S3LinkForKey(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.Bucket, s.Region, key)
}

func (s *S3Store) UploadHTML(ctx context.Context, pageURL, htmlContent string) (string, string, error) {
	key := s3KeyForURL(pageURL)

	obj := CrawlObject{
		URL:         pageURL,
		HTMLContent: htmlContent,
		TextContent: "",
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return "", "", fmt.Errorf("marshal crawl object: %w", err)
	}

	_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", "", fmt.Errorf("s3 put object: %w", err)
	}

	return key, s.S3LinkForKey(key), nil
}

func (s *S3Store) GetObject(ctx context.Context, s3Key string) (*CrawlObject, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object: %w", err)
	}

	defer out.Body.Close()

	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read s3 body: %w", err)
	}

	var obj CrawlObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshal crawl object: %w", err)
	}
	return &obj, nil
}

func (s *S3Store) UpdateTextContent(ctx context.Context, s3Key, textContent string) error {
	obj, err := s.GetObject(ctx, s3Key)
	if err != nil {
		return err
	}
	obj.TextContent = textContent

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshal updated object: %w", err)
	}

	_, err = s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.Bucket),
		Key:         aws.String(s3Key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	return err
}
