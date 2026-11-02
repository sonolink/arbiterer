# Arbiterer Privacy Policy

Effective date: October 19, 2026

This Privacy Policy applies to the App, the Action when it connects to the Service, the linking flow at `arbiterer.sonolink.dev`, and the API at `api.arbiterer.sonolink.dev`.

Related: [Terms of Service](https://github.com/sonolink/arbiterer/blob/main/TERMS.md) · [Security Policy](https://github.com/sonolink/arbiterer/blob/main/.github/SECURITY.md)

## Summary

We collect GitHub and Discord account IDs, Discord tokens, repository and pull request data, and basic server logs so we can link accounts, run checks, and keep the Service secure.
We do not collect repository contents, passwords, payment details, or email addresses from GitHub or Discord.
We share link status and Discord data with the repository's workflow, and Maintainers control what happens after that.
We do not sell your data or use it for advertising.

## Scope

This policy applies to the Service, the App, and the Action when it connects to the Service.
Terms used in this policy that are defined in §2 of our [Terms of Service](https://github.com/sonolink/arbiterer/blob/main/TERMS.md) have the same meaning here.
It does not apply to a copy of Arbiterer that someone else runs.
If you run your own copy, you are responsible for how it handles data.

## Who we are

Arbiterer is run by the SonoLink Development Team ("SonoLink", "we", "us").
For the Service, we are the controller for the data we store: link records (which GitHub account is linked to which Discord account for which repository), Discord tokens, and setup records (the comment the App posts on a pull request and its status).
You can reach us at [admin@arbiterer.com](mailto:admin@arbiterer.com) or on our [Discord server](https://discord.gg/tPHVWBPedt).

Maintainers are independent controllers for the Discord data their workflow receives and for what they do with it.
They decide what their Rules request and where that data goes.
If you need a data processing agreement, contact us at [admin@arbiterer.com](mailto:admin@arbiterer.com).

## Data we collect

We collect only the data we need to link accounts, run checks, and manage pull requests.
The table describes each category, why we use it, and how long we keep it.

| Data                             | What it is                                                                                                                                                                           | Source                               | Purpose                                                                       | Legal basis          | Stored                                  | Retention                                                                                                                       |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------ | ----------------------------------------------------------------------------- | -------------------- | --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| GitHub user ID                   | The numeric identifier of your GitHub account.                                                                                                                                       | Pull request event or GitHub sign-in | Link accounts and scope the link to a repository                              | Contract             | Yes                                     | While the link exists, until you ask us to delete it, or until no longer needed                                                 |
| GitHub username                  | Your public GitHub login, for example "octocat".                                                                                                                                     | GitHub                               | Post the setup comment                                                        | Contract             | Yes (in the GitHub comment)             | Until the comment is deleted; GitHub controls it                                                                                |
| Discord user ID and tokens       | Your numeric Discord ID, plus the access and refresh tokens that let us read your Discord data.                                                                                      | Discord authorization                | Identify the linked account, read profile and membership, and refresh access  | Contract             | Yes (tokens encrypted with AES-256-GCM) | While the link exists; an old token record may remain after replacement until you ask us to delete it or it is no longer needed |
| Repository and pull request data | The repository ID, the pull request number, and the records we need to post comments and close or reopen the pull request.                                                           | Action OIDC token and request        | Scope links, update comments, rerun checks, and close or reopen pull requests | Contract             | Yes                                     | While needed to operate the Service                                                                                             |
| GitHub OAuth and OIDC tokens     | Short-lived tokens that confirm your GitHub identity and the calling repository.                                                                                                     | GitHub sign-in and the Action        | Verify your GitHub identity and the calling repository                        | Contract             | No                                      | Discarded after use                                                                                                             |
| Discord profile and membership   | The profile Discord returns, such as your user ID, username, display name, and avatar, and, on request, your member record in a server, such as your nickname, roles, and join date. | Discord API                          | Pass data to the repository's workflow                                        | Contract             | No                                      | Not stored by us; may appear in workflow logs                                                                                   |
| Linking cookies and URL token    | Browser cookies and a one-time link credential that keep the linking flow tied to your browser and repository.                                                                       | Our server                           | Secure the linking flow and bind it to your browser                           | Contract             | Yes                                     | 10 minutes by default for cookies; 15 minutes by default for the URL token                                                      |
| Server logs                      | Operational logs from our servers and hosting provider, such as errors and identifiers needed to debug problems and abuse.                                                           | Our servers and hosting provider     | Security, abuse prevention, and debugging                                     | Legitimate interests | Yes                                     | Only as long as needed                                                                                                          |

We also set two linking cookies: `arbiterer_link_state` and `arbiterer_link`.
They are `HttpOnly`, `Secure`, and `SameSite=Lax`, are limited to the `/v1/link` path, and expire after 10 minutes by default.
The link URL also carries a sealed token with your GitHub user ID, repository, and pull request number, which expires after 15 minutes by default.
The cookies and link token are sealed with AES-GCM under a separate configured key from the database encryption key.

On public repositories, Actions logs may be publicly visible.
If this is the case, Discord profile and member data passed to a workflow may appear in those logs if a Maintainer prints it.

### Sign-in permissions

When you sign in with GitHub, we read your GitHub user ID and username.
When you authorize Discord, we read your Discord user ID and profile details such as your username and avatar, and, only when a Rule asks for it, your member record in a Discord server, such as your nickname, roles, and join date.
We do not ask either provider for your email address or password.

## What we do not collect

We do not ask for passwords, payment details, or repository contents.
We do not collect analytics or advertising data, track you across websites, sell your data, or use it to train machine-learning models.

If you email us, we receive your email address so we can reply.

## Who we share data with

We share data only in these situations:

- The repository's workflow and Maintainers receive your link status, your Discord profile, and, when a Rule asks for it, your member record in the requested server.
- GitHub processes data when we use its API to post comments, close and reopen pull requests, rerun the job, and serve the OIDC signing keys we use to verify the Action's token.
- Discord processes data when we use its API to sign you in, read your profile and member record, and refresh your access.
- Our hosting and database providers process data on our behalf so we can run the Service.
- Authorities may receive data if the law or legal process requires it, or if we need to protect rights, safety, or the Service.
- A successor may receive data if we merge, acquire, or transfer the Service.

We do not sell, rent, or share your data for advertising.

## International transfers

GitHub, Discord, and our hosting provider may process data outside your country, including in the United States.
Where the law requires safeguards, we use appropriate mechanisms such as adequacy decisions or standard contractual clauses.
Their own privacy policies also apply.

## Security

We encrypt Discord access and refresh tokens at rest with AES-256-GCM, and bind each encrypted token to its Discord account and token type.
We seal linking cookies and tokens with AES-GCM under a separate configured key and keep encryption keys in Docker secrets outside the database.
The linking cookies are `HttpOnly`, `Secure`, and `SameSite=Lax`, limited to the `/v1/link` path, so browsers only send them over HTTPS.
Linking is bound to your browser with a single-use GitHub state cookie and a matching Discord nonce, and the GitHub account must match the account the link was issued for.
The Action authenticates with a short-lived GitHub OIDC token; we verify its signature, issuer, and audience, and use its repository claims to scope the request.
GitHub App installation tokens are limited to the repository being checked.

No system is perfectly secure.
If we learn of a breach that affects your data, we will notify affected users and any required authorities as the law requires.

## Retention and deletion

The table gives the retention period for each category.
We keep link records and Discord tokens while the link exists, until you ask us to delete them, or until we no longer need them.
If your Discord authorization stops working, we stop using it and report the link as revoked, but we do not delete the record automatically.
If you link a different Discord account, we replace the active link.
The old Discord token record may remain until you ask us to delete it or it is no longer needed.

We keep setup comment and auto-close records while they are needed to operate the Service.
They may remain after a pull request is closed or merged.
We delete them on request or when they are no longer needed.

We keep server logs only as long as needed to run, secure, and debug the Service, then delete or anonymize them.
If we keep backups, we delete them on our normal backup rotation.

Uninstalling the App stops the Service for that repository.
Revoking Discord access makes the stored grant unusable, and revoking GitHub access prevents future GitHub sign-ins for linking.
None of these actions deletes stored records automatically.
Ask us if you want them removed.

## Your rights and choices

You can revoke Arbiterer's Discord access under Authorized Apps in your Discord settings.
You can revoke GitHub access under Authorized GitHub Apps in your GitHub settings.
If you are a Maintainer, you can uninstall the App and remove the Action from your workflow.
Revoking access does not delete data that is already stored.

To access, correct, or delete your data, email [admin@arbiterer.com](mailto:admin@arbiterer.com).
Please include your GitHub username or ID, your Discord username or ID, and the repository involved.
We may ask you to verify that you control the accounts.
We will respond within the time required by law.
We may keep some data if the law requires it or if we need it to establish, exercise, or defend legal claims.
If we cannot fulfill a request, we will explain why.

### GDPR, UK GDPR and other Data Protection Regulations

If you are located in the EEA, the United Kingdom, or other jurisdictions, Data Protection Regulations may grant you the right to restrict or object to processing, receive your data in a portable format, withdraw consent where we rely on it, and complain to your local data protection authority.
You can contact us first, or complain directly.

## Children

Arbiterer is not directed to children under 13, or under the minimum age required in your country.
GitHub and Discord have their own age requirements.
If we learn that we have collected data from a child below the applicable minimum age, we will delete it.

## Changes to this policy

We will update this policy when our data practices change and will change the effective date above.
For significant changes, we will give at least 30 days' notice in a prominent way, such as in the repository and on our Discord server, and we will ask for consent where the law requires it.

## Contact

- Privacy questions and requests: [admin@arbiterer.com](mailto:admin@arbiterer.com)
- Discord: [https://discord.gg/tPHVWBPedt](https://discord.gg/tPHVWBPedt)
- Security issues: follow our [Security Policy](https://github.com/sonolink/arbiterer/blob/main/.github/SECURITY.md) and do not report them in public issues.
- See also our [Terms of Service](https://github.com/sonolink/arbiterer/blob/main/TERMS.md).
