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
	PublishLogFile(name string, path string)
}
