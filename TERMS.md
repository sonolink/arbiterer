# Arbiterer Terms of Service

Effective date: October 19, 2026

Applies to: the Service, including the linking flow at `arbiterer.sonolink.dev` and the API at `api.arbiterer.sonolink.dev`, the App, and the Action when it connects to the Service.
Related: [Privacy Policy](https://github.com/sonolink/arbiterer/blob/main/PRIVACY.md) · [Security Policy](https://github.com/sonolink/arbiterer/blob/main/.github/SECURITY.md)

## 1. Agreement to these Terms

By installing the App, using the Action with the Service, or linking accounts, you agree to these Terms and our [Privacy Policy](https://github.com/sonolink/arbiterer/blob/main/PRIVACY.md).
If you do not agree, do not use the Service.
These Terms are between you and the SonoLink Development Team ("SonoLink", "we", "us").
If you only open a Contribution without linking accounts, the repository's Maintainer is responsible for telling you that the repository uses Arbiterer.

## 2. Definitions

In these Terms:

- **Service (or hosted Service):** the hosted Arbiterer service, made up of the App, the linking flow at `arbiterer.sonolink.dev`, the API at `api.arbiterer.sonolink.dev`, and the Action when it connects to the Service.
- **App:** the Arbiterer GitHub App.
- **Action:** the `sonolink/arbiterer` GitHub Action, run through GitHub Actions.
- **GitHub:** the GitHub platform and services operated by GitHub, Inc. or its affiliates.
- **Discord:** the Discord platform and services operated by Discord Inc. or its affiliates.
- **GitHub Actions:** GitHub's service for running automated workflows, including the workflow that runs the Action.
- **linking flow:** the account-linking flow at `arbiterer.sonolink.dev`, where you sign in with GitHub and then authorize Discord.
- **account:** a GitHub account or a Discord account, as the context requires.
- **Maintainer:** a person or organization that installs the App or configures the Action on a repository.
- **Contribution:** a pull request submitted to a repository that uses the Service.
- **Contributor:** a person who makes a Contribution, or who links accounts through the Service.
- **Rules:** JavaScript configured by a Maintainer in the Action to decide whether a Contributor passes.
- **check:** the pass, fail, or not-linked result the Action reports for a Contribution.
- **Discord server:** a server (guild) on Discord; "our Discord server" means the community server linked in the Contact section.
- **code:** the open-source code in the Arbiterer repository.
- **Self-hosted copy:** a copy of the code run by someone other than us.
- **Terms:** these Terms of Service.

## 3. Eligibility

You must be at least 13 years old, or the minimum age required in your country, and old enough to use GitHub and Discord under their terms, whichever is higher.
You must comply with GitHub's and Discord's terms and policies.
You must have the legal capacity to agree to these Terms in your jurisdiction, and, if you use the Service for or on behalf of an organization or other legal entity, the authority to bind it.

You may not use the Service if you are located in, organized under the laws of, or ordinarily resident in a country or territory subject to comprehensive sanctions, if you are listed on a restricted-party list, or if your use would violate applicable export control or sanctions laws.

## 4. The Service

Arbiterer links a Contributor's GitHub account to a Discord account for a repository.
When a Contribution is opened, the Action asks our API whether the author is linked, runs the Maintainer's Rules against the Discord data, and reports the result as a check.

On repositories where the App is installed, it may read Contribution metadata, post and edit its own comments, rerun the Action job after linking, and, if the Maintainer enables it, close and reopen Contributions.

Linking shows that the same browser session authorized both accounts.
It does not verify your real-world identity, and Arbiterer is not an identity verification, background-check, or consumer-reporting service.

The Service is free for now.
We may add limits or paid features, or change the Service, with notice where practical.
The code is open source under the Apache License 2.0.
That license governs your use of the code.
These Terms govern your use of the Service.
If you run a Self-hosted copy, these Terms do not apply to it and you are responsible for it.

Arbiterer is not affiliated with or endorsed by GitHub or Discord.
Their own terms and privacy policies apply to your use of their platforms.

## 5. Open source and ownership

The Action and server code are licensed under the Apache License 2.0.
See [LICENSE](https://github.com/sonolink/arbiterer/blob/main/LICENSE).
These Terms do not limit any rights you have under that license for the code.
If these Terms conflict with the license about the code, the license controls.

The Arbiterer name, logo, and the Service's content, apart from your data and the code, belong to SonoLink or its licensors.
You may not use our trademarks without permission, except as allowed by law.
GitHub and Discord names and logos belong to their owners.
If you send us feedback or suggestions, you give us a non-exclusive, worldwide, royalty-free license to use them to improve the Service.
You keep ownership of your GitHub and Discord data.

## 6. Maintainer responsibilities

If you are a Maintainer, you are responsible for the following.

### 6.1 Rules and decisions

You are responsible for the Rules you write, the decisions those Rules make, and compliance with applicable law.
Rules must be lawful and relevant to the repository, including under discrimination, employment, privacy, and consumer protection laws.

### 6.2 Use of Discord data

You must use the Discord data your workflow receives only to decide on Contributions and operate the Service.
Do not store, publish, sell, or share it for advertising, tracking, or any other purpose, except as required by law or to enforce these Terms.
Do not use `resolveMember` to query servers unrelated to your repository or community, or to track Contributors across servers; the Service does not verify that the server you request is related to the repository.
Do not use the Service to read Discord data for anyone other than the Contribution author you are checking.
Do not print Discord profile or member data to workflow logs.
On public repositories, Actions logs are public.

### 6.3 Transparency

Tell Contributors that the repository uses Arbiterer and link to the [Privacy Policy](https://github.com/sonolink/arbiterer/blob/main/PRIVACY.md) and these Terms, for example in your contributing guide or pull request template.

### 6.4 Workflow security

You configure and control the workflow, and its security is your responsibility.
`pull_request_target` is required for the Action to work on Contributions from forks, and it runs with the base repository's permissions and secrets.
Never check out or run untrusted pull request code in a `pull_request_target` workflow.
Do not put secrets in the environment Rules can read.
Grant only the permissions the workflow needs, including `id-token: write` for the Action.
To the maximum extent permitted by law, we are not responsible for losses caused by an incorrectly configured workflow, including exposed secrets or tokens, execution of untrusted code, or overly broad permissions.

### 6.5 Rules security

Rules run in your GitHub Actions runner, not ours, so reviewing and running them safely is your responsibility.
Rules are arbitrary JavaScript with access to `require`, the `github` client, and the workflow environment, including `process.env`.
Only run Rules you trust and review them before use.
Untrusted Rules can access the workflow's token, secrets, and the runner, and §13 limits our liability for losses they cause.

### 6.6 Account security

By using Arbiterer, you agree to secure the App installation, repository settings, workflow files, and any tokens or secrets used to operate or communicate with Arbiterer, and to take full responsibility for any problem, issue, mistake, security breach, or other incident resulting partly or entirely from your failure to do so.

### 6.7 Ending use

Uninstall the App and remove the Action when you no longer want to use the Service, and ask us to delete stored data where needed.

## 7. Contributor responsibilities

If you are a Contributor, you are responsible for the following.

### 7.1 You must

- Link only GitHub and Discord accounts that you own and control.
- Keep your accounts secure.
- Do not link an account for someone else or impersonate anyone.
- Comply with GitHub's and Discord's terms and all applicable laws.

### 7.2 You should know

- You are responsible for activity under your accounts.
- Linking is per repository.
- When you link, the repository's workflow may receive your Discord profile and, if its Rules request it, your member record in a Discord server.
- The Maintainer controls those Rules and what they do with the data.

### 7.3 Leaving the Service

- You can stop using the Service at any time by revoking Arbiterer's access in Discord.
- Revoke GitHub access too if you want to prevent re-linking.
- Ask us to delete your data.

## 8. Acceptable use

You must not:

- link or try to link an account you do not own, or impersonate another person;
- use the Service to harass, discriminate against, track, or profile anyone, or to collect Discord data for any purpose other than gating Contributions;
- use `resolveMember` or the Service to query Discord servers unrelated to the repository or community, or to build a database of Discord memberships;
- access, or try to access, another repository's links or data, or another person's account;
- attack, overload, probe, scan, or circumvent the Service's security, authentication, rate limits, or access controls;
- introduce malicious code, including malicious Rules, or try to take tokens, secrets, or other users' data;
- use the Service to make decisions that have legal or similarly significant effects about individuals, such as decisions about employment, housing, credit, healthcare, or insurance, or in a way that violates the law;
- use the Service in a way that violates GitHub's or Discord's terms or policies;
- misrepresent your affiliation with SonoLink, GitHub, or Discord;
- use the Service in violation of applicable export control or sanctions laws.

## 9. Third-party services

The Service depends on GitHub and Discord.
Their availability, APIs, terms, and privacy policies are outside our control.
We are not responsible for their outages, changes, data practices, or decisions.
The Service may link to third-party sites, and we are not responsible for them.

## 10. Privacy

Our [Privacy Policy](https://github.com/sonolink/arbiterer/blob/main/PRIVACY.md) explains what data we collect, and how and why we use it.
It is incorporated into these Terms by reference.
If these Terms conflict with the Privacy Policy about personal data, the Privacy Policy prevails.

Maintainers are responsible for providing any notices and getting any consents required for their use of Contributor data.
Do not put sensitive personal data in Rules, comments, or other inputs.
Do not use the Service to process special categories of personal data unless you have a lawful basis and appropriate safeguards.

## 11. Availability, changes, and termination

We run the Service on a best-effort basis.
We do not guarantee availability, uptime, or that checks will always run or complete.
If our server, GitHub, or Discord is unavailable, checks may fail and Contributions that the Service closed may stay closed until things recover.
You shall not rely on Arbiterer as the only safeguard for anything important, including security, compliance, or access control.

We do not provide a support contract or service-level agreement.
Support is best effort through the repository and our Discord server.

We may change, pause, or discontinue the Service or any feature, with notice where practical.
You can stop using the Service at any time as described in §6.7 and §7.
We may suspend or terminate your access, or remove content, if you violate these Terms, if your use puts the Service or others at risk, or if the law requires it.
We will give notice where practical, except where the law or the risk requires immediate action.
If you are subject to sanctions or your use would violate export control or sanctions laws, we may suspend or terminate your access immediately, without notice and without compensation.
We are not liable for suspension or termination except as required by law.
Sections 5, 8, 10, 12, 13, 14, 15, and 17 survive termination.

When your access ends, we handle stored data as described in the Privacy Policy.
Termination does not by itself delete stored link records; you can ask us to delete them.

## 12. Disclaimers

**The Service is provided "as is" and "as available", without warranties of any kind, express or implied, to the maximum extent permitted by law.**
We disclaim implied warranties of merchantability, fitness for a particular purpose, title, and non-infringement.

We do not warrant that the Service will be uninterrupted, timely, secure, error-free, or free of harmful components, or that data will not be lost.
We do not endorse any Maintainer's Rules or decisions, and Maintainers are solely responsible for them.
Some jurisdictions do not allow some of these exclusions, so parts of this section may not apply to you.

## 13. Limitation of liability

**To the maximum extent permitted by law, our liability is limited as follows.**
SonoLink and its members, officers, and service providers will not be liable for indirect, incidental, special, consequential, exemplary, or punitive damages, or for lost profits, revenue, data, goodwill, or business interruption, arising out of or related to the Service or these Terms.

This includes losses caused by a Maintainer's Rules, Contributions that were blocked, closed, or allowed through, and outages or changes of GitHub, Discord, or the Service.

To the maximum extent permitted by law, our total liability for all claims arising out of or related to the Service or these Terms will not exceed whichever is greater: the amount you paid us for the Service in the 12 months before the claim, or USD 100.

Nothing in these Terms limits liability that cannot be limited by law, including liability for fraud, intent, gross negligence, death or personal injury, or mandatory consumer rights.
The Service is free, and you use it at your own risk.

## 14. Indemnity

This section applies only if you are a Maintainer or you use the Service for an organization.
To the maximum extent permitted by law, you will indemnify, defend, and hold harmless SonoLink and its members, officers, and service providers from claims, damages, losses, liabilities, costs, and expenses, including reasonable legal fees, arising out of or related to:

- your use of the Service;
- your Rules or your use of Discord data;
- your violation of these Terms, applicable law, or third-party rights;
- your content or conduct.

Individual Contributors acting for themselves do not indemnify us under these Terms.
We may take over the defense of a matter subject to indemnity, and you will cooperate.
This section does not apply to the extent a claim arises from our own willful misconduct or gross negligence.

## 15. Governing law and disputes

These Terms are governed by the laws of the Netherlands, without regard to conflict-of-law rules.
The competent courts of the Netherlands have exclusive jurisdiction, except where mandatory law gives you the right to bring a claim elsewhere.
If you are a consumer, you keep the protection of the mandatory laws of your country of residence.

Before filing a claim, please contact us at [admin@arbiterer.com](mailto:admin@arbiterer.com) so we can try to resolve the issue informally.
Nothing in this section prevents either party from seeking interim, injunctive, or other equitable relief.

## 16. Changes to these Terms

We may update these Terms.
The effective date at the top shows the latest version.
We will give at least 30 days' notice of material changes in a prominent way, such as in the repository and on our Discord server, before they take effect.
If you continue to use the Service after the effective date, you accept the updated Terms.
If you do not agree, stop using the Service.
Changes do not apply retroactively, except for changes required by law or that are retroactive by nature, such as sanctions or export control measures.

## 17. General

These Terms and the Privacy Policy are the whole agreement between you and us about the Service.
The Apache License 2.0 governs the code and is separate.
If a provision is found unenforceable, the rest stays in effect and the unenforceable provision will be enforced to the maximum extent permitted.
Our failure to enforce a provision is not a waiver.
You may not assign these Terms without our written consent.
We may assign them in connection with a merger, acquisition, reorganization, or transfer of the Service, subject to applicable law.
There are no third-party beneficiaries, except for SonoLink's members, officers, and service providers for §13 and §14.

## Contact

- Questions about these Terms: [admin@arbiterer.com](mailto:admin@arbiterer.com) or our [Discord server](https://discord.gg/tPHVWBPedt).
- Security issues: follow the [Security Policy](https://github.com/sonolink/arbiterer/blob/main/.github/SECURITY.md).
- Do not report them in public issues.
- Privacy requests: see the [Privacy Policy](https://github.com/sonolink/arbiterer/blob/main/PRIVACY.md) or email [admin@arbiterer.com](mailto:admin@arbiterer.com).
