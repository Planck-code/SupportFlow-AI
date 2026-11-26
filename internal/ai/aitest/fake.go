// Package aitest provides a deterministic model client for service tests.
package aitest

import (
	"context"

	"github.com/Planck-code/SupportFlow-AI/internal/ai"
)

type Fake struct {
	Content string
	Err     error
}

var _ ai.Client = Fake{}

func (f Fake) Complete(ctx context.Context, _ []ai.Message) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.Err != nil {
		return "", f.Err
	}
	return f.Content, nil
}
