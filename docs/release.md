# AWS Load Balancer Controller Release Process

## Create the Release Commit

Run `hack/set-version` to set the new version number and commit the resulting changes. 
This is called the "release commit".

## Merge the Release Commit

Create a pull request with the release commit. Get it reviewed and merged to `main`.

Upon merge to `main`, GitHub Actions will create a release tag for the new release.

If the release is a ".0-beta.1" release, GitHub Actions will also create a release branch
for the minor version.

(Remaining steps in process yet to be documented.)