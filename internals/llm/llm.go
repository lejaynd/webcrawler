package llm

import (
	"sync"
)

type ContextChunk struct {
	TextContent string
	URL         string
}

type ContextForLLM struct {
	Contexts []ContextChunk
	mu       sync.Mutex
}

func NewContextForLLM() *ContextForLLM {
	return &ContextForLLM{
		Contexts: make([]ContextChunk, 0),
	}
}

func (c *ContextForLLM) AddContext(TextContent string, URL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := ContextChunk{
		TextContent: TextContent,
		URL:         URL,
	}
	c.Contexts = append(c.Contexts, ctx)
}

func (c *ContextForLLM) GetContexts() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := ""
	for _, ctx := range c.Contexts {
		result += "[" + ctx.TextContent + " source: " + ctx.URL + "]\n"
	}
	return result
}

func (c *ContextForLLM) PageCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.Contexts)
}

func (c *ContextForLLM) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Contexts = make([]ContextChunk, 0)
}
