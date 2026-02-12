package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// blockedByGraphQLResponse represents the GraphQL response for blockedBy relationships.
type blockedByGraphQLResponse struct {
	Data struct {
		Repository struct {
			Issue struct {
				BlockedBy struct {
					Nodes []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
					} `json:"nodes"`
				} `json:"blockedBy"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

// fetchBlockingIssues queries the GitHub GraphQL API for issues that block the given issue.
// Returns a slice of open blocking issue number strings, or an error if the API call fails.
func (c *Controller) fetchBlockingIssues(ctx context.Context, issueID string) ([]string, error) {
	owner, name, err := parseRepoOwnerName(c.config.Repository)
	if err != nil {
		return nil, fmt.Errorf("cannot parse repository: %w", err)
	}

	issueNum, err := strconv.Atoi(issueID)
	if err != nil {
		return nil, fmt.Errorf("invalid issue number %q: %w", issueID, err)
	}

	query := fmt.Sprintf(`{ repository(owner: %q, name: %q) { issue(number: %d) { blockedBy(first: 50) { nodes { number state } } } } }`,
		owner, name, issueNum)

	cmd := c.execCommand(ctx, "gh", "api", "graphql", "-f", "query="+query)
	cmd.Env = c.envWithGitHubToken()

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %w", err)
	}

	var resp blockedByGraphQLResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	var ids []string
	for _, node := range resp.Data.Repository.Issue.BlockedBy.Nodes {
		if strings.EqualFold(node.State, "OPEN") {
			ids = append(ids, strconv.Itoa(node.Number))
		}
	}
	return ids, nil
}

// detectBlockingIssues queries the GitHub blockedBy API with caching and exponential
// backoff retry (1s, 2s, 4s, 8s, 16s). Returns an error after 6 failed attempts.
func (c *Controller) detectBlockingIssues(ctx context.Context, issueID string) ([]string, error) {
	if cached, ok := c.blockedByCache[issueID]; ok {
		return cached, nil
	}

	var ids []string
	var lastErr error
	delays := []time.Duration{0, 1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for attempt, delay := range delays {
		if attempt > 0 {
			c.logWarning("BlockedBy API failed for #%s (attempt %d/6), retrying in %s: %v",
				issueID, attempt, delay, lastErr)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ids, lastErr = c.fetchBlockingIssues(ctx, issueID)
		if lastErr == nil {
			c.blockedByCache[issueID] = ids
			return ids, nil
		}
	}
	return nil, fmt.Errorf("blockedBy API failed for #%s after 6 attempts: %w", issueID, lastErr)
}
