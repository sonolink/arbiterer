const audience = process.env.ARBITERER_OIDC_AUDIENCE?.trim() || "https://arbiterer.com";
const serverUrl = process.env.ARBITERER_SERVER_URL?.trim() || "https://api.arbiterer.com/v1";
const maxRetries = Number(process.env.ARBITERER_ERROR_MAX_RETRIES?.trim() || 3);
const maxRetriesLimit = 10;

if (!Number.isInteger(maxRetries) || maxRetries < 0 || maxRetries > maxRetriesLimit) {
  throw new Error(`ARBITERER_ERROR_MAX_RETRIES must be an integer between 0 and ${maxRetriesLimit}.`);
}

const maxAttempts = 1 + maxRetries;
const retryableStatuses = new Set([500, 502, 503, 504]);
const retryDelayBaseMs = 1000; // 1 second
const retryDelayMaxMs = 30000; // 30 seconds
const timeoutMs = 30000; // 30 seconds

function endpointFor(path) {
  try {
    return new URL(`${serverUrl.replace(/\/$/, "")}/${path.replace(/^\//, "")}`);
  } catch {
    throw new Error(`ARBITERER_SERVER_URL is not a valid URL: ${serverUrl}`);
  }
}

/**
 * Sends one JSON POST to the Arbiterer API, authenticated by a fresh GitHub
 * OIDC token for this workflow run. Retries transient failures (network errors
 * and HTTP 50*) with exponential backoff.
 *
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {string} options.path API path, e.g. "discord/resolve".
 * @param {Object} options.body
 * @returns {Promise<Object>} Parsed JSON response body.
 */
module.exports = async function post({ core, path, body }) {
  const endpoint = endpointFor(path);
  const oidcToken = await core.getIDToken(audience);

  let response;
  for (let attempt = 1; ; attempt++) {
    let failure;
    try {
      response = await fetch(endpoint, {
        method: "POST",
        redirect: "error",
        headers: {
          Authorization: `Bearer ${oidcToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(timeoutMs),
      });

      if (!retryableStatuses.has(response.status) || attempt >= maxAttempts) {
        break;
      }

      await response.body?.cancel();
      failure = `returned HTTP ${response.status}`;
    } catch (error) {
      const reason = error.name === "TimeoutError" ? `timed out after ${timeoutMs / 1000}s` : `failed: ${error.cause?.code ?? error.cause?.message ?? error.message}`;
      if (attempt >= maxAttempts) {
        throw new Error(`Server ${path} ${reason}`, { cause: error });
      }

      failure = reason;
    }

    const delayMs = Math.min(retryDelayBaseMs * 2 ** (attempt - 1), retryDelayMaxMs) * (0.5 + Math.random());
    core.info(`Server ${path} ${failure}, retrying in ${Math.round(delayMs)}ms ` + `(attempt ${attempt + 1} of ${maxAttempts}).`);
    await new Promise((resolve) => setTimeout(resolve, delayMs));
  }

  if (response.ok) {
    return await response.json().catch(() => {
      throw new Error("Server returned an invalid JSON response.");
    });
  }

  let detail;
  if (response.headers.get("content-type")?.includes("application/problem+json")) {
    const problem = await response.json().catch(() => null);
    detail = problem?.detail;
  }

  const message = `Server ${path} failed with HTTP ${response.status}.` + (detail ? ` Detail: ${detail}` : "");
  const error = new Error(message);
  error.status = response.status;
  throw error;
};
