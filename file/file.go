package file

import (
	"archive/tar"
	"compress/gzip"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mensylisir/xmcores/common"
)

// PathExists checks if a path exists.
// It returns true if the path exists, false otherwise.
// It distinguishes between "not exist" and other errors. If an error other than "not exist" occurs,
// it will also return false and the error.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil // Path exists
	}
	if os.IsNotExist(err) {
		return false, nil // Path does not exist, no error for the caller in this specific case
	}
	return false, err // An error occurred (e.g., permission denied)
}

// CreateDir creates a directory and all its parents if they don't exist.
// It uses common.FileMode0755 for directory permissions.
func CreateDir(path string) error {
	// Check if path exists and is already a directory
	info, err := os.Stat(path)
	if err == nil { // Path exists
		if info.IsDir() {
			return nil // Already a directory, nothing to do
		}
		return fmt.Errorf("path %s exists but is not a directory", path)
	}

	// If error is "not exist", proceed to create
	if os.IsNotExist(err) {
		// Using common.FileMode0755 from your constants
		return os.MkdirAll(path, common.FileMode0755)
	}

	// Some other error occurred during Stat
	return fmt.Errorf("failed to check directory %s: %w", path, err)
}

// IsDir checks if the given path is a directory.
func IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // Not a directory because it doesn't exist
		}
		return false, err // Other error
	}
	return info.IsDir(), nil
}

// CountDirFiles recursively counts the number of regular files in a directory.
// It skips directories.
func CountDirFiles(dirName string) (int, error) {
	isDir, err := IsDir(dirName)
	if err != nil {
		return 0, fmt.Errorf("failed to check if %s is a directory: %w", dirName, err)
	}
	if !isDir {
		return 0, fmt.Errorf("%s is not a directory", dirName)
	}

	var count int
	walkErr := filepath.WalkDir(dirName, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// If the error is about a specific path, we might choose to log it and continue,
			// or return the error to stop the walk. For counting, often we want to continue if possible.
			// However, if WalkDir itself returns an error (e.g., permission denied on the root dirName),
			// that error will be propagated.
			if errors.Is(err, fs.SkipDir) { // Allow skipping directories if WalkDir itself skips
				return err
			}
			// Log non-critical errors and continue? Or fail hard?
			// For now, let's propagate the error from the walk function.
			// fmt.Fprintf(os.Stderr, "Warning: error accessing path %s: %v. Skipping.\n", path, err)
			// return nil // to continue counting other files
			return err // to stop and return the error
		}

		if !d.IsDir() && d.Type().IsRegular() { // Ensure it's a regular file
			count++
		}
		return nil
	})

	if walkErr != nil {
		return 0, fmt.Errorf("error walking directory %s: %w", dirName, walkErr)
	}
	return count, nil
}

// FileMD5 calculates the MD5 checksum of a file.
func FileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close() // Ensure file is closed

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to copy file content to hash for %s: %w", path, err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// LocalMd5Sum is a wrapper around FileMD5 that panics on error.
// Consider if panicking is the desired behavior for a utility function.
// It's generally better to return errors.
// Renamed to CalculateFileMD5OrPanic for clarity if panic is intended.
// Or better, remove it and let caller handle FileMD5's error.
// For now, I'll keep it but emphasize that returning error is preferred.
func LocalMd5Sum(src string) (string, error) { // Changed to return error
	md5Str, err := FileMD5(src)
	if err != nil {
		// Instead of Fatalf, return the error. The caller can decide how to log/handle it.
		return "", fmt.Errorf("get file md5 for %s failed: %w", src, err)
	}
	return md5Str, nil
}

// CreateFileDir creates the full directory path for a given file name if it doesn't exist.
// e.g., for "./aa/bb/xxx.txt", it ensures "./aa/bb" exists.
func CreateFileDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" { // No directory part or current directory
		return nil
	}
	return CreateDir(dir) // Uses the improved CreateDir
}

// Mkdir is now just an alias to CreateDir for backward compatibility,
// but CreateDir is preferred for its clearer name and slightly better logic.
// Deprecated: Use CreateDir instead for clarity.
func Mkdir(dirName string) error {
	return CreateDir(dirName)
}

// WriteFile writes content to a file, creating parent directories if necessary.
// It uses common.FileMode0755 for directories and common.FileMode0644 for the file.
func WriteFile(filePath string, content []byte) error {
	if err := CreateFileDir(filePath); err != nil {
		return fmt.Errorf("failed to create directory for file %s: %w", filePath, err)
	}

	// Using common.FileMode0644 from your constants
	err := os.WriteFile(filePath, content, common.FileMode0644)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filePath, err)
	}
	return nil
}

// Tar archives the source path (file or directory) into a gzipped tarball at the destination path.
// trimPathPrefix is removed from the paths of files/directories within the tar archive.
// If trimPathPrefix is empty, paths in tar will be relative to srcPath.
func Tar(srcPath, dstTarball, trimPathPrefix string) error {
	fw, err := os.Create(dstTarball)
	if err != nil {
		return fmt.Errorf("failed to create destination tarball %s: %w", dstTarball, err)
	}
	defer fw.Close()

	gw := gzip.NewWriter(fw)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Clean srcPath and trimPathPrefix for reliable comparison and manipulation
	srcPath = filepath.Clean(srcPath)
	if trimPathPrefix != "" {
		trimPathPrefix = filepath.Clean(trimPathPrefix)
	}

	return filepath.WalkDir(srcPath, func(currentPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s during tar: %w", currentPath, err)
		}

		// Create header
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get FileInfo for %s: %w", currentPath, err)
		}

		link := ""
		if d.Type()&os.ModeSymlink != 0 {
			link, err = os.Readlink(currentPath)
			if err != nil {
				return fmt.Errorf("failed to read symlink %s: %w", currentPath, err)
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", currentPath, err)
		}

		// Determine the name of the file/directory in the tar archive (hdr.Name)
		var pathInTar string
		if trimPathPrefix != "" {
			// If trimPathPrefix is provided, try to make currentPath relative to it.
			// Ensure currentPath is actually under trimPathPrefix.
			if strings.HasPrefix(currentPath, trimPathPrefix) {
				// Make relative to trimPathPrefix
				pathInTar, err = filepath.Rel(trimPathPrefix, currentPath)
				if err != nil {
					return fmt.Errorf("failed to calculate relative path for %s from trim prefix %s: %w", currentPath, trimPathPrefix, err)
				}
			} else {
				// If currentPath is not under trimPathPrefix, this is likely a misconfiguration.
				// Fallback: make it relative to the parent of srcPath, so srcPath itself is a top-level entry.
				// Or one might choose to error out.
				// For robustness, let's use the base name if it's srcPath, or relative to srcPath's dir.
				if currentPath == srcPath {
					pathInTar = filepath.Base(srcPath)
				} else {
					// This fallback might not be ideal, depends on exact requirements.
					// It might be better to error if trimPathPrefix is given but not applicable.
					// For now, being lenient:
					pathInTar, err = filepath.Rel(filepath.Dir(srcPath), currentPath)
					if err != nil {
						return fmt.Errorf("failed to calculate relative path for %s from src parent %s (trim prefix mismatch): %w", currentPath, filepath.Dir(srcPath), err)
					}
				}
				// Optionally, log a warning here about trimPathPrefix not applying.
				// logger.Log.Warnf("trimPathPrefix '%s' does not apply to '%s', using path relative to source's parent.", trimPathPrefix, currentPath)
			}
		} else {
			// No trimPathPrefix: make paths relative to srcPath.
			// So, if srcPath is "/a/b/c", and currentPath is "/a/b/c/d/e.txt",
			// pathInTar should be "d/e.txt".
			// If currentPath is srcPath itself (e.g. "/a/b/c"), pathInTar should be "c" (its base name).
			if currentPath == srcPath {
				pathInTar = filepath.Base(srcPath)
			} else {
				pathInTar, err = filepath.Rel(srcPath, currentPath)
				if err != nil {
					return fmt.Errorf("failed to calculate relative path for %s from src %s: %w", currentPath, srcPath, err)
				}
			}
		}

		hdr.Name = filepath.ToSlash(pathInTar) // Use POSIX separators

		// If the source was a directory and its name in tar becomes "." (e.g. tarring contents of "dir" as ".")
		// and the user wants the directory itself as a top-level entry, hdr.Name should be filepath.Base(srcPath).
		// However, the current logic makes currentPath == srcPath result in Base(srcPath).
		// If pathInTar is "." (e.g. filepath.Rel("foo", "foo")), it means we're at the root of the relative calculation.
		// If srcPath is "a/b" and currentPath is "a/b" and trimPathPrefix is "a", pathInTar is "b".
		// If srcPath is "a/b" and currentPath is "a/b" and no trimPathPrefix, pathInTar is "b".
		// This seems correct.
		// Avoid empty names, though filepath.Base should prevent this for the root.
		if hdr.Name == "" {
			// This case should ideally not be reached if logic is correct,
			// but as a safeguard:
			if d.IsDir() && currentPath == srcPath { // if srcPath itself becomes an empty name dir
				hdr.Name = filepath.Base(srcPath) // use its own name
			} else if hdr.Name == "" { // Still empty? This is an issue.
				return fmt.Errorf("generated empty tar entry name for path: %s", currentPath)
			}
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header for %s (tar name: %s): %w", currentPath, hdr.Name, err)
		}

		// If it's a regular file, write its content
		if d.Type().IsRegular() {
			file, err := os.Open(currentPath)
			if err != nil {
				return fmt.Errorf("failed to open file %s for tarring: %w", currentPath, err)
			}
			// No need for defer here, as we close it explicitly in this block or an error occurs before next iteration.

			if _, err := io.Copy(tw, file); err != nil {
				file.Close() // Close explicitly before returning error from Copy
				return fmt.Errorf("failed to copy content of %s to tar archive: %w", currentPath, err)
			}
			file.Close() // Close successfully after copy
		}
		return nil
	})
}

// Untar extracts a gzipped tarball (srcTarball) to the destination directory (dstDir).
func Untar(srcTarball, dstDir string) error {
	fr, err := os.Open(srcTarball)
	if err != nil {
		return fmt.Errorf("failed to open source tarball %s: %w", srcTarball, err)
	}
	defer fr.Close()

	gr, err := gzip.NewReader(fr)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader for %s: %w", srcTarball, err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("error reading tar header from %s: %w", srcTarball, err)
		}
		if hdr == nil { // Should not happen if err is nil and not EOF
			continue
		}

		// Important: Sanitize hdr.Name to prevent path traversal attacks (zip slip)
		// filepath.Join will clean paths, but ensuring no ".." components escape dstDir is crucial.
		targetPath := filepath.Join(dstDir, hdr.Name)
		// Check if the cleaned targetPath is still within dstDir
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(dstDir)+string(os.PathSeparator)) && filepath.Clean(targetPath) != filepath.Clean(dstDir) {
			return fmt.Errorf("invalid tar entry path: %s (potential zip slip attack)", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			// Create directory if it doesn't exist.
			// Use hdr.Mode for permissions, but ensure it's a directory mode.
			// os.MkdirAll is generally safe and will use correct permissions for intermediate dirs.
			if err := os.MkdirAll(targetPath, fs.FileMode(hdr.Mode)|0700); err != nil { // Ensure execute for user at least
				// Or use common.FileMode0755 as a standard
				// if err := os.MkdirAll(targetPath, common.FileMode0755); err != nil {
				return fmt.Errorf("failed to create directory %s from tar: %w", targetPath, err)
			}
		case tar.TypeReg:
			// Create parent directory for the file
			parentDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(parentDir, common.FileMode0755); err != nil {
				return fmt.Errorf("failed to create parent directory %s for file from tar: %w", parentDir, err)
			}

			// Create the file
			// Use O_TRUNC to overwrite if file already exists, which is typical for untar.
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, fs.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s from tar: %w", targetPath, err)
			}
			// defer file.Close() // Close in loop for each file

			if _, err := io.Copy(file, tr); err != nil {
				file.Close() // Close explicitly before returning error
				return fmt.Errorf("failed to write content to file %s from tar: %w", targetPath, err)
			}
			file.Close() // Close successfully written file

		case tar.TypeSymlink:
			// Create parent directory for the symlink
			parentDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(parentDir, common.FileMode0755); err != nil {
				return fmt.Errorf("failed to create parent directory %s for symlink from tar: %w", parentDir, err)
			}
			// Check if symlink already exists and remove it to avoid error on os.Symlink if it's a different type
			if _, lstatErr := os.Lstat(targetPath); lstatErr == nil {
				if removeErr := os.Remove(targetPath); removeErr != nil {
					return fmt.Errorf("failed to remove existing file at symlink target %s: %w", targetPath, removeErr)
				}
			}

			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s from tar: %w", targetPath, hdr.Linkname, err)
			}

		default:
			// Handle other types if necessary (e.g., hard links, char/block devices)
			// For now, we can log or ignore them.
			// fmt.Printf("Unsupported tar entry type %c for %s\n", hdr.Typeflag, hdr.Name)
			// For KubeKey, regular files, directories, and symlinks are probably the most important.
		}
	}
	return nil
}
