// Package github talks to GitHub: it verifies the OIDC tokens GitHub Actions
// issues to workflow runs, drives the OAuth login flow for linking a
// contributor's account, and authenticates as the Arbiterer GitHub App to
// act on repositories where it is installed.
package github
