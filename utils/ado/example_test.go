package ado_test

import (
	"fmt"
	"log"
	"os"

	"github.com/microsoft/storm/utils/ado"
)

func ExampleMarshalMatrix() {
	matrix, err := ado.MarshalMatrix(map[string]map[string]string{
		"vm_host": {"SCENARIO": "vm-host", "IMAGE": "installer.iso"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(matrix))
	// Output: {"vm_host":{"IMAGE":"installer.iso","SCENARIO":"vm-host"}}
}

func ExampleSetVariable() {
	if err := ado.SetVariable(os.Stdout, "IMAGE", "installer.iso", true); err != nil {
		log.Fatal(err)
	}
	// Output: ##vso[task.setvariable variable=IMAGE;isOutput=true]installer.iso
}
