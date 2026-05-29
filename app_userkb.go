package main

import (
	"context"
	"errors"
	"strings"

	"futrixdata/platform/internal/aichat"
	"futrixdata/platform/internal/userkb"
)

type userKBModel struct {
	delegate aichat.Model
}

func (m userKBModel) Chat(ctx context.Context, systemPrompt string, messages []userkb.Message) (string, error) {
	if m.delegate == nil {
		return "", errors.New("ai provider not configured")
	}
	converted := make([]aichat.Message, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		converted = append(converted, aichat.Message{Role: role, Content: msg.Content})
	}
	return m.delegate.Chat(ctx, systemPrompt, converted)
}

type userKBModelResolver struct {
	delegate aichat.ModelResolver
}

func newUserKBModelResolver(delegate aichat.ModelResolver) userkb.ModelResolver {
	if delegate == nil {
		return nil
	}
	return userKBModelResolver{delegate: delegate}
}

func (r userKBModelResolver) Resolve(aiConfigID string) (userkb.Model, error) {
	if r.delegate == nil {
		return nil, errors.New("ai provider not configured")
	}
	model, err := r.delegate.Resolve(aiConfigID)
	if err != nil {
		return nil, err
	}
	return userKBModel{delegate: model}, nil
}

func (a *App) UserKBList() (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.List(ctx)
}

func (a *App) UserKBCreateCategory(input userkb.CategoryCreateInput) (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.CreateCategory(ctx, input)
}

func (a *App) UserKBUpdateCategory(id string, input userkb.CategoryUpdateInput) (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.UpdateCategory(ctx, id, input)
}

func (a *App) UserKBDeleteCategory(id string) (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.DeleteCategory(ctx, id)
}

func (a *App) UserKBUploadFiles(categoryID string, files []userkb.UploadFileInput) (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.UploadFiles(ctx, categoryID, files, "")
}

func (a *App) UserKBDeleteFile(fileID string) (userkb.ViewState, error) {
	if a.userKB == nil {
		return userkb.ViewState{}, errors.New("user knowledge base is not available")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.userKB.DeleteFile(ctx, fileID)
}
