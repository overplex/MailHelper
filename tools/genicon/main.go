//go:build windows

// Command genicon renders icon.png into winres/ at the sizes Windows asks
// for. The PNGs are the input for go-winres, which produces the .syso
// resource linked into the executable:
//
//	go run ./tools/genicon
//	go-winres make --arch amd64 --out rsrc
//
// Both steps only need re-running when the artwork or the manifest changes;
// the generated .syso is committed, the PNGs are not.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"mailhelper/internal/appicon"
)

const outDir = "winres"

func main() {
	source, err := os.ReadFile("icon.png")
	if err != nil {
		log.Fatalf("read icon.png: %v (run this from the project root)", err)
	}
	if err := appicon.Load(source); err != nil {
		log.Fatal(err)
	}

	for _, size := range []int32{256, 48, 32, 16} {
		data, err := appicon.EncodePNG(size)
		if err != nil {
			log.Fatalf("render %d: %v", size, err)
		}
		path := filepath.Join(outDir, fmt.Sprintf("icon%d.png", size))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s", path)
	}
}
