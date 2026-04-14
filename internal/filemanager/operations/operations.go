// Package operations provides file and directory operations with safety checks.
package operations

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/laborin/emed/internal/core/pathutil"
)

// FileOperations provides safe file and directory operations.
type FileOperations struct {
	workingDir string
}

// New creates a new FileOperations instance with the given working directory.
func New(workingDir string) *FileOperations {
	return &FileOperations{
		workingDir: workingDir,
	}
}

// CreateFile creates a new file in the specified directory.
func (f *FileOperations) CreateFile(dirPath, name string) error {
	if name == "" {
		return fmt.Errorf("file name cannot be empty")
	}

	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("file name cannot contain path separators")
	}

	fullPath := filepath.Join(dirPath, name)

	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", name)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	return nil
}

// CreateDirectory creates a new directory in the specified path.
func (f *FileOperations) CreateDirectory(dirPath, name string) error {
	if name == "" {
		return fmt.Errorf("directory name cannot be empty")
	}

	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("directory name cannot contain path separators")
	}

	fullPath := filepath.Join(dirPath, name)

	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("directory already exists: %s", name)
	}

	err := os.Mkdir(fullPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	return nil
}

// DeletePath deletes a file or directory at the given path.
func (f *FileOperations) DeletePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	workDir, err := filepath.Abs(f.workingDir)
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	if !pathutil.IsWithin(workDir, absPath) {
		return fmt.Errorf("cannot delete files outside working directory")
	}

	if absPath == workDir {
		return fmt.Errorf("cannot delete working directory")
	}

	return os.RemoveAll(absPath)
}

// RenamePath renames a file or directory to a new name.
func (f *FileOperations) RenamePath(oldPath, newName string) error {
	if newName == "" {
		return fmt.Errorf("new name cannot be empty")
	}

	if strings.Contains(newName, "/") || strings.Contains(newName, "\\") {
		return fmt.Errorf("new name cannot contain path separators")
	}

	dir := filepath.Dir(oldPath)
	newPath := filepath.Join(dir, newName)

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("destination already exists: %s", newName)
	}

	return os.Rename(oldPath, newPath)
}

// CopyPath copies a file or directory from source to destination.
func (f *FileOperations) CopyPath(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return f.copyDirectory(src, dst)
	}

	return f.copyFile(src, dst)
}

// MovePath moves a file or directory from source to destination.
func (f *FileOperations) MovePath(src, dst string) error {
	absPath, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	workDir, err := filepath.Abs(f.workingDir)
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	if !pathutil.IsWithin(workDir, absPath) {
		return fmt.Errorf("cannot move files outside working directory")
	}

	absDst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("failed to get destination absolute path: %v", err)
	}
	if !pathutil.IsWithin(workDir, absDst) {
		return fmt.Errorf("cannot move files outside working directory")
	}

	return os.Rename(absPath, absDst)
}

func (f *FileOperations) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}

func (f *FileOperations) copyDirectory(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = f.copyDirectory(srcPath, dstPath)
		} else {
			err = f.copyFile(srcPath, dstPath)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// ValidatePath checks if a path is within the working directory.
func (f *FileOperations) ValidatePath(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %v", err)
	}

	workDir, err := filepath.Abs(f.workingDir)
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	if !pathutil.IsWithin(workDir, absPath) {
		return fmt.Errorf("path is outside working directory")
	}

	return nil
}
