package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// User is the struct each serialized author profile decodes into.
// Only used here for validation — the file on disk stores the raw blob, not this.
type User struct {
	Shortname string   `json:"shortname"`
	Longname  string   `json:"longname"`
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Ex        bool     `json:"ex"`
	Groups    []string `json:"groups"`
	FromGit   bool     `json:"from_git,omitempty"`
	Platform  string   `json:"platform,omitempty"`
}

// IndexEntry is one row in the top-level index.json — enough to look
// someone up without fetching every author file individually.
type IndexEntry struct {
	UUID     string `json:"uuid"`
	Login    string `json:"login"`    // GitHub login that owns this entry — source of truth for auth
	Username string `json:"username"` // platform-specific, NOT unique — display/lookup only
	Platform string `json:"platform"`
	Path     string `json:"path"`
}

const indexPath = "index.json"

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// decodeUser expects the serialized payload to always be a JSON array,
// even for a single-author submission, and unwraps it to the one entry.
// This is purely a validation step — we never write the decoded User
// back to disk, only the original blob.
func decodeUser(encoded string) (User, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return User{}, fmt.Errorf("invalid base64: %w", err)
	}

	var users []User
	if err := json.Unmarshal(raw, &users); err != nil {
		return User{}, fmt.Errorf("invalid user json: %w", err)
	}

	if len(users) != 1 {
		return User{}, fmt.Errorf("expected exactly 1 user in payload, got %d", len(users))
	}

	u := users[0]
	if u.Shortname == "" || u.Username == "" {
		return User{}, fmt.Errorf("missing required fields")
	}

	return u, nil
}

// writeAuthorFile stores the original base64 blob as-is, not the
// unmarshaled/pretty-printed JSON.
func writeAuthorFile(login, uuid, encoded string) (string, error) {
	dir := filepath.Join("data", login)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, uuid+".txt")
	if err := os.WriteFile(path, []byte(encoded), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func loadIndex() ([]IndexEntry, error) {
	var index []IndexEntry

	raw, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, fmt.Errorf("parsing existing index: %w", err)
	}
	return index, nil
}

func writeIndex(index []IndexEntry) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0o644)
}

// updateIndex inserts a new entry, or updates an existing one — but only
// if the submitting login already owns that uuid.
func updateIndex(entry IndexEntry) error {
	index, err := loadIndex()
	if err != nil {
		return err
	}

	for i, e := range index {
		if e.UUID == entry.UUID {
			if e.Login != entry.Login {
				return fmt.Errorf("uuid %s is owned by %s, not %s", entry.UUID, e.Login, entry.Login)
			}
			index[i] = entry
			return writeIndex(index)
		}
	}

	index = append(index, entry)
	return writeIndex(index)
}

func main() {
	args := os.Args[1:]
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: user_check <uuid> <encoded> <author_login>")
		os.Exit(1)
	}

	uuid, encoded, authorLogin := args[0], args[1], args[2]

	if !uuidRe.MatchString(uuid) {
		fmt.Fprintln(os.Stderr, "uuid is not well-formed")
		os.Exit(1)
	}
	if authorLogin == "" {
		fmt.Fprintln(os.Stderr, "missing author login")
		os.Exit(1)
	}

	user, err := decodeUser(encoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode failed:", err)
		os.Exit(1)
	}

	path, err := writeAuthorFile(authorLogin, uuid, encoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "write failed:", err)
		os.Exit(1)
	}

	if err := updateIndex(IndexEntry{
		UUID:     uuid,
		Login:    authorLogin,
		Username: user.Username,
		Platform: user.Platform,
		Path:     path,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "index update failed:", err)
		os.Exit(1)
	}

	fmt.Println(path) // bash reads this for the commit message / issue comment
	os.Exit(0)
}