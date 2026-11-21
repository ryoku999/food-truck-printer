package worker

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
	"unsafe"
)

var (
	moveFile     = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
	jobIDPattern = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
)

type Result struct {
	Attempt     int    `json:"attempt"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	SafeToRetry bool   `json:"safeToRetry"`
}

type Pending struct {
	JobID   string  `json:"jobId"`
	Attempt int     `json:"attempt"`
	Result  *Result `json:"result,omitempty"`
}

type Journal struct {
	dir  string
	lock syscall.Handle
}

func OpenJournal(root, baseURL string) (*Journal, error) {
	if root == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cache, "food-truck-printer", "state")
	}
	key := sha256.Sum256([]byte(baseURL))
	dir := filepath.Join(root, fmt.Sprintf("%x", key[:]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	filename, err := syscall.UTF16PtrFromString(filepath.Join(dir, "agent.lock"))
	if err != nil {
		return nil, err
	}
	// An exclusive handle is released by Windows even after a process crash.
	handle, err := syscall.CreateFile(filename, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("abrir bloqueo del agente (¿ya hay una instancia?): %w", err)
	}
	return &Journal{dir: dir, lock: handle}, nil
}

func (j *Journal) Close() error { return syscall.CloseHandle(j.lock) }
func (j *Journal) path() string { return filepath.Join(j.dir, "pending.json") }

func (j *Journal) Load() (*Pending, error) {
	data, err := os.ReadFile(j.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pending Pending
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, fmt.Errorf("diario ilegible: %w", err)
	}
	if err := pending.validate(); err != nil {
		return nil, err
	}
	return &pending, nil
}

func (p Pending) validate() error {
	if !jobIDPattern.MatchString(p.JobID) || p.Attempt < 1 {
		return fmt.Errorf("trabajo o intento inválido en el diario")
	}
	if p.Result != nil && (p.Result.Attempt != p.Attempt || (p.Result.Status != "PRINTED" && p.Result.Status != "FAILED")) {
		return fmt.Errorf("resultado inválido en el diario")
	}
	return nil
}

func (j *Journal) Save(pending Pending) error {
	if err := pending.validate(); err != nil {
		return err
	}
	file, err := os.CreateTemp(j.dir, "pending-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	writeErr := json.NewEncoder(file).Encode(pending)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return durableMove(file.Name(), j.path())
}

func (j *Journal) Clear() error { return os.Remove(j.path()) }

// Archive retains a copy for review when the API rejects an obsolete attempt.
func (j *Journal) Archive(pending Pending) error {
	return durableMove(j.path(), filepath.Join(j.dir, fmt.Sprintf("review-%s-%d-%d.json", pending.JobID, pending.Attempt, time.Now().UnixNano())))
}

func durableMove(from, to string) error {
	source, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	const replaceExistingAndWriteThrough = 0x1 | 0x8
	ok, _, callErr := moveFile.Call(uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(target)), replaceExistingAndWriteThrough)
	if ok == 0 {
		return fmt.Errorf("guardar diario: %w", callErr)
	}
	return nil
}
