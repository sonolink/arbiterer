const audience = process.env.ARBITERER_OIDC_AUDIENCE?.trim() || 'https://arbiterer.com';
const serverUrl = process.env.ARBITERER_SERVER_URL?.trim() || 'https://api.arbiterer.com/v1';
const maxRetries = Number(process.env.ARBITERER_ERROR_MAX_RETRIES?.trim() || 3);
const failOnMissingApp = process.env.ARBITERER_FAIL_ON_MISSING_APP?.trim().toLowerCase() === 'true';
const commentOnLinked = process.env.ARBITERER_COMMENT_ON_LINKED?.trim().toLowerCase() !== 'false';

const guildId = process.env.ARBITERER_GUILD_ID?.trim() || undefined;

let endpoint;
try {
  endpoint = new URL(`${serverUrl.replace(/\/$/, '')}/resolve`);
} catch {
  throw new Error(`ARBITERER_SERVER_URL is not a valid URL: ${serverUrl}`);
}

const maxRetriesLimit = 10;
if (!Number.isInteger(maxRetries) || maxRetries < 0 || maxRetries > maxRetriesLimit) {
  throw new Error(`ARBITERER_ERROR_MAX_RETRIES must be an integer between 0 and ${maxRetriesLimit}.`);
}

const retryableStatuses = new Set([500, 502, 503, 504]);
const maxAttempts = 1 + maxRetries;
const retryDelayBaseMs = 1000; // 1 second
const retryDelayMaxMs = 30000; // 30 seconds
const timeoutMs = 30000; // 30 seconds

/**
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @returns {Promise<void>}
 */
module.exports = async function resolve({ core, context }) {
  if (context.eventName !== 'pull_request' && context.eventName !== 'pull_request_target') {
    core.info(`Skipping: event "${context.eventName}" is not a pull request.`);
    return;
  }

  const data = /** @type {import('@octokit/openapi-webhooks-types').components['schemas']['pull-request'] | undefined} */ (
    context.payload.pull_request
  );
  if (!data) {
    throw new Error('Missing pull_request data in the workflow event payload.');
  }

  const user = data.user;
  if (!user) {
    throw new Error('Missing pull_request.user data in the workflow event payload.');
  }
  if (user.login === 'ghost' || user.type !== 'User') {
    core.info(`Skipping: pull request author is not a valid GitHub user (login: ${user.login}, type: ${user.type}).`);
    return;
  }
  
  const userId = user.id;
  const issueNumber = data.number;
  const oidcToken = await core.getIDToken(audience);

  const body = JSON.stringify({
    github_user_id: String(userId),
    ...(guildId ? { guild_id: guildId } : {}),
    ...(issueNumber ? { issue_number: issueNumber } : {}),
    ...(commentOnLinked ? {} : { skip_linked_comment: true }),
  });

  let response;
  for (let attempt = 1; ; attempt++) {
    let failure;
    try {
      response = await fetch(endpoint, {
        method: 'POST',
        redirect: 'error',
        headers: {
          Authorization: `Bearer ${oidcToken}`,
          'Content-Type': 'application/json',
        },
        body,
        signal: AbortSignal.timeout(timeoutMs),
      });

      if (!retryableStatuses.has(response.status) || attempt >= maxAttempts) {
        break;
      }

      await response.body?.cancel();
      failure = `returned HTTP ${response.status}`;
    } catch (error) {
      const reason = error.name === 'TimeoutError'
        ? `timed out after ${timeoutMs / 1000}s`
        : `failed: ${error.cause?.code ?? error.cause?.message ?? error.message}`;
      if (attempt >= maxAttempts) {
        throw new Error(`Server resolve ${reason}`, { cause: error });
      }

      failure = reason;
    }

    // Back off exponentially with jitter.
    const delayMs = Math.min(retryDelayBaseMs * 2 ** (attempt - 1), retryDelayMaxMs) * (0.5 + Math.random());
    core.info(
      `Server resolve ${failure}, retrying in ${Math.round(delayMs)}ms ` +
      `(attempt ${attempt + 1} of ${maxAttempts}).`,
    );
    await new Promise((resolve) => setTimeout(resolve, delayMs));
  }

  if (!response.ok) {
    let detail;
    if (response.headers.get('content-type')?.includes('application/problem+json')) {
      const problem = await response.json().catch(() => null);
      detail = problem?.detail ?? problem?.title;
    }

    let message = `Server resolve failed with HTTP ${response.status}.`;
    if (detail) {
      message += ` Detail: ${detail}`;
    }
    throw new Error(message);
  }

  const result = await response.json().catch(() => {
    throw new Error('Server returned an invalid JSON response.');
  });

  if (typeof result?.status !== 'string' || !result.status) {
    throw new Error('Server returned an invalid status.');
  }

  core.setOutput('status', result.status);
  core.setOutput('link-url', result.link_url ?? '');
  core.setOutput('member', JSON.stringify(result.member ?? null));

  if (result.app_install_url) {
    await reportMissingApp({ core, installUrl: result.app_install_url, fail: failOnMissingApp });
  }
};

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
  const title = 'Server GitHub App not installed';
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
