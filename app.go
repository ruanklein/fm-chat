package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ruanklein/fmgo"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	client     *fmgo.Client
	store      *conversationStore
	setupErr   error
	userAvatar string
	cancels    map[string]context.CancelFunc
	cancelsMu  sync.Mutex
}

type ModelStatus struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

type ChatEvent struct {
	ConversationID string `json:"conversationID"`
	MessageID      string `json:"messageID"`
	Text           string `json:"text"`
}

func NewApp() *App {
	return &App{cancels: map[string]context.CancelFunc{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	directory, err := os.UserConfigDir()
	if err != nil {
		a.setupErr = fmt.Errorf("find application support directory: %w", err)
		return
	}

	a.store, err = newConversationStore(filepath.Join(directory, "FM Chat"))
	if err != nil {
		a.setupErr = err
		return
	}

	if homeDirectory, err := os.UserHomeDir(); err == nil {
		imagePath := filepath.Join(directory, "FM Chat", accountImageFile)
		if err := captureAccountImage(ctx, homeDirectory, imagePath); err == nil {
			if imageData, readErr := os.ReadFile(imagePath); readErr == nil {
				a.userAvatar = imageDataURL(imageData)
			}
		}
	}

	a.client, err = fmgo.New()
	if err != nil {
		a.setupErr = err
	}
}

func (a *App) GetUserAvatar() string {
	return a.userAvatar
}

func (a *App) GetModelStatus() ModelStatus {
	if a.setupErr != nil {
		return ModelStatus{Message: a.setupErr.Error()}
	}

	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()

	availability, err := a.client.Available(ctx)
	if err != nil {
		return ModelStatus{Message: err.Error()}
	}
	for _, model := range availability.Models {
		if model.Model != fmgo.ModelSystem {
			continue
		}
		if model.Available {
			return ModelStatus{Available: true, Message: "On-device model ready"}
		}
		return ModelStatus{Message: model.Reason}
	}
	return ModelStatus{Message: "The on-device model is unavailable"}
}

func (a *App) ListConversations() ([]Conversation, error) {
	if a.setupErr != nil {
		return nil, a.setupErr
	}
	return a.store.list(), nil
}

func (a *App) CreateConversation() (Conversation, error) {
	if a.setupErr != nil {
		return Conversation{}, a.setupErr
	}
	return a.store.create()
}

func (a *App) GetConversation(id string) (Conversation, error) {
	if a.setupErr != nil {
		return Conversation{}, a.setupErr
	}
	return a.store.get(id)
}

func (a *App) DeleteConversation(id string) error {
	if a.setupErr != nil {
		return a.setupErr
	}
	if a.isGenerating(id) {
		return errors.New("stop the response before deleting this chat")
	}
	return a.store.delete(id)
}

func (a *App) ChooseImages() ([]Attachment, error) {
	if a.setupErr != nil {
		return nil, a.setupErr
	}

	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose images",
		Filters: []runtime.FileFilter{{
			DisplayName: "Images",
			Pattern:     "*.png;*.jpg;*.jpeg;*.heic;*.gif;*.webp",
		}},
	})
	if err != nil {
		return nil, err
	}
	return a.store.importImages(paths)
}

func (a *App) SendMessage(conversationID, prompt string, attachments []Attachment) (Message, error) {
	if a.setupErr != nil {
		return Message{}, a.setupErr
	}
	if strings.TrimSpace(prompt) == "" && len(attachments) == 0 {
		return Message{}, errors.New("write a message or attach an image")
	}
	if a.isGenerating(conversationID) {
		return Message{}, errors.New("this conversation is already generating a response")
	}

	imagePaths, err := a.store.imagePaths(attachments)
	if err != nil {
		return Message{}, err
	}

	userMessage, assistantMessage, transcriptPath, err := a.store.startResponse(
		conversationID,
		strings.TrimSpace(prompt),
		attachments,
	)
	if err != nil {
		return Message{}, err
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.setCancel(conversationID, cancel)

	request := fmgo.Request{
		Prompt:         userMessage.Content,
		Images:         imagePaths,
		Model:          fmgo.ModelSystem,
		SaveTranscript: transcriptPath,
	}
	if _, err := os.Stat(transcriptPath); err == nil {
		request.Resume = transcriptPath
	}

	go a.streamResponse(ctx, conversationID, assistantMessage.ID, request)
	return assistantMessage, nil
}

func (a *App) CancelGeneration(conversationID string) {
	a.cancelsMu.Lock()
	cancel := a.cancels[conversationID]
	a.cancelsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) streamResponse(ctx context.Context, conversationID, messageID string, request fmgo.Request) {
	defer a.clearCancel(conversationID)

	stream, err := a.client.Stream(ctx, request)
	if err != nil {
		a.emitError(conversationID, messageID, err)
		return
	}
	defer stream.Close()

	var response strings.Builder
	for stream.Next() {
		chunk := stream.Text()
		response.WriteString(chunk)
		if err := a.store.updateMessage(conversationID, messageID, response.String()); err != nil {
			a.emitError(conversationID, messageID, err)
			return
		}
		runtime.EventsEmit(a.ctx, "chat:chunk", ChatEvent{
			ConversationID: conversationID,
			MessageID:      messageID,
			Text:           chunk,
		})
	}

	if err := stream.Err(); err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		a.emitError(conversationID, messageID, err)
		return
	}
	runtime.EventsEmit(a.ctx, "chat:complete", ChatEvent{
		ConversationID: conversationID,
		MessageID:      messageID,
		Text:           response.String(),
	})
}

func (a *App) emitError(conversationID, messageID string, err error) {
	runtime.EventsEmit(a.ctx, "chat:error", ChatEvent{
		ConversationID: conversationID,
		MessageID:      messageID,
		Text:           err.Error(),
	})
}

func (a *App) isGenerating(conversationID string) bool {
	a.cancelsMu.Lock()
	defer a.cancelsMu.Unlock()
	_, generating := a.cancels[conversationID]
	return generating
}

func (a *App) setCancel(conversationID string, cancel context.CancelFunc) {
	a.cancelsMu.Lock()
	defer a.cancelsMu.Unlock()
	a.cancels[conversationID] = cancel
}

func (a *App) clearCancel(conversationID string) {
	a.cancelsMu.Lock()
	defer a.cancelsMu.Unlock()
	delete(a.cancels, conversationID)
}
