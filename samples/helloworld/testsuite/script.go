package helloworld

import "fmt"

type HelloWorldScriptSet struct {
	HelloWorld HelloWorldScript `cmd:"" help:"Prints hello world messages"`
}

type HelloWorldScript struct {
	Names []string `arg:"" help:"List of names to greet"`
}

func (s *HelloWorldScript) Run() error {
	for _, d := range s.Names {
		fmt.Println("Hello, " + d + "!")
	}

	return nil
}
