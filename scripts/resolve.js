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
 * @param {string} [options.guildId]
 * @returns {Promise<void>}
 */
module.exports = async function resolve({ core, context, guildId }) {
  guildId = guildId?.trim() || undefined;
  const userId = context.payload.pull_request
    ? context.payload.pull_request.user?.id
    : context.payload.sender?.id;

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

  const status = result.status;
  const setupUrl = result.setup_url ?? '';
  core.setOutput('status', status);
  core.setOutput('setup-url', setupUrl);
  core.setOutput('member', result.member ?? null);
}
