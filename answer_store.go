package fastgocaptcha

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"strings"
	"sync"
	"time"
)

// AnswerStore is a bounded, concurrent store for text, arithmetic or application codes.
// Every verification attempt consumes the answer, including an incorrect attempt.
// It starts no goroutines; expired entries are reclaimed on Put.
type AnswerStore struct {
	mu       sync.Mutex
	ttl      time.Duration
	capacity int
	entries  map[string]answerEntry
}
type answerEntry struct {
	digest     [32]byte
	expires    time.Time
	ignoreCase bool
}

func NewAnswerStore(ttl time.Duration, capacity int) (*AnswerStore, error) {
	if ttl <= 0 || capacity <= 0 {
		return nil, errors.New("TTL and capacity must be positive")
	}
	return &AnswerStore{ttl: ttl, capacity: capacity, entries: make(map[string]answerEntry)}, nil
}

// Put stores only an answer hash and returns an unpredictable ID. Use ignoreCase for text CAPTCHAs.
func (s *AnswerStore) Put(answer string, ignoreCase bool) (string, error) {
	if answer == "" || len(answer) > 256 {
		return "", errors.New("answer length must be 1..256 bytes")
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	if ignoreCase {
		answer = strings.ToUpper(answer)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, value := range s.entries {
		if !now.Before(value.expires) {
			delete(s.entries, key)
		}
	}
	if len(s.entries) >= s.capacity {
		return "", errors.New("answer store is full")
	}
	s.entries[id] = answerEntry{sha256.Sum256([]byte(answer)), now.Add(s.ttl), ignoreCase}
	return id, nil
}
func (s *AnswerStore) Verify(id, answer string) bool {
	s.mu.Lock()
	entry, ok := s.entries[id]
	delete(s.entries, id)
	s.mu.Unlock()
	if !ok || !time.Now().Before(entry.expires) || len(answer) > 256 {
		return false
	}
	if entry.ignoreCase {
		answer = strings.ToUpper(answer)
	}
	digest := sha256.Sum256([]byte(answer))
	return subtle.ConstantTimeCompare(entry.digest[:], digest[:]) == 1
}
func (s *AnswerStore) Delete(id string) { s.mu.Lock(); delete(s.entries, id); s.mu.Unlock() }
