# Releasing (Terraform Registry)

The provider is published on the public Terraform Registry as
`CanuteTheGreat/rustinion`. The registry only reads GitHub, so the flow is:

1. **Source of truth** is `terraform-provider-rustinion/` in the Rustinion
   monorepo on Forgejo (`git.canutethegreat.com/CanuteTheGreat/rustinion`).
2. **Sync**: the monorepo's `ci-cd.yml` job `publish-terraform-provider`
   (main only, after `live-e2e` ran the full TF_ACC suite and
   `terraform-provider-goreleaser-check` passed) copies this directory into
   the standalone Forgejo repo `CanuteTheGreat/terraform-provider-rustinion`
   (one commit per change, message names the monorepo commit) using that
   repo's write deploy key (`TF_PROVIDER_DEPLOY_KEY` secret).
3. **Tag**: when `VERSION` names a version that has no `v<VERSION>` tag yet,
   the same job tags the synced commit. Bumping `VERSION` is how a release
   is made; nothing else is needed.
4. **Mirror**: the Forgejo repo push-mirrors (sync on commit) to
   `github.com/CanuteTheGreat/terraform-provider-rustinion` (public).
   Never push to the GitHub copy directly: the mirror force-syncs.
5. **Build + sign**: on GitHub, `.github/workflows/release.yml` runs
   goreleaser on the tag: every OS/arch zip, `SHA256SUMS`, the registry
   manifest, and a detached GPG signature `SHA256SUMS.sig`, then verifies
   the signature. Secrets on the GitHub repo: `GPG_PRIVATE_KEY`,
   `PASSPHRASE`.
6. **Registry**: HashiCorp's GitHub App picks the release up within minutes.

The Forgejo copy has Actions disabled, so `.github/workflows/release.yml`
only ever runs on GitHub.

## Signing key

RSA 4096, no expiry, `Errant Solutions (Rustinion Terraform provider releases)
<terraform-provider@errant.solutions>`, fingerprint
`4F1D6FD94A2232E158AE21DD642AE4F04B94C1F1`. The public key must be listed
under the registry namespace's Signing Keys (registry.terraform.io, Settings
-> Signing Keys); `terraform init` checks every download against it. A copy
of the public key is in this directory as `signing-key.asc`.

## Checks that run on every push (monorepo CI)

- `terraform-provider-goreleaser-check`: `go vet`, `go test`,
  `goreleaser check`, a full `--snapshot` build.
- `live-e2e`: the TF_ACC acceptance suite against a real server.
