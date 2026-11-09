const fs = require("fs");
const { execSync } = require("child_process");

const VERSION_FILE = "internal/version/version.go";
const CHANGELOG_FILE = "CHANGELOG.md";

const VERSION_RE = /^const Version = "(\d+)\.(\d+)\.(\d+)"$/m;

// The Unreleased body: everything between the "## [Unreleased]" heading and
// the next "## [" heading, or the link definitions at the end of the file
const UNRELEASED_SECTION_RE =
    /^## \[Unreleased\]\n\n([\s\S]*?)(?=\n## \[|\n\[[^\]]+\]:|(?![\s\S]))/m;

// [Unreleased]: https://github.com/owner/repo/compare/v1.0.0...HEAD
const UNRELEASED_LINK_RE =
    /^\[Unreleased\]: https:\/\/github\.com\/([^/\s]+)\/([^/\s]+)\/compare\/v\d+\.\d+\.\d+\.\.\.HEAD$/m;

// The fresh "## [Unreleased]" the file gets reset to after every release
const UNRELEASED_SECTION = [
    "## [Unreleased]",
    "",
    "### Added",
    "",
    "### Changed",
    "",
    "### Deprecated",
    "",
    "### Removed",
    "",
    "### Fixed",
    "",
    "### Security",
].join("\n");

const versionNumber = (version) =>
    `${version.major}.${version.minor}.${version.patch}`;

let info = console.log;
let warn = console.warn;

function readFile(file) {
    const content = fs.readFileSync(file, "utf8");
    return {
        content: content.replace(/\r\n/g, "\n").replace(/\n$/, ""),
        eol: content.includes("\r\n") ? "\r\n" : "\n",
    };
}

function writeFile(file, content, eol) {
    fs.writeFileSync(file, content.replace(/\n/g, eol) + "\n");
}

function readVersion() {
    const { content } = readFile(VERSION_FILE);
    const match = content.match(VERSION_RE);
    if (!match) {
        throw new Error(`Could not find 'const Version' in ${VERSION_FILE}`);
    }
    return {
        major: Number(match[1]),
        minor: Number(match[2]),
        patch: Number(match[3]),
    };
}

// Starts development on the next cycle: minor +1, patch reset
function writeVersion(version) {
    const { content, eol } = readFile(VERSION_FILE);
    const match = content.match(VERSION_RE);
    const bumped = {
        major: version.major,
        minor: version.minor + 1,
        patch: 0,
    };
    writeFile(
        VERSION_FILE,
        content.replace(match[0], `const Version = "${versionNumber(bumped)}"`),
        eol,
    );
    return bumped;
}

// Splits an Unreleased body into its "### Category" blocks, dropping empty
// ones so a release section never carries empty categories
function extractReleaseBody(section) {
    const lines = section.split("\n");
    const blocks = [];
    let heading = null;
    let body = [];

    const flush = () => {
        const text = body.join("\n").trim();
        if (heading && text) {
            blocks.push(`${heading}\n\n${text}`);
        }
        heading = null;
        body = [];
    };

    for (const line of lines) {
        if (line.startsWith("### ")) {
            flush();
            heading = line;
            continue;
        }
        if (heading || body.length > 0) {
            body.push(line);
        }
    }
    flush();

    return blocks.join("\n\n").trimEnd();
}

function versionHeading(version) {
    const date = new Date().toISOString().slice(0, 10);
    return `## [${versionNumber(version)}] - ${date}`;
}

// Takes the repo owner/name from the workflow context.
function repo(context) {
    let owner = context?.repo?.owner;
    let name = context?.repo?.repo;

    if (!owner || !name) {
        try {
            const match = execSync("git remote get-url origin", {
                encoding: "utf8",
            }).match(/(?:github\.com[:/])([^/]+)\/([^/.]+?)(?:\.git)?$/);
            if (match) {
                owner = match[1];
                name = match[2];
            }
        } catch {
            // No git remote available
        }
    }

    return owner && name ? { owner, name } : null;
}

function bumpChangelog(version, context) {
    const { content, eol } = readFile(CHANGELOG_FILE);
    const number = versionNumber(version);

    // Guard against running twice for the same release
    if (content.includes(`## [${number}]`)) {
        warn(
            `Release section for v${number} already exists in ${CHANGELOG_FILE}, skipping changelog update`,
        );
        return;
    }

    const match = content.match(UNRELEASED_SECTION_RE);
    if (!match) {
        throw new Error(`Could not find '## [Unreleased]' in ${CHANGELOG_FILE}`);
    }
    const body = match[1];

    const head = content.slice(0, match.index);
    const rest = content.slice(match.index + match[0].length).replace(/^\n/, "");

    // The Unreleased notes become the new release section
    const releaseBody = extractReleaseBody(body);
    const releaseSection = releaseBody.length > 0
        ? `${versionHeading(version)}\n\n${releaseBody}\n\n`
        : "";

    // Link block before, version-swapped; none, rebuilt from the repo origin
    const existing = rest.match(UNRELEASED_LINK_RE);
    let links;
    if (existing) {
        const [, owner, name] = existing;
        links =
            `${existing[0].replace(/v\d+\.\d+\.\d+\.\.\.HEAD/, `v${number}...HEAD`)}\n` +
            `[${number}]: https://github.com/${owner}/${name}/releases/tag/v${number}`;
    } else {
        const origin = repo(context);
        if (!origin) {
            throw new Error(
                `Could not determine repository owner/name to build the Unreleased link in ${CHANGELOG_FILE}`,
            );
        }
        links =
            `[Unreleased]: https://github.com/${origin.owner}/${origin.name}/compare/v${number}...HEAD\n` +
            `[${number}]: https://github.com/${origin.owner}/${origin.name}/releases/tag/v${number}`;
    }

    const tail = existing
        ? rest.replace(UNRELEASED_LINK_RE, links)
        : `${rest}\n\n${links}`;

    writeFile(
        CHANGELOG_FILE,
        `${head}${UNRELEASED_SECTION}\n\n${releaseSection}${tail}`,
        eol,
    );
    info(
        releaseBody
            ? `Moved Unreleased changes into the v${number} section of ${CHANGELOG_FILE}`
            : `No Unreleased changes to move into the v${number} section of ${CHANGELOG_FILE}`,
    );
}

module.exports = async ({ core, context } = {}) => {
    if (core) {
        info = core.info.bind(core);
        warn = core.warning.bind(core);
    }

    const currentVersion = readVersion();
    info(`Current version: ${versionNumber(currentVersion)}`);

    bumpChangelog(currentVersion, context);

    const bumped = writeVersion(currentVersion);
    info(`Bumped version to: ${versionNumber(bumped)}`);

    if (core) {
        core.exportVariable("NEW_VERSION", versionNumber(bumped));
    }
};
