package iso_test

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/microsoft/storm/facilities/iso"
)

func ExampleBuild() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Put the guest's configuration files in seed/ before building.
	if err := iso.Build(ctx, "seed.iso", "SEED", os.DirFS("seed")); err != nil {
		log.Fatal(err)
	}
}
