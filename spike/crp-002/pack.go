package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// pack writes an MCPB bundle: a zip with manifest.json at the root and the
// given binaries under server/, marked executable so macOS can run them.
// The MCPB command line tool needs Node.js; this avoids installing it.
//
//	crp002-spike pack <manifest.json> <out.mcpb> <binary>...
func pack(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: crp002-spike pack <manifest.json> <out.mcpb> <binary>...")
	}
	out, err := os.Create(args[1])
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	add := func(name, src string, mode os.FileMode) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	}
	if err := add("manifest.json", args[0], 0o644); err != nil {
		return err
	}
	for _, bin := range args[2:] {
		if err := add("server/"+filepath.Base(bin), bin, 0o755); err != nil {
			return err
		}
	}
	return zw.Close()
}
