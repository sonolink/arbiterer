const post = require("./api");

/**
 * Closes the pull request on the server, and records the close so a passing run can open it again. 
 * A close is skipped when the pull request is already closed, and an unrecorded close is warned about.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {string} options.reason Why the pull request is being closed.
 * @returns {Promise<void>}
 */
async function closePullRequest({ core, context, reason }) {
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

  if (res?.closed === false) {
    core.info(`Pull request #${pullNumber} is already closed, leaving it as is.`);
    return;
  }

  if (res?.recorded === false) {
    core.warning(
      `Closed pull request #${pullNumber} but could not record the close, so a passing run will not open it again.`,
      { title: "Record failed" },
    );
    return;
  }

  core.info(`Closed pull request #${pullNumber} because ${reason}.`);
}

/**
 * Opens the pull request on the server when a previous run closed it. The
 * server opens only when the app was the last to close it, it is still
 * closed, and it is not merged, so a maintainer's close is never overridden.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @returns {Promise<void>}
 */
async function openPullRequest({ core, context }) {
  const pullNumber = context.payload.pull_request?.number;

  if (!pullNumber) {
    core.info("Not opening: no pull request number in the event payload.");
    return;
  }

  let res;
  try {
    res = await post({ core, path: "pulls/open", body: { issue_number: pullNumber } });
  } catch (error) {
    core.warning(`Could not open pull request #${pullNumber}: ${error.message}`, { title: "Open failed" });
    return;
  }

  if (res?.opened) {
    core.info(`Opened pull request #${pullNumber} because the author is linked and the rules are satisfied.`);
    return;
  }

  const reason = res?.reason ? `: ${res.reason}` : "";
  core.info(`Not opening pull request #${pullNumber}${reason}.`);
}

module.exports = { closePullRequest, openPullRequest };
