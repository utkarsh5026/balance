# GitHub Actions Workflows

This directory contains GitHub Actions workflows for the Balance load balancer project. All workflows are configured to run automatically on relevant events.

## Workflows Overview

### 1. CI Workflow (`ci.yml`)

**Purpose:** Continuous Integration - ensures code quality and functionality

**Triggers:**
- Push to `main` branch
- Pull requests to `main` branch

**Jobs:**
- **Test**: Runs all unit tests on Linux, Windows, and macOS with coverage reporting
- **Race Detection**: Runs tests with Go's race detector to catch concurrency issues
- **Lint**: Runs code quality checks (`go vet`, `gofmt`, `golangci-lint`)
- **Build**: Verifies the project builds successfully on all platforms

**Coverage Reporting:**
- Coverage reports are automatically uploaded to Codecov
- Set `CODECOV_TOKEN` in repository secrets for private repos

**Status Badge:**
```markdown
[![CI](https://github.com/YOUR_USERNAME/balance/workflows/CI/badge.svg)](https://github.com/YOUR_USERNAME/balance/actions/workflows/ci.yml)
```

---

### 2. Benchmark Workflow (`benchmark.yml`)

**Purpose:** Performance testing and regression detection

**Triggers:**
- Push to `main` branch
- Pull requests to `main` branch
- Manual trigger via workflow_dispatch

**Jobs:**
- **Performance Benchmark**: Runs all benchmark tests in `pkg/balance/`
- **Benchmark Comparison**: Compares PR performance against base branch

**Features:**
- Stores benchmark results as artifacts (30-day retention)
- Posts benchmark results as PR comments
- Alerts if performance degrades by >150%
- Uses `benchstat` for statistical comparison

**Manual Trigger:**
```bash
# Via GitHub UI: Actions → Benchmark → Run workflow
```

---

### 3. Release Workflow (`release.yml`)

**Purpose:** Automated release creation and binary distribution

**Triggers:**
- Git tags matching `v*.*.*` (e.g., `v1.0.0`, `v2.1.3`)

**Build Matrix:**
- **Linux**: amd64, arm64
- **Windows**: amd64
- **macOS**: amd64, arm64

**Features:**
- Uses GoReleaser for streamlined releases
- Generates SHA256 checksums for all binaries
- Creates GitHub release with auto-generated changelog
- Includes example configs in release archives
- Fallback manual build if GoReleaser fails

**Creating a Release:**
```bash
# Create and push a version tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0

# The workflow will automatically:
# 1. Run all tests
# 2. Build binaries for all platforms
# 3. Create GitHub release with artifacts
```

**GoReleaser Configuration:**
See [`.goreleaser.yml`](../../.goreleaser.yml) for detailed release settings.

---

### 4. Security Workflow (`security.yml`)

**Purpose:** Security scanning and vulnerability detection

**Triggers:**
- Push to `main` branch
- Pull requests to `main` branch
- Weekly schedule (Sundays at midnight UTC)
- Manual trigger via workflow_dispatch

**Jobs:**

1. **Dependency Scan**
   - Verifies go.mod integrity
   - Checks for known vulnerabilities with `govulncheck`
   - Ensures dependencies are properly tracked

2. **Gosec** (Go Security Checker)
   - Static security analysis
   - Detects common security issues
   - Uploads results to GitHub Security tab

3. **CodeQL Analysis**
   - Deep semantic code analysis
   - Security and quality queries
   - Integration with GitHub Advanced Security

4. **License Compliance**
   - Scans all dependencies for license information
   - Generates license report artifact

5. **Trivy Scanner**
   - Vulnerability scanning for dependencies
   - Focuses on CRITICAL and HIGH severity issues

6. **Staticcheck**
   - Advanced static analysis
   - Detects bugs, performance issues, and stylistic problems

7. **Nancy**
   - Sonatype OSS Index integration
   - Additional dependency vulnerability checking

**Viewing Results:**
- Security findings appear in the "Security" tab of your repository
- SARIF reports are uploaded for supported scanners
- License reports are available as workflow artifacts

---

## Setup Requirements

### Required Secrets

1. **CODECOV_TOKEN** (Optional - for private repos)
   - Get token from [codecov.io](https://codecov.io)
   - Add to: Settings → Secrets and variables → Actions → New repository secret

2. **GITHUB_TOKEN** (Automatic)
   - Automatically provided by GitHub Actions
   - No manual setup required

### Repository Settings

1. **Enable Actions:**
   - Settings → Actions → General → Allow all actions

2. **Workflow Permissions:**
   - Settings → Actions → General → Workflow permissions
   - Select "Read and write permissions"
   - Check "Allow GitHub Actions to create and approve pull requests"

3. **Branch Protection** (Recommended):
   - Settings → Branches → Add rule for `main`
   - Require status checks: CI / Test, CI / Lint, CI / Build
   - Require branches to be up to date before merging

### Security Tab Setup

1. **Enable Dependency Graph:**
   - Settings → Code security and analysis
   - Enable "Dependency graph"

2. **Enable Dependabot:**
   - Enable "Dependabot alerts"
   - Enable "Dependabot security updates"

3. **Enable Code Scanning:**
   - Enable "Code scanning"
   - CodeQL analysis runs automatically via security.yml

---

## Workflow Status Badges

Add these badges to your main README.md:

```markdown
[![CI](https://github.com/YOUR_USERNAME/balance/workflows/CI/badge.svg)](https://github.com/YOUR_USERNAME/balance/actions/workflows/ci.yml)
[![Security](https://github.com/YOUR_USERNAME/balance/workflows/Security/badge.svg)](https://github.com/YOUR_USERNAME/balance/actions/workflows/security.yml)
[![codecov](https://codecov.io/gh/YOUR_USERNAME/balance/branch/main/graph/badge.svg)](https://codecov.io/gh/YOUR_USERNAME/balance)
```

---

## Maintenance

### Updating Go Version

When updating to a new Go version:

1. Update in all workflow files:
   - `ci.yml`: `go-version: '1.XX.x'`
   - `benchmark.yml`: `go-version: '1.XX.x'`
   - `release.yml`: `go-version: '1.XX.x'`
   - `security.yml`: `go-version: '1.XX.x'`

2. Update in project:
   - `go.mod`: `go 1.XX`

3. Test locally before committing

### Workflow Debugging

View detailed logs:
1. Go to Actions tab
2. Click on the workflow run
3. Click on the job name
4. Expand step details

Enable debug logging:
```bash
# Set repository secrets:
ACTIONS_RUNNER_DEBUG=true
ACTIONS_STEP_DEBUG=true
```

---

## Cost Considerations

**GitHub Actions Minutes:**
- Public repos: Unlimited free minutes
- Private repos: 2,000 free minutes/month (Team), 3,000 (Enterprise)

**Optimizations in place:**
- Go module caching (speeds up builds)
- Conditional job execution
- Parallel job execution where possible
- Short benchmark runs in CI (3s per benchmark)

**Storage:**
- Workflow artifacts retained for 30 days (configurable)
- Coverage reports don't count toward storage limits

---

## Troubleshooting

### Common Issues

1. **golangci-lint timeout**
   - Increase timeout in `ci.yml`: `args: --timeout=10m`

2. **Benchmark comparison fails**
   - Expected on first PR - base branch needs benchmark history

3. **CodeQL analysis fails**
   - Ensure repository has "Code scanning" enabled
   - Check that `GITHUB_TOKEN` has security-events: write permission

4. **Release workflow doesn't trigger**
   - Ensure tag matches pattern `v*.*.*`
   - Check that tag is pushed: `git push origin v1.0.0`

### Getting Help

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GoReleaser Documentation](https://goreleaser.com)
- [Project Issues](https://github.com/YOUR_USERNAME/balance/issues)

---

## Contributing

When contributing to workflows:

1. Test workflow changes in a fork first
2. Use `workflow_dispatch` for manual testing
3. Keep workflows simple and maintainable
4. Document any new secrets or requirements
5. Update this README with changes

---

**Last Updated:** 2025-01-21
