package videoedit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTranscriptionEndpoint = "https://api.openai.com/v1/audio/transcriptions"
	defaultTranscriptionModel    = "whisper-1"
	defaultTranscriptionLanguage = "en"
	maxTranscriptionResponse     = int64(64 << 20)
)

// TranscribeOptions configures either the OpenAI transcription transport or a
// local external-command adapter. LocalCommand is an argv vector (never a
// shell expression) and may use {audio}, {output}, {language}, {model}, and
// {prompt} placeholders. The command must write either OpenAI verbose_json or
// a Transcript JSON document to stdout or to {output}.
type TranscribeOptions struct {
	SourceID string

	APIKey   string
	Endpoint string
	BaseURL  string
	Model    string
	Language string
	Prompt   string
	Glossary []string

	HTTPClient *http.Client
	FFmpegPath string

	// PreparedAudio skips FFmpeg extraction when inputAudio is already a
	// 16-kHz mono MP3 produced by ExtractAnalysisAudio.
	PreparedAudio bool

	// CachePath enables read-through/write-through JSON caching. RefreshCache
	// ignores an existing cache while still replacing it atomically on success.
	CachePath    string
	RefreshCache bool

	LocalCommand    []string
	LocalCommandEnv []string

	// MaxRetries defaults to two. Set it to a negative value to disable
	// retries. RetryBaseDelay defaults to one second and MaxRetryDelay to 30
	// seconds. Retries apply only to transport failures, HTTP 429, and 5xx.
	MaxRetries     int
	RetryBaseDelay time.Duration
	MaxRetryDelay  time.Duration
}

// ExtractAnalysisAudio creates a 16-kHz mono MP3 suitable for transcription.
// It writes through a temporary sibling and atomically renames on success, so
// interrupted FFmpeg processes never leave a valid-looking partial output.
func ExtractAnalysisAudio(ctx context.Context, source, out string) error {
	ffmpegPath := strings.TrimSpace(os.Getenv("FFMPEG_PATH"))
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return extractAnalysisAudio(ctx, ffmpegPath, source, out)
}

// Transcribe returns word- and segment-level timestamps in integer
// microseconds. Unless PreparedAudio is true, it first extracts a normalized
// analysis MP3 with FFmpeg. API keys and Authorization headers are never
// included in command arguments or error output.
func Transcribe(ctx context.Context, inputAudio string, opts TranscribeOptions) (Transcript, error) {
	if strings.TrimSpace(inputAudio) == "" {
		return Transcript{}, errors.New("input audio path is required")
	}
	if opts.CachePath != "" && !opts.RefreshCache {
		cached, found, err := readTranscriptCache(opts.CachePath)
		if err != nil {
			return Transcript{}, fmt.Errorf("read transcript cache: %w", err)
		}
		if found {
			return cached, nil
		}
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = defaultTranscriptionModel
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = defaultTranscriptionLanguage
	}
	prompt := transcriptionPrompt(opts.Prompt, opts.Glossary)

	audioPath := inputAudio
	if !opts.PreparedAudio {
		temporary, err := os.CreateTemp("", "video-edit-analysis-*.mp3")
		if err != nil {
			return Transcript{}, fmt.Errorf("create temporary analysis audio: %w", err)
		}
		audioPath = temporary.Name()
		if err := temporary.Close(); err != nil {
			_ = os.Remove(audioPath)
			return Transcript{}, fmt.Errorf("close temporary analysis audio: %w", err)
		}
		_ = os.Remove(audioPath)
		defer os.Remove(audioPath)
		ffmpegPath := strings.TrimSpace(opts.FFmpegPath)
		if ffmpegPath == "" {
			ffmpegPath = strings.TrimSpace(os.Getenv("FFMPEG_PATH"))
		}
		if ffmpegPath == "" {
			ffmpegPath = "ffmpeg"
		}
		if err := extractAnalysisAudio(ctx, ffmpegPath, inputAudio, audioPath); err != nil {
			return Transcript{}, err
		}
	}

	var (
		payload  []byte
		provider string
		err      error
	)
	if len(opts.LocalCommand) > 0 {
		provider = "local-command"
		payload, err = runLocalTranscriber(ctx, audioPath, language, model, prompt, opts)
	} else {
		provider = "openai"
		payload, err = requestOpenAITranscription(ctx, audioPath, language, model, prompt, opts)
	}
	if err != nil {
		return Transcript{}, err
	}

	transcript, err := parseTranscriptionPayload(payload, provider, model, language, opts.SourceID, inputAudio)
	if err != nil {
		return Transcript{}, fmt.Errorf("parse transcription response: %w", err)
	}
	if opts.CachePath != "" {
		if err := writeTranscriptCache(opts.CachePath, transcript); err != nil {
			return Transcript{}, fmt.Errorf("write transcript cache: %w", err)
		}
	}
	return transcript, nil
}

func extractAnalysisAudio(ctx context.Context, ffmpegPath, source, out string) error {
	if strings.TrimSpace(source) == "" {
		return errors.New("analysis audio source path is required")
	}
	if strings.TrimSpace(out) == "" {
		return errors.New("analysis audio output path is required")
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("stat analysis audio source: %w", err)
	}
	directory := filepath.Dir(out)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create analysis audio directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".analysis-audio-*.mp3")
	if err != nil {
		return fmt.Errorf("create temporary analysis audio: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close temporary analysis audio: %w", err)
	}
	_ = os.Remove(temporaryPath)
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	command := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-y",
		"-i", source,
		"-map", "0:a:0",
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "libmp3lame",
		"-b:a", "64k",
		"-map_metadata", "-1",
		temporaryPath,
	)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("extract 16-kHz mono analysis audio: %w: %s", err, truncateText(stderr.String(), 4096))
	}
	if !fileIsNonEmpty(temporaryPath) {
		return errors.New("FFmpeg produced empty analysis audio")
	}
	if err := os.Rename(temporaryPath, out); err != nil {
		return fmt.Errorf("publish analysis audio: %w", err)
	}
	keepTemporary = false
	return nil
}

func transcriptionPrompt(base string, glossary []string) string {
	var parts []string
	if base = strings.TrimSpace(base); base != "" {
		parts = append(parts, base)
	}
	var terms []string
	for _, term := range glossary {
		if term = strings.TrimSpace(term); term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) > 0 {
		parts = append(parts, "Terminology: "+strings.Join(terms, ", "))
	}
	return strings.Join(parts, "\n\n")
}

func requestOpenAITranscription(ctx context.Context, audioPath, language, model, prompt string, opts TranscribeOptions) ([]byte, error) {
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" && strings.TrimSpace(opts.BaseURL) != "" {
		endpoint = strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/") + "/v1/audio/transcriptions"
	}
	if endpoint == "" {
		endpoint = defaultTranscriptionEndpoint
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = 2
	} else if maxRetries < 0 {
		maxRetries = 0
	}
	baseDelay := opts.RetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = time.Second
	}
	maxDelay := opts.MaxRetryDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}

	for attempt := 0; ; attempt++ {
		payload, retryAfter, retryable, err := requestOpenAITranscriptionOnce(
			ctx, client, endpoint, apiKey, audioPath, language, model, prompt,
		)
		if err == nil {
			return payload, nil
		}
		if !retryable || attempt >= maxRetries {
			return nil, err
		}
		delay := exponentialRetryDelay(baseDelay, maxDelay, attempt)
		if retryAfter != nil {
			delay = *retryAfter
			if delay > maxDelay {
				delay = maxDelay
			}
		}
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func requestOpenAITranscriptionOnce(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	audioPath string,
	language string,
	model string,
	prompt string,
) ([]byte, *time.Duration, bool, error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeResult := make(chan error, 1)
	go func() {
		err := writeTranscriptionMultipart(multipartWriter, audioPath, language, model, prompt)
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
		writeResult <- err
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		_ = reader.CloseWithError(err)
		return nil, nil, false, fmt.Errorf("create transcription request: %w", err)
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := client.Do(request)
	if err != nil {
		_ = reader.CloseWithError(err)
		<-writeResult
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		return nil, nil, true, fmt.Errorf("send transcription request: %s", redactSecret(err.Error(), apiKey))
	}
	defer response.Body.Close()
	responseBytes, readErr := readLimited(response.Body, maxTranscriptionResponse)
	writerErr := <-writeResult
	if readErr != nil {
		if ctx.Err() != nil {
			return nil, nil, false, ctx.Err()
		}
		return nil, nil, true, fmt.Errorf("read transcription response: %w", readErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := truncateText(strings.TrimSpace(string(responseBytes)), 4096)
		retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, parseRetryAfter(response.Header.Get("Retry-After"), time.Now()), retryable,
			fmt.Errorf("transcription request returned %s: %s", response.Status, redactSecret(message, apiKey))
	}
	if writerErr != nil {
		return nil, nil, false, fmt.Errorf("stream transcription request: %w", writerErr)
	}
	return responseBytes, nil, false, nil
}

func parseRetryAfter(value string, now time.Time) *time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		return &delay
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		delay := retryAt.Sub(now)
		if delay < 0 {
			delay = 0
		}
		return &delay
	}
	return nil
}

func exponentialRetryDelay(base, maximum time.Duration, attempt int) time.Duration {
	delay := base
	for step := 0; step < attempt; step++ {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func writeTranscriptionMultipart(writer *multipart.Writer, audioPath, language, model, prompt string) error {
	fields := [][2]string{
		{"model", model},
		{"response_format", "verbose_json"},
		{"timestamp_granularities[]", "word"},
		{"timestamp_granularities[]", "segment"},
		{"language", language},
	}
	if prompt != "" {
		fields = append(fields, [2]string{"prompt", prompt})
	}
	for _, field := range fields {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return err
		}
	}
	file, err := os.Open(audioPath)
	if err != nil {
		return err
	}
	defer file.Close()
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func runLocalTranscriber(ctx context.Context, audioPath, language, model, prompt string, opts TranscribeOptions) ([]byte, error) {
	if len(opts.LocalCommand) == 0 || strings.TrimSpace(opts.LocalCommand[0]) == "" {
		return nil, errors.New("local transcription command is empty")
	}
	outputFile, err := os.CreateTemp("", "video-edit-local-transcript-*.json")
	if err != nil {
		return nil, fmt.Errorf("create local transcription output: %w", err)
	}
	outputPath := outputFile.Name()
	if err := outputFile.Close(); err != nil {
		_ = os.Remove(outputPath)
		return nil, fmt.Errorf("close local transcription output: %w", err)
	}
	defer os.Remove(outputPath)

	replacements := map[string]string{
		"{audio}":    audioPath,
		"{output}":   outputPath,
		"{language}": language,
		"{model}":    model,
		"{prompt}":   prompt,
	}
	hasAudioPlaceholder := false
	hasOutputPlaceholder := false
	commandVector := make([]string, len(opts.LocalCommand))
	for index, argument := range opts.LocalCommand {
		hasAudioPlaceholder = hasAudioPlaceholder || strings.Contains(argument, "{audio}")
		hasOutputPlaceholder = hasOutputPlaceholder || strings.Contains(argument, "{output}")
		for placeholder, value := range replacements {
			argument = strings.ReplaceAll(argument, placeholder, value)
		}
		commandVector[index] = argument
	}
	if !hasAudioPlaceholder {
		commandVector = append(commandVector, audioPath)
	}

	command := exec.CommandContext(ctx, commandVector[0], commandVector[1:]...)
	command.Env = sanitizedEnvironment(append(os.Environ(), opts.LocalCommandEnv...))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("local transcription command failed: %w: %s", err, truncateText(stderr.String(), 4096))
	}
	if hasOutputPlaceholder {
		filePayload, err := os.ReadFile(outputPath)
		if err != nil {
			return nil, fmt.Errorf("read local transcription output: %w", err)
		}
		if len(bytes.TrimSpace(filePayload)) > 0 {
			return filePayload, nil
		}
	}
	if stdout.Len() == 0 {
		return nil, errors.New("local transcription command produced no JSON output")
	}
	return stdout.Bytes(), nil
}

func sanitizedEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		switch strings.ToUpper(name) {
		case "OPENAI_API_KEY", "OPENAI_ORG_ID", "OPENAI_ORGANIZATION":
			continue
		default:
			result = append(result, variable)
		}
	}
	return result
}

type openAITranscriptionResponse struct {
	Language string                       `json:"language"`
	Duration float64                      `json:"duration"`
	Text     string                       `json:"text"`
	Words    []openAITranscriptionWord    `json:"words"`
	Segments []openAITranscriptionSegment `json:"segments"`
}

type openAITranscriptionWord struct {
	Word  string  `json:"word"`
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type openAITranscriptionSegment struct {
	Text       string   `json:"text"`
	Start      float64  `json:"start"`
	End        float64  `json:"end"`
	AvgLogProb *float64 `json:"avg_logprob"`
}

func parseTranscriptionPayload(payload []byte, provider, model, language, sourceID, sourceAudio string) (Transcript, error) {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(payload, &shape); err != nil {
		return Transcript{}, err
	}
	if _, ok := shape["duration_us"]; ok {
		var transcript Transcript
		if err := json.Unmarshal(payload, &transcript); err != nil {
			return Transcript{}, err
		}
		fillTranscriptMetadata(&transcript, provider, model, language, sourceID, sourceAudio)
		if err := validateTranscriptTimestamps(transcript); err != nil {
			return Transcript{}, err
		}
		if transcript.ContentHash == "" {
			transcript.ContentHash = transcriptContentHash(transcript)
		}
		return transcript, nil
	}

	var response openAITranscriptionResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return Transcript{}, err
	}
	transcript := Transcript{
		SchemaVersion: CurrentSchemaVersion,
		SourceID:      sourceID,
		Provider:      provider,
		Model:         model,
		Language:      response.Language,
		Text:          strings.TrimSpace(response.Text),
		SourceAudio:   sourceAudio,
	}
	if transcript.SourceID == "" {
		transcript.SourceID = filepath.Base(sourceAudio)
	}
	if transcript.Language == "" {
		transcript.Language = language
	}

	for index, sourceSegment := range response.Segments {
		startUS, err := secondsToMicroseconds(sourceSegment.Start)
		if err != nil {
			return Transcript{}, fmt.Errorf("segment %d start: %w", index, err)
		}
		endUS, err := secondsToMicroseconds(sourceSegment.End)
		if err != nil {
			return Transcript{}, fmt.Errorf("segment %d end: %w", index, err)
		}
		if endUS < startUS {
			return Transcript{}, fmt.Errorf("segment %d ends before it starts", index)
		}
		segmentText := strings.TrimSpace(sourceSegment.Text)
		segmentID := stableID("segment", fmt.Sprintf("%s\x00%d\x00%d\x00%s", transcript.SourceID, startUS, endUS, segmentText), 0)
		segment := TranscriptSegment{
			ID:       segmentID,
			Index:    index,
			SourceID: transcript.SourceID,
			Text:     segmentText,
			StartUS:  startUS,
			EndUS:    endUS,
		}
		if sourceSegment.AvgLogProb != nil && !math.IsNaN(*sourceSegment.AvgLogProb) && !math.IsInf(*sourceSegment.AvgLogProb, 0) {
			confidence := math.Exp(*sourceSegment.AvgLogProb)
			segment.Confidence = &confidence
		}
		transcript.Segments = append(transcript.Segments, segment)
	}

	for index, sourceWord := range response.Words {
		startUS, err := secondsToMicroseconds(sourceWord.Start)
		if err != nil {
			return Transcript{}, fmt.Errorf("word %d start: %w", index, err)
		}
		endUS, err := secondsToMicroseconds(sourceWord.End)
		if err != nil {
			return Transcript{}, fmt.Errorf("word %d end: %w", index, err)
		}
		if endUS < startUS {
			return Transcript{}, fmt.Errorf("word %d ends before it starts", index)
		}
		wordText := sourceWord.Word
		if wordText == "" {
			wordText = sourceWord.Text
		}
		wordText = strings.TrimSpace(wordText)
		wordID := stableID("word", fmt.Sprintf("%s\x00%d\x00%d\x00%s", transcript.SourceID, startUS, endUS, wordText), 0)
		transcript.Words = append(transcript.Words, TranscriptWord{
			ID:        wordID,
			Index:     index,
			SegmentID: containingSegmentID(transcript.Segments, startUS, endUS),
			SourceID:  transcript.SourceID,
			Text:      wordText,
			StartUS:   startUS,
			EndUS:     endUS,
		})
	}

	if response.Duration > 0 {
		durationUS, err := secondsToMicroseconds(response.Duration)
		if err != nil {
			return Transcript{}, fmt.Errorf("duration: %w", err)
		}
		transcript.DurationUS = durationUS
	}
	if transcript.DurationUS == 0 {
		for _, segment := range transcript.Segments {
			transcript.DurationUS = maxInt64(transcript.DurationUS, segment.EndUS)
		}
		for _, word := range transcript.Words {
			transcript.DurationUS = maxInt64(transcript.DurationUS, word.EndUS)
		}
	}
	if transcript.Text == "" && len(transcript.Words) > 0 {
		var words []string
		for _, word := range transcript.Words {
			words = append(words, word.Text)
		}
		transcript.Text = strings.Join(words, " ")
	}
	transcript.ContentHash = transcriptContentHash(transcript)
	return transcript, nil
}

func fillTranscriptMetadata(transcript *Transcript, provider, model, language, sourceID, sourceAudio string) {
	if transcript.SchemaVersion == "" {
		transcript.SchemaVersion = CurrentSchemaVersion
	}
	if transcript.Provider == "" {
		transcript.Provider = provider
	}
	if transcript.Model == "" {
		transcript.Model = model
	}
	if transcript.Language == "" {
		transcript.Language = language
	}
	if transcript.SourceID == "" {
		transcript.SourceID = sourceID
	}
	if transcript.SourceID == "" {
		transcript.SourceID = filepath.Base(sourceAudio)
	}
	if transcript.SourceAudio == "" {
		transcript.SourceAudio = sourceAudio
	}
	for index := range transcript.Segments {
		transcript.Segments[index].Index = index
		if transcript.Segments[index].SourceID == "" {
			transcript.Segments[index].SourceID = transcript.SourceID
		}
	}
	for index := range transcript.Words {
		transcript.Words[index].Index = index
		if transcript.Words[index].SourceID == "" {
			transcript.Words[index].SourceID = transcript.SourceID
		}
	}
}

func validateTranscriptTimestamps(transcript Transcript) error {
	for index, segment := range transcript.Segments {
		if segment.StartUS < 0 || segment.EndUS < segment.StartUS {
			return fmt.Errorf("segment %d has invalid timestamp range", index)
		}
	}
	for index, word := range transcript.Words {
		if word.StartUS < 0 || word.EndUS < word.StartUS {
			return fmt.Errorf("word %d has invalid timestamp range", index)
		}
	}
	return nil
}

func secondsToMicroseconds(seconds float64) (int64, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return 0, fmt.Errorf("invalid seconds value %v", seconds)
	}
	if seconds > float64(math.MaxInt64)/1_000_000 {
		return 0, fmt.Errorf("seconds value %v overflows microseconds", seconds)
	}
	return int64(math.Round(seconds * 1_000_000)), nil
}

func containingSegmentID(segments []TranscriptSegment, startUS, endUS int64) string {
	midpoint := startUS + (endUS-startUS)/2
	for _, segment := range segments {
		if midpoint >= segment.StartUS && midpoint <= segment.EndUS {
			return segment.ID
		}
	}
	return ""
}

func transcriptContentHash(transcript Transcript) string {
	canonical := struct {
		SourceID   string              `json:"source_id"`
		Model      string              `json:"model"`
		Language   string              `json:"language"`
		DurationUS int64               `json:"duration_us"`
		Text       string              `json:"text"`
		Words      []TranscriptWord    `json:"words"`
		Segments   []TranscriptSegment `json:"segments"`
	}{
		SourceID:   transcript.SourceID,
		Model:      transcript.Model,
		Language:   transcript.Language,
		DurationUS: transcript.DurationUS,
		Text:       transcript.Text,
		Words:      transcript.Words,
		Segments:   transcript.Segments,
	}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func readTranscriptCache(path string) (Transcript, bool, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Transcript{}, false, nil
	}
	if err != nil {
		return Transcript{}, false, err
	}
	var transcript Transcript
	if err := json.Unmarshal(contents, &transcript); err != nil {
		return Transcript{}, false, err
	}
	if err := validateTranscriptTimestamps(transcript); err != nil {
		return Transcript{}, false, err
	}
	return transcript, true, nil
}

func writeTranscriptCache(path string, transcript Transcript) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".transcript-cache-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(transcript); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keepTemporary = false
	return nil
}

func redactSecret(message, secret string) string {
	if secret == "" {
		return message
	}
	return strings.ReplaceAll(message, secret, "[REDACTED]")
}

func truncateText(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "..."
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
