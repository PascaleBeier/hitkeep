package assetstore

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStoreMissingRootOperationsHaveNoSideEffects(t *testing.T) {
	store := New(t.TempDir())
	siteID, qrID := uuid.New(), uuid.New()
	key := filepath.ToSlash(filepath.Join(qrCodeAssetPrefix, siteID.String(), qrID.String(), "asset.png"))

	if _, err := store.Open(key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open error = %v, want wrapped not-exist", err)
	}
	if err := store.Delete(key); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteQRCodeAssetDir(siteID, qrID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteQRCodeAssetsForSite(siteID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Root()); !os.IsNotExist(err) {
		t.Fatalf("asset root was created: %v", err)
	}
}

func TestStorePutOpenDeleteRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	siteID, firstQRID, secondQRID := uuid.New(), uuid.New(), uuid.New()
	firstKey, err := store.PutQRCodeAsset(siteID, firstQRID, "first", "asset.png", "image/png", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	if got := readAsset(t, store, firstKey); !bytes.Equal(got, []byte("first")) {
		t.Errorf("first asset = %q, want %q", got, "first")
	}
	if replacementKey, err := store.PutQRCodeAsset(siteID, firstQRID, "first", "asset.png", "image/png", []byte("replacement")); err != nil || replacementKey != firstKey {
		t.Fatalf("replacement = %q, %v; want %q, nil", replacementKey, err, firstKey)
	}
	if got := readAsset(t, store, firstKey); !bytes.Equal(got, []byte("replacement")) {
		t.Errorf("replacement asset = %q, want %q", got, "replacement")
	}
	secondKey, err := store.PutQRCodeAsset(siteID, secondQRID, "second", "asset.png", "image/png", []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(firstKey); err != nil {
		t.Fatal(err)
	}
	if got := readAsset(t, store, secondKey); !bytes.Equal(got, []byte("second")) {
		t.Errorf("remaining asset = %q, want %q", got, "second")
	}
	if err := store.Delete(secondKey); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.Root())
	if err != nil {
		t.Fatalf("asset root removed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("asset root has %d entries after final delete, want empty", len(entries))
	}
}

func TestPutQRCodeAssetCleansTemporaryFileAfterRenameFailure(t *testing.T) {
	store := New(t.TempDir())
	siteID, qrID := uuid.New(), uuid.New()
	parent := filepath.Join(store.Root(), qrCodeAssetPrefix, siteID.String(), qrID.String())
	if err := os.MkdirAll(filepath.Join(parent, "checksum.png"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutQRCodeAsset(siteID, qrID, "checksum", "asset.png", "image/png", []byte("asset")); err == nil {
		t.Fatal("PutQRCodeAsset succeeded with a directory at its destination")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".qr-asset-") {
			t.Errorf("temporary asset %q was not removed", entry.Name())
		}
	}
}

func TestStoreRejectsSymlinkEscapes(t *testing.T) {
	outside := t.TempDir()
	siteID, qrID := uuid.New(), uuid.New()
	outsideQR := filepath.Join(outside, siteID.String(), qrID.String())
	if err := os.MkdirAll(filepath.Join(outside, siteID.String(), "sibling"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideQR, 0755); err != nil {
		t.Fatal(err)
	}
	for path, contents := range map[string][]byte{
		filepath.Join(outside, "sentinel"):                         []byte("root sentinel"),
		filepath.Join(outside, siteID.String(), "sibling", "keep"): []byte("site sentinel"),
		filepath.Join(outsideQR, "checksum.png"):                   []byte("put target"),
		filepath.Join(outsideQR, "asset.png"):                      []byte("open/delete target"),
		filepath.Join(outsideQR, "keep.png"):                       []byte("qr sentinel"),
	} {
		if err := os.WriteFile(path, contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	outsideTree := treeSnapshot(t, outside)

	store := New(t.TempDir())
	if err := os.MkdirAll(store.Root(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store.Root(), qrCodeAssetPrefix)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	key := filepath.ToSlash(filepath.Join(qrCodeAssetPrefix, siteID.String(), qrID.String(), "asset.png"))

	if _, err := store.PutQRCodeAsset(siteID, qrID, "checksum", "asset.png", "image/png", []byte("asset")); err == nil {
		t.Error("PutQRCodeAsset succeeded through an intermediate symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if _, err := store.Open(key); err == nil {
		t.Error("Open succeeded through an intermediate symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if err := store.Delete(key); err == nil {
		t.Error("Delete succeeded through an intermediate symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if err := store.DeleteQRCodeAssetDir(siteID, qrID); err == nil {
		t.Error("DeleteQRCodeAssetDir succeeded through an intermediate symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if err := store.DeleteQRCodeAssetsForSite(siteID); err == nil {
		t.Error("DeleteQRCodeAssetsForSite succeeded through an intermediate symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)

	store = New(t.TempDir())
	parent := filepath.Join(store.Root(), qrCodeAssetPrefix, siteID.String(), qrID.String())
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	finalPath := filepath.Join(parent, "final.png")
	finalKey := filepath.ToSlash(filepath.Join(qrCodeAssetPrefix, siteID.String(), qrID.String(), "final.png"))
	outsideSentinel := filepath.Join(outside, "sentinel")
	if err := os.Symlink(outsideSentinel, finalPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(finalKey); err == nil {
		t.Error("Open succeeded through a final-file symlink")
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if err := store.Delete(finalKey); err != nil {
		t.Fatalf("Delete final-file symlink: %v", err)
	}
	if _, err := os.Lstat(finalPath); !os.IsNotExist(err) {
		t.Fatalf("final-file symlink remains: %v", err)
	}
	assertTreeUnchanged(t, outside, outsideTree)
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSentinel, finalPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutQRCodeAsset(siteID, qrID, "final", "asset.png", "image/png", []byte("replacement")); err != nil {
		t.Fatalf("PutQRCodeAsset replacing final-file symlink: %v", err)
	}
	if info, err := os.Lstat(finalPath); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("final asset after rooted rename = %v, %v; want regular file", info, err)
	}
	if got, err := os.ReadFile(finalPath); err != nil || !bytes.Equal(got, []byte("replacement")) {
		t.Fatalf("final asset after rooted rename = %q, %v; want replacement, nil", got, err)
	}
	assertTreeUnchanged(t, outside, outsideTree)
}

func readAsset(t *testing.T, store *Store, key string) []byte {
	t.Helper()
	file, err := store.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	entries := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			entries[relative] = "directory"
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries[relative] = string(contents)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func assertTreeUnchanged(t *testing.T, root string, want map[string]string) {
	t.Helper()
	if got := treeSnapshot(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("outside tree changed: got %#v, want %#v", got, want)
	}
}
