// Package update looks up the latest GitHub release for this build.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Release is a newer GitHub release, when one has been seen.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// Checker polls the GitHub releases API. A build with no repo does nothing.
type Checker struct {
	repo    string
	current string
	client  *http.Client

	mu      sync.RWMutex
	release *Release
}

// New returns a checker. repo is owner/name. current is this binary's version.
func New(repo, current string) *Checker {
	if !repoPattern.MatchString(repo) {
		repo = ""
	}
	return &Checker{
		repo:    repo,
		current: current,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Loop checks immediately and then every 6 hours until ctx is cancelled.
func (c *Checker) Loop(ctx context.Context) {
	if c.repo == "" {
		return
	}
	c.check()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.check()
		}
	}
}

// Available returns the latest release when it is newer than this build.
func (c *Checker) Available(newer func(latest, current string) bool) *Release {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.release == nil || !newer(c.release.Version, c.current) {
		return nil
	}
	copy := *c.release
	return &copy
}

func (c *Checker) check() {
	rel, err := fetchLatest(c.client, c.repo)
	if err != nil {
		log.Printf("update check: %v", err)
		return
	}
	c.mu.Lock()
	c.release = rel
	c.mu.Unlock()
}

func fetchLatest(client *http.Client, repo string) (*Release, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "nut-allergy")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases: %s", res.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.TagName == "" {
		return nil, fmt.Errorf("github release has no tag")
	}
	return &Release{Version: payload.TagName, URL: payload.HTMLURL}, nil
}
