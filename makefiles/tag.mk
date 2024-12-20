# Use PWD instead of CURDIR for better cross-platform compatibility
ROOT_DIR := $(shell pwd)

.PHONY: tag tag-cardinal tag-sign tag-nakama tag-component

# scripts/tag identifies the most current version based on git tags, makes
# a best-guess about the next logical version number, applies the tag to
# a git commit, and pushed the tag to origin.
tag:
	@bash "$(ROOT_DIR)/scripts/tag.sh"

# Generic component tagging target
tag-component:
	@test -n "$(COMPONENT)" || (echo "Error: COMPONENT variable is required" && exit 1)
	@$(MAKE) tag TAG_PREFIX=v COMPONENT=$(COMPONENT)

# Legacy format targets that create both old and new format tags
tag-cardinal:
	@$(MAKE) tag TAG_PREFIX=cardinal/v
	@$(MAKE) tag-component COMPONENT=cardinal

tag-sign:
	@$(MAKE) tag TAG_PREFIX=sign/v
	@$(MAKE) tag-component COMPONENT=sign

tag-nakama:
	@$(MAKE) tag TAG_PREFIX=relay/nakama/v
	@$(MAKE) tag-component COMPONENT=nakama
