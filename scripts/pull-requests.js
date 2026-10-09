const post = require("./api");
const closerLogin = process.env.ARBITERER_CLOSER_LOGIN?.trim() || "github-actions[bot]";

/**
 * Closes the pull request and records the close on the server so a passing run
 * can reopen it. Cleanup failure never masks the check itself.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @param {string} options.reason Why the pull request is being closed.
 * @returns {Promise<void>}
 */
async function closePullRequest({ core, context, github, reason }) {
  const pullNumber = context.payload.pull_request?.number;

  if (!pullNumber) {
    core.info("Not closing: no pull request number in the event payload.");
    return;
  }

  let res;
  try {
    res = await post({ core, path: "pulls/close", body: { issue_number: pullNumber } });
  } catch (error) {
    core.warning(`Could not close pull request #${pullNumber}: ${error.message}`, { title: "Close failed" });
    return;
  }

  core.info(`Closed pull request #${pullNumber} because ${reason}.`);
  await recordClosed({ core, pullNumber, closed: res?.closed ?? true });
}

/**
 * Reopens the pull request when the action closed it and it now passes. The
 * server record says the action may have closed it; the timeline confirms it
 * was the last to close it. Cleanup failure never masks the check.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @returns {Promise<void>}
 */
async function reopenPullRequest({ core, context, github }) {
  const pullNumber = context.payload.pull_request?.number;

  if (!pullNumber) {
    core.info("Not reopening: no pull request number in the event payload.");
    return;
  }

  const owner = context.repo.owner;
  const repo = context.repo.repo;

  let pull;
  try {
    ({ data: pull } = await github.rest.pulls.get({ owner, repo, pull_number: pullNumber }));
  } catch (error) {
    core.warning(`Could not read pull request #${pullNumber}: ${error.message}`, { title: "Reopen failed" });
    return;
  }

  if (pull.merged) {
    core.info(`Not reopening pull request #${pullNumber}: it is merged.`);
    return;
  }

  if (pull.state === "open") {
    core.info(`Not reopening pull request #${pullNumber}: it is already open.`);
    await recordClosed({ core, pullNumber, closed: false });
    return;
  }

  if (!(await lastClosedByArbiterer({ core, github, owner, repo, pullNumber }))) {
    core.info(`Not reopening pull request #${pullNumber}: the action was not the last to close it.`);
    return;
  }

  try {
    await github.rest.pulls.update({ owner, repo, pull_number: pullNumber, state: "open" });
  } catch (error) {
    core.warning(`Could not reopen pull request #${pullNumber}: ${error.message}`, { title: "Reopen failed" });
    return;
  }

  await recordClosed({ core, pullNumber, closed: false });
  core.info(`Reopened pull request #${pullNumber} because the author is linked and the rules are satisfied.`);
}

// lastClosedByArbiterer reports whether the most recent close of the pull
// request was made by the closer login. Any ambiguity means no.
async function lastClosedByArbiterer({ core, github, owner, repo, pullNumber }) {
  let events;
  try {
    events = await github.paginate(github.rest.issues.listEventsForTimeline, {
      owner,
      repo,
      issue_number: pullNumber,
      per_page: 100,
    });
  } catch (error) {
    core.warning(`Could not determine who closed pull request #${pullNumber}: ${error.message}`, { title: "Reopen skipped" });
    return false;
  }

  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].event === "closed") {
      return events[i].actor?.login === closerLogin;
    }
  }

  return false;
}

// recordClosed tells the server whether the action closed the pull request.
// Failure is cleanup, never the check itself.
async function recordClosed({ core, pullNumber, closed }) {
  try {
    await post({ core, path: "pulls/closed", body: { issue_number: pullNumber, closed } });
  } catch (error) {
    core.warning(`Could not record pull request #${pullNumber} as ${closed ? "closed by the action" : "reopened"} on the server: ${error.message}`, {
      title: "State not recorded",
    });
  }
}

module.exports = { closePullRequest, reopenPullRequest };
