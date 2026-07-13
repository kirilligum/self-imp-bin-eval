package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
)

type ExportedObject struct {
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (w *GarageWriter) Export(ctx context.Context, outputDir string, prefixes []string) ([]ExportedObject, error) {
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("at least one artifact prefix is required")
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	objects := make([]ExportedObject, 0)
	for _, prefix := range prefixes {
		if strings.TrimSpace(prefix) == "" {
			return nil, fmt.Errorf("artifact prefix cannot be blank")
		}
		matched := false
		for object := range w.client.ListObjects(ctx, w.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if object.Err != nil {
				return nil, object.Err
			}
			matched = true
			if _, duplicate := seen[object.Key]; duplicate {
				continue
			}
			exported, err := w.exportObject(ctx, outputDir, object)
			if err != nil {
				return nil, err
			}
			seen[object.Key] = struct{}{}
			objects = append(objects, exported)
		}
		if !matched {
			return nil, fmt.Errorf("no Garage artifacts found for prefix %q", prefix)
		}
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (w *GarageWriter) exportObject(ctx context.Context, outputDir string, object minio.ObjectInfo) (ExportedObject, error) {
	relativePath, err := safeArtifactPath(object.Key)
	if err != nil {
		return ExportedObject{}, err
	}
	destination := filepath.Join(outputDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return ExportedObject{}, err
	}

	source, err := w.client.GetObject(ctx, w.bucket, object.Key, minio.GetObjectOptions{})
	if err != nil {
		return ExportedObject{}, err
	}
	defer source.Close()
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return ExportedObject{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), source)
	closeErr := destinationFile.Close()
	if copyErr != nil {
		return ExportedObject{}, copyErr
	}
	if closeErr != nil {
		return ExportedObject{}, closeErr
	}
	if written != object.Size {
		return ExportedObject{}, fmt.Errorf("Garage artifact %q size = %d, want %d", object.Key, written, object.Size)
	}
	return ExportedObject{Key: object.Key, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func safeArtifactPath(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, `\`) {
		return "", fmt.Errorf("unsafe Garage artifact key %q", key)
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe Garage artifact key %q", key)
	}
	return clean, nil
}
