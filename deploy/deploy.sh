#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
values_file="$repo_root/deploy/helm/values.yaml"
sync_now="false"

if [[ "${1:-}" == "--sync-now" ]]; then
  sync_now="true"
fi

if [[ -f "$repo_root/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$repo_root/.env"
  set +a
fi

: "${GHCR_TOKEN:?GHCR_TOKEN is required}"
: "${ARGOCD_TOKEN:?ARGOCD_TOKEN is required}"
: "${ARGOCD_SERVER:?ARGOCD_SERVER is required}"

git_sha="$(git -C "$repo_root" rev-parse --short HEAD)"
git_branch="${GIT_BRANCH:-$(git -C "$repo_root" branch --show-current)}"
ghcr_username="${GHCR_USERNAME:-${GITHUB_ACTOR:-galpodlipnik1}}"
image="ghcr.io/galpodlipnik1/rbf-api:${git_sha}"

if [[ -z "$git_branch" ]]; then
  echo "Unable to determine git branch. Set GIT_BRANCH explicitly." >&2
  exit 1
fi

echo "$GHCR_TOKEN" | docker login ghcr.io -u "$ghcr_username" --password-stdin
docker build -t "$image" "$repo_root/server"
docker push "$image"

sed -i.bak -E "s#(^[[:space:]]*tag:[[:space:]]*).+#\\1\"${git_sha}\"#" "$values_file"
rm -f "${values_file}.bak"

if ! git -C "$repo_root" diff --quiet -- "$values_file"; then
  git -C "$repo_root" add "$values_file"
  git -C "$repo_root" -c user.name='github-actions[bot]' -c user.email='41898282+github-actions[bot]@users.noreply.github.com' commit -m "deploy: rbf-api ${git_sha}"
  git -C "$repo_root" push origin "HEAD:${git_branch}"
fi

if [[ "$sync_now" == "true" ]]; then
  argocd app sync rbf-api --auth-token "$ARGOCD_TOKEN" --server "$ARGOCD_SERVER"
  argocd app wait rbf-api --auth-token "$ARGOCD_TOKEN" --server "$ARGOCD_SERVER" --health --timeout 120
fi
