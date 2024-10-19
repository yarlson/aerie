package ssh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindSSHKey(t *testing.T) {
	// Create temporary SSH keys in a temp directory
	tempDir := t.TempDir()
	sshKeyPath = tempDir

	keyContent := []byte("test-key")
	keyNames := []string{"id_rsa", "id_ecdsa", "id_ed25519"}

	// Write test keys
	for _, name := range keyNames {
		keyPath := filepath.Join(tempDir, name)
		err := os.WriteFile(keyPath, keyContent, 0600)
		assert.NoError(t, err)
	}

	// Override the home directory to point to tempDir
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	_ = os.Setenv("HOME", tempDir)

	// Test with no keyPath, should find id_rsa first
	key, err := FindSSHKey("")
	assert.NoError(t, err)
	assert.Equal(t, keyContent, key)

	// Test with specified keyPath
	specifiedKeyPath := filepath.Join(tempDir, "custom_key")
	err = os.WriteFile(specifiedKeyPath, keyContent, 0600)
	assert.NoError(t, err)

	key, err = FindSSHKey(specifiedKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, keyContent, key)

	// Test when no keys are found
	_ = os.RemoveAll(tempDir)
	key, err = FindSSHKey("")
	assert.Error(t, err)
	assert.Nil(t, key)
}
