/*
Copyright 2026-present SonoLink.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/storage"
)

const (
	githubEventHeader     = "X-GitHub-Event"
	githubSignatureHeader = "X-Hub-Signature-256"
	issueCommentEvent     = "issue_comment"
	actionCreated         = "created"

	// marketplacePurchaseEvent is sent by the Marketplace listing's webhook when
	// an account buys, changes or cancels a plan.
	// https://docs.github.com/en/apps/github-marketplace/using-the-github-marketplace-api-in-your-app/webhook-events-for-the-github-marketplace-api
	marketplacePurchaseEvent = "marketplace_purchase"

	commandPrefix  = "/arbiterer"
	recheckCommand = "recheck"

	// maxWebhookRequestBytes is 5 MiB because issue_comment payloads include
	// the full issue and comment bodies; GitHub caps deliveries at 25 MB.
	maxWebhookRequestBytes = 5 << 20
	recheckCooldown        = 5 * time.Minute
)

// webhookUser is the part of a GitHub user in a webhook payload the command
// handler needs.
type webhookUser struct {
	ID int64 `json:"id"`
}

// issueCommentPayload is the part of GitHub's issue_comment webhook payload the
// command handler reads.
type issueCommentPayload struct {
	Action string `json:"action"`
	Issue  struct {
		Number      int64        `json:"number"`
		User        *webhookUser `json:"user"`
		State       string       `json:"state"`
		ClosedAt    *time.Time   `json:"closed_at"`
		PullRequest *struct {
			MergedAt *time.Time `json:"merged_at"`
		} `json:"pull_request"`
	} `json:"issue"`
	Comment struct {
		ID   int64        `json:"id"`
		Body string       `json:"body"`
		User *webhookUser `json:"user"`
	} `json:"comment"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// Marketplace purchase actions, spelled as GitHub sends them.
const (
	marketplacePurchased             = "purchased"
	marketplaceChanged               = "changed"
	marketplacePendingChange         = "pending_change"
	marketplacePendingChangeCanceled = "pending_change_cancelled" //nolint:misspell // GitHub's spelling.
	marketplaceCanceled              = "cancelled"                //nolint:misspell // GitHub's spelling.
)

const (
	componentType     = "type"
	componentChildren = "components"
	componentContent  = "content"
)

// marketplacePurchase is the part of a Marketplace purchase in a
// marketplace_purchase payload worth logging.
type marketplacePurchase struct {
	Account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
	OnFreeTrial bool `json:"on_free_trial"`
	Plan        struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"plan"`
}

// marketplacePurchasePayload is the part of GitHub's marketplace_purchase
// webhook payload the handler reads.
type marketplacePurchasePayload struct {
	Action                      string               `json:"action"`
	EffectiveDate               time.Time            `json:"effective_date"`
	MarketplacePurchase         marketplacePurchase  `json:"marketplace_purchase"`
	PreviousMarketplacePurchase *marketplacePurchase `json:"previous_marketplace_purchase"`
}

// handleGitHubWebhook answers POST /v1/github/webhooks: it handles commands
// from pull request comments, such as /arbiterer recheck, and records
// Marketplace plan changes.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, ok := s.readBody(w, r, maxWebhookRequestBytes)
	if !ok {
		return
	}

	if !s.githubClient.VerifyWebhook(body, r.Header.Get(githubSignatureHeader)) {
		s.logger.Warn("rejecting webhook with invalid signature")
		s.writeProblem(w, r, http.StatusUnauthorized, "invalid signature")

		return
	}

	switch r.Header.Get(githubEventHeader) {
	case issueCommentEvent:
		var payload issueCommentPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			s.writeProblem(w, r, http.StatusBadRequest, "invalid webhook payload")

			return
		}

		s.handleIssueComment(r.Context(), payload)
	case marketplacePurchaseEvent:
		var payload marketplacePurchasePayload
		if err := json.Unmarshal(body, &payload); err != nil {
			s.writeProblem(w, r, http.StatusBadRequest, "invalid webhook payload")

			return
		}

		s.handleMarketplacePurchase(r.Context(), payload)
	}

	s.writeJSON(w, http.StatusOK, struct{}{})
}

// handleMarketplacePurchase logs a Marketplace plan change and announces it in
// the Discord channel.
func (s *Server) handleMarketplacePurchase(ctx context.Context, payload marketplacePurchasePayload) {
	purchase := payload.MarketplacePurchase
	login := purchase.Account.Login
	profile := "https://github.com/" + login

	s.logger.Info(
		"marketplace purchase",
		"action", payload.Action,
		"account", login,
		"account_id", purchase.Account.ID,
		"plan", purchase.Plan.Name,
		"plan_id", purchase.Plan.ID,
		"effective_date", payload.EffectiveDate,
	)

	title, color := "Marketplace: "+payload.Action, 0x0969da
	switch payload.Action {
	case marketplacePurchased:
		title, color = "New Marketplace install", 0x2da44e
	case marketplaceChanged:
		title = "Marketplace plan changed"
	case marketplacePendingChange:
		title, color = "Marketplace plan change scheduled", 0xd4a72c
	case marketplacePendingChangeCanceled:
		title, color = "Scheduled Marketplace change cancelled", 0xd4a72c
	case marketplaceCanceled:
		title, color = "Marketplace plan cancelled", 0xcf222e
	}

	plan := purchase.Plan.Name
	if previous := payload.PreviousMarketplacePurchase; previous != nil && previous.Plan.Name != plan {
		plan = previous.Plan.Name + " → " + plan
	}

	if purchase.OnFreeTrial {
		plan += " (free trial)"
	}

	details := fmt.Sprintf(
		"**Plan:** %s\n**Account type:** %s\n**Effective:** <t:%d:F>",
		plan, purchase.Account.Type, payload.EffectiveDate.Unix(),
	)

	container, err := json.Marshal(map[string]any{
		componentType:  17,
		"accent_color": color,
		componentChildren: []map[string]any{
			{
				componentType: 9,
				componentChildren: []map[string]any{
					{componentType: 10, componentContent: "### " + title},
					{componentType: 10, componentContent: "[" + login + "](" + profile + ")"},
				},
				"accessory": map[string]any{
					componentType: 11,
					"media":       map[string]any{"url": fmt.Sprintf("https://avatars.githubusercontent.com/u/%d?v=4", purchase.Account.ID)},
					"description": login + "'s avatar",
				},
			},
			{componentType: 14, "divider": true, "spacing": 1},
			{componentType: 10, componentContent: details},
			{
				componentType: 1,
				componentChildren: []map[string]any{
					{componentType: 2, "style": 5, "label": "View on GitHub", "url": profile},
				},
			},
		},
	})
	if err == nil {
		err = s.discordClient.SendWebhook(ctx, discord.WebhookPayload{
			Components: []json.RawMessage{container},
			Flags:      discord.FlagIsComponentsV2,
		})
	}

	if err != nil {
		s.logger.Error("sending marketplace notification to discord", "error", err, "action", payload.Action)
	}
}

// handleIssueComment runs a command from a pull request comment when its author
// posted it. Anything else is ignored.
func (s *Server) handleIssueComment(ctx context.Context, payload issueCommentPayload) {
	if payload.Action != actionCreated || payload.Issue.PullRequest == nil {
		return
	}

	if payload.Issue.User == nil || payload.Comment.User == nil ||
		payload.Issue.User.ID != payload.Comment.User.ID {
		return
	}

	if payload.Issue.PullRequest.MergedAt != nil || !isRecheckCommand(payload.Comment.Body) {
		return
	}

	repo := payload.Repository.FullName

	if payload.Issue.State == "closed" {
		closedByApp, err := s.closedByApp(
			ctx,
			payload.Repository.ID,
			payload.Issue.Number,
			payload.Issue.ClosedAt,
		)
		if err != nil {
			s.logger.Error(
				"checking whether the app closed the pull request",
				"error", err,
				"repository", repo,
				"issue_number", payload.Issue.Number,
			)

			return
		}

		if !closedByApp {
			return
		}
	}

	s.recheckPullRequest(ctx, payload, repo)
}

// recheckPullRequest reruns the workflow run stored for the pull request. It
// reacts with +1 when the rerun starts and confused when it cannot.
func (s *Server) recheckPullRequest(
	ctx context.Context,
	payload issueCommentPayload,
	repo string,
) {
	token, err := s.githubClient.CreateInstallationToken(ctx, payload.Repository.ID, repo)
	if err != nil {
		s.logger.Warn(
			"skipping recheck: cannot create installation token",
			"error", err,
			"repository", repo,
		)

		return
	}

	runID, err := s.store.TakePullRequestRun(
		ctx,
		payload.Repository.ID,
		payload.Issue.Number,
		recheckCooldown,
	)
	if errors.Is(err, storage.ErrCooldown) {
		s.reactToCommand(ctx, token, payload, "confused")
		s.logger.Info(
			"skipping recheck: cooldown active",
			"repository", repo,
			"issue_number", payload.Issue.Number,
		)

		return
	}

	if errors.Is(err, storage.ErrNotFound) {
		s.reactToCommand(ctx, token, payload, "confused")
		s.logger.Info(
			"skipping recheck: no workflow run available",
			"repository", repo,
			"issue_number", payload.Issue.Number,
		)

		return
	}

	if err != nil {
		s.reactToCommand(ctx, token, payload, "confused")
		s.logger.Error(
			"taking pull request run for recheck",
			"error", err,
			"repository", repo,
			"issue_number", payload.Issue.Number,
		)

		return
	}

	if err := s.githubClient.RerunWorkflowRun(ctx, token, repo, runID); err != nil {
		if err := s.store.ReleasePullRequestRun(
			ctx,
			payload.Repository.ID,
			payload.Issue.Number,
		); err != nil {
			s.logger.Error(
				"releasing pull request run cooldown",
				"error", err,
				"repository", repo,
				"issue_number", payload.Issue.Number,
			)
		}

		s.reactToCommand(ctx, token, payload, "confused")
		s.logger.Warn(
			"rerunning workflow run after recheck",
			"error", err,
			"repository", repo,
			"run_id", runID,
		)

		return
	}

	s.reactToCommand(ctx, token, payload, "+1")

	s.logger.Info(
		"rerunning workflow run after recheck",
		"repository", repo,
		"issue_number", payload.Issue.Number,
		"run_id", runID,
	)
}

// closedByApp reports whether the app closed the pull request at closedAt.
func (s *Server) closedByApp(
	ctx context.Context,
	repositoryID int64,
	issueNumber int64,
	closedAt *time.Time,
) (bool, error) {
	record, err := s.store.AutoClosedPullRequest(ctx, repositoryID, issueNumber)
	if errors.Is(err, storage.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return closedAt != nil &&
		closedAt.Truncate(time.Second).Equal(record.ClosedAt.Truncate(time.Second)), nil
}

// reactToCommand adds a reaction to the command comment.
func (s *Server) reactToCommand(
	ctx context.Context,
	token string,
	payload issueCommentPayload,
	content string,
) {
	if err := s.githubClient.CreateCommentReaction(
		ctx,
		token,
		payload.Repository.FullName,
		payload.Comment.ID,
		content,
	); err != nil {
		s.logger.Warn(
			"reacting to command comment",
			"error", err,
			"repository", payload.Repository.FullName,
			"comment_id", payload.Comment.ID,
		)
	}
}

// isRecheckCommand reports whether body carries /arbiterer recheck.
func isRecheckCommand(body string) bool {
	fields := strings.Fields(body)
	if len(fields) < 2 || !strings.EqualFold(fields[0], commandPrefix) {
		return false
	}

	return strings.EqualFold(fields[1], recheckCommand)
}
