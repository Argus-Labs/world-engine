
.PHONY: tag tag-cardinal tag-sign tag-nakama

# scripts/tag identifies the most current version based on git tags, makes
# a best-guess about the next logical version number, applies the tag to
# a git commit, and pushed the tag to origin.
tag:
	@scripts/tag.sh

tag-cardinal:
	@$(MAKE) tag TAG_PREFIX=cardinal/v

tag-sign:
	@$(MAKE) tag TAG_PREFIX=sign/v

tag-nakama:
	@$(MAKE) tag TAG_PREFIX=relay/nakama/v
