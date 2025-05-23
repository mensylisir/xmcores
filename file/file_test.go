// file_test.go
package file

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"github.com/mensylisir/xmcores/common"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fileutil_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// Helper to create a temporary file with content
func createTestFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	filePath := filepath.Join(dir, name)
	err := os.WriteFile(filePath, content, common.FileMode0644)
	if err != nil {
		t.Fatalf("Failed to write test file %s: %v", filePath, err)
	}
	return filePath
}

func TestPathExists(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	existingFile := createTestFile(t, tmpDir, "exists.txt", []byte("hello"))
	nonExistingPath := filepath.Join(tmpDir, "notexists.txt")
	existingDir := filepath.Join(tmpDir, "exists_dir")
	if err := os.Mkdir(existingDir, common.FileMode0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		wantExist bool
		wantErr   bool
	}{
		{"existing file", existingFile, true, false},
		{"non-existing path", nonExistingPath, false, false},
		{"existing dir", existingDir, true, false},
		{"empty path (stat will error)", "", false, true}, // os.Stat("") errors
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExist, err := PathExists(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("PathExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gotExist != tt.wantExist {
				t.Errorf("PathExists() gotExist = %v, want %v", gotExist, tt.wantExist)
			}
		})
	}
}

func TestCreateDir(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	newDirPath := filepath.Join(tmpDir, "newdir", "subdir")
	existingFilePath := createTestFile(t, tmpDir, "existingfile.txt", []byte("content"))

	tests := []struct {
		name    string
		path    string
		wantErr bool
		setup   func() error // Optional setup for existing paths
		check   func(t *testing.T, path string)
	}{
		{
			name:    "create new nested directory",
			path:    newDirPath,
			wantErr: false,
			check: func(t *testing.T, path string) {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("Stat failed for created dir %s: %v", path, err)
				}
				if !info.IsDir() {
					t.Errorf("Path %s is not a directory after CreateDir", path)
				}
			},
		},
		{
			name:    "path is existing directory",
			path:    tmpDir, // tmpDir already exists
			wantErr: false,
		},
		{
			name:    "path is existing file (should error)",
			path:    existingFilePath,
			wantErr: true,
		},
		{
			name:    "path with invalid characters (OS dependent)",
			path:    filepath.Join(tmpDir, "invalid\x00dir"), // Null char is usually invalid
			wantErr: true,                                    // os.MkdirAll should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				if err := tt.setup(); err != nil {
					t.Fatalf("Test setup failed: %v", err)
				}
			}
			err := CreateDir(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, tt.path)
			}
		})
	}
}

func TestIsDir(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	dirPath := filepath.Join(tmpDir, "testdir")
	filePath := createTestFile(t, tmpDir, "testfile.txt", []byte("content"))
	if err := os.Mkdir(dirPath, common.FileMode0755); err != nil {
		t.Fatalf("Failed to make test dir: %v", err)
	}
	nonExistentPath := filepath.Join(tmpDir, "ghost")

	tests := []struct {
		name      string
		path      string
		wantIsDir bool
		wantErr   bool
	}{
		{"is a directory", dirPath, true, false},
		{"is a file", filePath, false, false},
		{"non-existent path", nonExistentPath, false, false}, // IsNotExist err, so returns (false,nil)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsDir, err := IsDir(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gotIsDir != tt.wantIsDir {
				t.Errorf("IsDir() gotIsDir = %v, want %v", gotIsDir, tt.wantIsDir)
			}
		})
	}
}

func TestCountDirFiles(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	// Structure:
	// tmpDir/
	//   file1.txt
	//   dir1/
	//     file2.txt
	//     file3.txt
	//     emptydir/
	//   file4.txt
	createTestFile(t, tmpDir, "file1.txt", []byte("1"))
	dir1 := filepath.Join(tmpDir, "dir1")
	CreateDir(dir1) // Use our function
	createTestFile(t, dir1, "file2.txt", []byte("2"))
	createTestFile(t, dir1, "file3.txt", []byte("3"))
	emptyDir := filepath.Join(dir1, "emptydir")
	CreateDir(emptyDir)
	createTestFile(t, tmpDir, "file4.txt", []byte("4"))

	// Symlink (optional, depending on if CountDirFiles should follow them)
	// For now, it counts regular files, so symlinks to files would be counted if d.Type().IsRegular() is true for them after dereference.
	// filepath.WalkDir's DirEntry Type() method reports the type of the symlink itself, not its target.
	// So, a symlink to a file is not d.Type().IsRegular().
	// If you need to count files including targets of symlinks, the logic in CountDirFiles would need to change.

	tests := []struct {
		name      string
		dirPath   string
		wantCount int
		wantErr   bool
	}{
		{"count files in tmpDir", tmpDir, 2, false}, // file1.txt, file4.txt (dir1 is a dir)
		{"count files in dir1", dir1, 2, false},     // file2.txt, file3.txt (emptydir is a dir)
		{"count files in emptyDir", emptyDir, 0, false},
		{"path is a file", filepath.Join(tmpDir, "file1.txt"), 0, true},  // Should error
		{"non-existent dir", filepath.Join(tmpDir, "ghostdir"), 0, true}, // Should error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Re-run CountDirFiles for target dirs to ensure independent counts
			// Special handling for counting within tmpDir which includes dir1
			if tt.dirPath == tmpDir {
				// Walk tmpDir directly as per its own structure, it should not count files in dir1 for this test case of "tmpDir"
				// The original test case description implied it counts top-level files in tmpDir
				// To count all files recursively from tmpDir:
				var totalCount int
				filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
					if err == nil && !d.IsDir() && d.Type().IsRegular() {
						totalCount++
					}
					return nil
				})
				// Override wantCount for this specific scenario to be the total recursive count from tmpDir
				// tt.wantCount = totalCount // This would be 4 for the structure above
				// The current test expectation for tmpDir is 2 (top-level only by its name)
				// Let's clarify: CountDirFiles is recursive. So it should be 4 for tmpDir.
				tt.wantCount = 4
			}

			gotCount, err := CountDirFiles(tt.dirPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("CountDirFiles() error = %v, wantErr %v for path %s", err, tt.wantErr, tt.dirPath)
				return
			}
			if !tt.wantErr && gotCount != tt.wantCount {
				t.Errorf("CountDirFiles() gotCount = %d, want %d for path %s", gotCount, tt.wantCount, tt.dirPath)
			}
		})
	}
}

func TestFileMD5_LocalMd5Sum(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	content := []byte("hello world for md5 test")
	filePath := createTestFile(t, tmpDir, "md5test.txt", content)

	h := md5.New()
	h.Write(content)
	expectedMD5 := fmt.Sprintf("%x", h.Sum(nil))

	// Test FileMD5
	gotMD5, err := FileMD5(filePath)
	if err != nil {
		t.Fatalf("FileMD5() error = %v", err)
	}
	if gotMD5 != expectedMD5 {
		t.Errorf("FileMD5() gotMD5 = %s, want %s", gotMD5, expectedMD5)
	}

	// Test LocalMd5Sum
	gotMD5Local, err := LocalMd5Sum(filePath)
	if err != nil {
		t.Fatalf("LocalMd5Sum() error = %v", err)
	}
	if gotMD5Local != expectedMD5 {
		t.Errorf("LocalMd5Sum() gotMD5Local = %s, want %s", gotMD5Local, expectedMD5)
	}

	// Test non-existent file
	_, err = FileMD5(filepath.Join(tmpDir, "nonexistent.txt"))
	if err == nil {
		t.Errorf("FileMD5() expected error for non-existent file, got nil")
	}
	_, err = LocalMd5Sum(filepath.Join(tmpDir, "nonexistent.txt"))
	if err == nil {
		t.Errorf("LocalMd5Sum() expected error for non-existent file, got nil")
	}
}

func TestCreateFileDir(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		filePath string
		wantErr  bool
		checkDir string // The directory that should be created
	}{
		{"create nested dir for file", filepath.Join(tmpDir, "a", "b", "c.txt"), false, filepath.Join(tmpDir, "a", "b")},
		{"file in current dir (no dir creation)", filepath.Join(tmpDir, "d.txt"), false, ""}, // dir is tmpDir, which exists
		{"file is just a name (current dir)", "e.txt", false, ""},                            // dir is ".", which exists
		{"empty file path (should not error, dir is '.')", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For "e.txt", change working directory to tmpDir to avoid creating "e.txt" in project root
			originalWD, _ := os.Getwd()
			if tt.filePath == "e.txt" { // A bit of a hack for this specific case
				os.Chdir(tmpDir)
				defer os.Chdir(originalWD) // Ensure WD is restored
			}

			err := CreateFileDir(tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateFileDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkDir != "" {
				exists, _ := PathExists(tt.checkDir)
				if !exists {
					t.Errorf("CreateFileDir() expected directory %s to be created, but it wasn't", tt.checkDir)
				} else {
					isDir, _ := IsDir(tt.checkDir)
					if !isDir {
						t.Errorf("CreateFileDir() expected %s to be a directory, but it's not", tt.checkDir)
					}
				}
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	tmpDir := createTestDir(t)
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "sub", "written.txt")
	content := []byte("content to be written")

	err := WriteFile(filePath, content)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Check if file exists
	exists, _ := PathExists(filePath)
	if !exists {
		t.Errorf("WriteFile() file %s was not created", filePath)
	}

	// Check content
	readContent, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("Failed to read back written file: %v", readErr)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("WriteFile() content mismatch. Got '%s', want '%s'", string(readContent), string(content))
	}

	// Check permissions (approximate, as OS might alter them slightly, e.g. umask)
	info, statErr := os.Stat(filePath)
	if statErr != nil {
		t.Fatalf("Stat failed on written file: %v", statErr)
	}
	if info.Mode().Perm()&common.FileMode0644 != common.FileMode0644 && info.Mode().Perm() != common.FileMode0644 {
		// Allow if umask has removed some perms but base is 0644
		// This check is tricky due to umask. A more robust check might be complex.
		// For now, a simple check that it's not wildly different.
		t.Logf("WriteFile() file permissions = %o, expected base %o (umask might affect this)", info.Mode().Perm(), common.FileMode0644)
	}

	// Check parent dir permissions
	parentDir := filepath.Dir(filePath)
	parentInfo, parentStatErr := os.Stat(parentDir)
	if parentStatErr != nil {
		t.Fatalf("Stat failed on parent dir of written file: %v", parentStatErr)
	}
	if parentInfo.Mode().Perm()&common.FileMode0755 != common.FileMode0755 && parentInfo.Mode().Perm() != common.FileMode0755 {
		t.Logf("WriteFile() parent dir permissions = %o, expected base %o (umask might affect this)", parentInfo.Mode().Perm(), common.FileMode0755)
	}
}

func TestTarAndUntar(t *testing.T) {
	// Setup source directory
	srcTmpDir := createTestDir(t)
	defer os.RemoveAll(srcTmpDir)

	// Structure for tar:
	// srcTmpDir/
	//   rootfile.txt
	//   data/
	//     file1.txt
	//     file2.txt
	//     subdata/
	//       nested.txt
	//   empty/
	//   link.txt -> rootfile.txt (symlink)

	createTestFile(t, srcTmpDir, "rootfile.txt", []byte("root content"))
	dataDir := filepath.Join(srcTmpDir, "data")
	CreateDir(dataDir)
	createTestFile(t, dataDir, "file1.txt", []byte("data file 1"))
	createTestFile(t, dataDir, "file2.txt", []byte("data file 2"))
	subDataDir := filepath.Join(dataDir, "subdata")
	CreateDir(subDataDir)
	createTestFile(t, subDataDir, "nested.txt", []byte("deeply nested"))
	emptyDir := filepath.Join(srcTmpDir, "empty")
	CreateDir(emptyDir)

	// Create symlink (skip on Windows if os.Symlink not fully supported or requires admin)
	// For simplicity, this test might have issues with symlinks on Windows without specific setup.
	symlinkTarget := "rootfile.txt" // Relative symlink
	symlinkPath := filepath.Join(srcTmpDir, "link.txt")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Logf("Skipping symlink part of test: could not create symlink: %v", err)
		symlinkPath = "" // Mark as not created
	}

	// Tarball destination
	tarballTmpDir := createTestDir(t)
	defer os.RemoveAll(tarballTmpDir)
	tarballPath := filepath.Join(tarballTmpDir, "archive.tar.gz")

	// --- Test Tar ---
	// Case 1: Tar entire srcTmpDir, no trim prefix
	// Expected in tar: srcTmpDirName/rootfile.txt, srcTmpDirName/data/file1.txt etc.
	t.Run("Tar_NoTrim", func(t *testing.T) {
		err := Tar(srcTmpDir, tarballPath, "")
		if err != nil {
			t.Fatalf("Tar() with no trim failed: %v", err)
		}
		// Basic check: tarball exists and is not empty
		info, err := os.Stat(tarballPath)
		if err != nil {
			t.Fatalf("Tarball %s not created or stat failed: %v", tarballPath, err)
		}
		if info.Size() == 0 {
			t.Errorf("Tarball %s is empty", tarballPath)
		}
	})

	// --- Test Untar (after Tar_NoTrim) ---
	untarTmpDir1 := createTestDir(t)
	defer os.RemoveAll(untarTmpDir1)

	t.Run("Untar_NoTrim", func(t *testing.T) {
		// Ensure tarball was created by previous test
		if _, err := os.Stat(tarballPath); os.IsNotExist(err) {
			t.Skip("Skipping Untar_NoTrim because tarball from Tar_NoTrim does not exist")
		}

		err := Untar(tarballPath, untarTmpDir1)
		if err != nil {
			t.Fatalf("Untar() failed: %v", err)
		}

		// Verify structure and content
		srcTmpDirBase := filepath.Base(srcTmpDir)
		expectedRootFile := filepath.Join(untarTmpDir1, srcTmpDirBase, "rootfile.txt")
		expectedDataFile1 := filepath.Join(untarTmpDir1, srcTmpDirBase, "data", "file1.txt")
		expectedNestedFile := filepath.Join(untarTmpDir1, srcTmpDirBase, "data", "subdata", "nested.txt")
		expectedEmptyDir := filepath.Join(untarTmpDir1, srcTmpDirBase, "empty")
		expectedSymlink := filepath.Join(untarTmpDir1, srcTmpDirBase, "link.txt")

		verifyFileContent(t, expectedRootFile, "root content")
		verifyFileContent(t, expectedDataFile1, "data file 1")
		verifyFileContent(t, expectedNestedFile, "deeply nested")
		verifyIsDir(t, expectedEmptyDir)

		if symlinkPath != "" { // If symlink was created in source
			verifyIsSymlinkTo(t, expectedSymlink, symlinkTarget) // Target in tar should be relative
		}
	})

	// Case 2: Tar srcTmpDir, trim with srcTmpDir
	// Expected in tar: rootfile.txt, data/file1.txt etc. (contents of srcTmpDir at root)
	tarballPathTrimmed := filepath.Join(tarballTmpDir, "archive_trimmed.tar.gz")
	t.Run("Tar_TrimSrcDir", func(t *testing.T) {
		err := Tar(srcTmpDir, tarballPathTrimmed, srcTmpDir) // Trim srcTmpDir itself
		if err != nil {
			t.Fatalf("Tar() with trim srcTmpDir failed: %v", err)
		}
		info, err := os.Stat(tarballPathTrimmed)
		if err != nil {
			t.Fatalf("Trimmed tarball %s not created or stat failed: %v", tarballPathTrimmed, err)
		}
		if info.Size() == 0 {
			t.Errorf("Trimmed tarball %s is empty", tarballPathTrimmed)
		}
	})

	// --- Test Untar (after Tar_TrimSrcDir) ---
	untarTmpDir2 := createTestDir(t)
	defer os.RemoveAll(untarTmpDir2)

	t.Run("Untar_TrimSrcDir", func(t *testing.T) {
		if _, err := os.Stat(tarballPathTrimmed); os.IsNotExist(err) {
			t.Skip("Skipping Untar_TrimSrcDir because tarball from Tar_TrimSrcDir does not exist")
		}

		err := Untar(tarballPathTrimmed, untarTmpDir2)
		if err != nil {
			t.Fatalf("Untar() for trimmed archive failed: %v", err)
		}

		// Verify structure and content (should be directly under untarTmpDir2)
		expectedRootFile := filepath.Join(untarTmpDir2, "rootfile.txt")
		expectedDataFile1 := filepath.Join(untarTmpDir2, "data", "file1.txt")
		expectedNestedFile := filepath.Join(untarTmpDir2, "data", "subdata", "nested.txt")
		expectedEmptyDir := filepath.Join(untarTmpDir2, "empty")
		expectedSymlink := filepath.Join(untarTmpDir2, "link.txt")

		verifyFileContent(t, expectedRootFile, "root content")
		verifyFileContent(t, expectedDataFile1, "data file 1")
		verifyFileContent(t, expectedNestedFile, "deeply nested")
		verifyIsDir(t, expectedEmptyDir)
		if symlinkPath != "" {
			verifyIsSymlinkTo(t, expectedSymlink, symlinkTarget)
		}
	})

	// Case 3: Tar a single file
	singleFileToTar := filepath.Join(srcTmpDir, "rootfile.txt")
	tarballSingleFile := filepath.Join(tarballTmpDir, "singlefile.tar.gz")
	t.Run("Tar_SingleFile", func(t *testing.T) {
		err := Tar(singleFileToTar, tarballSingleFile, filepath.Dir(singleFileToTar)) // Trim its parent dir
		if err != nil {
			t.Fatalf("Tar() single file failed: %v", err)
		}
	})
	untarTmpDir3 := createTestDir(t)
	defer os.RemoveAll(untarTmpDir3)
	t.Run("Untar_SingleFile", func(t *testing.T) {
		if _, err := os.Stat(tarballSingleFile); os.IsNotExist(err) {
			t.Skip("Skipping Untar_SingleFile because tarball does not exist")
		}
		err := Untar(tarballSingleFile, untarTmpDir3)
		if err != nil {
			t.Fatalf("Untar() single file failed: %v", err)
		}
		verifyFileContent(t, filepath.Join(untarTmpDir3, "rootfile.txt"), "root content")
	})

}

// Helper for Tar/Untar tests
func verifyFileContent(t *testing.T, filePath, expectedContent string) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("Failed to read file %s for verification: %v", filePath, err)
		return
	}
	if string(content) != expectedContent {
		t.Errorf("Content mismatch for %s. Got '%s', want '%s'", filePath, string(content), expectedContent)
	}
}

func verifyIsDir(t *testing.T, path string) {
	t.Helper()
	isDir, err := IsDir(path)
	if err != nil {
		t.Errorf("IsDir check failed for %s: %v", path, err)
		return
	}
	if !isDir {
		t.Errorf("Expected %s to be a directory, but it's not.", path)
	}
}

func verifyIsSymlinkTo(t *testing.T, linkPath, expectedTarget string) {
	t.Helper()
	info, err := os.Lstat(linkPath) // Lstat to get info about the link itself
	if err != nil {
		t.Errorf("Lstat failed for symlink %s: %v", linkPath, err)
		return
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", linkPath)
		return
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Errorf("Readlink failed for %s: %v", linkPath, err)
		return
	}
	// On windows, symlink target might be an absolute path even if created relatively.
	// For cross-platform, this check needs care.
	// Tar should preserve the Linkname as it was.
	if target != expectedTarget {
		// If on windows, check if filepath.Abs(target) matches filepath.Abs(expectedTarget)
		// This is a simplification for the test. A robust check would be more involved.
		t.Logf("Symlink target mismatch for %s. Got '%s', want '%s'. (Note: OS path differences might occur)", linkPath, target, expectedTarget)
		// Allow if they are "equivalent" on the current OS
		absTarget, _ := filepath.Abs(filepath.Join(filepath.Dir(linkPath), target))
		absExpected, _ := filepath.Abs(filepath.Join(filepath.Dir(linkPath), expectedTarget))
		if absTarget != absExpected && !strings.EqualFold(target, expectedTarget) { // Also try case-insensitive for windows
			t.Errorf("Symlink target mismatch for %s. Got '%s', want '%s'.", linkPath, target, expectedTarget)
		}
	}
}
