package terraform

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed modules/vm/gcp/*.tf
var gcpVMFiles embed.FS

const (
	ProviderGCP   = "gcp"
	ProviderAWS   = "aws"
	ProviderAzure = "azure"
)

// GetVMFiles returns the embedded terraform files for the provider.
func GetVMFiles(provider string) (fs.FS, error) {
	switch provider {
	case ProviderGCP:
		return fs.Sub(gcpVMFiles, "modules/vm/gcp")
	case ProviderAWS:
		return nil, fmt.Errorf("AWS terraform modules not yet implemented")
	case ProviderAzure:
		return nil, fmt.Errorf("Azure terraform modules not yet implemented")
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// WriteVMFiles writes terraform files for the provider to destDir.
func WriteVMFiles(provider string, destDir string) error {
	fsys, err := GetVMFiles(provider)
	if err != nil {
		return err
	}

	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".tf" {
			return err
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}
		return os.WriteFile(filepath.Join(destDir, filepath.Base(path)), content, 0644)
	})
}
