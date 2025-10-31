package artifacts

type ArtifactBroker interface {
	// Allows the test case to publish a log file located at the given path to
	// the output directory given to the runnable, when provided. If a log
	// directory was not provided to the runnable, this function will be a
	// no-op.
	//
	// The path must be an absolute path to a file on the local filesystem. When
	// successful, the log file will be copied to the output directory and
	// renamed to `<log_dir>/<test_case_name>/<name>`. If multiple files are
	// published with the same name, only the last one will be kept. The
	// `<name>` may be a path, in which case the directories will be created as
	// needed.
	//
	// If the path does not resolve to a file, or any other error occurs, the
	// test will be marked as an error.
	PublishLogFile(name string, source string)

	// Allows the test case to publish an arbitrary artifact located at the
	// given path to the artifact output directory, when provided. If an output
	// directory was not provided to the runnable, this function will be a
	// no-op.
	//
	// Inputs:
	//   - directory: An optional sub-directory within the output directory where
	//     the artifact will be stored. If empty, the artifact will be stored at
	//     the root of the folder.
	//   - source: The path to the file to be published as an artifact. It may be
	//     a relative path, but absolute paths are recommended.
	//
	// When an output directory is provided to the runnable, the artifact will be
	// copied to `<output_dir>/<directory>/<filename>`, where `<filename>` is
	// derived from the `source` path. If multiple artifacts are published with
	// the same name, only the last one will be kept.
	//
	// If the path does not resolve to a file, or any other error occurs, the
	// test will be marked as an error.
	PublishArtifact(directory string, source string)

	// Allows test cases to publish an arbitrary artifact located at the given
	// path to Azure DevOps Artifacts.
	//
	// Inputs:
	//   - name: The name of the artifact to be published.
	//   - directory: An optional folder within Azure DevOps Artifacts where the
	//     artifact will be stored. If empty, the artifact will be stored at the
	//     root level.
	//   - source: The path to the file to be published as an artifact. It may be
	//     a relative path, but absolute paths are recommended.
	//
	// If the path does not resolve to a file, or any other error occurs, the
	// test will be marked as an error.
	//
	// Note: implementations do NOT block on this call, so the file at `source`
	// MUST be kept available until the end of the Azure DevOps job.
	UploadArtifact(name string, directory string, source string)
}
