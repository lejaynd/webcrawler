package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type GroqClient struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func NewGroqClient(apiKey string, model string) *GroqClient {
	if model == "" {
		model = "llama-3.1-8b-instant"
	}
	return &GroqClient{
		baseURL: "https://api.groq.com/openai/v1",
		model:   model,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 3 * time.Minute,
		},
	}
}

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model    string        `json:"model"`
	Messages []groqMessage `json:"messages"`
}

type groqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (g *GroqClient) AskWithContext(question string, crawledContext string) (string, []string, error) {
	prompt := fmt.Sprintf(`You are a helpful assistant. Answer the user's question using ONLY the provided context from crawled web pages. Each context chunk is wrapped in brackets with its source URL.

CONTEXT:
%s

QUESTION: %s

Instructions:
- Answer the question based on the context above.
- At the end, list the source URLs you used as citations.
- Format your response EXACTLY as:

ANSWER:
Your answer here.

CITATIONS:
- https://example.com/page1
- https://example.com/page2
`, crawledContext, question)

	reqBody := groqRequest{
		Model: g.model,
		Messages: []groqMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[Groq] Sending request to %s/chat/completions (model: %s, prompt: %d chars)\n", g.baseURL, g.model, len(prompt))

	req, err := http.NewRequest("POST", g.baseURL+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return "", nil, fmt.Errorf("failed to create groq request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("groq request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read groq response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("groq returned status %d: %s", resp.StatusCode, string(body))
	}

	var groqResp groqResponse
	if err := json.Unmarshal(body, &groqResp); err != nil {
		return "", nil, fmt.Errorf("failed to parse groq response: %w", err)
	}

	if len(groqResp.Choices) == 0 || groqResp.Choices[0].Message.Content == "" {
		return "", nil, fmt.Errorf("groq returned empty response")
	}

	responseContent := groqResp.Choices[0].Message.Content

	log.Printf("[Groq] Got response: %d chars\n", len(responseContent))

	answer, citations := parseResponse(responseContent)
	return answer, citations, nil
}

func parseResponse(response string) (string, []string) {
	answerPart := ""
	var citations []string

	parts := strings.SplitN(response, "CITATIONS:", 2)
	if len(parts) == 2 {
		answerPart = strings.TrimSpace(parts[0])
		answerPart = strings.TrimPrefix(answerPart, "ANSWER:")
		answerPart = strings.TrimSpace(answerPart)

		citationLines := strings.Split(strings.TrimSpace(parts[1]), "\n")
		for _, line := range citationLines {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "- ")
			line = strings.TrimSpace(line)
			if line != "" {
				citations = append(citations, line)
			}
		}
	} else {
		answerPart = strings.TrimPrefix(strings.TrimSpace(response), "ANSWER:")
		answerPart = strings.TrimSpace(answerPart)
	}

	return answerPart, citations
}
