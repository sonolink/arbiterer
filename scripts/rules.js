const rulesSource = process.env.ARBITERER_RULES?.trim() || "";

/**
 * Runs the maintainer's `rules` module against the resolved Discord member object and fails the step when any
 * rule is not satisfied. A broken script is a hard error, an empty rules list is a pass.
 * @param {Object} options
 * @param {typeof import('@actions/core')} options.core
 * @param {typeof import('@actions/github').context} options.context
 * @param {typeof import('@actions/github')} options.github
 * @param {string} options.guildId
 * @param {Object} options.member
 * @returns {Promise<'pass' | 'fail'>}
 */
module.exports = async function evaluateRules({ core, context, github, guildId, member }) {
  if (!rulesSource) {
    core.setOutput("verdict", "pass");
    core.setOutput("summary", "");
    return "pass";
  }

  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const params = ["require", "core", "context", "github", "guildId", "member"];

  let evaluate;
  try {
    evaluate = new AsyncFunction(...params, `return (\n${rulesSource}\n)`);
  } catch {
    try {
      evaluate = new AsyncFunction(...params, rulesSource);
    } catch (error) {
      throw new Error("The `rules` input is not valid JavaScript.", { cause: error });
    }
  }

  let result;
  try {
    result = await evaluate(require, core, context, github, guildId, member);
  } catch (error) {
    throw new Error(`The \`rules\` script threw: ${error.message}`, { cause: error });
  }

  if (result === undefined) {
    throw new Error("The `rules` script returned undefined. Finish with `return { rules: [...] }` or a bare object literal.");
  }

  if (typeof result !== "object" || result === null || !Array.isArray(result.rules)) {
    throw new Error("The `rules` script must return an object with a `rules` array.");
  }

  const rules = result.rules;
  const problems = [];
  for (let index = 0; index < rules.length; index++) {
    const rule = rules[index];
    if (typeof rule !== "object" || rule === null || Array.isArray(rule)) {
      problems.push(`rules[${index}] is not an object.`);
      continue;
    }

    const label = typeof rule.id == "string" && rule.id ? rule.id : `rules[${index}]`;
    if (typeof rule.id !== "string" || !rule.id) {
      problems.push(`${label} is missing a string \'id\'.`);
    }
    if (typeof rule.description !== "string" || !rule.description) {
      problems.push(`${label} is missing a string \'description\'.`);
    }
  }

  if (problems.length) {
    throw new Error(`The \`rules\` script returned invalid rules:\n- ${problems.join("\n- ")}`);
  }

  const failures = rules.filter((rule) => rule.passed !== true);

  if (!failures.length) {
    core.info(`Rules: ${rules.length} checked, all satisfied.`);
    core.setOutput("verdict", "pass");
    core.setOutput("summary", "");
    return "pass";
  }

  const detail = (rule) => (rule.detail == null ? "" : String(rule.detail));
  const describe = (rule) => (detail(rule) ? `${rule.description} (${detail(rule)})` : rule.description);
  const first = describe(failures[0]);

  core.setOutput("verdict", "fail");
  core.setOutput("summary", first);

  await core.summary
    .addHeading("Discord membership rules not satisfied", 3)
    .addTable([
      [
        { data: "Rule", header: true },
        { data: "Result", header: true },
        { data: "Detail", header: true },
      ],
      ...rules.map((rule) => [rule.description, rule.passed === true ? "pass" : "fail", detail(rule)]),
    ])
    .addRaw(`${failures.length} of ${rules.length} rules not satisfied. First: ${first}`, true)
    .write();

  for (const rule of failures) {
    const reason = rule.passed === false ? "not satisfied" : "returned no result";
    core.warning(`${rule.description}: ${reason}.`, { title: `Rule ${rule.id}` });
  }

  core.setFailed(`${failures.length} of ${rules.length} Discord membership rules not satisfied. First: ${first}`);
  return "fail";
};
