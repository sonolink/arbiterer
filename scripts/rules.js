const { closePullRequest, openPullRequest } = require("./pull-requests");
const rulesSource = process.env.ARBITERER_RULES?.trim() || "";
const rulesMaxBytes = 40 * 1024; // 40 KiB (GitHub caps a single env var at 48 KiB).
if (rulesSource && Buffer.byteLength(rulesSource, "utf8") > rulesMaxBytes) {
  throw new Error(
    `The \`rules\` input is too large (${Buffer.byteLength(rulesSource, "utf8")} bytes; ` +
      `keep it under ${rulesMaxBytes} bytes). GitHub Actions truncates environment variables above 48 KiB, ` +
      "so large policies belong in a repository file, not in the workflow.",
  );
}
const closeOnFailure = process.env.ARBITERER_CLOSE_ON_FAILURE?.trim().toLowerCase() !== "false";

/**
 * Runs the maintainer's `rules` source as an async function. The script decides
 * the outcome by calling core.setFailed, core.warning and core.summary; nothing
 * is returned or validated. `resolveMember(guildId)` reads guild membership
 * through Arbiterer API, so no access token is involved.
 *
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @param {Object} options.user Raw Discord user object for a linked account.
 * @param {(guildId: string) => Promise<Object | null>} options.resolveMember
 * @param {boolean} [options.autoClosed] True when a previous run closed the
 *   pull request and has not opened it, so a passing run should open it.
 * @returns {Promise<void>}
 */
module.exports = async function evaluateRules({ core, context, github, user, autoClosed = false, resolveMember }) {
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const params = ["require", "core", "context", "github", "user", "resolveMember"];

  let evaluate;
  try {
    evaluate = new AsyncFunction(...params, rulesSource);
  } catch (error) {
    throw new Error("The `rules` input is not valid JavaScript.", { cause: error });
  }

  let rulesFailed = false;
  const ruleCore = {
    ...core,
    setFailed(message, options) {
      rulesFailed = true;
      return core.setFailed(message, options);
    },
  };

  try {
    await evaluate(require, ruleCore, context, github, user, resolveMember);
  } catch (error) {
    core.warning("The `rules` script crashed and the pull request was not closed. Fix the rules, then re-run the check.", { title: "Rules crashed" });
    throw new Error(`The \`rules\` script threw: ${error.message}`, { cause: error });
  }

  if (closeOnFailure) {
    if (rulesFailed) {
      await closePullRequest({ core, context, reason: "rules were not satisfied" });
    } else if (autoClosed) {
      await openPullRequest({ core, context });
    }
  }
};
