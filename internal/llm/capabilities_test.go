package llm

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ============================================================================
// Mock implementations
// ============================================================================

type mockTextGenerator struct {
	generateResponseFn          func(string, []*tgbotapi.Message, *tgbotapi.Message, float32) (string, error)
	generateResponseFromTextFn  func(string, string, float32) (string, error)
	generateArbitraryResponseFn func(string, string, float32) (string, error)
	generateResponseByTypeFn    func(ResponseType, string, string, float32) (string, error)
}

func (m *mockTextGenerator) GenerateResponse(sp string, h []*tgbotapi.Message, lm *tgbotapi.Message, t float32) (string, error) {
	if m.generateResponseFn != nil {
		return m.generateResponseFn(sp, h, lm, t)
	}
	return "mock response", nil
}
func (m *mockTextGenerator) GenerateResponseFromTextContext(sp, ct string, t float32) (string, error) {
	if m.generateResponseFromTextFn != nil {
		return m.generateResponseFromTextFn(sp, ct, t)
	}
	return "mock text context response", nil
}
func (m *mockTextGenerator) GenerateArbitraryResponse(sp, ct string, t float32) (string, error) {
	if m.generateArbitraryResponseFn != nil {
		return m.generateArbitraryResponseFn(sp, ct, t)
	}
	return "mock arbitrary response", nil
}
func (m *mockTextGenerator) GenerateResponseByType(rt ResponseType, sp, ct string, t float32) (string, error) {
	if m.generateResponseByTypeFn != nil {
		return m.generateResponseByTypeFn(rt, sp, ct, t)
	}
	return "mock typed response", nil
}

type mockAudioTranscriber struct {
	transcribeAudioFn func([]byte, string) (string, error)
}

func (m *mockAudioTranscriber) TranscribeAudio(data []byte, mime string) (string, error) {
	if m.transcribeAudioFn != nil {
		return m.transcribeAudioFn(data, mime)
	}
	return "mock transcription", nil
}

type mockEmbedder struct {
	embedContentFn func(string) ([]float32, error)
}

func (m *mockEmbedder) EmbedContent(text string) ([]float32, error) {
	if m.embedContentFn != nil {
		return m.embedContentFn(text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

type mockImageAnalyzer struct {
	generateContentWithImageFn func(context.Context, string, []byte, string) (string, error)
}

func (m *mockImageAnalyzer) GenerateContentWithImage(ctx context.Context, sp string, img []byte, cap string) (string, error) {
	if m.generateContentWithImageFn != nil {
		return m.generateContentWithImageFn(ctx, sp, img, cap)
	}
	return "mock image analysis", nil
}

type mockImageGenerator struct {
	generateImageWithEditFn func(context.Context, []byte, string) ([]byte, error)
}

func (m *mockImageGenerator) GenerateImageWithEdit(ctx context.Context, img []byte, prompt string) ([]byte, error) {
	if m.generateImageWithEditFn != nil {
		return m.generateImageWithEditFn(ctx, img, prompt)
	}
	return []byte{0x00, 0x01, 0x02}, nil
}

type mockAudioGenerator struct {
	generateAudioFn func(string, AudioParams) ([]byte, error)
}

func (m *mockAudioGenerator) GenerateAudio(text string, params AudioParams) ([]byte, error) {
	if m.generateAudioFn != nil {
		return m.generateAudioFn(text, params)
	}
	return []byte{0xFF, 0xFE}, nil
}

type mockCloser struct {
	closeFn func() error
}

func (m *mockCloser) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// ============================================================================
// Compile-time checks: каждый mock удовлетворяет интерфейсу
// ============================================================================

var _ TextGenerator = (*mockTextGenerator)(nil)
var _ AudioTranscriber = (*mockAudioTranscriber)(nil)
var _ Embedder = (*mockEmbedder)(nil)
var _ ImageAnalyzer = (*mockImageAnalyzer)(nil)
var _ ImageGenerator = (*mockImageGenerator)(nil)
var _ AudioGenerator = (*mockAudioGenerator)(nil)
var _ Closer = (*mockCloser)(nil)

// ============================================================================
// Compile-time check: композитный LLMClient удовлетворяется mock-ом
// ============================================================================

// mockLLMClient — полная mock-реализация композитного LLMClient.
type mockLLMClient struct {
	*mockTextGenerator
	*mockAudioTranscriber
	*mockEmbedder
	*mockImageAnalyzer
	*mockImageGenerator
	*mockAudioGenerator
	*mockCloser
}

var _ LLMClient = (*mockLLMClient)(nil)

// ============================================================================
// Тесты: Изоляция capability-интерфейсов
// ============================================================================

// TestTextGeneratorIsolation проверяет, что TextGenerator работает без других интерфейсов.
func TestTextGeneratorIsolation(t *testing.T) {
	var tg TextGenerator = &mockTextGenerator{}
	resp, err := tg.GenerateResponse("prompt", nil, nil, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}
}

// TestAudioTranscriberIsolation проверяет изоляцию AudioTranscriber.
func TestAudioTranscriberIsolation(t *testing.T) {
	var at AudioTranscriber = &mockAudioTranscriber{}
	text, err := at.TranscribeAudio([]byte{0x00}, "audio/ogg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty transcription")
	}
}

// TestEmbedderIsolation проверяет изоляцию Embedder.
func TestEmbedderIsolation(t *testing.T) {
	var e Embedder = &mockEmbedder{}
	vec, err := e.EmbedContent("test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) == 0 {
		t.Error("expected non-empty embedding vector")
	}
}

// TestImageAnalyzerIsolation проверяет изоляцию ImageAnalyzer.
func TestImageAnalyzerIsolation(t *testing.T) {
	var ia ImageAnalyzer = &mockImageAnalyzer{}
	desc, err := ia.GenerateContentWithImage(context.Background(), "prompt", []byte{0x00}, "caption")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc == "" {
		t.Error("expected non-empty description")
	}
}

// TestImageGeneratorIsolation проверяет изоляцию ImageGenerator.
func TestImageGeneratorIsolation(t *testing.T) {
	var ig ImageGenerator = &mockImageGenerator{}
	img, err := ig.GenerateImageWithEdit(context.Background(), []byte{0x00}, "edit prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(img) == 0 {
		t.Error("expected non-empty image data")
	}
}

// TestAudioGeneratorIsolation проверяет изоляцию AudioGenerator.
func TestAudioGeneratorIsolation(t *testing.T) {
	var ag AudioGenerator = &mockAudioGenerator{}
	audio, err := ag.GenerateAudio("hello", AudioParams{VoiceName: "default", Temperature: 0.5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(audio) == 0 {
		t.Error("expected non-empty audio data")
	}
}

// TestCloserIsolation проверяет изоляцию Closer.
func TestCloserIsolation(t *testing.T) {
	var c Closer = &mockCloser{}
	if err := c.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCompatLLMClient проверяет, что композитный LLMClient работает.
func TestCompatLLMClient(t *testing.T) {
	var client LLMClient = &mockLLMClient{
		mockTextGenerator:    &mockTextGenerator{},
		mockAudioTranscriber: &mockAudioTranscriber{},
		mockEmbedder:         &mockEmbedder{},
		mockImageAnalyzer:    &mockImageAnalyzer{},
		mockImageGenerator:   &mockImageGenerator{},
		mockAudioGenerator:   &mockAudioGenerator{},
		mockCloser:           &mockCloser{},
	}

	// Проверяем, что все методы доступны
	resp, err := client.GenerateResponse("p", nil, nil, 0.5)
	if err != nil || resp == "" {
		t.Error("GenerateResponse failed")
	}

	text, err := client.TranscribeAudio([]byte{0x00}, "audio/ogg")
	if err != nil || text == "" {
		t.Error("TranscribeAudio failed")
	}

	vec, err := client.EmbedContent("text")
	if err != nil || len(vec) == 0 {
		t.Error("EmbedContent failed")
	}

	desc, err := client.GenerateContentWithImage(context.Background(), "p", []byte{0x00}, "c")
	if err != nil || desc == "" {
		t.Error("GenerateContentWithImage failed")
	}

	img, err := client.GenerateImageWithEdit(context.Background(), []byte{0x00}, "edit")
	if err != nil || len(img) == 0 {
		t.Error("GenerateImageWithEdit failed")
	}

	audio, err := client.GenerateAudio("hello", AudioParams{})
	if err != nil || len(audio) == 0 {
		t.Error("GenerateAudio failed")
	}

	if err := client.Close(); err != nil {
		t.Error("Close failed")
	}
}
