package github

import (
	"context"
	"fmt"
	"net/http"
)

// RerunWorkflow asks GitHub to re-run the given workflow run.
func (c *Client) RerunWorkflow(ctx context.Context, token string, repo string, runID int64) error {
	if err := c.sendRequest(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/repos/%s/actions/runs/%d/rerun", repo, runID),
		token,
		map[string]any{},
		nil,
	); err != nil {
		return fmt.Errorf("github: rerunning workflow run %d: %w", runID, err)
	}

	return nil
}
