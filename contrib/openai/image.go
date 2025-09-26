package openai

import (
	"context"

	"github.com/go-kratos/blades"
)

// ImageProvider is an implementation of ModelProvider for OpenAI's image generation models.
// https://platform.openai.com/docs/api-reference/images/create
type ImageProvider struct{}

// NewImageProvider creates a new instance of ImageProvider.
func NewImageProvider() blades.ModelProvider {
	return &ImageProvider{}
}

// Generate generates an image based on the provided ModelRequest.
func (p *ImageProvider) Generate(context.Context, *blades.ModelRequest, ...blades.ModelOption) (*blades.ModelResponse, error) {
	panic("not implemented")
	return nil, nil
}

// NewStream is not supported for image generation and returns nil.
func (p *ImageProvider) NewStream(context.Context, *blades.ModelRequest, ...blades.ModelOption) (blades.Streamer[*blades.ModelResponse], error) {
	panic("not implemented")
	return nil, nil
}
