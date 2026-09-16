package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

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

type IndexEntry struct {
	UUID     string `json:"uuid"`
	Login    string `json:"login"`
	Username string `json:"username"`
	Platform string `json:"platform"`
	Path     string `json:"path"`
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// decodeUser expects the serialized payload to always be a JSON array,
// even for a single-author submission, and unwraps it to the one entry.
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

func writeAuthorFile(login, uuid string, u User) (string, error) {
	dir := filepath.Join("data", login)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, uuid+".json")
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func updateIndex(entry IndexEntry) error {
	const indexPath = "index.json"

	var index []IndexEntry
	if raw, err := os.ReadFile(indexPath); err == nil {
		if err := json.Unmarshal(raw, &index); err != nil {
			return fmt.Errorf("parsing existing index: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	replaced := false
	for i, e := range index {
		if e.UUID == entry.UUID {
			index[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		index = append(index, entry)
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0o644)
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

	path, err := writeAuthorFile(authorLogin, uuid, user)
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

	fmt.Println(path)
	os.Exit(0)
}
