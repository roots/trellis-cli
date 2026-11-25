// Command fingerprint compares two servers' effective configuration.
//
//	fingerprint probe | ssh root@host bash -s > host.txt
//	fingerprint facts host.txt
//	fingerprint compare [-config compare.yml] trellis-before.txt trellis-after.txt converge-before.txt converge-after.txt
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/roots/trellis-cli/pkg/converge/fingerprint"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fingerprint:", err)
		os.Exit(1)
	}
}

func load(path string) (fingerprint.Facts, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return fingerprint.Parse(string(b)), nil
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: fingerprint probe | facts <file> | compare [-config f] <trellis-before> <trellis-after> <converge-before> <converge-after>")
	}
	switch args[0] {
	case "probe":
		_, err := os.Stdout.WriteString(fingerprint.Probe)
		return err
	case "facts":
		f, err := load(args[1])
		if err != nil {
			return err
		}
		for _, k := range f.Keys() {
			fmt.Printf("%s = %s\n", k, f[k])
		}
		return nil
	case "compare":
		fs := flag.NewFlagSet("compare", flag.ContinueOnError)
		cfgPath := fs.String("config", "", "ignore/expected config (YAML)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 4 {
			return fmt.Errorf("compare needs 4 probe files")
		}
		var cfg *fingerprint.Config
		if *cfgPath != "" {
			b, err := os.ReadFile(*cfgPath)
			if err != nil {
				return err
			}
			if cfg, err = fingerprint.LoadConfig(b); err != nil {
				return err
			}
		}
		var facts [4]fingerprint.Facts
		for i := range facts {
			f, err := load(fs.Arg(i))
			if err != nil {
				return err
			}
			facts[i] = f
		}
		fingerprint.Report(os.Stdout, fingerprint.Compare(facts[0], facts[1], facts[2], facts[3], cfg))
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}
