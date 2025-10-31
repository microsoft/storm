package run

type CommonRunnableOpts struct {
	Watch  bool    `short:"w" help:"Watch the output live"`
	LogDir *string `short:"l" help:"Optional directory to save logs to. Will be created if it does not exist." type:"path"`
	JUnit  *string `short:"j" help:"Produce JUnit XML output at the given path." type:"path"`
	Output *string `short:"o" long:"artifact-output-dir" help:"Optional directory to output artifacts to." type:"path"`
}
