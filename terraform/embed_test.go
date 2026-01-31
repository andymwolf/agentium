package terraform

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestGetVMFiles_GCP(t *testing.T) {
	fsys, err := GetVMFiles(ProviderGCP)
	if err != nil {
		t.Fatalf("GetVMFiles(GCP) returned error: %v", err)
	}
	if fsys == nil {
		t.Fatal("GetVMFiles(GCP) returned nil filesystem")
	}

	// Verify main.tf exists
	_, err = fs.Stat(fsys, "main.tf")
	if err != nil {
		t.Errorf("main.tf not found in embedded files: %v", err)
	}

	// Verify versions.tf exists
	_, err = fs.Stat(fsys, "versions.tf")
	if err != nil {
		t.Errorf("versions.tf not found in embedded files: %v", err)
	}
}

func TestGetVMFiles_AWS(t *testing.T) {
	_, err := GetVMFiles(ProviderAWS)
	if err == nil {
		t.Error("GetVMFiles(AWS) should return error (not yet implemented)")
	}
}

func TestGetVMFiles_Azure(t *testing.T) {
	_, err := GetVMFiles(ProviderAzure)
	if err == nil {
		t.Error("GetVMFiles(Azure) should return error (not yet implemented)")
	}
}

func TestGetVMFiles_UnknownProvider(t *testing.T) {
	_, err := GetVMFiles("unknown")
	if err == nil {
		t.Error("GetVMFiles(unknown) should return error")
	}
}

func TestWriteVMFiles_GCP(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "terraform-embed-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Write files
	err = WriteVMFiles(ProviderGCP, tempDir)
	if err != nil {
		t.Fatalf("WriteVMFiles(GCP) returned error: %v", err)
	}

	// Verify main.tf was created
	mainTfPath := filepath.Join(tempDir, "main.tf")
	info, err := os.Stat(mainTfPath)
	if err != nil {
		t.Errorf("main.tf not created: %v", err)
	} else {
		// Verify permissions are 0644
		mode := info.Mode().Perm()
		if mode != 0644 {
			t.Errorf("main.tf has wrong permissions: got %o, want 0644", mode)
		}
	}

	// Verify versions.tf was created
	versionsTfPath := filepath.Join(tempDir, "versions.tf")
	info, err = os.Stat(versionsTfPath)
	if err != nil {
		t.Errorf("versions.tf not created: %v", err)
	} else {
		// Verify permissions are 0644
		mode := info.Mode().Perm()
		if mode != 0644 {
			t.Errorf("versions.tf has wrong permissions: got %o, want 0644", mode)
		}
	}

	// Verify content is not empty
	content, err := os.ReadFile(mainTfPath)
	if err != nil {
		t.Errorf("Failed to read main.tf: %v", err)
	} else if len(content) == 0 {
		t.Error("main.tf is empty")
	}
}

func TestWriteVMFiles_InvalidProvider(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "terraform-embed-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = WriteVMFiles("invalid", tempDir)
	if err == nil {
		t.Error("WriteVMFiles(invalid) should return error")
	}
}
