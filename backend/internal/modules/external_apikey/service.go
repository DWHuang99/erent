package externalapikey

import (
	"context"
	"erent/internal/dto/request"
	"erent/internal/security"
	"errors"
	"net/url"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid external api key request")

type Service struct {
	repository    *Repository
	encryptionKey []byte
}

func NewService(repository *Repository, encryptionKey []byte) *Service {
	return &Service{repository: repository, encryptionKey: append([]byte(nil), encryptionKey...)}
}

func validPath(path string) bool {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(path, "\\\r\n?#") {
		return false
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == ".." || segment == "." {
			return false
		}
	}
	return true
}

func (s *Service) prepare(req request.ExternalApikeyRequest) (*ExternalApikey, error) {
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	u, err := url.Parse(req.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(req.Endpoint, "\\?#") || !validPath(u.EscapedPath()) {
		return nil, ErrInvalidRequest
	}
	if strings.TrimSpace(req.ExternalApikey) == "" || strings.ContainsAny(req.ExternalApikey, "\r\n") {
		return nil, ErrInvalidRequest
	}
	for _, suffix := range []string{req.Suffix.ChatCompletions, req.Suffix.Responses, req.Suffix.Messages} {
		if !validPath(suffix) || (suffix != "" && strings.Trim(suffix, "/ ") == "") || strings.ContainsAny(suffix, " \t") {
			return nil, ErrInvalidRequest
		}
	}
	encrypted, err := security.Encrypt(s.encryptionKey, []byte(req.ExternalApikey))
	if err != nil {
		return nil, err
	}
	return &ExternalApikey{KeyHash: keyHash(req.ExternalApikey), Ciphertext: string(encrypted), Endpoint: strings.TrimRight(req.Endpoint, "/"), Suffix: req.Suffix}, nil
}

func listItem(key ExternalApikey) ListItem {
	return ListItem{ID: key.ID, Endpoint: key.Endpoint, Suffix: key.Suffix, CreatedAt: key.CreatedAt, UpdatedAt: key.UpdatedAt}
}

func (s *Service) Create(ctx context.Context, userID uint64, req request.ExternalApikeyRequest) (*ListItem, error) {
	if userID == 0 {
		return nil, ErrInvalidRequest
	}
	key, err := s.prepare(req)
	if err != nil {
		return nil, err
	}
	key.UserID = userID
	if err := s.repository.Add(ctx, key); err != nil {
		return nil, err
	}
	item := listItem(*key)
	return &item, nil
}

func (s *Service) Update(ctx context.Context, userID, id uint64, req request.ExternalApikeyRequest) error {
	if userID == 0 || id == 0 {
		return ErrInvalidRequest
	}
	key, err := s.prepare(req)
	if err != nil {
		return err
	}
	return s.repository.Update(ctx, userID, id, key)
}

func (s *Service) Delete(ctx context.Context, userID, id uint64) error {
	if userID == 0 || id == 0 {
		return ErrInvalidRequest
	}
	return s.repository.Delete(ctx, userID, id)
}

func (s *Service) List(ctx context.Context, userID uint64) ([]ListItem, error) {
	if userID == 0 {
		return nil, ErrInvalidRequest
	}
	keys, err := s.repository.GetUsersApiKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]ListItem, 0, len(keys))
	for _, key := range keys {
		items = append(items, listItem(key))
	}
	return items, nil
}

// ResolveURL selects the configured suffix for the incoming protocol path.
// Empty suffixes preserve that path. Endpoint paths are retained when joining.
func (s *Service) ResolveURL(key ExternalApikey, currentPath string) (string, error) {
	var suffix string
	switch currentPath {
	case "/v1/chat/completions":
		suffix = key.Suffix.ChatCompletions
	case "/v1/responses":
		suffix = key.Suffix.Responses
	case "/v1/messages":
		suffix = key.Suffix.Messages
	default:
		return "", ErrInvalidRequest
	}
	if suffix == "" {
		suffix = currentPath
	}
	if !validPath(suffix) {
		return "", ErrInvalidRequest
	}
	return strings.TrimRight(key.Endpoint, "/") + "/" + strings.TrimLeft(suffix, "/"), nil
}
