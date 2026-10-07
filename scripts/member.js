const post = require("./api");

/**
 * Builds the helper a rules script uses to read a guild member record.
 *
 * The call goes to Arbiterer, not to Discord, because the access token stays
 * sealed on the server. The endpoint is fixed, so a rules script cannot point
 * this anywhere the workflow was not already trusted to reach.
 *
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {string} options.githubUserId Pull request author.
 * @returns {(guildId: string) => Promise<Object | null>}
 */
module.exports = function createMemberHelper({ core, githubUserId }) {
  return async function resolveMember(guildId) {
    const id = String(guildId ?? "").trim();
    if (!id) {
      throw new Error("resolveMember needs a guild id.");
    }

    let result;
    try {
      result = await post({
        core,
        path: "discord/member",
        body: { github_user_id: githubUserId, guild_id: id },
      });
    } catch (error) {
      if (error.status === 404) {
        throw new Error("The pull request author has no Discord account linked.");
      }
      throw error;
    }

    if (result?.status === "revoked") {
      throw new Error(
        "The Discord link has expired or been revoked. Ask the author to re-link, then re-run the check.",
      );
    }

    return result?.member ?? null;
  };
};