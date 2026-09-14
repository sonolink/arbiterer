const audience = process.env.ARBITERER_OIDC_AUDIENCE?.trim();
const serverUrl = process.env.ARBITERER_SERVER_URL?.trim();

if (!audience || !serverUrl) {
  throw new Error('The action maintainer must configure the OIDC audience and server URL in the environment variables.');
}

/**
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {ReturnType<typeof import('@actions/github').getOctokit>} options.github
 * @returns {Promise<void>}
 */
module.exports = async function resolve({ core, context, github }) {
  const guildId = core.getInput('guild-id', { required: false }) || undefined;
  const userId = context.payload.pull_request
    ? context.payload.pull_request.user?.id
    : context.payload.sender?.id;
  const issueNumber = context.payload.pull_request?.number;

  if (!userId) {
    throw new Error('Cannot determine the GitHub user ID from the workflow event.');
  }

  const endpoint = new URL(`${serverUrl.replace(/\/$/, '')}/v1/resolve`);

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
    }),
    signal: AbortSignal.timeout(30000),
  });
  if (!response.ok) {
    throw new Error(`Arbiterer resolve failed with HTTP ${response.status}.`);
  }
  const result = await response.json();
  if (typeof result.status !== 'string' || !result.status) {
    throw new Error('Arbiterer returned an invalid status.');
  }

  const status = result.status;
  const setupUrl = result.setup_url ?? '';
  core.setOutput('status', status);
  core.setOutput('setup-url', setupUrl);
  core.setOutput('member', result.member ?? null);

  if (status === 'linked' || !issueNumber) return;

  const author = `@${context.payload.pull_request.user.login}`;
  const messages = {
    unlinked: `Please [link your Discord account](${setupUrl}) to your GitHub account.`,
    revoked: `Your Discord account link has expired or was revoked. Please [re-link your account](${setupUrl}).`,
    not_a_member: 'Your GitHub account is linked, but you are not in the required Discord server. Please join the server.',
  };
  const body = `${messages[status] ?? `Unable to confirm your Discord account link. Status: ${status}`}`;
  await github.rest.issues.createComment({
    ...context.repo,
    issue_number: issueNumber,
    body: `${author}, ${body}`,
  });
}
