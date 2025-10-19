package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/go-kratos/blades"
	"github.com/go-kratos/blades/tools"
	"google.golang.org/genai"
)

func convertMessageToGenAI(req *blades.ModelRequest) (*genai.Content, []*genai.Content, error) {
	var (
		system   *genai.Content
		contents []*genai.Content
	)
	for _, msg := range req.Messages {
		switch msg.Role {
		case blades.RoleSystem:
			parts, err := convertMessagePartsToGenAI(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			system = &genai.Content{Parts: parts}
		case blades.RoleUser:
			parts, err := convertMessagePartsToGenAI(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: parts})
		case blades.RoleAssistant:
			parts, err := convertMessagePartsToGenAI(msg.Parts)
			if err != nil {
				return nil, nil, err
			}
			// If message has tool calls, add them as FunctionCall parts
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					// Parse arguments back to map
					var args map[string]any
					if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
						return nil, nil, fmt.Errorf("unmarshaling tool arguments: %w", err)
					}
					parts = append(parts, genai.NewPartFromFunctionCall(tc.Name, args))
				}
			}
			contents = append(contents, &genai.Content{Role: genai.RoleModel, Parts: parts})
		case blades.RoleTool:
			// Tool result messages - convert to function response parts
			var parts []*genai.Part
			for _, tc := range msg.ToolCalls {
				// Create response map with result
				response := map[string]any{
					"output": tc.Result,
				}
				parts = append(parts, genai.NewPartFromFunctionResponse(tc.Name, response))
			}
			// Tool results are sent as user role in Gemini
			contents = append(contents, &genai.Content{Role: genai.RoleUser, Parts: parts})
		}
	}
	return system, contents, nil
}

func convertMessagePartsToGenAI(parts []blades.Part) ([]*genai.Part, error) {
	res := make([]*genai.Part, 0, len(parts))
	for _, part := range parts {
		switch v := part.(type) {
		case blades.TextPart:
			res = append(res, &genai.Part{Text: v.Text})
		case blades.DataPart:
			res = append(res, &genai.Part{
				InlineData: &genai.Blob{
					Data:        v.Bytes,
					DisplayName: v.Name,
					MIMEType:    string(v.MIMEType),
				},
			})
		case blades.FilePart:
			res = append(res, &genai.Part{
				FileData: &genai.FileData{
					FileURI:     v.URI,
					DisplayName: v.Name,
					MIMEType:    string(v.MIMEType),
				},
			})
		default:
			return nil, fmt.Errorf("unsupported part type: %T", part)
		}
	}
	return res, nil
}

func convertBladesToolsToGenAI(tools []*tools.Tool) ([]*genai.Tool, error) {
	genaiTools := make([]*genai.Tool, 0, len(tools))
	for _, tool := range tools {
		genaiTool, err := convertBladesToolToGenAI(tool)
		if err != nil {
			return nil, fmt.Errorf("converting tool %s: %w", tool.Name, err)
		}
		if genaiTool != nil {
			genaiTools = append(genaiTools, genaiTool)
		}
	}
	return genaiTools, nil
}

func convertBladesToolToGenAI(tool *tools.Tool) (*genai.Tool, error) {
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			&genai.FunctionDeclaration{
				Name:                 tool.Name,
				Description:          tool.Description,
				ParametersJsonSchema: tool.InputSchema,
			},
		},
	}, nil
}

func convertGenAIToBlades(resp *genai.GenerateContentResponse) (*blades.ModelResponse, error) {
	message := &blades.Message{
		Role:   blades.RoleAssistant,
		Status: blades.StatusIncomplete,
	}
	for _, candidate := range resp.Candidates {
		if candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			// Check if this is a function call
			if part.FunctionCall != nil {
				// Convert to blades.ToolCall
				args, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					return nil, fmt.Errorf("marshaling function call args: %w", err)
				}
				message.ToolCalls = append(message.ToolCalls, &blades.ToolCall{
					ID:        part.FunctionCall.ID,
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				})
			} else {
				// Regular part (text, file, inline data)
				bladesPart, err := convertGenAIPartToBlades(part)
				if err != nil {
					return nil, err
				}
				message.Parts = append(message.Parts, bladesPart)
			}
		}
	}
	return &blades.ModelResponse{Message: message}, nil
}

// convertGenAIPartToBlades converts a GenAI Part to Blades Part
func convertGenAIPartToBlades(part *genai.Part) (blades.Part, error) {
	if part.FileData != nil {
		return blades.FilePart{
			URI:      part.FileData.FileURI,
			Name:     part.FileData.DisplayName,
			MIMEType: blades.MIMEType(part.FileData.MIMEType),
		}, nil
	}
	if part.InlineData != nil {
		return blades.DataPart{
			Bytes:    part.InlineData.Data,
			Name:     part.InlineData.DisplayName,
			MIMEType: blades.MIMEType(part.InlineData.MIMEType),
		}, nil
	}
	return blades.TextPart{Text: part.Text}, nil
}
