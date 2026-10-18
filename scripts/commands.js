const post = require("./api");

/**
 * @typedef {Object} CommandInput
 * @property {typeof import('@actions/core')} core
 * @property {typeof import('@actions/github').context} context The `issue_comment` event context.
 * @property {typeof import('@actions/github')} github
 * @property {Object} pullRequest The pull request.
 * @property {Object} comment The comment that contains the command.
 */

/**
 * @typedef {Object} Command
 * @property {(input: CommandInput) => string | null} authorize Returns why the commenter may not run the command, or null when they may.
 * @property {(input: CommandInput) => Promise<boolean>} run Runs the command. Resolves true when the
 *   verification flow should then run for the pull request, false when the command is fully handled.
 */

/**
 * Only the pull request author may run the command.
 * @param {CommandInput} input
 * @returns {string | null}
 */
function authorOnly({ comment, pullRequest }) {
    return comment.user?.id === pullRequest.user?.id ? null : `@${comment.user?.login} is not the pull request author`;
}

/**
 * The available commands.
 * @type {Record<string, Command>}
 */
const commands = {
    recheck: {
        authorize: (input) => authorOnly(input) ?? (input.pullRequest.merged ? "the pull request is already merged" : null),
        run: async () => true,
    },
};

/**
 * Parses a comment of the form `/arbiterer <command>`, such as `/arbiterer recheck`. The comment must
 * start with `/arbiterer`, followed by the name of a command in `commands`; both are matched ignoring
 * case, and anything after the command name is ignored. Returns null when the comment is not a known command.
 * @param {string | undefined} body
 * @returns {{ name: string, command: Command } | null}
 */
function parseCommand(body) {
    const [prefix, name] = (body ?? "").trim().toLowerCase().split(/\s+/, 2);
    if (prefix !== "/arbiterer") {
        return null;
    }

    return Object.hasOwn(commands, name) ? { name, command: commands[name] } : null;
}

/**
 * Handles a command comment on a pull request.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context An `issue_comment` event context.
 * @param {typeof import('@actions/github')} options.github
 * @returns {Promise<{ name: string, context: typeof import('@actions/github').context } | null>}
 */
async function commandContext({ core, context, github }) {
    const { issue, comment, action } = context.payload;

    if (action !== "created") {
        core.info(`Skipping: comment action "${action}" is not a new comment.`);
        return null;
    }
    if (!issue?.pull_request) {
        core.info("Skipping: the comment is not on a pull request.");
        return null;
    }

    const parsed = parseCommand(comment?.body);
    if (!parsed) {
        core.info("Skipping: the comment is not a known command.");
        return null;
    }

    const { data: pullRequest } = await github.rest.pulls.get({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: issue.number,
    });

    const input = { core, context, github, pullRequest, comment };
    const denied = parsed.command.authorize(input);
    if (denied) {
        core.info(`Skipping /${parsed.name}: ${denied}.`);
        return null;
    }

    await reactToCommand(input);

    if (!(await parsed.command.run(input))) {
        return null;
    }

    return {
        name: parsed.name,
        context: Object.assign(Object.create(context), {
            eventName: "pull_request_target",
            payload: { ...context.payload, pull_request: pullRequest },
        }),
    };
}

/**
 * Reacts to the command comment through the Arbiterer app, so the reaction comes from the app
 * and not from the workflow's token. Best effort: a failed reaction must not fail the check.
 * @param {CommandInput} input
 * @returns {Promise<void>}
 */
async function reactToCommand({ core, comment }) {
    try {
        await post({
            core,
            path: "github/comments/react",
            body: { comment_id: comment.id, content: "+1" },
        });
    } catch (error) {
        core.info(`Could not react to the command comment: ${error.message}`);
    }
}

module.exports = { commandContext };
