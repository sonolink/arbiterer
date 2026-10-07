<div align="center">

![Arbiterer](assets/banner.png)

A GitHub Action that lets your Discord community decide whose pull requests get through.

[![License](https://img.shields.io/github/license/sonolink/arbiterer)](LICENSE)
[![Discord](https://img.shields.io/discord/1471146455002775624?label=discord)](https://discord.gg/tPHVWBPedt)
<!--[![CI](https://github.com/sonolink/arbiterer/actions/workflows/ci.yml/badge.svg)](https://github.com/sonolink/arbiterer/actions/workflows/ci.yml)-->

</div>

## How it works

1. A contributor opens a pull request.
2. Arbiterer checks whether the author has linked their GitHub account to a Discord account. If not, it comments on the PR with a link to do so.
3. Once linked, your rules run against the author's Discord identity. The check passes or fails depending on the result.

Rules are optional. Without them, the author only needs to link their accounts.

> [!NOTE]
> Links are scoped per repository and strictly **one-to-one**: within a repository, one GitHub account maps to one Discord account and vice versa. The same Discord account can still be linked to different GitHub accounts in _different_ repositories.

## Getting Started

### Installation

Install the [Arbiterer GitHub App](https://github.com/apps/arbiterer) on your repository or organization. The app posts the linking comments on pull requests.

> [!CAUTION]
> The app must be installed on every repository that uses the action. If it isn't, the action fails on every run.

### Usage

Add a workflow to `.github/workflows/`:

> [!WARNING]
> Use `pull_request_target`, so the workflow and your rules always run from your default branch. Never check out or execute the pull request's code in the same job, since that would run untrusted code with access to your repository's permissions and secrets.

```yaml
name: Arbiterer

on:
  pull_request_target:
    types: [opened, synchronize, reopened]

permissions:
  contents: read
  pull-requests: write
  id-token: write

jobs:
  arbiterer:
    runs-on: ubuntu-latest
    steps:
      - uses: sonolink/arbiterer@v1.0.0
        with:
          rules: |
            // Optional. Plain JavaScript, see "Rules" below.
```

Drop the `with` block entirely if authors only need to link their accounts.

### Rules

The `rules` input is a JavaScript snippet run once per pull request. It has access to:

| Name                     | Description                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `user`                   | The author's Discord [user object](https://docs.discord.com/developers/resources/user#user-object) (flags, MFA status, etc.).                                       |
| `resolveMember(guildId)` | Async. Returns the author's [guild member object](https://docs.discord.com/developers/resources/guild#guild-member-object), or `null` if they aren't in the server. |
| `core`                   | The [GitHub Actions toolkit](https://github.com/actions/toolkit). Call `core.setFailed(message)` to fail the check and show `message` to the contributor.           |

For example, to require MFA, guild membership, and a week of tenure:

```yaml
- uses: sonolink/arbiterer@v1.0.0
  with:
    rules: |
      const GUILD = "112233445566778899";
      const weekMs = 7 * 24 * 60 * 60 * 1000;

      if (!user.mfa_enabled) {
        core.setFailed("You don't have two-factor auth enabled on your Discord account.");
        return;
      }

      const member = await resolveMember(GUILD);

      if (!member) {
        core.setFailed("You are not a member of https://discord.gg/tPHVWBPedt");
        return;
      }

      const joined = member.joined_at ? Date.parse(member.joined_at) : 0;
      if (!joined || Date.now() - joined <= weekMs) {
        core.setFailed(`Wait a week after joining. You joined ${member.joined_at ?? "recently"}.`);
      }
```

## License

Apache License 2.0. See the [License File](LICENSE).

<br>

<p align="center">
	<img src="https://raw.githubusercontent.com/catppuccin/catppuccin/main/assets/footers/gray0_ctp_on_line.svg?sanitize=true" />
</p>

<p align="center">
        <i><code>&copy; 2026 <a href="https://github.com/sonolink">SonoLink Development Team</a></code></i>
</p>
