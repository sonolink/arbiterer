const audience = process.env.ARBITERER_OIDC_AUDIENCE?.trim();
const serverUrl = process.env.ARBITERER_SERVER_URL?.trim();

if (!audience || !serverUrl) {
  throw new Error('The action maintainer must configure the OIDC audience and server URL in the environment variables.');
}

/**
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @returns {Promise<void>}
 */
module.exports = async function resolve({ core, context }) {
  const guildId = process.env.ARBITERER_GUILD_ID?.trim() || undefined;
  const failOnMissingApp = process.env.ARBITERER_FAIL_ON_MISSING_APP?.trim().toLowerCase() === 'true';
  const userId = context.payload.pull_request
    ? context.payload.pull_request.user?.id
    : context.payload.sender?.id;
  const issueNumber = context.payload.pull_request?.number;

  if (!userId) {
    throw new Error('Cannot determine the GitHub user ID from the workflow event.');
  }

  const endpoint = new URL(`${serverUrl.replace(/\/$/, '')}/resolve`);

  const oidcToken = await core.getIDToken(audience);

  const response = await fetch(endpoint, {
    method: 'POST',
    redirect: 'error',
    headers: {
      Authorization: `Bearer ${oidcToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      github_user_id: String(userId),
      ...(guildId ? { guild_id: guildId } : {}),
      ...(issueNumber ? { issue_number: issueNumber } : {}),
    }),
    signal: AbortSignal.timeout(30000),
  });

  if (!response.ok) {
    let detail;
    if (response.headers.get('content-type')?.includes('application/problem+json')) {
      const problem = await response.json().catch(() => null);
      detail = problem?.detail ?? problem?.title;
    }
    throw new Error(
      detail
        ? `Arbiterer resolve failed with HTTP ${response.status}: ${detail}`
        : `Arbiterer resolve failed with HTTP ${response.status}.`,
    );
  }

  const result = await response.json().catch(() => {
    throw new Error('Arbiterer returned an invalid JSON response.');
  });

  if (typeof result.status !== 'string' || !result.status) {
    throw new Error('Arbiterer returned an invalid status.');
  }

  core.setOutput('status', result.status);
  core.setOutput('link-url', result.link_url ?? '');
  core.setOutput('member', JSON.stringify(result.member ?? null));

  if (result.app_install_url) {
    await reportMissingApp({ core, installUrl: result.app_install_url, fail: failOnMissingApp });
  }
}

/**
 * Tells maintainers the GitHub App is missing, in the job summary and as an
 * annotation, failing the step when they asked for that.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {string} options.installUrl
 * @param {boolean} options.fail
 * @returns {Promise<void>}
 */
async function reportMissingApp({ core, installUrl, fail }) {
  const title = 'Arbiterer GitHub App not installed';
  const reason =
    'The Arbiterer GitHub App is not installed on this repository, so it could not post the setup comment ' +
    'telling the pull request author how to link their accounts.';

  await core.summary
    .addHeading(title, 3)
    .addRaw(reason, true)
    .addLink('Install the Arbiterer GitHub App', installUrl)
    .write();

  const message = `${reason} A maintainer can install it here: ${installUrl}`;
  if (fail) {
    core.error(message, { title });
    process.exitCode = 1;
  } else {
    core.warning(message, { title });
  }
}
