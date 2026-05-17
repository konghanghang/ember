const DEFAULT_REPOSITORY = 'konghanghang/ember'
const DEFAULT_REPOSITORY_URL = `https://github.com/${DEFAULT_REPOSITORY}`

/**
 * normalizeRepositoryUrl turns either `owner/repo` or an absolute URL into the
 * canonical GitHub repository URL used by source links.
 */
export function normalizeRepositoryUrl(value: string | undefined): string {
  const normalized = value?.trim().replace(/\/+$/, '')

  if (!normalized) {
    return DEFAULT_REPOSITORY_URL
  }

  if (/^https?:\/\//i.test(normalized)) {
    return normalized
  }

  return `https://github.com/${normalized}`
}

/**
 * normalizeCommitSha accepts only commit-like hex strings so accidental build
 * placeholders do not become broken GitHub commit links.
 */
export function normalizeCommitSha(value: string | undefined): string {
  const normalized = value?.trim() ?? ''
  return /^[0-9a-f]{7,40}$/i.test(normalized) ? normalized : ''
}

const repositoryUrl = normalizeRepositoryUrl(
  import.meta.env.VITE_GITHUB_REPOSITORY_URL || import.meta.env.VITE_GITHUB_REPOSITORY
)
const commitSha = normalizeCommitSha(import.meta.env.VITE_GIT_COMMIT_SHA)

export const buildInfo = {
  repositoryUrl,
  commitSha,
  shortCommitSha: commitSha ? commitSha.slice(0, 7) : 'dev',
  commitUrl: commitSha ? `${repositoryUrl}/commit/${commitSha}` : repositoryUrl,
}
