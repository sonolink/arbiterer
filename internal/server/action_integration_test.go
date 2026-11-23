//go:build integration

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
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/sonolink/arbiterer/internal/storage"
)

// linkForTest connects a GitHub user to a new Discord account in each of the
// repositories, and returns the Discord account.
func linkForTest(t *testing.T, githubUserID string, repositoryIDs ...int64) storage.DiscordUser {
	t.Helper()

	user := storage.DiscordUser{
		ID:                    nextID(),
		EncryptedAccessToken:  []byte("access"),
		EncryptedRefreshToken: []byte("refresh"),
		TokenExpiresAt:        time.Now().Add(time.Hour),
	}

	for _, repositoryID := range repositoryIDs {
		if err := testStore.LinkGitHubDiscord(t.Context(), githubUserID, repositoryID, &user); err != nil {
			t.Fatalf("LinkGitHubDiscord: %v", err)
		}
	}

	return user
}

// connected reports whether the GitHub user is linked in the repository.
func connected(t *testing.T, githubUserID string, repositoryID int64) bool {
	t.Helper()

	_, err := testStore.DiscordUserByConnection(t.Context(), githubUserID, repositoryID)
	if errors.Is(err, storage.ErrNotFound) {
		return false
	}

	if err != nil {
		t.Fatalf("DiscordUserByConnection: %v", err)
	}

	return true
}

// TestUnlinkGitHubUserKeepsOtherRepositories checks that Unlink only removes
// the link in the pull request's repository, and only hands back the Discord
// account once nothing else uses it, so its token is not revoked too early.
func TestUnlinkGitHubUserKeepsOtherRepositories(t *testing.T) {
	t.Parallel()

	githubUserID := strconv.FormatInt(nextID(), 10)
	first, second := nextID(), nextID()
	user := linkForTest(t, githubUserID, first, second)

	removed, err := testStore.UnlinkGitHubUser(t.Context(), githubUserID, first)
	if err != nil {
		t.Fatalf("UnlinkGitHubUser: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("UnlinkGitHubUser returned %d accounts still linked elsewhere, want none", len(removed))
	}

	if connected(t, githubUserID, first) {
		t.Error("still linked in the unlinked repository")
	}

	if !connected(t, githubUserID, second) {
		t.Error("the link in the other repository was removed")
	}

	removed, err = testStore.UnlinkGitHubUser(t.Context(), githubUserID, second)
	if err != nil {
		t.Fatalf("UnlinkGitHubUser: %v", err)
	}

	if len(removed) != 1 || removed[0].ID != user.ID || string(removed[0].EncryptedAccessToken) != "access" {
		t.Fatalf("UnlinkGitHubUser = %+v, want the account %d with its token", removed, user.ID)
	}

	if _, err := testStore.DiscordUserByID(t.Context(), user.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("DiscordUserByID error = %v, want ErrNotFound once the last link is gone", err)
	}
}

// TestDeleteGitHubUserRemovesEveryLink checks that Delete my data removes the
// author's links in every repository and leaves other authors alone.
func TestDeleteGitHubUserRemovesEveryLink(t *testing.T) {
	t.Parallel()

	githubUserID := strconv.FormatInt(nextID(), 10)
	otherUserID := strconv.FormatInt(nextID(), 10)
	first, second := nextID(), nextID()

	user := linkForTest(t, githubUserID, first, second)
	other := linkForTest(t, otherUserID, first)

	removed, err := testStore.DeleteGitHubUser(t.Context(), githubUserID)
	if err != nil {
		t.Fatalf("DeleteGitHubUser: %v", err)
	}

	if len(removed) != 1 || removed[0].ID != user.ID {
		t.Fatalf("DeleteGitHubUser = %+v, want only the account %d", removed, user.ID)
	}

	for _, repositoryID := range []int64{first, second} {
		if connected(t, githubUserID, repositoryID) {
			t.Errorf("still linked in repository %d", repositoryID)
		}
	}

	if !connected(t, otherUserID, first) {
		t.Error("another author's link was removed")
	}

	if _, err := testStore.DiscordUserByID(t.Context(), other.ID); err != nil {
		t.Errorf("another author's account was removed: %v", err)
	}
}

// TestDeleteGitHubUserWithoutLinks checks that deleting an author with nothing
// stored is not an error, since the link in the comment can be followed twice.
func TestDeleteGitHubUserWithoutLinks(t *testing.T) {
	t.Parallel()

	removed, err := testStore.DeleteGitHubUser(t.Context(), strconv.FormatInt(nextID(), 10))
	if err != nil {
		t.Fatalf("DeleteGitHubUser: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("DeleteGitHubUser = %+v, want nothing", removed)
	}
}

// TestUpsertPullRequestRunCooldown checks that a rerun of the same workflow
// run keeps the recheck cooldown, while a new run, such as one from a new
// push, clears it.
func TestUpsertPullRequestRunCooldown(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	repositoryID, issueNumber := nextID(), nextID()
	firstRun, secondRun := nextID(), nextID()

	if err := testStore.UpsertPullRequestRun(ctx, repositoryID, issueNumber, firstRun); err != nil {
		t.Fatalf("UpsertPullRequestRun: %v", err)
	}

	if _, err := testStore.TakePullRequestRun(ctx, repositoryID, issueNumber, recheckCooldown); err != nil {
		t.Fatalf("TakePullRequestRun: %v", err)
	}

	// The rerun reports back with the same run id.
	if err := testStore.UpsertPullRequestRun(ctx, repositoryID, issueNumber, firstRun); err != nil {
		t.Fatalf("UpsertPullRequestRun: %v", err)
	}

	if _, err := testStore.TakePullRequestRun(ctx, repositoryID, issueNumber, recheckCooldown); !errors.Is(err, storage.ErrCooldown) {
		t.Errorf("TakePullRequestRun after a rerun error = %v, want ErrCooldown", err)
	}

	if err := testStore.UpsertPullRequestRun(ctx, repositoryID, issueNumber, secondRun); err != nil {
		t.Fatalf("UpsertPullRequestRun: %v", err)
	}

	runID, err := testStore.TakePullRequestRun(ctx, repositoryID, issueNumber, recheckCooldown)
	if err != nil {
		t.Fatalf("TakePullRequestRun after a new run: %v", err)
	}

	if runID != secondRun {
		t.Errorf("TakePullRequestRun = %d, want the new run %d", runID, secondRun)
	}
}

// TestSetupCommentStoresActionStatuses checks the migration that adds the
// statuses the action links write.
func TestSetupCommentStoresActionStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []commentStatus{commentRechecking, commentDeleted} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			comment := &storage.SetupComment{
				RepositoryID: nextID(),
				IssueNumber:  nextID(),
				CommentID:    commentID,
				Status:       string(status),
			}

			if err := testStore.UpsertSetupComment(t.Context(), comment); err != nil {
				t.Fatalf("UpsertSetupComment: %v", err)
			}

			stored, err := testStore.SetupCommentByPullRequest(t.Context(), comment.RepositoryID, comment.IssueNumber)
			if err != nil {
				t.Fatalf("SetupCommentByPullRequest: %v", err)
			}

			if stored.Status != string(status) {
				t.Errorf("stored status = %q, want %q", stored.Status, status)
			}
		})
	}
}

// TestTakePullRequestRunWithoutCooldownStartsOne checks that the rerun after
// linking, which ignores the cooldown, still starts it. The comment offers
// Recheck while that rerun finishes.
func TestTakePullRequestRunWithoutCooldownStartsOne(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	repositoryID, issueNumber, runID := nextID(), nextID(), nextID()

	if err := testStore.UpsertPullRequestRun(ctx, repositoryID, issueNumber, runID); err != nil {
		t.Fatalf("UpsertPullRequestRun: %v", err)
	}

	if _, err := testStore.TakePullRequestRun(ctx, repositoryID, issueNumber, 0); err != nil {
		t.Fatalf("TakePullRequestRun without a cooldown: %v", err)
	}

	if _, err := testStore.TakePullRequestRun(ctx, repositoryID, issueNumber, recheckCooldown); !errors.Is(err, storage.ErrCooldown) {
		t.Errorf("TakePullRequestRun error = %v, want ErrCooldown", err)
	}
}
