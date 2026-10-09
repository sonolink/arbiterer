/**
 * Closes the pull request. Failing to close is cleanup, not the check itself,
 * so it must not mask the failure that already set the exit code.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @param {string} options.reason Why the pull request is being closed.
 * @returns {Promise<void>}
 */
module.exports = async function closePullRequest({ core, context, github, reason }) {
  const pullNumber = context.payload.pull_request?.number;

  if (!pullNumber) {
    core.info("Not closing: no pull request number in the event payload.");
    return;
  }

  try {
    await github.rest.pulls.update({
      owner: context.repo.owner,
      repo: context.repo.repo,
      pull_number: pullNumber,
      state: "closed",
    });
  } catch (error) {
    core.warning(`Could not close pull request #${pullNumber}: ${error.message}`, { title: "Close failed" });
    return;
  }

  core.info(`Closed pull request #${pullNumber} because ${reason}.`);
};
