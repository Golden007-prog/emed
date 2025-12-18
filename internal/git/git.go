// Package git provides git-related utilities for the editor.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BlameInfo contains parsed git blame information for a line.
type BlameInfo struct {
	Author    string
	Email     string
	Timestamp time.Time
	Summary   string
	Hash      string
}

// FindRepoRoot finds the git repository root for a given file path.
func FindRepoRoot(filePath string) string {
	dir := filepath.Dir(filePath)
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetBranch returns the current branch name for a given file's repository.
func GetBranch(filePath string) string {
	dir := filepath.Dir(filePath)

	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return ""
	}

	cmd = exec.Command("git", "-C", dir, "branch", "--show-current")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output))
	}
	return "detached"
}

// GetBlameForLine returns blame information for a specific line in a file.
func GetBlameForLine(filePath string, lineNum int) (*BlameInfo, error) {
	dir := filepath.Dir(filePath)

	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("not a git repository")
	}

	lineArg := fmt.Sprintf("-L%d,%d", lineNum, lineNum)
	cmd = exec.Command("git", "-C", dir, "blame", "--porcelain", lineArg, filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git blame failed: %v", err)
	}

	return parseBlameOutput(string(output))
}

func parseBlameOutput(output string) (*BlameInfo, error) {
	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid blame output")
	}

	info := &BlameInfo{}

	parts := strings.Fields(lines[0])
	if len(parts) >= 1 {
		info.Hash = parts[0]
	}

	if info.Hash == "0000000000000000000000000000000000000000" {
		return nil, fmt.Errorf("uncommitted changes")
	}

	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "author ") {
			info.Author = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-mail ") {
			info.Email = strings.Trim(strings.TrimPrefix(line, "author-mail "), "<>")
		} else if strings.HasPrefix(line, "author-time ") {
			timestampStr := strings.TrimPrefix(line, "author-time ")
			if ts, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
				info.Timestamp = time.Unix(ts, 0)
			}
		} else if strings.HasPrefix(line, "summary ") {
			info.Summary = strings.TrimPrefix(line, "summary ")
		}
	}

	return info, nil
}

// FormatBlameInfo formats blame information for inline display.
func FormatBlameInfo(info *BlameInfo) string {
	if info == nil {
		return ""
	}

	age := formatTimeAgo(info.Timestamp)

	author := info.Author
	if len(author) > 15 {
		author = author[:12] + "..."
	}

	summary := info.Summary
	if len(summary) > 40 {
		summary = summary[:37] + "..."
	}

	return fmt.Sprintf("   %s, %s: %s", author, age, summary)
}

// FormatDetailedBlameInfo formats blame information for modal display.
func FormatDetailedBlameInfo(info *BlameInfo, filePath string, lineNum int) string {
	if info == nil {
		return ""
	}

	shortHash := info.Hash
	if len(shortHash) > 8 {
		shortHash = shortHash[:8]
	}

	dateStr := info.Timestamp.Format("2006-01-02 15:04:05")
	age := formatTimeAgo(info.Timestamp)

	return fmt.Sprintf(`[yellow]Commit:[white]  %s

[yellow]Author:[white]  %s <%s>

[yellow]Date:[white]    %s (%s)

[yellow]Message:[-]
%s

[gray]File: %s, Line: %d[-]`,
		shortHash,
		info.Author, info.Email,
		dateStr, age,
		info.Summary,
		filePath, lineNum)
}

func formatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}
