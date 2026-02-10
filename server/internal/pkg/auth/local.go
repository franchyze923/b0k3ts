package auth

import (
	badgerKV "b0k3ts/internal/pkg/badger"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidUsernameOrPassword = errors.New("invalid username or password")
	ErrUserAlreadyExists         = errors.New("user already exists")
	ErrUserNotFound              = errors.New("user not found")
)

type UserRecord struct {
	Username      string    `json:"username"`
	PasswordHash  string    `json:"password_hash"` // bcrypt hash, NOT plaintext
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Disabled      bool      `json:"disabled"`
	Email         string    `json:"email"`
	Groups        []string  `json:"groups"`
	Administrator bool      `json:"administrator"`
}

type Store struct {
	DB *badger.DB

	// Optional tuning:
	// bcrypt.DefaultCost is usually fine; 12 is a common choice.
	BcryptCost int
}

func NewStore(db *badger.DB) *Store {
	return &Store{
		DB:         db,
		BcryptCost: bcrypt.DefaultCost,
	}
}

// UserExists checks if the user key exists in Badger.
// It does NOT validate password; it only checks presence.
func (s *Store) UserExists(username string) (bool, error) {
	username = normalizeUsername(username)
	if username == "" {
		return false, errors.New("username is required")
	}

	_, err := badgerKV.PullKV(s.DB, userKey(username))
	if err == nil {
		return true, nil
	}
	if isKeyNotFound(err) {
		return false, nil
	}
	return false, err
}

// EnsureUser creates the user only if it does not exist yet.
// Returns created=true if a new user was inserted, created=false if it already existed.
func (s *Store) EnsureUser(username, password string, administrator bool) (created bool, err error) {
	exists, err := s.UserExists(username)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := s.CreateUser(username, password, administrator); err != nil {
		if errors.Is(err, ErrUserAlreadyExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateUser creates (or refuses to overwrite) a local user.
// Password is hashed with bcrypt and only the hash is stored.
func (s *Store) CreateUser(username, password string, administrator bool) error {
	username = normalizeUsername(username)
	if username == "" {
		return errors.New("username is required")
	}
	if password == "" {
		return errors.New("password is required")
	}

	// Check if exists
	if _, err := s.GetUser(username); err == nil {
		return ErrUserAlreadyExists
	} else if !errors.Is(err, ErrUserNotFound) {
		return err
	}

	cost := s.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC()
	rec := UserRecord{
		Username:      username,
		PasswordHash:  string(hashBytes),
		CreatedAt:     now,
		UpdatedAt:     now,
		Disabled:      false,
		Email:         username + "@local",
		Administrator: administrator,
	}

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal user record: %w", err)
	}

	return badgerKV.PutKV(s.DB, userKey(username), b)
}

// GetUser fetches a user record (including hash). Do not return the hash to clients.
func (s *Store) GetUser(username string) (*UserRecord, error) {
	username = normalizeUsername(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	b, err := badgerKV.PullKV(s.DB, userKey(username))
	if err != nil {
		// Badger returns an error when key not found; treat that as user-not-found.
		if isKeyNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	var rec UserRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal user record: %w", err)
	}

	return &rec, nil
}

// ValidateUser verifies a username+password against the stored bcrypt hash.
// It returns ErrInvalidUsernameOrPassword for any auth failure (including user not found).
func (s *Store) ValidateUser(username, password string) (*UserRecord, error) {

	username = normalizeUsername(username)
	if username == "" || password == "" {
		return nil, ErrInvalidUsernameOrPassword
	}

	rec, err := s.GetUser(username)
	if err != nil {
		// Avoid user enumeration
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidUsernameOrPassword
		}
		return nil, err
	}

	if rec.Disabled {
		return nil, ErrInvalidUsernameOrPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidUsernameOrPassword
	}

	// Success
	return rec, nil
}

// ChangePassword replaces the stored bcrypt hash with a new one after verifying the old password.
func (s *Store) ChangePassword(username, oldPassword, newPassword string) error {
	rec, err := s.ValidateUser(username, oldPassword)
	if err != nil {
		return err
	}
	if newPassword == "" {
		return errors.New("new password is required")
	}

	cost := s.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), cost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	rec.PasswordHash = string(hashBytes)
	rec.UpdatedAt = time.Now().UTC()

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal updated user record: %w", err)
	}

	return badgerKV.PutKV(s.DB, userKey(rec.Username), b)
}

func (s *Store) DisableUser(username string, disabled bool) error {
	rec, err := s.GetUser(username)
	if err != nil {
		return err
	}
	rec.Disabled = disabled
	rec.UpdatedAt = time.Now().UTC()

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal updated user record: %w", err)
	}
	return badgerKV.PutKV(s.DB, userKey(username), b)
}

func (s *Store) DeleteUser(username string) error {
	username = normalizeUsername(username)
	if username == "" {
		return errors.New("username is required")
	}
	return badgerKV.DeleteKV(s.DB, userKey(username))
}

func userKey(username string) string {
	return "local_users/" + username
}

func normalizeUsername(u string) string {
	u = strings.TrimSpace(u)
	u = strings.ToLower(u)
	return u
}

// Badger's not-found error is badger.ErrKeyNotFound.
func isKeyNotFound(err error) bool {
	return errors.Is(err, badger.ErrKeyNotFound)
}

// UpdatePassword sets a new password for the given user (without requiring the old password).
// Useful for admin resets or "forgot password" flows.
func (s *Store) UpdatePassword(username, newPassword string) error {
	username = normalizeUsername(username)
	if username == "" {
		return errors.New("username is required")
	}
	if newPassword == "" {
		return errors.New("new password is required")
	}

	rec, err := s.GetUser(username)
	if err != nil {
		return err
	}
	if rec.Disabled {
		return ErrInvalidUsernameOrPassword
	}

	cost := s.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), cost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	rec.PasswordHash = string(hashBytes)
	rec.UpdatedAt = time.Now().UTC()

	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal updated user record: %w", err)
	}

	return badgerKV.PutKV(s.DB, userKey(rec.Username), b)
}
