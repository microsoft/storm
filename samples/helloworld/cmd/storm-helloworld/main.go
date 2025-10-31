package main

import (
	"github.com/microsoft/storm"

	helloworld "github.com/microsoft/storm/samples/helloworld/testsuite"
)

func main() {
	storm := storm.CreateSuite("hello-world")

	// Add hello world scenario
	storm.AddScenario(&helloworld.HelloWorldScenario{})

	// Add hello world helper
	storm.AddHelper(&helloworld.HelloWorldHelper{})

	// Add hello world script
	storm.AddScriptSet(&helloworld.HelloWorldScriptSet{})

	storm.Run()
}
