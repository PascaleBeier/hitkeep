package assetstore

import (
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
)

const qrCodeAssetPrefix = "qr-codes"

type Store struct {
	root string
}

func New(dataPath string) *Store {
	root := strings.TrimSpace(dataPath)
	if root == "" {
		root = "data"
	}
	return &Store{root: filepath.Join(root, "assets")}
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) PutQRCodeAsset(siteID, qrID uuid.UUID, checksum, filename, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("asset data is empty")
	}
	ext := assetExtension(filename, contentType)
	if ext == "" {
		return "", fmt.Errorf("unsupported asset content type %q", contentType)
	}
	key := filepath.ToSlash(filepath.Join(
		qrCodeAssetPrefix,
		siteID.String(),
		qrID.String(),
		safeChecksum(checksum)+ext,
	))
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return "", fmt.Errorf("create asset root: %w", err)
	}
	root, err := s.openRoot()
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(key), 0755); err != nil {
		return "", fmt.Errorf("create asset directory: %w", err)
	}
	tmpKey := filepath.ToSlash(filepath.Join(filepath.Dir(key), ".qr-asset-"+uuid.NewString()))
	tmp, err := root.OpenFile(tmpKey, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", fmt.Errorf("create temporary asset: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(tmpKey)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temporary asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temporary asset: %w", err)
	}
	if err := root.Rename(tmpKey, key); err != nil {
		return "", fmt.Errorf("store asset: %w", err)
	}
	cleanup = false
	return key, nil
}

func (s *Store) Open(key string) (*os.File, error) {
	key, err := assetKey(key)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenInRoot(s.root, key)
	if err != nil {
		return nil, fmt.Errorf("open asset: %w", err)
	}
	return file, nil
}

func (s *Store) Delete(key string) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	key, err := assetKey(key)
	if err != nil {
		return err
	}
	root, err := s.openRoot()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(key); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete asset: %w", err)
	}
	_ = pruneEmptyParents(root, filepath.Dir(key))
	return nil
}

func (s *Store) DeleteQRCodeAssetDir(siteID, qrID uuid.UUID) error {
	return s.removeAll(filepath.Join(qrCodeAssetPrefix, siteID.String(), qrID.String()))
}

func (s *Store) DeleteQRCodeAssetsForSite(siteID uuid.UUID) error {
	return s.removeAll(filepath.Join(qrCodeAssetPrefix, siteID.String()))
}

func (s *Store) removeAll(relative string) error {
	key, err := assetKey(relative)
	if err != nil {
		return err
	}
	root, err := s.openRoot()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.RemoveAll(key); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete asset directory: %w", err)
	}
	_ = pruneEmptyParents(root, filepath.Dir(key))
	return nil
}

func (s *Store) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("open asset root: %w", err)
	}
	return root, nil
}

func assetKey(key string) (string, error) {
	key = filepath.Clean(filepath.FromSlash(strings.TrimSpace(key)))
	if key == "." || key == "" || filepath.IsAbs(key) || strings.HasPrefix(key, ".."+string(filepath.Separator)) || key == ".." {
		return "", fmt.Errorf("invalid asset key")
	}
	return key, nil
}

func assetExtension(filename, contentType string) string {
	contentType, _, _ = mime.ParseMediaType(strings.TrimSpace(contentType))
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return ext
	default:
		return ""
	}
}

func safeChecksum(checksum string) string {
	checksum = strings.TrimSpace(strings.TrimPrefix(checksum, "sha256:"))
	var b strings.Builder
	for _, r := range checksum {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "asset"
	}
	return b.String()
}

func pruneEmptyParents(root *os.Root, dir string) error {
	for dir != "." {
		if err := root.Remove(dir); err != nil {
			if os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) {
				return nil
			}
			return err
		}
		dir = filepath.Dir(dir)
	}
	return nil
}
