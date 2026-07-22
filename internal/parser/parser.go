package parser

import (
	"strings"
	"time"
	"unicode"
)

// LogEntry represents a single parsed log line.
type LogEntry struct {
	Time    time.Time
	Level   string
	Message string
	Raw     string
	Fields  map[string]string
}

// MessageType categorises the log message.
type MessageType int

const (
	MsgReceivedRequest             MessageType = iota // "received request"
	MsgRoutingRequest                                 // "routing request"
	MsgAttemptingStreaming                            // "attempting streaming model"
	MsgStreamingCompleted                             // "streaming completed"
	MsgStreamingFailedProvider                        // "streaming request failed via provider"
	MsgAttemptingModel                                // "attempting model" (non-streaming fallback)
	MsgNonRetryableError                              // "non-retryable error..."
	MsgRequestError                                   // "request error"
	MsgSendingStreamError                             // "sending stream error"
	MsgContextCanceledStreaming                        // "request context canceled during non-streaming fallback"
	MsgContextCanceledFallback                         // "request context canceled after model attempt, stopping fallback"
	MsgOpenAIStreamingFailed                           // "openai streaming failed"
	MsgModelSucceeded                                  // "model succeeded"
	MsgRequestCompleted                                // "request completed"
	MsgConfigChanged                                   // "config file changed, reloading"
	MsgConfigReloaded                                  // "config reloaded successfully"
	MsgConfigReloadFailed                              // "config reload failed"
	MsgConfigWatcherStarted                            // "config watcher started"
	MsgLogLevelUpdated                                 // "log level updated"
	MsgStorageDBOpened                                 // "storage database opened"
	MsgStarting                                        // "starting routatic-proxy"
	MsgShuttingDown                                    // "shutting down server..."
	MsgServerStopped                                   // "server stopped"
	MsgOther                                           // unrecognised
)

var messageTypes = map[string]MessageType{
	"received request":                                              MsgReceivedRequest,
	"routing request":                                               MsgRoutingRequest,
	"attempting streaming model":                                    MsgAttemptingStreaming,
	"streaming completed":                                           MsgStreamingCompleted,
	"streaming request failed via provider":                         MsgStreamingFailedProvider,
	"attempting model":                                              MsgAttemptingModel,
	"non-retryable error (skipping circuit breaker), trying fallback": MsgNonRetryableError,
	"request error":                                                 MsgRequestError,
	"sending stream error":                                          MsgSendingStreamError,
	"request context canceled during non-streaming fallback":        MsgContextCanceledStreaming,
	"request context canceled after model attempt, stopping fallback": MsgContextCanceledFallback,
	"openai streaming failed":                                       MsgOpenAIStreamingFailed,
	"model succeeded":                                               MsgModelSucceeded,
	"request completed":                                             MsgRequestCompleted,
	"config file changed, reloading":                                MsgConfigChanged,
	"config reloaded successfully":                                  MsgConfigReloaded,
	"config reload failed":                                          MsgConfigReloadFailed,
	"config watcher started":                                        MsgConfigWatcherStarted,
	"log level updated":                                             MsgLogLevelUpdated,
	"storage database opened":                                       MsgStorageDBOpened,
	"starting routatic-proxy":                                       MsgStarting,
	"shutting down server...":                                       MsgShuttingDown,
	"server stopped":                                                MsgServerStopped,
}

func (mt MessageType) String() string {
	for s, m := range messageTypes {
		if m == mt {
			return s
		}
	}
	return "unknown"
}

// Mnemonic returns a short human-readable label.
func (mt MessageType) Mnemonic() string {
	switch mt {
	case MsgReceivedRequest:
		return "RECEIVED"
	case MsgRoutingRequest:
		return "ROUTING"
	case MsgAttemptingStreaming:
		return "STREAMING"
	case MsgStreamingCompleted:
		return "COMPLETED"
	case MsgStreamingFailedProvider:
		return "STREAM FAIL"
	case MsgAttemptingModel:
		return "FALLBACK"
	case MsgNonRetryableError:
		return "FATAL"
	case MsgRequestError:
		return "ERROR"
	case MsgSendingStreamError:
		return "SEND ERR"
	case MsgContextCanceledStreaming, MsgContextCanceledFallback:
		return "CANCELED"
	case MsgOpenAIStreamingFailed:
		return "OPENAI ERR"
	case MsgModelSucceeded:
		return "OK"
	case MsgRequestCompleted:
		return "DONE"
	case MsgConfigChanged, MsgConfigReloaded, MsgConfigWatcherStarted,
		MsgLogLevelUpdated, MsgStorageDBOpened, MsgStarting,
		MsgShuttingDown, MsgServerStopped, MsgConfigReloadFailed:
		return "SYSTEM"
	default:
		return "?"
	}
}

// ClassifyMessage maps a raw msg string to a MessageType.
func ClassifyMessage(msg string) MessageType {
	if t, ok := messageTypes[msg]; ok {
		return t
	}
	return MsgOther
}

func isWordChar(b byte) bool {
	return unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b)) || b == '_' || b == '-'
}

// boundary marks where a key=value pair starts in the raw line.
type boundary struct {
	key      string
	keyStart int // byte offset of the first char of the key
	eqPos    int // byte offset of the =
}

// knownKeys is the set of valid log field names.  Used to disambiguate
// concatenated key=value pairs such as latency=3.1sinput_tokens=0.
var knownKeys = map[string]bool{
	"time":                        true,
	"level":                       true,
	"msg":                         true,
	"model":                       true,
	"streaming":                   true,
	"messages":                    true,
	"tools":                       true,
	"max_tokens":                  true,
	"scenario":                    true,
	"provider":                    true,
	"tokens":                      true,
	"latency":                     true,
	"input_tokens":                true,
	"output_tokens":               true,
	"cache_read_input_tokens":     true,
	"cache_creation_input_tokens": true,
	"error":                       true,
	"attempt":                     true,
	"total":                       true,
	"remaining":                   true,
	"status":                      true,
	"message":                     true,
	"path":                        true,
	"host":                        true,
	"port":                        true,
	"base_url":                    true,
	"config_file":                 true,
}

// resolveKey checks whether candidate (a maximal word-token preceding a =)
// is or ends with a known key.  Returns the key and its byte offset, or
// ("", 0) when no known key is found.
func resolveKey(candidate string, startPos int) (key string, keyStart int) {
	if candidate == "" {
		return "", 0
	}
	if knownKeys[candidate] {
		return candidate, startPos
	}
	// The value runs directly into the key without a space; check whether
	// candidate ends with a known key.  Pick the longest match so that
	// e.g. cache_creation_input_tokens beats input_tokens.
	for k := range knownKeys {
		if len(k) < len(candidate) && strings.HasSuffix(candidate, k) {
			if key == "" || len(k) > len(key) {
				key = k
				keyStart = startPos + len(candidate) - len(k)
			}
		}
	}
	if key != "" {
		return key, keyStart
	}
	return "", 0
}

// Span describes one key=value span in the raw log line.
type Span struct {
	Key    string
	Value  string
	Start  int // byte offset of the first char of the key
	EqPos  int // byte offset of the '='
	End    int // byte offset of the end of value (exclusive, past closing " if quoted)
}

// Spans returns all key=value spans in the raw line.
func Spans(line string) []Span {
	var bs []boundary
	inQuote := false
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote || line[i] != '=' {
			continue
		}
		j := i - 1
		for j >= 0 && isWordChar(line[j]) {
			j--
		}
		candidate := line[j+1 : i]
		if candidate == "" {
			continue
		}
		key, ks := resolveKey(candidate, j+1)
		if key == "" {
			continue
		}
		bs = append(bs, boundary{key: key, keyStart: ks, eqPos: i})
	}

	spans := make([]Span, 0, len(bs))
	for idx, b := range bs {
		valStart := b.eqPos + 1
		var value string
		var valEnd int

		if valStart >= len(line) {
			value = ""
			valEnd = len(line)
		} else if line[valStart] == '"' {
			valStart++ // skip opening "
			end := strings.IndexByte(line[valStart:], '"')
			if end < 0 {
				value = line[valStart:]
				valEnd = len(line)
			} else {
				value = line[valStart : valStart+end]
				valEnd = valStart + end + 1 // include closing "
			}
		} else if idx+1 < len(bs) {
			raw := line[valStart:bs[idx+1].keyStart]
			value = strings.TrimRight(raw, " ")
			valEnd = bs[idx+1].keyStart
		} else {
			value = strings.TrimSpace(line[valStart:])
			valEnd = len(line)
		}

		spans = append(spans, Span{
			Key:   b.key,
			Value: value,
			Start: b.keyStart,
			EqPos: b.eqPos,
			End:   valEnd,
		})
	}
	return spans
}

// extractKeyValues parses all key=value pairs from a log line using a
// two-pass approach:
//
//  1. Scan for every occurrence of [word chars]= that sits outside quoted
//     strings.  Each candidate is resolved against known keys, which handles
//     concatenated pairs such as latency=3.1sinput_tokens=0.
//  2. For each boundary, the value starts after = and runs up to (but not
//     including) the next boundary's key, or end-of-line.
func extractKeyValues(line string) map[string]string {
	var bs []boundary
	inQuote := false
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote || line[i] != '=' {
			continue
		}
		// Walk back from = to find the maximal word token.
		j := i - 1
		for j >= 0 && isWordChar(line[j]) {
			j--
		}
		candidate := line[j+1 : i]
		if candidate == "" {
			continue
		}
		key, ks := resolveKey(candidate, j+1)
		if key == "" {
			continue
		}
		bs = append(bs, boundary{key: key, keyStart: ks, eqPos: i})
	}

	fields := make(map[string]string, len(bs))
	for idx, b := range bs {
		valStart := b.eqPos + 1
		if valStart >= len(line) {
			fields[b.key] = ""
			continue
		}

		var value string
		if line[valStart] == '"' {
			valStart++ // skip opening "
			end := strings.IndexByte(line[valStart:], '"')
			if end < 0 {
				value = line[valStart:]
			} else {
				value = line[valStart : valStart+end]
			}
		} else if idx+1 < len(bs) {
			raw := line[valStart:bs[idx+1].keyStart]
			value = strings.TrimRight(raw, " ")
		} else {
			value = strings.TrimSpace(line[valStart:])
		}

		fields[b.key] = value
	}
	return fields
}

// ParseLine parses a single key=value log line.
func ParseLine(line string) (*LogEntry, error) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, nil
	}

	fields := extractKeyValues(line)

	entry := &LogEntry{
		Raw:    line,
		Fields: fields,
	}

	if t, ok := fields["time"]; ok {
		if parsed, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", t); err == nil {
			entry.Time = parsed
		}
	}
	entry.Level = fields["level"]
	entry.Message = fields["msg"]

	return entry, nil
}
