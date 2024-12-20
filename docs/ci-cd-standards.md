# CI/CD Standards

## Versioning
- All services use semantic versioning
- Tags follow pattern: v{major}.{minor}.{patch}
- Component names included in image tags

## Multi-architecture Support
- All services support linux/amd64,linux/arm64
- Uses Docker Buildx for efficient multi-arch builds
- Platform-specific optimizations in Dockerfiles

## Container Image Tagging
- Format: ghcr.io/{org}/{repo}-{component}:{version}
- Tags include: full version, major.minor, and commit SHA

## Implementation Details

### Tag Management
- Use `make tag-component COMPONENT=<component>` for new releases
- Legacy format tags are maintained for backward compatibility
- Components: nakama, evm, cardinal, sign

### Docker Build Process
- Shared workflow template in `.github/workflows/templates/docker-build.yaml`
- Automated builds triggered by version tags
- Multi-architecture images built using Docker Buildx
- Images pushed to GitHub Container Registry (ghcr.io)

### Release Process
1. Create a new version tag:
   ```bash
   make tag-component COMPONENT=<component>
   ```
2. CI/CD pipeline automatically:
   - Builds multi-arch images
   - Tags images appropriately
   - Pushes to container registry

### Examples

Tag Format:
```
v1.2.3                    # Full version tag
ghcr.io/org/repo-nakama:1.2.3  # Full version image
ghcr.io/org/repo-nakama:1.2    # Minor version image
ghcr.io/org/repo-nakama:sha-abc123 # Commit SHA image
```

Multi-arch Support:
```dockerfile
FROM --platform=$TARGETPLATFORM base-image:tag
```

### Migration Notes
- Legacy tag format (`component/v*.*.*`) remains supported
- New standardized format (`v*.*.*`) preferred for all new releases
- Component information moved from tag prefix to image name suffix
