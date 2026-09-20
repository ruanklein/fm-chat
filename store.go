package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	ID          string       `json:"id"`
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	CreatedAt   time.Time    `json:"createdAt"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

type conversationRecord struct {
	Conversation Conversation `json:"conversation"`
	Transcript   string       `json:"transcript"`
}

type conversationFile struct {
	Conversations []conversationRecord `json:"conversations"`
}

type conversationStore struct {
	directory string
	mu        sync.Mutex
	records   map[string]conversationRecord
}

func newConversationStore(directory string) (*conversationStore, error) {
	if err := os.MkdirAll(filepath.Join(directory, "attachments"), 0o700); err != nil {
		return nil, fmt.Errorf("create application data directory: %w", err)
	}

	store := &conversationStore{
		directory: directory,
		records:   map[string]conversationRecord{},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *conversationStore) create() (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	conversation := Conversation{
		ID:        newID(),
		Title:     "New chat",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	s.records[conversation.ID] = conversationRecord{
		Conversation: conversation,
		Transcript:   filepath.Join(s.directory, conversation.ID+".json"),
	}
	if err := s.save(); err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func (s *conversationStore) list() []Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()

	conversations := make([]Conversation, 0, len(s.records))
	for _, record := range s.records {
		conversations = append(conversations, record.Conversation)
	}
	sort.Slice(conversations, func(left, right int) bool {
		return conversations[left].UpdatedAt.After(conversations[right].UpdatedAt)
	})
	return conversations
}

func (s *conversationStore) get(id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok {
		return Conversation{}, errors.New("conversation not found")
	}
	return record.Conversation, nil
}

func (s *conversationStore) delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok {
		return errors.New("conversation not found")
	}
	delete(s.records, id)
	if err := s.save(); err != nil {
		s.records[id] = record
		return err
	}

	for _, message := range record.Conversation.Messages {
		for _, attachment := range message.Attachments {
			if err := removeIfExists(s.attachmentPath(attachment)); err != nil {
				return err
			}
		}
	}
	return removeIfExists(record.Transcript)
}

func (s *conversationStore) importImages(paths []string) ([]Attachment, error) {
	attachments := make([]Attachment, 0, len(paths))
	for _, sourcePath := range paths {
		attachment, err := s.copyImage(sourcePath)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (s *conversationStore) imagePaths(attachments []Attachment) ([]string, error) {
	paths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		path := s.attachmentPath(attachment)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("attachment %q is no longer available", attachment.Name)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func (s *conversationStore) startResponse(conversationID, prompt string, attachments []Attachment) (Message, Message, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[conversationID]
	if !ok {
		return Message{}, Message{}, "", errors.New("conversation not found")
	}

	now := time.Now()
	userMessage := Message{ID: newID(), Role: "user", Content: prompt, CreatedAt: now, Attachments: attachments}
	assistantMessage := Message{ID: newID(), Role: "assistant", CreatedAt: now, Attachments: []Attachment{}}
	record.Conversation.Messages = append(record.Conversation.Messages, userMessage, assistantMessage)
	record.Conversation.UpdatedAt = now
	if record.Conversation.Title == "New chat" && prompt != "" {
		record.Conversation.Title = titleFor(prompt)
	}
	s.records[conversationID] = record
	if err := s.save(); err != nil {
		return Message{}, Message{}, "", err
	}
	return userMessage, assistantMessage, record.Transcript, nil
}

func (s *conversationStore) updateMessage(conversationID, messageID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[conversationID]
	if !ok {
		return errors.New("conversation not found")
	}
	for index := range record.Conversation.Messages {
		if record.Conversation.Messages[index].ID != messageID {
			continue
		}
		record.Conversation.Messages[index].Content = content
		record.Conversation.UpdatedAt = time.Now()
		s.records[conversationID] = record
		return s.save()
	}
	return errors.New("message not found")
}

func (s *conversationStore) load() error {
	data, err := os.ReadFile(s.historyPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read conversation history: %w", err)
	}

	var history conversationFile
	if err := json.Unmarshal(data, &history); err != nil {
		return fmt.Errorf("read conversation history: %w", err)
	}
	for _, record := range history.Conversations {
		if record.Conversation.Messages == nil {
			record.Conversation.Messages = []Message{}
		}
		s.records[record.Conversation.ID] = record
	}
	return nil
}

func (s *conversationStore) save() error {
	records := make([]conversationRecord, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	data, err := json.Marshal(conversationFile{Conversations: records})
	if err != nil {
		return fmt.Errorf("save conversation history: %w", err)
	}

	temporaryPath := s.historyPath() + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf("save conversation history: %w", err)
	}
	if err := os.Rename(temporaryPath, s.historyPath()); err != nil {
		return fmt.Errorf("save conversation history: %w", err)
	}
	return nil
}

func (s *conversationStore) copyImage(sourcePath string) (Attachment, error) {
	if !isImagePath(sourcePath) {
		return Attachment{}, errors.New("documents are not supported by the Foundation Models CLI")
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return Attachment{}, err
	}
	defer source.Close()

	attachment := Attachment{ID: newID(), Name: filepath.Base(sourcePath), MimeType: mime.TypeByExtension(filepath.Ext(sourcePath))}
	destination, err := os.OpenFile(s.attachmentPath(attachment), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Attachment{}, err
	}
	defer destination.Close()
	if _, err := io.Copy(destination, source); err != nil {
		return Attachment{}, err
	}
	return attachment, nil
}

func (s *conversationStore) attachmentPath(attachment Attachment) string {
	return filepath.Join(s.directory, "attachments", attachment.ID+filepath.Ext(attachment.Name))
}

func (s *conversationStore) historyPath() string {
	return filepath.Join(s.directory, "conversations.json")
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove local chat data: %w", err)
	}
	return nil
}

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".heic", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func newID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func titleFor(prompt string) string {
	words := strings.Fields(prompt)
	if len(words) > 7 {
		words = words[:7]
	}
	return strings.Join(words, " ")
}
