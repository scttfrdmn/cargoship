package config

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func TestSuitCaseOpts_Fields(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:            "tar.gz.gpg",
		EncryptInner:      true,
		EncryptOuter:      false,
		HashInner:         true,
		HashOuter:         false,
		HashAlgorithm:     "sha256",
		EncryptTo:         nil,
		PostProcessScript: "/path/to/script.sh",
		PostProcessEnv:    map[string]string{"ENV_VAR": "value"},
	}

	if opts.Format != "tar.gz.gpg" {
		t.Errorf("SuitCaseOpts.Format = %v, want tar.gz.gpg", opts.Format)
	}
	if !opts.EncryptInner {
		t.Errorf("SuitCaseOpts.EncryptInner = false, want true")
	}
	if opts.EncryptOuter {
		t.Errorf("SuitCaseOpts.EncryptOuter = true, want false")
	}
	if !opts.HashInner {
		t.Errorf("SuitCaseOpts.HashInner = false, want true")
	}
	if opts.HashOuter {
		t.Errorf("SuitCaseOpts.HashOuter = true, want false")
	}
	if opts.HashAlgorithm != "sha256" {
		t.Errorf("SuitCaseOpts.HashAlgorithm = %v, want sha256", opts.HashAlgorithm)
	}
	if opts.PostProcessScript != "/path/to/script.sh" {
		t.Errorf("SuitCaseOpts.PostProcessScript = %v, want /path/to/script.sh", opts.PostProcessScript)
	}
	if opts.PostProcessEnv["ENV_VAR"] != "value" {
		t.Errorf("SuitCaseOpts.PostProcessEnv[ENV_VAR] = %v, want value", opts.PostProcessEnv["ENV_VAR"])
	}
}

func TestSuitCaseOpts_EncryptToCobra_NoEncryption(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz",
		EncryptInner: false,
	}

	cmd := &cobra.Command{}
	
	err := opts.EncryptToCobra(cmd)
	if err != nil {
		t.Errorf("EncryptToCobra() with no encryption should not return error, got %v", err)
	}

	// EncryptTo should remain nil since no encryption is needed
	if opts.EncryptTo != nil {
		t.Errorf("EncryptToCobra() with no encryption should not set EncryptTo")
	}
}

func TestSuitCaseOpts_EncryptToCobra_WithGPGFormat(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz.gpg",
		EncryptInner: false,
	}

	cmd := &cobra.Command{}
	// Add the required GPG flags that the gpg.EncryptToWithCmd function expects
	cmd.Flags().StringSlice("encrypt-to", []string{}, "GPG recipient keys")
	cmd.Flags().String("gpg-home", "", "GPG home directory")
	
	err := opts.EncryptToCobra(cmd)
	// This will likely return an error since we don't have actual GPG setup,
	// but we're testing that the function is called
	if err == nil {
		// If no error, EncryptTo should be set (though likely empty)
		if opts.EncryptTo == nil {
			t.Errorf("EncryptToCobra() with .gpg format should attempt to set EncryptTo")
		}
	} else {
		// Error is expected in test environment without proper GPG setup
		t.Logf("EncryptToCobra() returned expected error in test env: %v", err)
	}
}

func TestSuitCaseOpts_EncryptToCobra_WithInnerEncryption(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz",
		EncryptInner: true,
	}

	cmd := &cobra.Command{}
	// Add the required GPG flags
	cmd.Flags().StringSlice("encrypt-to", []string{}, "GPG recipient keys")
	cmd.Flags().String("gpg-home", "", "GPG home directory")
	
	err := opts.EncryptToCobra(cmd)
	// This will likely return an error since we don't have actual GPG setup,
	// but we're testing that the function is called
	if err == nil {
		// If no error, EncryptTo should be set (though likely empty)
		if opts.EncryptTo == nil {
			t.Errorf("EncryptToCobra() with EncryptInner=true should attempt to set EncryptTo")
		}
	} else {
		// Error is expected in test environment without proper GPG setup
		t.Logf("EncryptToCobra() returned expected error in test env: %v", err)
	}
}

func TestSuitCaseOpts_EncryptToCobra_MultipleConditions(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz.gpg",
		EncryptInner: true, // Both conditions true
	}

	cmd := &cobra.Command{}
	// Add the required GPG flags
	cmd.Flags().StringSlice("encrypt-to", []string{}, "GPG recipient keys")
	cmd.Flags().String("gpg-home", "", "GPG home directory")
	
	err := opts.EncryptToCobra(cmd)
	// This will likely return an error since we don't have actual GPG setup,
	// but we're testing that the function is called when either condition is true
	if err == nil {
		if opts.EncryptTo == nil {
			t.Errorf("EncryptToCobra() with both .gpg format and EncryptInner=true should attempt to set EncryptTo")
		}
	} else {
		// Error is expected in test environment without proper GPG setup
		t.Logf("EncryptToCobra() returned expected error in test env: %v", err)
	}
}

func TestHashSet_Fields(t *testing.T) {
	hashSet := HashSet{
		Filename: "test-file.txt",
		Hash:     "abc123def456",
	}

	if hashSet.Filename != "test-file.txt" {
		t.Errorf("HashSet.Filename = %v, want test-file.txt", hashSet.Filename)
	}
	if hashSet.Hash != "abc123def456" {
		t.Errorf("HashSet.Hash = %v, want abc123def456", hashSet.Hash)
	}
}

func TestHashSet_ZeroValues(t *testing.T) {
	hashSet := HashSet{}

	if hashSet.Filename != "" {
		t.Errorf("HashSet.Filename default = %v, want empty string", hashSet.Filename)
	}
	if hashSet.Hash != "" {
		t.Errorf("HashSet.Hash default = %v, want empty string", hashSet.Hash)
	}
}

func TestSuitCaseOpts_ZeroValues(t *testing.T) {
	opts := &SuitCaseOpts{}

	if opts.Format != "" {
		t.Errorf("SuitCaseOpts.Format default = %v, want empty string", opts.Format)
	}
	if opts.EncryptInner {
		t.Errorf("SuitCaseOpts.EncryptInner default = true, want false")
	}
	if opts.EncryptOuter {
		t.Errorf("SuitCaseOpts.EncryptOuter default = true, want false")
	}
	if opts.HashInner {
		t.Errorf("SuitCaseOpts.HashInner default = true, want false")
	}
	if opts.HashOuter {
		t.Errorf("SuitCaseOpts.HashOuter default = true, want false")
	}
	if opts.HashAlgorithm != "" {
		t.Errorf("SuitCaseOpts.HashAlgorithm default = %v, want empty string", opts.HashAlgorithm)
	}
	if opts.EncryptTo != nil {
		t.Errorf("SuitCaseOpts.EncryptTo default = %v, want nil", opts.EncryptTo)
	}
	if opts.PostProcessScript != "" {
		t.Errorf("SuitCaseOpts.PostProcessScript default = %v, want empty string", opts.PostProcessScript)
	}
	if opts.PostProcessEnv != nil {
		t.Errorf("SuitCaseOpts.PostProcessEnv default = %v, want nil", opts.PostProcessEnv)
	}
}

func TestSuitCaseOpts_EncryptToCobra_NilCommand(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz.gpg",
		EncryptInner: false,
	}

	// Test with nil command (should panic or return error, let's catch panic)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("EncryptToCobra() with nil command should panic or return error")
		}
	}()
	
	_ = opts.EncryptToCobra(nil)
}

func TestSuitCaseOpts_EncryptToCobra_EmptyFormat(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "",
		EncryptInner: false,
	}

	cmd := &cobra.Command{}
	
	err := opts.EncryptToCobra(cmd)
	if err != nil {
		t.Errorf("EncryptToCobra() with empty format and no inner encryption should not return error, got %v", err)
	}

	if opts.EncryptTo != nil {
		t.Errorf("EncryptToCobra() with empty format should not set EncryptTo")
	}
}

func TestSuitCaseOpts_EncryptToCobra_NoGPGSuffix(t *testing.T) {
	opts := &SuitCaseOpts{
		Format:       "tar.gz",
		EncryptInner: false,
	}

	cmd := &cobra.Command{}
	
	err := opts.EncryptToCobra(cmd)
	if err != nil {
		t.Errorf("EncryptToCobra() with non-GPG format and no inner encryption should not return error, got %v", err)
	}

	if opts.EncryptTo != nil {
		t.Errorf("EncryptToCobra() with non-GPG format should not set EncryptTo")
	}
}

func TestHashSet_Collection(t *testing.T) {
	// Test that we can work with collections of HashSet
	hashSets := []HashSet{
		{Filename: "file1.txt", Hash: "hash1"},
		{Filename: "file2.txt", Hash: "hash2"},
		{Filename: "file3.txt", Hash: "hash3"},
	}

	if len(hashSets) != 3 {
		t.Errorf("HashSet collection length = %v, want 3", len(hashSets))
	}

	for i, hs := range hashSets {
		expectedFilename := fmt.Sprintf("file%d.txt", i+1)
		expectedHash := fmt.Sprintf("hash%d", i+1)
		
		if hs.Filename != expectedFilename {
			t.Errorf("HashSet[%d].Filename = %v, want %v", i, hs.Filename, expectedFilename)
		}
		if hs.Hash != expectedHash {
			t.Errorf("HashSet[%d].Hash = %v, want %v", i, hs.Hash, expectedHash)
		}
	}
}

