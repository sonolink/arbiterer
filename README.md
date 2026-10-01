# Arbiterer

Arbiterer connects GitHub accounts to Discord accounts. When someone opens a pull request, it can tell you whether the author has linked their Discord account, and whether they're a member of your Discord server. If they haven't linked yet, Arbiterer leaves a comment on the PR with a link to do it.

It has two parts:

- **A Go server** that handles the account linking (GitHub and Discord OAuth) and stores the results in Postgres.
- **A GitHub Action** that asks the server about a PR author, using GitHub's OIDC token so no secrets need to be shared with your workflow.

## Using the action

Add a workflow to your repo:

```yaml
name: Check Discord link
on:
  pull_request_target:

permissions:
  id-token: write
  pull-requests: write

jobs:
  resolve:
    runs-on: ubuntu-latest
    steps:
      - id: arbiterer
        uses: sonolink/arbiterer@main
        with:
          guild-id: "123456789012345678" # optional
      - run: echo "Status is ${{ steps.arbiterer.outputs.status }}"
```

A few things to know:

- The job needs `id-token: write`, otherwise it can't get an OIDC token.
- For PRs from forks, use `pull_request_target` instead of `pull_request`.
- The [Arbiterer GitHub App](https://github.com/apps/arbiterer/installations/new) has to be installed on the repo so it can post the comment. If it isn't, the action only warns, unless you set `fail-on-missing-app: true`.
- Bot authors and non-PR events are skipped.

### Inputs

| Input                 | Default | What it does                                                             |
| --------------------- | ------- | ------------------------------------------------------------------------ |
| `guild-id`            |         | Discord server to check membership in. Leave out to only check the link. |
| `fail-on-missing-app` | `false` | Fail the step if the GitHub App isn't installed.                         |
| `comment-on-linked`   | `true`  | Comment on the PR even when the author is already linked.                |

### Outputs

GitHub Actions outputs are always strings, so the types below describe what's inside that string.

| Output     | Type                              | What it is                                                                                                                                                             |
| ---------- | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `status`   | enum (see below)                  | The account link status.                                                                                                                                               |
| `link-url` | URL, or an empty string           | Where the author can link or re-link their accounts. Only set for `unlinked` and `revoked`.                                                                            |
| `member`   | JSON object, or the string `null` | The Discord [guild member](https://discord.com/developers/docs/resources/guild#guild-member-object) object. `null` unless `status` is `linked` and `guild-id` was set. |

`status` is one of:

| Value          | Meaning                                                                                        |
| -------------- | ---------------------------------------------------------------------------------------------- |
| `linked`       | The author has linked their Discord account (and is in the server, if `guild-id` is set).      |
| `unlinked`     | The author hasn't linked a Discord account yet.                                                |
| `revoked`      | They linked before, but the Discord authorization no longer works, so they need to link again. |
| `not_a_member` | They're linked, but not a member of the server given in `guild-id`.                            |

To use the member object in a later step, parse it with `fromJSON(steps.arbiterer.outputs.member)`.

## Running the server locally

You'll need Go, Postgres, and a Discord application and GitHub App to test the OAuth flows.

1. Copy `.env.example` to `.env` and fill in the Discord and GitHub credentials. The two encryption keys can be generated with `openssl rand -base64 32`.
2. Start Postgres and run the migrations (the repo uses [goose](https://github.com/pressly/goose); the settings are already in `.env.example`). There's a devcontainer in `.devcontainer/` that sets this up for you.
3. Run it:

   ```sh
   go run ./cmd/arbiterer
   ```

It listens on `127.0.0.1:8080` by default.
## License

See [LICENSE](LICENSE).
