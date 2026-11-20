package artifacts

import "io"

// ArtifactBroker publishes test artifacts. Publication errors mark active tests
// as errored; failure-hook brokers log errors without changing test results.
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
	// The source must resolve to a regular file.
	PublishLogFile(name string, source string)

	// Allows the test case to publish an arbitrary artifact located at the
	// given path to the artifact output directory, when provided. If an output
	// directory was not provided to the runnable, this function will be a
	// no-op.
	//
	// Inputs:
	//   - destination: the relative file path (including filename) within the
	//     output directory where the artifact will be stored.
	//   - source: The path to the file to be published as an artifact. It may be
	//     a relative path, but absolute paths are recommended.
	//
	// If multiple artifacts are published with the same name, only the last one
	// will be kept.
	//
	// The source must resolve to a regular file.
	PublishArtifact(destination string, source string)

	// Same as PublishArtifact but takes the artifact data as a byte slice
	// instead of a file path. If an output directory was not provided to the
	// runnable, this function will be a no-op.
	//
	// Inputs:
	//   - destination: the relative file path (including filename) within the
	//     output directory where the artifact will be stored.
	//   - data: The artifact data to be written to the file.
	//
	// If multiple artifacts are published with the same name, only the last one
	// will be kept.
	PublishArtifactData(destination string, data []byte)

	// Similar to PublishArtifactData, but returns a WriteCloser that can be
	// used to stream data to the artifact file. The caller must close the
	// returned WriteCloser when done writing data.
	//
	// Inputs:
	//   - destination: the relative file path (including filename) within the
	//     output directory where the artifact will be stored.
	//
	// If multiple artifacts are published with the same name, only the last one
	// will be kept.
	//
	// In failure hooks, creation errors are logged and return a discard writer.
	//
	// Because the returned WriteCloser remains active until it is closed, any
	// calls to this function with the same destination before the previous
	// WriteCloser is closed will result in an error.
	StreamArtifactData(destination string) io.WriteCloser

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
	// The source must resolve to a regular file.
	//
	// Note: implementations do NOT block on this call, so the file at `source`
	// MUST be kept available until the end of the Azure DevOps job.
	UploadArtifact(name string, directory string, source string)
}
