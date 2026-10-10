<div align="center">

![Arbiterer](assets/banner.png)

A GitHub Action that lets your Discord community decide whose contributions get through.

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
> Use `pull_request_target` for pull requests from forks, so the workflow and your rules always run from your default branch.

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
      - uses: sonolink/arbiterer@v1
```

That's all you need if authors only have to link their accounts. To enforce conditions on the linked Discord account, add [rules](#rules).

## Configuration

### Inputs

All inputs are optional.

| Input               | Description                                                                                                                                                                                                                           | Default |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `rules`             | JavaScript that decides whether the author passes. See [Rules](#rules). Leave empty to only require a linked Discord account.                                                                                                         | `""`    |
| `close-on-failure`  | Close the pull request when the author has not linked their accounts or fails the rules, and open it again once they pass. Needs the `pull_requests: write` permission on the Arbiterer app and `reopened` in the workflow's `types`. | `false` |
| `comment-on-linked` | Post a comment on the pull request confirming the author is already linked. Set to `false` to only comment when the author still has to act. A comment that earlier asked them to act is always updated.                              | `true`  |

### Rules

The `rules` input is a JavaScript snippet run once per pull request. It has access to:

| Name                     | Description                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `user`                   | The author's Discord [user object](https://docs.discord.com/developers/resources/user#user-object) (flags, MFA status, etc.).                                       |
| `resolveMember(guildId)` | Async. Returns the author's [guild member object](https://docs.discord.com/developers/resources/guild#guild-member-object), or `null` if they aren't in the server. |
| `core`                   | The [GitHub Actions toolkit](https://github.com/actions/toolkit). Call `core.setFailed(message)` to fail the check and show `message` to the contributor.           |

For example, to require MFA, guild membership, and a week of tenure:

```yaml
- uses: sonolink/arbiterer@v1
  with:
    rules: |
      const GUILD = "1471146455002775624";
      const weekMs = 7 * 24 * 60 * 60 * 1000;

      if (!user.mfa_enabled) {
        core.setFailed("User doesn't have two-factor auth enabled on their Discord account.");
        return;
      }

      const member = await resolveMember(GUILD);

      if (!member) {
        core.setFailed("User is not a member of https://discord.gg/tPHVWBPedt");
        return;
      }

      const joined = member.joined_at ? Date.parse(member.joined_at) : 0;
      if (!joined || Date.now() - joined <= weekMs) {
        core.setFailed(`User joined ${member.joined_at ?? "recently"}. Please wait 7 days after joining.`);
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
