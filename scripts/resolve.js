const evaluateRules = require("./rules");
const createMemberHelper = require("./member");
const post = require("./api");
const commentOnLinked = process.env.ARBITERER_COMMENT_ON_LINKED?.trim().toLowerCase() !== "false";
const rulesSource = process.env.ARBITERER_RULES?.trim() || "";

/**
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @returns {Promise<void>}
 */
module.exports = async function resolve({ core, context, github }) {
  if (context.eventName !== "pull_request" && context.eventName !== "pull_request_target") {
    core.info(`Skipping: event "${context.eventName}" is not a pull request.`);
    return;
  }

  const data = /** @type {import('@octokit/openapi-webhooks-types').components['schemas']['pull-request'] | undefined} */ (context.payload.pull_request);
  if (!data) {
    throw new Error("Missing pull_request data in the workflow event payload.");
  }

  const user = data.user;
  if (!user) {
    throw new Error("Missing pull_request.user data in the workflow event payload.");
  }
  if (user.login === "ghost" || user.type !== "User") {
    core.info(`Skipping: pull request author is not a valid GitHub user (login: ${user.login}, type: ${user.type}).`);
    return;
  }

  const userId = user.id;
  const issueNumber = data.number;

  const result = await post({
    core,
    path: "discord/resolve",
    body: {
      github_user_id: String(userId),
      ...(issueNumber ? { issue_number: issueNumber } : {}),
      ...(commentOnLinked ? {} : { skip_linked_comment: true }),
    },
  });

  if (typeof result?.status !== "string" || !result.status) {
    throw new Error("Server returned an invalid status.");
  }

  core.setOutput("status", result.status);
  core.setOutput("link-url", result.link_url ?? "");
  core.setOutput("user", JSON.stringify(result.user ?? null));

  if (result.app_install_url) {
    await reportMissingApp({ core, installUrl: result.app_install_url });
  }

  if (result.status !== "linked") {
    if (rulesSource) {
      core.info("Skipping rules: the pull request author has no Discord account linked yet.");
    }
    return;
  }

  if (!result.user) {
    throw new Error("Server reported the account as linked but did not return the user.");
  }

  await evaluateRules({
    core,
    context,
    github,
    user: result.user,
    resolveMember: createMemberHelper({ core, githubUserId: String(userId) }),
  });
};

/**
 * Fails the step when the GitHub App is missing, reporting it in the job
 * summary and as an error annotation. Linking cannot work without the app,
 * so this is always fatal rather than a configurable warning.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {string} options.installUrl
 * @returns {Promise<void>}
 */
async function reportMissingApp({ core, installUrl }) {
  const title = "Server GitHub App not installed";
  const reason = "The Arbiterer GitHub App is not installed on this repository, so it could not post the setup comment telling the pull request author how to link their accounts.";

  await core.summary.addHeading(title, 3).addRaw(reason, true).addLink("Install the Arbiterer GitHub App", installUrl).write();

  core.error(`${reason} A maintainer can install it here: ${installUrl}`, { title });
  process.exitCode = 1;
}
