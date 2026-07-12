package videoedit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTranscribeStreamsOpenAIMultipartAndConvertsTimestamps(t *testing.T) {
	t.Parallel()

	const apiKey = "unit-test-secret"
	var gotFields map[string][]string
	var gotAudio string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			return nil, fmt.Errorf("request method = %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer "+apiKey {
			return nil, fmt.Errorf("unexpected Authorization header")
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			return nil, err
		}
		gotFields = request.MultipartForm.Value
		file, _, err := request.FormFile("file")
		if err != nil {
			return nil, err
		}
		defer file.Close()
		audio, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		gotAudio = string(audio)

		payload := []byte(`{
  "language": "en",
  "duration": 2.345678,
  "text": "Hello mat.",
  "words": [
    {"word": "Hello", "start": 0.1234567, "end": 0.75},
    {"word": "mat.", "start": 0.80, "end": 2.345678}
  ],
  "segments": [
    {"text": "Hello mat.", "start": 0.10, "end": 2.345678, "avg_logprob": -0.25}
  ]
}`)
		return httpResponse(request, http.StatusOK, "application/json", payload), nil
	})}

	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("prepared audio bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(t.TempDir(), "transcript.json")
	transcript, err := Transcribe(context.Background(), audioPath, TranscribeOptions{
		SourceID:      "camera-a",
		APIKey:        apiKey,
		Endpoint:      "https://api.example.test/v1/audio/transcriptions",
		Model:         "whisper-1",
		Prompt:        "Enterprise Vibe Code",
		Glossary:      []string{"Jiu-Jitsu", "De La Riva"},
		HTTPClient:    client,
		PreparedAudio: true,
		CachePath:     cachePath,
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}

	if gotAudio != "prepared audio bytes" {
		t.Fatalf("uploaded audio = %q", gotAudio)
	}
	if !reflect.DeepEqual(gotFields["model"], []string{"whisper-1"}) {
		t.Fatalf("model fields = %#v", gotFields["model"])
	}
	if !reflect.DeepEqual(gotFields["response_format"], []string{"verbose_json"}) {
		t.Fatalf("response format = %#v", gotFields["response_format"])
	}
	if !reflect.DeepEqual(gotFields["timestamp_granularities[]"], []string{"word", "segment"}) {
		t.Fatalf("timestamp granularities = %#v", gotFields["timestamp_granularities[]"])
	}
	if !reflect.DeepEqual(gotFields["language"], []string{"en"}) {
		t.Fatalf("language fields = %#v", gotFields["language"])
	}
	wantPrompt := "Enterprise Vibe Code\n\nTerminology: Jiu-Jitsu, De La Riva"
	if !reflect.DeepEqual(gotFields["prompt"], []string{wantPrompt}) {
		t.Fatalf("prompt fields = %#v, want %q", gotFields["prompt"], wantPrompt)
	}

	if transcript.Provider != "openai" || transcript.Model != "whisper-1" || transcript.Language != "en" {
		t.Fatalf("transcript metadata = %#v", transcript)
	}
	if transcript.SourceID != "camera-a" || transcript.SourceAudio != audioPath {
		t.Fatalf("source metadata = %#v", transcript)
	}
	if transcript.DurationUS != 2_345_678 {
		t.Fatalf("DurationUS = %d", transcript.DurationUS)
	}
	if len(transcript.Words) != 2 || len(transcript.Segments) != 1 {
		t.Fatalf("words/segments = %d/%d", len(transcript.Words), len(transcript.Segments))
	}
	if transcript.Words[0].StartUS != 123_457 || transcript.Words[1].EndUS != 2_345_678 {
		t.Fatalf("word timestamps = %#v", transcript.Words)
	}
	if transcript.Segments[0].StartUS != 100_000 || transcript.Segments[0].EndUS != 2_345_678 {
		t.Fatalf("segment timestamps = %#v", transcript.Segments)
	}
	if transcript.Words[0].SegmentID != transcript.Segments[0].ID || transcript.Words[1].SegmentID != transcript.Segments[0].ID {
		t.Fatalf("word segment links = %#v, segment = %#v", transcript.Words, transcript.Segments[0])
	}
	if len(transcript.ContentHash) != 64 {
		t.Fatalf("ContentHash length = %d", len(transcript.ContentHash))
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("transcript cache was not written: %v", err)
	}
}

func TestTranscribeUsesCacheWithoutCallingProvider(t *testing.T) {
	t.Parallel()

	cachePath := filepath.Join(t.TempDir(), "transcript.json")
	want := Transcript{
		SchemaVersion: CurrentSchemaVersion,
		SourceID:      "cached-source",
		Provider:      "cached-provider",
		DurationUS:    1_000_000,
		Text:          "Already transcribed.",
	}
	if err := writeTranscriptCache(cachePath, want); err != nil {
		t.Fatal(err)
	}
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, fmt.Errorf("provider must not be called")
	})}

	got, err := Transcribe(context.Background(), "/path/does/not/need/to/exist.mov", TranscribeOptions{
		CachePath:     cachePath,
		HTTPClient:    client,
		PreparedAudio: true,
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if called {
		t.Fatal("provider was called despite a valid cache")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cached transcript = %#v, want %#v", got, want)
	}
}

func TestTranscribeLocalCommandAdapter(t *testing.T) {
	if os.Getenv("VIDEO_EDIT_LOCAL_HELPER") == "1" {
		return
	}

	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("local audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "must-not-reach-local-command")
	transcript, err := Transcribe(context.Background(), audioPath, TranscribeOptions{
		SourceID:      "local-source",
		PreparedAudio: true,
		LocalCommand: []string{
			os.Args[0],
			"-test.run=TestTranscribeLocalHelperProcess",
			"--",
			"{audio}",
			"{output}",
		},
		LocalCommandEnv: []string{"VIDEO_EDIT_LOCAL_HELPER=1"},
	})
	if err != nil {
		t.Fatalf("Transcribe() with local adapter error = %v", err)
	}
	if transcript.Provider != "local-command" || transcript.SourceID != "local-source" {
		t.Fatalf("transcript metadata = %#v", transcript)
	}
	if len(transcript.Words) != 1 || transcript.Words[0].StartUS != 250_000 || transcript.Words[0].EndUS != 750_000 {
		t.Fatalf("local words = %#v", transcript.Words)
	}
}

func TestTranscribeLocalHelperProcess(t *testing.T) {
	if os.Getenv("VIDEO_EDIT_LOCAL_HELPER") != "1" {
		return
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		fmt.Fprintln(os.Stderr, "OpenAI API key leaked to local adapter")
		os.Exit(2)
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || len(os.Args) <= separator+2 {
		fmt.Fprintln(os.Stderr, "helper arguments missing")
		os.Exit(2)
	}
	audioPath := os.Args[separator+1]
	outputPath := os.Args[separator+2]
	if _, err := os.ReadFile(audioPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	payload := []byte(`{
  "language":"en",
  "duration":1.0,
  "text":"Local.",
  "words":[{"word":"Local.","start":0.25,"end":0.75}],
  "segments":[{"text":"Local.","start":0.0,"end":1.0}]
}`)
	if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestTranscribeRedactsAPIKeyFromHTTPError(t *testing.T) {
	t.Parallel()

	const secret = "do-not-print-this-key"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, request.Body)
		return httpResponse(request, http.StatusUnauthorized, "application/json", []byte(`{"error":"bad key `+secret+`"}`)), nil
	})}
	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Transcribe(context.Background(), audioPath, TranscribeOptions{
		APIKey:        secret,
		Endpoint:      "https://api.example.test/v1/audio/transcriptions",
		HTTPClient:    client,
		PreparedAudio: true,
	})
	if err == nil {
		t.Fatal("Transcribe() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked API key: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error did not identify redaction: %v", err)
	}
}

func TestTranscribeRetries429AndServerErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		_, _ = io.Copy(io.Discard, request.Body)
		switch attempts {
		case 1:
			response := httpResponse(request, http.StatusTooManyRequests, "application/json", []byte(`{"error":"slow down"}`))
			response.Header.Set("Retry-After", "0")
			return response, nil
		case 2:
			return httpResponse(request, http.StatusBadGateway, "application/json", []byte(`{"error":"upstream"}`)), nil
		default:
			return httpResponse(request, http.StatusOK, "application/json", []byte(`{"language":"en","duration":1,"text":"Done."}`)), nil
		}
	})}
	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcript, err := Transcribe(context.Background(), audioPath, TranscribeOptions{
		APIKey:         "test-key",
		Endpoint:       "https://api.example.test/v1/audio/transcriptions",
		HTTPClient:     client,
		PreparedAudio:  true,
		MaxRetries:     2,
		RetryBaseDelay: time.Millisecond,
		MaxRetryDelay:  5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if transcript.Text != "Done." {
		t.Fatalf("transcript = %#v", transcript)
	}
}

func TestTranscribeDoesNotRetryOtherClientErrors(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		_, _ = io.Copy(io.Discard, request.Body)
		return httpResponse(request, http.StatusBadRequest, "application/json", []byte(`{"error":"invalid request"}`)), nil
	})}
	audioPath := filepath.Join(t.TempDir(), "analysis.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Transcribe(context.Background(), audioPath, TranscribeOptions{
		APIKey:         "test-key",
		Endpoint:       "https://api.example.test/v1/audio/transcriptions",
		HTTPClient:     client,
		PreparedAudio:  true,
		MaxRetries:     5,
		RetryBaseDelay: time.Millisecond,
	})
	if err == nil {
		t.Fatal("Transcribe() error = nil")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestParseTranscriptionPayloadRejectsInvalidTimestamp(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(map[string]any{
		"duration": 1,
		"text":     "bad",
		"words": []map[string]any{
			{"word": "bad", "start": 0.9, "end": 0.2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseTranscriptionPayload(payload, "openai", "whisper-1", "en", "source", "audio.mp3")
	if err == nil || !strings.Contains(err.Error(), "ends before it starts") {
		t.Fatalf("error = %v, want invalid range", err)
	}
}
