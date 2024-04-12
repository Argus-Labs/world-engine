#!/bin/bash

# This tag.sh script: 
# 1. Uses TAG_PREFIX to identify the most recent version of a particular project
# 2. Increments the patch number as a best-guess of the new version to release.
# 3. Prompts the user for a tag to use (the best-guess is used if nothing is given)
# 4. Applies the new tag to the current git commit
# 5. Pushes the new tag to origin

# Ensure the script exits if any command fails
set -e

# Set the TAG_PREFIX from an environment variable, default to 'alpha' if not set
if [ -z "$TAG_PREFIX" ]; then
  echo "Error: TAG_PREFIX is not set." >&2
  exit 1
fi


# Fetch all tags and filter by prefix
git fetch --tags
current_tag=$(git tag -l "$TAG_PREFIX*" | sort -V | tail -n1)
if [ -z "$current_tag" ]; then
    echo "No tags found with prefix '$TAG_PREFIX'. Starting from zero."
    major=0
    minor=0
    patch=0
else
    echo "Current tag is: $current_tag"
    # Remove prefix and split the version number
    version_number=${current_tag#$TAG_PREFIX}
    IFS='.' read -ra ADDR <<< "$version_number"
    major=${ADDR[0]}
    minor=${ADDR[1]}
    patch=${ADDR[2]}
    let "patch+=1"  # Increment the patch version by default
fi

## REMOVE THIS SUGGGESTIONS VERSION. JUST SAY DEFAULT
# Suggest the new version
new_version="$major.$minor.$patch"
echo "Suggested new version: $new_version"

# Ask the user for the new version components
read -p "Enter new version [$new_version]: " input_version
input_version=${input_version:-$new_version}  # Use the suggested version if user input is empty

# Create the new tag with prefix
new_tag="$TAG_PREFIX$input_version"
#git tag $new_tag

# Push the new tag to the remote repository
#git push origin $new_tag

echo "New tag $new_tag pushed to origin."

