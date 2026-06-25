# justfile — local release tooling for terraform-provider-torii (runs in Docker)
#
# The Go/goreleaser/tfplugindocs toolchain runs inside a container, so the host
# only needs Docker + git + (optionally) gh. Git plumbing (version bump, commit,
# tag, push) stays on the host because it needs your push credentials.
#
# Publishing model: you do NOT push to the Terraform Registry directly. The
# registry is connected to the GitHub repo (toriigateorg/terraform-provider-torii)
# and auto-ingests each GitHub *release* whose tag is vX.Y.Z. This justfile builds,
# GPG-signs and creates that release via goreleaser; the registry pulls from there.
#
# Version source of truth: the VERSION file at the repo root (plain X.Y.Z, no "v").
# `just release` bumps it, commits, and creates the annotated tag vX.Y.Z.
#
# Required tooling on the HOST: just, docker, git, gpg, gh (or a GITHUB_TOKEN).
# gpg is needed only to export the signing key; signing itself happens in the
# container against a throwaway keyring, so it never contends with your host
# gpg-agent for the keyring lock.
#
# Required environment / one-time setup:
#   GPG_FINGERPRINT  40-char hex fingerprint of the signing key. Get it cleanly with:
#                    gpg --list-secret-keys --with-colons <key-email> | awk -F: '/^fpr:/{print $10; exit}'
#                    The key is exported from the host keyring and imported into an
#                    isolated GNUPGHOME inside the container (cleaned up after).
#   GPG_PASSWORD     passphrase for the signing key, if it has one (optional; loopback pinentry).
#   GITHUB_TOKEN     token with repo scope for creating the release. If unset, we fall
#                    back to `gh auth token`, so `gh auth login` on the host is enough.
#   ONE-TIME: upload the ASCII-armored GPG *public* key to the registry namespace
#            settings (registry.terraform.io -> toriigateorg -> Settings -> GPG Keys).
#            Without it the registry rejects the signature.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Container image carrying Go + goreleaser. Pin to a concrete tag for reproducible
# releases (e.g. goreleaser/goreleaser:v2.5.0).
image := "goreleaser/goreleaser:latest"

# Common `docker run` flags. Runs as the host user so build outputs (dist/, docs/)
# and any go.mod/go.sum changes are owned by you, not root. All caches live under
# ./.cache (bind-mounted, host-owned) to avoid named-volume permission issues.
_run := "docker run --rm" \
    + " --user \"$(id -u):$(id -g)\"" \
    + " -e HOME=/app/.cache/home" \
    + " -e GOPATH=/app/.cache/go" \
    + " -e GOMODCACHE=/app/.cache/go/pkg/mod" \
    + " -e GOCACHE=/app/.cache/go-build" \
    + " -v \"$(pwd)\":/app -w /app"

# List available recipes.
default:
    @just --list

# Print the current version (from the VERSION file).
version:
    @cat VERSION

# Build a snapshot release in Docker (no GPG, no publish) to validate the config/build.
release-snapshot:
    mkdir -p .cache/home
    {{ _run }} {{ image }} release --snapshot --clean --skip=publish,sign

# (Re)generate the docs/ folder from the provider schema and examples/ (in Docker).
docs:
    mkdir -p .cache/home
    {{ _run }} --entrypoint go {{ image }} \
        run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest \
        generate --provider-name torii

# Cut a release: bump version (patch|minor|major), commit, tag vX.Y.Z, push, then publish.
release KIND="patch":
    #!/usr/bin/env bash
    set -euo pipefail

    # --- preflight: git state (host) ---
    branch="$(git rev-parse --abbrev-ref HEAD)"
    if [[ "${branch}" != "main" ]]; then
        echo "ERROR: releases must be cut from 'main' (currently on '${branch}')." >&2
        exit 1
    fi
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "ERROR: working tree is not clean. Commit or stash changes before releasing." >&2
        git status --short >&2
        exit 1
    fi

    # --- fail fast on release credentials BEFORE we tag/push (publish re-validates fully) ---
    if [[ -z "${GPG_FINGERPRINT:-}" ]]; then
        echo "ERROR: GPG_FINGERPRINT is unset. Export it before releasing:" >&2
        echo "       export GPG_FINGERPRINT=\$(gpg --list-secret-keys --with-colons <key-email> | awk -F: '/^fpr:/{print \$10; exit}')" >&2
        exit 1
    fi
    if [[ -z "${GITHUB_TOKEN:-}" ]] && ! gh auth token >/dev/null 2>&1; then
        echo "ERROR: no GITHUB_TOKEN and 'gh auth token' failed. Run 'gh auth login' or export GITHUB_TOKEN." >&2
        exit 1
    fi

    # --- bump version ---
    kind="{{ KIND }}"
    cur="$(cat VERSION)"
    IFS='.' read -r major minor patch <<< "${cur}"
    case "${kind}" in
        major) major=$((major + 1)); minor=0; patch=0 ;;
        minor) minor=$((minor + 1)); patch=0 ;;
        patch) patch=$((patch + 1)) ;;
        *) echo "ERROR: KIND must be patch|minor|major (got '${kind}')." >&2; exit 1 ;;
    esac
    new="${major}.${minor}.${patch}"
    tag="v${new}"
    echo ">> bumping ${cur} -> ${new} (tag ${tag})"

    # --- commit, tag, push (host) ---
    echo "${new}" > VERSION
    git add VERSION
    git commit -m "release: ${tag}"
    git tag -a "${tag}" -m "${tag}"
    git push origin "${branch}"
    git push origin "${tag}"

    # --- build, sign, publish the tag now on HEAD ---
    just publish

# Build, GPG-sign and publish the release for the tag on HEAD (no version bump).
# Use this to retry publishing after a failed `release`, or to (re)publish an
# existing tag. HEAD must be on a vX.Y.Z tag.
publish:
    #!/usr/bin/env bash
    set -euo pipefail

    # --- preflight: environment ---
    if [[ -z "${GPG_FINGERPRINT:-}" ]]; then
        echo "ERROR: GPG_FINGERPRINT is unset. Set it to the 40-char hex fingerprint of your signing key:" >&2
        echo "       export GPG_FINGERPRINT=\$(gpg --list-secret-keys --with-colons <key-email> | awk -F: '/^fpr:/{print \$10; exit}')" >&2
        exit 1
    fi
    # Tolerate a pasted "Key fingerprint = AAAA BBBB ..." label and/or spaces.
    GPG_FINGERPRINT="${GPG_FINGERPRINT##*=}"
    GPG_FINGERPRINT="${GPG_FINGERPRINT//[[:space:]]/}"
    if ! [[ "${GPG_FINGERPRINT}" =~ ^[0-9A-Fa-f]{40}$ ]]; then
        echo "ERROR: GPG_FINGERPRINT is not a 40-char hex fingerprint: '${GPG_FINGERPRINT}'" >&2
        echo "       Get it with: gpg --list-secret-keys --with-colons <key-email> | awk -F: '/^fpr:/{print \$10; exit}'" >&2
        exit 1
    fi
    export GPG_FINGERPRINT
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        echo ">> GITHUB_TOKEN unset; falling back to 'gh auth token'..." >&2
        if ! GITHUB_TOKEN="$(gh auth token 2>/dev/null)" || [[ -z "${GITHUB_TOKEN}" ]]; then
            echo "ERROR: no GITHUB_TOKEN and 'gh auth token' failed. Run 'gh auth login' or export GITHUB_TOKEN." >&2
            exit 1
        fi
    fi
    # Passphrase-less keys are fine; default to empty so goreleaser's template resolves.
    export GITHUB_TOKEN GPG_PASSWORD="${GPG_PASSWORD:-}"

    # --- export the signing key from the host keyring ---
    # Done on the host (not by mounting ~/.gnupg) so the container never shares the
    # keyring lock with your host gpg-agent. Only the one signing key is exported,
    # and it is held in an env var (never written to the bind-mounted repo).
    export_args=(--export-secret-keys --armor)
    if [[ -n "${GPG_PASSWORD}" ]]; then
        export_args+=(--pinentry-mode loopback --passphrase "${GPG_PASSWORD}")
    fi
    GPG_PRIVATE_KEY="$(gpg "${export_args[@]}" "${GPG_FINGERPRINT}")"
    if [[ -z "${GPG_PRIVATE_KEY}" ]]; then
        echo "ERROR: failed to export secret key ${GPG_FINGERPRINT} from the host keyring." >&2
        exit 1
    fi
    export GPG_PRIVATE_KEY HOST_UID="$(id -u)" HOST_GID="$(id -g)"

    # --- build, sign, publish in Docker ---
    # Runs as root so we can install gnupg (the goreleaser image ships no working
    # gpg-agent, and gpg 2.x needs the agent to use the secret key). The keyring lives
    # in a container-internal GNUPGHOME (/root/.gnupg, NOT bind-mounted), so the private
    # key never lands on the host disk and there is no host agent to contend with. Go
    # caches use named volumes to avoid colliding with the non-root recipes' ./.cache.
    # dist/ is chown'd back to you at the end since the container writes it as root.
    docker run --rm \
        -e GPG_FINGERPRINT -e GPG_PASSWORD -e GPG_PRIVATE_KEY -e GITHUB_TOKEN \
        -e HOST_UID -e HOST_GID \
        -e GNUPGHOME=/root/.gnupg \
        -e GOPATH=/go -e GOMODCACHE=/go/pkg/mod -e GOCACHE=/root/.cache/go-build \
        -v "$(pwd)":/app -w /app \
        -v torii-go-mod:/go/pkg/mod \
        -v torii-go-build:/root/.cache/go-build \
        --entrypoint sh \
        {{ image }} -ec '
            command -v gpg-agent >/dev/null 2>&1 || apk add --no-cache gnupg >/dev/null
            git config --global --add safe.directory /app
            mkdir -p "$GNUPGHOME"; chmod 700 "$GNUPGHOME"
            printf "%s" "$GPG_PRIVATE_KEY" | gpg --batch --import
            goreleaser release --clean
            chown -R "$HOST_UID:$HOST_GID" dist go.mod go.sum 2>/dev/null || true
        '
