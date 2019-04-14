package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type changeMode byte

var validChangeTypes = "ACDMRTUXB"

const (
	modeAdded       changeMode = 'A'
	modeCopied      changeMode = 'C'
	modeDeleted     changeMode = 'D'
	modeModified    changeMode = 'M'
	modeRenamed     changeMode = 'R'
	modeTypeChanged changeMode = 'T'
	modeUnmerged    changeMode = 'U'
	modeUnknown     changeMode = 'X'
	modeBroken      changeMode = 'B'
)

func (t changeMode) String() string {
	switch t {
	case modeAdded:
		return "added"
	case modeCopied:
		return "copied"
	case modeDeleted:
		return "deleted"
	case modeModified:
		return "modified"
	case modeRenamed:
		return "renamed"
	case modeUnknown:
		return "unknown"
	case modeBroken:
		return "broken"
	default:
		return fmt.Sprintf("%s-invalid", string(t))
	}
}

type fileStat struct {
	mode    changeMode
	path    string
	oldPath string // for renames
}

func gitStats(ref string) ([]*fileStat, error) {
	var buf strings.Builder
	cmd := exec.Command("git", "diff", "--numstat", "--name-status", ref)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running git: %s", err)
	}
	lines := strings.Split(buf.String(), "\n")
	changes := make([]*fileStat, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		chops := strings.SplitN(line, "\t", 3)
		if len(chops) < 2 || len(chops[0]) == 0 {
			return nil, fmt.Errorf("unexpected git output: %q", line)
		}
		mod := chops[0][0]
		if strings.IndexByte(validChangeTypes, mod) == -1 {
			return nil, fmt.Errorf("invalid change type: %q", line)
		}
		change := &fileStat{
			mode:    changeMode(mod),
			path:    chops[1],
			oldPath: chops[1],
		}
		if changeMode(mod) == modeRenamed {
			if len(chops) != 3 {
				return nil, fmt.Errorf("unexpected git output: %q", line)
			}
			change.path = chops[2]
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func gitBlob(ref, path string) (io.Reader, error) {
	var buf bytes.Buffer
	cmd := exec.Command("git", "cat-file", "blob", ref+":"+path)
	cmd.Stdout = &buf
	return &buf, cmd.Run()
}

// gitLsTreeGoBlobs lists all .go files at the ref:path
func gitLsTreeGoBlobs(ref, path string) ([]string, error) {
	var buf, errbuf strings.Builder
	cmd := exec.Command("git", "ls-tree", ref+":"+path)
	cmd.Stdout = &buf
	cmd.Stderr = &errbuf
	if err := cmd.Run(); err != nil {
		if strings.Contains(errbuf.String(), "Not a valid object name") {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("git ls-tree %s:%s: %s", ref, path, err)
	}
	lines := strings.Split(buf.String(), "\n")
	blobs := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		chops := strings.Fields(line)
		if n := len(chops); n != 4 {
			return nil, fmt.Errorf("ls-tree: unexpected line: %s", line)
		}
		if chops[1] != "blob" {
			continue
		}
		if filepath.Ext(chops[3]) != ".go" {
			continue
		}
		if strings.HasSuffix(chops[3], "_test.go") {
			continue
		}
		blobs = append(blobs, chops[3])
	}
	return blobs, nil
}