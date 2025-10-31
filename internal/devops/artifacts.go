package devops

import (
	"fmt"
	"strings"
)

// PublishArtifact publishes an artifact to Azure DevOps.
//
// If folder is not an empty string, the artifact will be published inside of
// the specified folder. If folder is an empty string, the artifact will be
// published at the root level.
//
// Returns an error if the artifact name or source is empty.
//
// This function DOES NOT BLOCK, it only produces the logging command to upload
// the artifact. The actual upload is handled by Azure DevOps after the command
// is emitted. The uploaded file MUST be kept available at the source path until
// the end of the job.
//
// Expectations on the caller:
//
//   - Check that this function is only called when running in Azure DevOps
//     context.
//   - Ensure that the source path exists, is accessible, and is a regular file.
//   - Handle any errors returned by this function appropriately.
func PublishArtifact(folder string, name string, source string) error {
	if name == "" {
		return fmt.Errorf("artifact name cannot be empty")
	}
	if source == "" {
		return fmt.Errorf("artifact source cannot be empty")
	}

	// Delete leading slash from folder, if present
	folder = strings.TrimPrefix(folder, "/")

	if folder != "" {
		// Publish the artifact in the specified folder
		uploadArtifactInFolder(folder, name, source)
	} else {
		// Publish the artifact at the root level
		uploadArtifact(name, source)
	}

	// Publish the artifact
	return nil
}

// uploadArtifactInFolder produces the logging command to upload an artifact
// inside of a folder to Azure DevOps.
func uploadArtifactInFolder(folder string, name string, path string) {
	fmt.Fprintf(realStdOut, "##vso[artifact.upload containerfolder=%s;artifactname=%s]%s", folder, name, path)
}

// uploadArtifact produces the logging command to upload an artifact to
// Azure DevOps.
func uploadArtifact(name string, path string) {
	fmt.Fprintf(realStdOut, "##vso[artifact.upload artifactname=%s]%s", name, path)
}
